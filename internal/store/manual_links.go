package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ManualLinkStatusActive     = "active"
	ManualLinkStatusSuperseded = "superseded"
)

var (
	ErrManualLinkNotFound  = errors.New("manual link not found")
	ErrManualLinkConflict  = errors.New("manual link conflict")
	ErrManualLinkInvalid   = errors.New("manual link invalid")
	ErrManualLinkSuperseded = errors.New("manual link is superseded")
)

// ManualTopologyLink — ручная связь между двумя известными узлами (порт ↔ порт).
type ManualTopologyLink struct {
	ID            int64      `json:"id"`
	ADeviceID     int64      `json:"a_device_id"`
	AIfIndex      int        `json:"a_if_index"`
	BDeviceID     int64      `json:"b_device_id"`
	BIfIndex      int        `json:"b_if_index"`
	Note          *string    `json:"note,omitempty"`
	Status        string     `json:"status"`
	SupersededAt  *time.Time `json:"superseded_at,omitempty"`
	SupersededBy  *string    `json:"superseded_by,omitempty"`
	CreatedBy     *string    `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// Joined labels for UI (optional)
	ADeviceName *string `json:"a_device_name,omitempty"`
	BDeviceName *string `json:"b_device_name,omitempty"`
}

// NormalizeManualLinkEnds приводит пару к канону: a_device_id < b_device_id.
func NormalizeManualLinkEnds(aDev int64, aIf int, bDev int64, bIf int) (int64, int, int64, int, error) {
	if aDev <= 0 || bDev <= 0 || aIf <= 0 || bIf <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("%w: device_id and if_index must be positive", ErrManualLinkInvalid)
	}
	if aDev == bDev {
		return 0, 0, 0, 0, fmt.Errorf("%w: devices must differ", ErrManualLinkInvalid)
	}
	if aDev < bDev {
		return aDev, aIf, bDev, bIf, nil
	}
	return bDev, bIf, aDev, aIf, nil
}

var (
	rePortSlash = regexp.MustCompile(`(?i)(?:^|[^\d])(\d+)\s*/\s*(\d+)\s*$`)
	rePortWord  = regexp.MustCompile(`(?i)(?:port|if(?:index)?|ethernet)\s*[#:]?\s*(\d+)\s*$`)
	rePortPlain = regexp.MustCompile(`^\s*(\d+)\s*$`)
)

// ParseRemotePortIfIndex пытается извлечь номер порта/ifIndex из LLDP/CDP remote_port_id.
// Возвращает (ifIndex, true) при успехе.
func ParseRemotePortIfIndex(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	if m := rePortSlash.FindStringSubmatch(s); len(m) == 3 {
		n, err := strconv.Atoi(m[2])
		if err == nil && n > 0 {
			return n, true
		}
	}
	if m := rePortWord.FindStringSubmatch(s); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n, true
		}
	}
	if m := rePortPlain.FindStringSubmatch(s); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// ManualLinkMatchesNeighbor — совпадение ручной active-связи с LLDP/CDP neighbor.
// localDeviceID/localIf — сторона опроса; remoteDeviceID — резолв соседа.
// remotePortRaw — remote_port_id; если порт извлечь нельзя, достаточно пары устройств + локальный порт.
func ManualLinkMatchesNeighbor(link ManualTopologyLink, localDeviceID int64, localIf int, remoteDeviceID int64, remotePortRaw string) bool {
	if link.Status != ManualLinkStatusActive {
		return false
	}
	if remoteDeviceID <= 0 || localDeviceID <= 0 || localIf <= 0 {
		return false
	}
	var otherDev int64
	var otherIf int
	switch {
	case link.ADeviceID == localDeviceID && link.AIfIndex == localIf && link.BDeviceID == remoteDeviceID:
		otherDev, otherIf = link.BDeviceID, link.BIfIndex
	case link.BDeviceID == localDeviceID && link.BIfIndex == localIf && link.ADeviceID == remoteDeviceID:
		otherDev, otherIf = link.ADeviceID, link.AIfIndex
	default:
		return false
	}
	_ = otherDev
	if remIf, ok := ParseRemotePortIfIndex(remotePortRaw); ok {
		return remIf == otherIf
	}
	// Порт соседа не разобрали — достаточно пары устройств + локальный порт.
	return true
}

