package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
	"github.com/go-chi/chi/v5"
)

type tracerouteBody struct {
	Target  string `json:"target"`
	MaxHops *int   `json:"max_hops"`
}

func (s *Server) handleDeviceTraceroute(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	dev, err := s.st.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dev == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}

	var body tracerouteBody
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
			return
		}
	}
	target := strings.TrimSpace(body.Target)
	if target == "" {
		target = dev.Host
	}
	maxHops := 15
	if body.MaxHops != nil && *body.MaxHops >= 1 && *body.MaxHops <= 30 {
		maxHops = *body.MaxHops
	}

	s.audit(r, "device.traceroute", "device", &id, map[string]interface{}{
		"target":   target,
		"max_hops": maxHops,
	})

	res := netutil.Traceroute(r.Context(), target, maxHops, 45*time.Second)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":   id,
		"device_name": dev.Name,
		"target":      res.Target,
		"ok":          res.OK,
		"via":         res.Via,
		"hops":        res.Hops,
		"error":       res.Error,
	})
}

type tcpProbeBody struct {
	Target     string `json:"target"`
	Port       int    `json:"port"`
	TimeoutSec *int   `json:"timeout_sec"`
}

func (s *Server) handleDeviceTCPProbe(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	dev, err := s.st.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dev == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}

	var body tcpProbeBody
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
			return
		}
	}
	target := strings.TrimSpace(body.Target)
	if target == "" {
		target = dev.Host
	}
	port := body.Port
	if port <= 0 {
		port = 22
	}
	timeout := 3 * time.Second
	if body.TimeoutSec != nil && *body.TimeoutSec >= 1 && *body.TimeoutSec <= 30 {
		timeout = time.Duration(*body.TimeoutSec) * time.Second
	}

	s.audit(r, "device.tcp_probe", "device", &id, map[string]interface{}{
		"target":      target,
		"port":        port,
		"timeout_sec": int(timeout / time.Second),
	})

	res := netutil.TCPProbe(r.Context(), target, port, timeout)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":   id,
		"device_name": dev.Name,
		"target":      res.Target,
		"port":        res.Port,
		"open":        res.Open,
		"rtt_ms":      res.RTTMs,
		"banner":      res.Banner,
		"error":       res.Error,
	})
}
