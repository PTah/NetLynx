package store

import (
	"context"
	"errors"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"github.com/jackc/pgx/v5"
)

// EventIfaceLabels — подпись порта для payload событий (как колонка Description в UI).
type EventIfaceLabels struct {
	IfName  string
	IfDescr string
	IfAlias string
}

func eventDisplayDescr(override, cliDescr, ifDescr *string) string {
	if override != nil {
		if t := strings.TrimSpace(*override); t != "" {
			return t
		}
	}
	if cliDescr != nil {
		if t := strings.TrimSpace(*cliDescr); t != "" {
			return t
		}
	}
	if ifDescr != nil {
		return strings.TrimSpace(*ifDescr)
	}
	return ""
}

func labelsFromRow(ifName, override, cliDescr, ifDescr *string) EventIfaceLabels {
	display := eventDisplayDescr(override, cliDescr, ifDescr)
	name := ""
	if ifName != nil {
		name = strings.TrimSpace(*ifName)
	}
	alias := ""
	if override != nil {
		if t := strings.TrimSpace(*override); t != "" {
			alias = t
		}
	} else if cliDescr != nil {
		if t := strings.TrimSpace(*cliDescr); t != "" {
			alias = t
		}
	}
	return EventIfaceLabels{IfName: name, IfDescr: display, IfAlias: alias}
}

// ApplyEventIfaceLabels дополняет payload полями порта для таблицы событий.
func ApplyEventIfaceLabels(pl map[string]interface{}, labels EventIfaceLabels) {
	if pl == nil {
		return
	}
	if labels.IfDescr != "" {
		pl["if_descr"] = labels.IfDescr
	}
	if labels.IfName != "" {
		pl["if_name"] = labels.IfName
	}
	if labels.IfAlias != "" {
		pl["if_alias"] = labels.IfAlias
	}
}

// GetInterfaceEventLabels — description/if_name из device_interfaces.
func (s *Store) GetInterfaceEventLabels(ctx context.Context, deviceID int64, ifIndex int) (EventIfaceLabels, bool, error) {
	if deviceID <= 0 || ifIndex <= 0 {
		return EventIfaceLabels{}, false, nil
	}
	var ifName, override, cliDescr, ifDescr *string
	err := s.pool.QueryRow(ctx, `
		SELECT if_name, descr_override, cli_description, if_descr
		FROM device_interfaces
		WHERE device_id = $1 AND if_index = $2`,
		deviceID, ifIndex,
	).Scan(&ifName, &override, &cliDescr, &ifDescr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventIfaceLabels{}, false, nil
		}
		return EventIfaceLabels{}, false, err
	}
	return labelsFromRow(ifName, override, cliDescr, ifDescr), true, nil
}

type eventIfaceRef struct {
	deviceID int64
	ifIndex  int
}

func (s *Store) mapInterfaceEventLabels(ctx context.Context, refs []eventIfaceRef) (map[eventIfaceRef]EventIfaceLabels, error) {
	out := make(map[eventIfaceRef]EventIfaceLabels)
	if len(refs) == 0 {
		return out, nil
	}
	devIDs := make([]int64, len(refs))
	ifIndexes := make([]int32, len(refs))
	for i, r := range refs {
		devIDs[i] = r.deviceID
		ifIndexes[i] = int32(r.ifIndex)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT di.device_id, di.if_index, di.if_name, di.descr_override, di.cli_description, di.if_descr
		FROM device_interfaces di
		INNER JOIN unnest($1::bigint[], $2::int[]) AS t(device_id, if_index)
			ON di.device_id = t.device_id AND di.if_index = t.if_index`,
		devIDs, ifIndexes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var devID int64
		var ifIdx int
		var ifName, override, cliDescr, ifDescr *string
		if err := rows.Scan(&devID, &ifIdx, &ifName, &override, &cliDescr, &ifDescr); err != nil {
			return nil, err
		}
		out[eventIfaceRef{deviceID: devID, ifIndex: ifIdx}] = labelsFromRow(ifName, override, cliDescr, ifDescr)
	}
	return out, rows.Err()
}

// EnrichEventsWithIfaceLabels подставляет if_descr/if_name из БД (trap и старые события без подписи порта).
func (s *Store) EnrichEventsWithIfaceLabels(ctx context.Context, events []models.Event) error {
	if len(events) == 0 {
		return nil
	}
	seen := make(map[eventIfaceRef]struct{})
	var refs []eventIfaceRef
	for i := range events {
		if events[i].IfIndex == nil || *events[i].IfIndex <= 0 {
			continue
		}
		ref := eventIfaceRef{deviceID: events[i].DeviceID, ifIndex: *events[i].IfIndex}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil
	}
	labels, err := s.mapInterfaceEventLabels(ctx, refs)
	if err != nil {
		return err
	}
	for i := range events {
		if events[i].IfIndex == nil || *events[i].IfIndex <= 0 {
			continue
		}
		ref := eventIfaceRef{deviceID: events[i].DeviceID, ifIndex: *events[i].IfIndex}
		lbl, ok := labels[ref]
		if !ok {
			continue
		}
		if events[i].Payload == nil {
			events[i].Payload = map[string]interface{}{}
		}
		ApplyEventIfaceLabels(events[i].Payload, lbl)
	}
	return nil
}