func (s *Store) CreateManualLink(ctx context.Context, aDev int64, aIf int, bDev int64, bIf int, note, createdBy *string) (*ManualTopologyLink, error) {
	aDev, aIf, bDev, bIf, err := NormalizeManualLinkEnds(aDev, aIf, bDev, bIf)
	if err != nil {
		return nil, err
	}
	var noteVal any
	if note != nil {
		t := strings.TrimSpace(*note)
		if t != "" {
			noteVal = t
		}
	}
	var createdByVal any
	if createdBy != nil {
		t := strings.TrimSpace(*createdBy)
		if t != "" {
			createdByVal = t
		}
	}
	var link ManualTopologyLink
	err = s.pool.QueryRow(ctx, `
		INSERT INTO manual_topology_links (
			a_device_id, a_if_index, b_device_id, b_if_index, note, status, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,'active',$6,now(),now())
		RETURNING id, a_device_id, a_if_index, b_device_id, b_if_index, note, status,
			superseded_at, superseded_by, created_by, created_at, updated_at`,
		aDev, aIf, bDev, bIf, noteVal, createdByVal,
	).Scan(
		&link.ID, &link.ADeviceID, &link.AIfIndex, &link.BDeviceID, &link.BIfIndex,
		&link.Note, &link.Status, &link.SupersededAt, &link.SupersededBy, &link.CreatedBy,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "manual_topology_links_pair_uidx") ||
			strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("%w: link already exists", ErrManualLinkConflict)
		}
		return nil, err
	}
	return &link, nil
}

type ManualLinkPatch struct {
	ADeviceID *int64
	AIfIndex  *int
	BDeviceID *int64
	BIfIndex  *int
	Note      *string // pointer to empty string clears note
	Status    *string // "active" to restore
}

func (s *Store) UpdateManualLink(ctx context.Context, id int64, p ManualLinkPatch) (*ManualTopologyLink, error) {
	cur, err := s.GetManualLink(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, ErrManualLinkNotFound
	}

	aDev, aIf, bDev, bIf := cur.ADeviceID, cur.AIfIndex, cur.BDeviceID, cur.BIfIndex
	note := cur.Note
	status := cur.Status

	portsTouched := p.ADeviceID != nil || p.AIfIndex != nil || p.BDeviceID != nil || p.BIfIndex != nil
	if portsTouched && cur.Status == ManualLinkStatusSuperseded {
		return nil, fmt.Errorf("%w: cannot change ports", ErrManualLinkSuperseded)
	}
	if p.ADeviceID != nil {
		aDev = *p.ADeviceID
	}
	if p.AIfIndex != nil {
		aIf = *p.AIfIndex
	}
	if p.BDeviceID != nil {
		bDev = *p.BDeviceID
	}
	if p.BIfIndex != nil {
		bIf = *p.BIfIndex
	}
	aDev, aIf, bDev, bIf, err = NormalizeManualLinkEnds(aDev, aIf, bDev, bIf)
	if err != nil {
		return nil, err
	}
	if p.Note != nil {
		t := strings.TrimSpace(*p.Note)
		if t == "" {
			note = nil
		} else {
			note = &t
		}
	}
	if p.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*p.Status))
		if st != ManualLinkStatusActive && st != ManualLinkStatusSuperseded {
			return nil, fmt.Errorf("%w: bad status", ErrManualLinkInvalid)
		}
		status = st
	}

	var noteVal any
	if note != nil {
		noteVal = *note
	}
	var link ManualTopologyLink
	err = s.pool.QueryRow(ctx, `
		UPDATE manual_topology_links SET
			a_device_id = $2, a_if_index = $3, b_device_id = $4, b_if_index = $5,
			note = $6, status = $7,
			superseded_at = CASE WHEN $7 = 'active' THEN NULL ELSE superseded_at END,
			superseded_by = CASE WHEN $7 = 'active' THEN NULL ELSE superseded_by END,
			updated_at = now()
		WHERE id = $1
		RETURNING id, a_device_id, a_if_index, b_device_id, b_if_index, note, status,
			superseded_at, superseded_by, created_by, created_at, updated_at`,
		id, aDev, aIf, bDev, bIf, noteVal, status,
	).Scan(
		&link.ID, &link.ADeviceID, &link.AIfIndex, &link.BDeviceID, &link.BIfIndex,
		&link.Note, &link.Status, &link.SupersededAt, &link.SupersededBy, &link.CreatedBy,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrManualLinkNotFound
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "manual_topology_links_pair_uidx") {
			return nil, fmt.Errorf("%w: link already exists", ErrManualLinkConflict)
		}
		return nil, err
	}
	return &link, nil
}

func (s *Store) DeleteManualLink(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM manual_topology_links WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrManualLinkNotFound
	}
	return nil
}

