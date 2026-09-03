package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	MACInvestigationOpen     = "open"
	MACInvestigationResolved = "resolved"
	MACInvestigationIgnored  = "ignored"
)

type MACInvestigationStatus struct {
	MAC            string     `json:"mac"`
	Status         string     `json:"status"`
	Note           *string    `json:"note,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	UpdatedBy      *int64     `json:"updated_by,omitempty"`
	UpdatedByName  *string    `json:"updated_by_name,omitempty"`
}

func ValidateMACInvestigationStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case MACInvestigationOpen, MACInvestigationResolved, MACInvestigationIgnored:
		return strings.ToLower(strings.TrimSpace(status)), nil
	default:
		return "", fmt.Errorf("status: open, resolved или ignored")
	}
}

func (s *Store) GetMACInvestigationStatus(ctx context.Context, mac string) (*MACInvestigationStatus, error) {
	mac = strings.ToLower(strings.TrimSpace(mac))
	var st MACInvestigationStatus
	var note *string
	err := s.pool.QueryRow(ctx, `
		SELECT s.mac, s.status, s.note, s.updated_at, s.updated_by, u.username
		FROM mac_investigation_status s
		LEFT JOIN users u ON u.id = s.updated_by
		WHERE s.mac = $1
	`, mac).Scan(&st.MAC, &st.Status, &note, &st.UpdatedAt, &st.UpdatedBy, &st.UpdatedByName)
	if errors.Is(err, pgx.ErrNoRows) {
		return &MACInvestigationStatus{
			MAC:    mac,
			Status: MACInvestigationOpen,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	st.Note = note
	return &st, nil
}

func (s *Store) UpsertMACInvestigationStatus(ctx context.Context, mac, status string, note *string, userID *int64) (*MACInvestigationStatus, error) {
	mac = strings.ToLower(strings.TrimSpace(mac))
	st, err := ValidateMACInvestigationStatus(status)
	if err != nil {
		return nil, err
	}
	var noteVal *string
	if note != nil {
		t := strings.TrimSpace(*note)
		if t != "" {
			noteVal = &t
		}
	}
	if st == MACInvestigationOpen {
		_, err := s.pool.Exec(ctx, `DELETE FROM mac_investigation_status WHERE mac = $1`, mac)
		if err != nil {
			return nil, err
		}
		return &MACInvestigationStatus{MAC: mac, Status: MACInvestigationOpen}, nil
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO mac_investigation_status (mac, status, note, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (mac) DO UPDATE SET
			status = EXCLUDED.status,
			note = EXCLUDED.note,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
	`, mac, st, noteVal, userID)
	if err != nil {
		return nil, err
	}
	return s.GetMACInvestigationStatus(ctx, mac)
}

func (s *Store) MapMACInvestigationStatuses(ctx context.Context, macs []string) (map[string]MACInvestigationStatus, error) {
	out := map[string]MACInvestigationStatus{}
	if len(macs) == 0 {
		return out, nil
	}
	norm := make([]string, 0, len(macs))
	for _, m := range macs {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" {
			norm = append(norm, m)
		}
	}
	if len(norm) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT s.mac, s.status, s.note, s.updated_at, s.updated_by, u.username
		FROM mac_investigation_status s
		LEFT JOIN users u ON u.id = s.updated_by
		WHERE s.mac = ANY($1)
	`, norm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st MACInvestigationStatus
		var note *string
		if err := rows.Scan(&st.MAC, &st.Status, &note, &st.UpdatedAt, &st.UpdatedBy, &st.UpdatedByName); err != nil {
			return nil, err
		}
		st.Note = note
		out[st.MAC] = st
	}
	return out, rows.Err()
}
