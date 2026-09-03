package traprecv

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/gosnmp/gosnmp"
)

// LinkActionFunc — опциональный callback для port actions при effects=full.
type LinkActionFunc func(ctx context.Context, deviceID int64, ifIndex int, eventType string)

type Receiver struct {
	log       *slog.Logger
	st        *store.Store
	hub       *live.Hub
	hook      *notify.EventHook
	onAction  LinkActionFunc
	listen    string
	community string
}

func New(
	log *slog.Logger,
	st *store.Store,
	hub *live.Hub,
	listenAddr, community string,
	hook *notify.EventHook,
	onAction LinkActionFunc,
) *Receiver {
	if log == nil {
		log = slog.Default()
	}
	return &Receiver{
		log:       log,
		st:        st,
		hub:       hub,
		hook:      hook,
		onAction:  onAction,
		listen:    strings.TrimSpace(listenAddr),
		community: strings.TrimSpace(community),
	}
}

func (r *Receiver) Enabled() bool {
	return r.listen != ""
}

func (r *Receiver) Run(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}

	tl := gosnmp.NewTrapListener()
	tl.Params = &gosnmp.GoSNMP{}
	tl.OnNewTrap = func(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		r.handleTrap(packet, addr)
	}

	go func() {
		<-ctx.Done()
		tl.Close()
	}()

	r.log.Info("snmp trap receiver listen", "addr", r.listen)
	if err := tl.Listen(r.listen); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func communityAllowed(expected, got string) bool {
	if expected == "" {
		return true
	}
	return got == expected
}

func linkExpectedOper(label string) (int, bool) {
	switch strings.TrimSpace(label) {
	case "linkUp":
		return 1, true
	case "linkDown":
		return 2, true
	default:
		return 0, false
	}
}

func linkEventType(label string) (typ, sev string, ok bool) {
	switch strings.TrimSpace(label) {
	case "linkUp":
		return "LINK_UP", "info", true
	case "linkDown":
		return "LINK_DOWN", "warning", true
	default:
		return "", "", false
	}
}

func ignoreReaction(ignore map[int]store.PortEventIgnore, ifIdx *int, typ string) (blockEvents, blockNotify, blockActions bool) {
	if ifIdx == nil || len(ignore) == 0 {
		return false, false, false
	}
	rule, ok := ignore[*ifIdx]
	if !ok || !store.MatchesIgnoreEventTypes(typ, rule.EventTypes) {
		return false, false, false
	}
	return rule.BlockEvents, rule.BlockNotify, rule.BlockActions
}

func (r *Receiver) handleTrap(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	if packet == nil || addr == nil {
		return
	}
	if !communityAllowed(r.community, packet.Community) {
		return
	}

	sourceIP := addr.IP.String()
	cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	settings, err := r.st.GetSNMPTrapSettings(cctx)
	if err != nil {
		r.log.Warn("trap settings", "err", err)
		return
	}
	if !settings.ListenEnabled {
		return
	}

	deviceID, ok, err := r.st.FindDeviceIDByHost(cctx, sourceIP)
	if err != nil {
		r.log.Warn("trap lookup device", "source_ip", sourceIP, "err", err)
		ok = false
	}
	var deviceIDPtr *int64
	if ok {
		deviceIDPtr = &deviceID
	}

	payload := map[string]interface{}{
		"source_ip": sourceIP,
		"version":   fmt.Sprint(packet.Version),
	}
	if packet.Community != "" {
		payload["community"] = packet.Community
	}

	var trapOID string
	var ifIndex *int
	varbinds := make([]map[string]interface{}, 0, len(packet.Variables))
	for _, v := range packet.Variables {
		oid := strings.TrimSpace(v.Name)
		if oid == "" {
			continue
		}
		if strings.TrimPrefix(oid, ".") == "1.3.6.1.6.3.1.1.4.1.0" {
			trapOID = strings.TrimSpace(valueToString(v.Value))
		}
		if idx, okIdx := parseIfIndex(oid, v.Value); okIdx {
			i := idx
			ifIndex = &i
		}
		varbinds = append(varbinds, map[string]interface{}{
			"oid":   oid,
			"type":  v.Type.String(),
			"value": valueToString(v.Value),
		})
	}
	if trapOID != "" {
		payload["trap_oid"] = trapOID
	}
	if len(varbinds) > 0 {
		payload["varbinds"] = varbinds
	}

	dec := DecodeTrap(trapOID, ifIndex, payload)
	payload["trap_label"] = dec.Label
	payload["trap_summary"] = dec.Summary

	expectedOper, isLink := linkExpectedOper(dec.Label)
	if isLink && ok && ifIndex != nil {
		if err := r.st.UpsertTrapPendingLink(cctx, deviceID, *ifIndex, expectedOper, dec.Label, sourceIP); err != nil {
			r.log.Warn("trap pending link", "device_id", deviceID, "if_index", *ifIndex, "err", err)
		}
	}

	include := ParseIncludeLabels(settings.TrapIncludeLabels)
	labelOK := LabelMatchesInclude(dec.Label, include)

	if settings.LogEnabled && labelOK {
		_, err = r.st.InsertSNMPTrapLog(
			cctx, sourceIP, deviceIDPtr,
			fmt.Sprint(packet.Version), packet.Community, trapOID, ifIndex, payload,
		)
		if err != nil {
			r.log.Warn("trap insert log", "source_ip", sourceIP, "err", err)
		}
	}

	if isLink {
		if labelOK {
			r.maybeEmitLinkFromTrap(cctx, settings, deviceID, ok, ifIndex, dec.Label, sourceIP, payload)
		}
		return
	}

	// Не-link: в events только если журнал trap logs выключен (как раньше).
	if settings.LogEnabled || !ok || !labelOK {
		return
	}
	evID, err := r.st.InsertEvent(cctx, deviceID, ifIndex, "SNMP_TRAP", "info", payload)
	if err != nil {
		r.log.Warn("trap insert event", "device_id", deviceID, "source_ip", sourceIP, "err", err)
		return
	}
	if r.hub != nil {
		r.hub.Publish(live.EventPayload{
			EventID: evID, DeviceID: deviceID, IfIndex: ifIndex,
			EventType: "SNMP_TRAP", Severity: "info", Payload: payload,
		})
	}
}