func (s *Store) GetManualLink(ctx context.Context, id int64) (*ManualTopologyLink, error) {
	var link ManualTopologyLink
	err := s.pool.QueryRow(ctx, `
		SELECT id, a_device_id, a_if_index, b_device_id, b_if_index, note, status,
			superseded_at, superseded_by, created_by, created_at, updated_at
		FROM manual_topology_links WHERE id = $1`, id,
	).Scan(
		&link.ID, &link.ADeviceID, &link.AIfIndex, &link.BDeviceID, &link.BIfIndex,
		&link.Note, &link.Status, &link.SupersededAt, &link.SupersededBy, &link.CreatedBy,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &link, nil
}

// ListManualLinks — status ""|"all" = все; иначе фильтр. deviceID nil = все устройства.
func (s *Store) ListManualLinks(ctx context.Context, deviceID *int64, status string, limit int) ([]ManualTopologyLink, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	status = strings.ToLower(strings.TrimSpace(status))
	args := []interface{}{}
	where := []string{"1=1"}
	argN := 1
	if deviceID != nil && *deviceID > 0 {
		where = append(where, fmt.Sprintf("(m.a_device_id = $%d OR m.b_device_id = $%d)", argN, argN))
		args = append(args, *deviceID)
		argN++
	}
	if status != "" && status != "all" {
		where = append(where, fmt.Sprintf("m.status = $%d", argN))
		args = append(args, status)
		argN++
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT m.id, m.a_device_id, m.a_if_index, m.b_device_id, m.b_if_index, m.note, m.status,
			m.superseded_at, m.superseded_by, m.created_by, m.created_at, m.updated_at,
			da.name, db.name
		FROM manual_topology_links m
		LEFT JOIN devices da ON da.id = m.a_device_id
		LEFT JOIN devices db ON db.id = m.b_device_id
		WHERE %s
		ORDER BY m.updated_at DESC, m.id DESC
		LIMIT $%d`, strings.Join(where, " AND "), argN)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManualTopologyLink
	for rows.Next() {
		var link ManualTopologyLink
		if err := rows.Scan(
			&link.ID, &link.ADeviceID, &link.AIfIndex, &link.BDeviceID, &link.BIfIndex,
			&link.Note, &link.Status, &link.SupersededAt, &link.SupersededBy, &link.CreatedBy,
			&link.CreatedAt, &link.UpdatedAt, &link.ADeviceName, &link.BDeviceName,
		); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *Store) ListActiveManualLinks(ctx context.Context) ([]ManualTopologyLink, error) {
	return s.ListManualLinks(ctx, nil, ManualLinkStatusActive, 500)
}

// SupersedeManualLink marks active→superseded once; returns true if transition happened.
func (s *Store) SupersedeManualLink(ctx context.Context, id int64, byProtocol string) (bool, error) {
	byProtocol = strings.ToLower(strings.TrimSpace(byProtocol))
	if byProtocol == "" {
		byProtocol = "lldp"
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE manual_topology_links SET
			status = 'superseded',
			superseded_at = now(),
			superseded_by = $2,
			updated_at = now()
		WHERE id = $1 AND status = 'active'`, id, byProtocol)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SupersedeManualLinksMatchingNeighbors — снять ручные, совпавшие с резолвленными соседями.
// Не вызывается из poller: active manual линки sticky (приоритет над LLDP/CDP на карте).
// Оставлено для явного «принять авто-линк» / админ-инструментов.
func (s *Store) SupersedeManualLinksMatchingNeighbors(ctx context.Context, localDeviceID int64, protocol string, neighbors []PortNeighbor) ([]ManualTopologyLink, error) {
	if localDeviceID <= 0 || len(neighbors) == 0 {
		return nil, nil
	}
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	idx := buildDeviceNameIndex(devices)
	active, err := s.ListActiveManualLinks(ctx)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, nil
	}

	proto := strings.ToLower(strings.TrimSpace(protocol))
	if proto == "" {
		proto = "lldp"
	}

	var superseded []ManualTopologyLink
	for _, nb := range neighbors {
		if nb.IfIndex <= 0 {
			continue
		}
		rid, ok := resolveRemoteDeviceID(idx, nb)
		if !ok || rid == localDeviceID {
			continue
		}
		portRaw := derefStr(nb.RemotePortID)
		for _, link := range active {
			if !ManualLinkMatchesNeighbor(link, localDeviceID, nb.IfIndex, rid, portRaw) {
				continue
			}
			okTrans, err := s.SupersedeManualLink(ctx, link.ID, proto)
			if err != nil {
				return superseded, err
			}
			if okTrans {
				link.Status = ManualLinkStatusSuperseded
				now := time.Now().UTC()
				link.SupersededAt = &now
				sb := proto
				link.SupersededBy = &sb
				superseded = append(superseded, link)
			}
		}
	}
	return superseded, nil
}
