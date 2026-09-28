package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const snmpScanParallel = 20

type scanSNMPBody struct {
	CIDR        string   `json:"cidr"`
	Hosts       []string `json:"hosts"`
	SNMPVersion string   `json:"snmp_version"`
	Community   string   `json:"community"`
}

type scanSNMPHit struct {
	Host                string `json:"host"`
	SysName             string `json:"sys_name,omitempty"`
	SysDescr            string `json:"sys_descr,omitempty"`
	AlreadyInInventory  bool   `json:"already_in_inventory"`
	InventoryDeviceID   *int64 `json:"inventory_device_id,omitempty"`
	InventoryDeviceName string `json:"inventory_device_name,omitempty"`
	DiscoveredID        *int64 `json:"discovered_id,omitempty"`
}

type scanSNMPAddBody struct {
	Hosts          []string `json:"hosts"`
	SNMPVersion    string   `json:"snmp_version"`
	Community      string   `json:"community"`
	Location       string   `json:"location"`
	DeviceCategory string   `json:"device_category"`
	// Optional names keyed by host (sysName from scan UI).
	Names map[string]string `json:"names"`
}

type scanSNMPAddError struct {
	Host  string `json:"host"`
	Error string `json:"error"`
}

func (s *Server) handleScanSNMP(w http.ResponseWriter, r *http.Request) {
	var body scanSNMPBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	comm := strings.TrimSpace(body.Community)
	if comm == "" {
		writeError(w, http.StatusBadRequest, "community обязателен")
		return
	}
	ver := strings.TrimSpace(strings.ToLower(body.SNMPVersion))
	if ver == "" {
		ver = "v2c"
	}
	if ver != "v1" && ver != "v2c" {
		writeError(w, http.StatusBadRequest, "скан поддерживает только SNMP v1/v2c")
		return
	}
	targets, err := netutil.ExpandScanTargets(body.CIDR, body.Hosts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	type probeResult struct {
		host     string
		sysName  string
		sysDescr string
		ok       bool
	}
	results := make([]probeResult, len(targets))
	sem := make(chan struct{}, snmpScanParallel)
	var wg sync.WaitGroup
	for i, host := range targets {
		wg.Add(1)
		go func(i int, host string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			sysName, sysDescr, err := snmp.ProbeSys(snmp.ProbeSysOptions{
				Host:      host,
				Version:   ver,
				Community: comm,
				Timeout:   2 * time.Second,
				Retries:   0,
			})
			if err != nil {
				results[i] = probeResult{host: host, ok: false}
				return
			}
			results[i] = probeResult{host: host, sysName: sysName, sysDescr: sysDescr, ok: true}
		}(i, host)
	}
	wg.Wait()

	hits := make([]scanSNMPHit, 0)
	responded := 0
	skippedKnown := 0
	for _, pr := range results {
		if !pr.ok {
			continue
		}
		responded++
		hit := scanSNMPHit{
			Host:     pr.host,
			SysName:  pr.sysName,
			SysDescr: pr.sysDescr,
		}
		if id, name, ok, err := s.st.FindDeviceByHost(ctx, pr.host); err == nil && ok {
			hit.AlreadyInInventory = true
			hit.InventoryDeviceID = &id
			hit.InventoryDeviceName = name
			skippedKnown++
		}
		discID, err := s.st.UpsertDiscoveredFromScan(ctx, pr.host, pr.sysName)
		if err == nil {
			hit.DiscoveredID = &discID
		}
		hits = append(hits, hit)
	}
	_, _ = s.st.HealDiscoveredAlreadyInInventory(ctx)

	s.audit(r, "devices.scan_snmp", "devices", nil, map[string]interface{}{
		"total":         len(targets),
		"responded":     responded,
		"skipped_known": skippedKnown,
		"cidr":          strings.TrimSpace(body.CIDR),
		"snmp_version":  ver,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"total":         len(targets),
		"probed":        len(targets),
		"responded":     responded,
		"skipped_known": skippedKnown,
		"hits":          hits,
	})
}

func (s *Server) handleScanSNMPAdd(w http.ResponseWriter, r *http.Request) {
	var body scanSNMPAddBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	comm := strings.TrimSpace(body.Community)
	if comm == "" {
		writeError(w, http.StatusBadRequest, "community обязателен")
		return
	}
	ver := strings.TrimSpace(strings.ToLower(body.SNMPVersion))
	if ver == "" {
		ver = "v2c"
	}
	if ver != "v1" && ver != "v2c" {
		writeError(w, http.StatusBadRequest, "скан поддерживает только SNMP v1/v2c")
		return
	}
	if len(body.Hosts) == 0 {
		writeError(w, http.StatusBadRequest, "укажите hosts")
		return
	}
	if len(body.Hosts) > netutil.MaxScanHosts {
		writeError(w, http.StatusBadRequest, "слишком много hosts")
		return
	}

	var loc *string
	if l := strings.TrimSpace(body.Location); l != "" {
		loc = &l
	}
	cat := strings.TrimSpace(body.DeviceCategory)
	created, skipped := 0, 0
	var createdHosts []string
	var errs []scanSNMPAddError

	for _, raw := range body.Hosts {
		host := strings.TrimSpace(raw)
		if host == "" {
			continue
		}
		if err := netutil.ValidateDeviceHost(host); err != nil {
			errs = append(errs, scanSNMPAddError{Host: host, Error: err.Error()})
			continue
		}
		if _, _, ok, err := s.st.FindDeviceByHost(r.Context(), host); err != nil {
			errs = append(errs, scanSNMPAddError{Host: host, Error: err.Error()})
			continue
		} else if ok {
			skipped++
			continue
		}
		name := ""
		if body.Names != nil {
			name = strings.TrimSpace(body.Names[host])
		}
		if name == "" {
			name = host
		}
		id, err := s.st.CreateDevice(r.Context(), store.CreateDeviceInput{
			Name:           name,
			Host:           host,
			Location:       loc,
			DeviceCategory: cat,
			SNMPVersion:    ver,
			Community:      &comm,
		})
		if err != nil {
			if _, isDup := store.IsDuplicateIdentity(err); isDup {
				skipped++
				continue
			}
			errs = append(errs, scanSNMPAddError{Host: host, Error: err.Error()})
			continue
		}
		created++
		createdHosts = append(createdHosts, host)
		if identity := store.DiscoveredIdentityKey("", host, ""); identity != "" {
			if d, err := s.st.GetDiscoveredByIdentityKey(r.Context(), identity); err == nil && d != nil {
				_ = s.st.SetDiscoveredStatus(r.Context(), d.ID, store.DiscoveredStatusAdded, &id)
			}
		}
	}

	if createdHosts == nil {
		createdHosts = []string{}
	}
	if errs == nil {
		errs = []scanSNMPAddError{}
	}

	s.audit(r, "devices.scan_snmp_add", "devices", nil, map[string]interface{}{
		"created": created,
		"skipped": skipped,
		"errors":  len(errs),
		"total":   len(body.Hosts),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"created":       created,
		"skipped":       skipped,
		"created_hosts": createdHosts,
		"errors":        errs,
	})
}