func (r *Receiver) maybeEmitLinkFromTrap(
	ctx context.Context,
	settings store.SNMPTrapSettings,
	deviceID int64,
	deviceOK bool,
	ifIndex *int,
	label, sourceIP string,
	basePayload map[string]interface{},
) {
	if ifIndex == nil || *ifIndex <= 0 {
		return
	}
	trust := false
	var devName, devHost string
	if deviceOK {
		if d, err := r.st.GetDevice(ctx, deviceID); err == nil && d != nil {
			trust = d.TrustLinkTraps
			devName = d.Name
			devHost = d.Host
		}
	}
	if !store.AllowLinkTrapEvents(settings.LinkTrapEventsMode, deviceOK, trust) {
		return
	}
	typ, sev, ok := linkEventType(label)
	if !ok {
		return
	}

	ignoreMap, _ := r.st.GetPortEventIgnoreMap(ctx, deviceID)
	blockEv, blockNotify, blockActions := ignoreReaction(ignoreMap, ifIndex, typ)
	if blockEv {
		return
	}

	pl := map[string]interface{}{}
	for k, v := range basePayload {
		pl[k] = v
	}
	pl["source"] = "trap"
	pl["trap_confirmed"] = false
	pl["trap_label"] = label
	pl["host"] = sourceIP
	if ifIndex != nil {
		pl["if_index"] = *ifIndex
	}
	if labels, ok, err := r.st.GetInterfaceEventLabels(ctx, deviceID, *ifIndex); err != nil {
		r.log.Warn("trap link iface labels", "device_id", deviceID, "if_index", *ifIndex, "err", err)
	} else if ok {
		store.ApplyEventIfaceLabels(pl, labels)
	}

	evID, err := r.st.InsertEvent(ctx, deviceID, ifIndex, typ, sev, pl)
	if err != nil {
		r.log.Warn("trap link event", "device_id", deviceID, "type", typ, "err", err)
		return
	}
	if r.hub != nil {
		r.hub.Publish(live.EventPayload{
			EventID: evID, DeviceID: deviceID, DeviceName: devName, DeviceHost: devHost,
			IfIndex: ifIndex, EventType: typ, Severity: sev, Payload: pl,
		})
	}
	effects := store.NormalizeLinkTrapEffects(settings.LinkTrapEffects)
	if r.hook != nil && !blockNotify {
		r.hook.DispatchEvent(deviceID, devName, devHost, evID, ifIndex, typ, sev, pl)
	}
	if effects == store.LinkTrapEffectsFull && !blockActions && r.onAction != nil && ifIndex != nil {
		actCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		go func() {
			defer cancel()
			r.onAction(actCtx, deviceID, *ifIndex, typ)
		}()
	}
}

func valueToString(v interface{}) string {
	return snmp.SanitizeSNMPValue(v)
}

func parseIfIndex(oid string, val interface{}) (int, bool) {
	normalized := strings.TrimPrefix(strings.TrimSpace(oid), ".")
	if normalized != "1.3.6.1.2.1.2.2.1.1" && !strings.HasPrefix(normalized, "1.3.6.1.2.1.2.2.1.1.") {
		return 0, false
	}
	switch x := val.(type) {
	case int:
		if x > 0 {
			return x, true
		}
	case int64:
		if x > 0 && x <= int64(^uint(0)>>1) {
			return int(x), true
		}
	case uint:
		if x > 0 && x <= uint(^uint(0)>>1) {
			return int(x), true
		}
	case uint32:
		if x > 0 {
			return int(x), true
		}
	case uint64:
		if x > 0 && x <= uint64(^uint(0)>>1) {
			return int(x), true
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}
