package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DuplicateIdentityError — host или chassis MAC уже занят другим узлом.
type DuplicateIdentityError struct {
	Kind         string // "host" | "mac"
	Value        string
	ExistingID   int64
	ExistingName string
}

func (e *DuplicateIdentityError) Error() string {
	if e == nil {
		return "дубликат идентичности"
	}
	name := e.ExistingName
	if name == "" {
		name = "?"
	}
	switch e.Kind {
	case "mac":
		return fmt.Sprintf("MAC %s уже занят узлом «%s» (id=%d)", e.Value, name, e.ExistingID)
	default:
		return fmt.Sprintf("IP/host %s уже занят узлом «%s» (id=%d)", e.Value, name, e.ExistingID)
	}
}

func IsDuplicateIdentity(err error) (*DuplicateIdentityError, bool) {
	var d *DuplicateIdentityError
	if errors.As(err, &d) {
		return d, true
	}
	return nil, false
}

// CheckDeviceIdentity проверяет, что host и/или chassis MAC свободны
// (excludeID — свой id при UPDATE, 0 при INSERT).
func (s *Store) CheckDeviceIdentity(ctx context.Context, host string, chassisMAC *string, excludeID int64) error {
	host = strings.TrimSpace(host)
	if host != "" {
		var id int64
		var name string
		err := s.pool.QueryRow(ctx, `
			SELECT id, name FROM devices
			WHERE lower(btrim(host)) = lower(btrim($1))
			  AND ($2::bigint = 0 OR id <> $2)`,
			host, excludeID,
		).Scan(&id, &name)
		if err == nil {
			return &DuplicateIdentityError{Kind: "host", Value: host, ExistingID: id, ExistingName: name}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if chassisMAC != nil {
		mac, ok := NormalizeMACQuery(*chassisMAC)
		if ok {
			hex := macHexDigits(mac)
			if len(hex) >= 6 {
				var id int64
				var name string
				err := s.pool.QueryRow(ctx, `
					SELECT id, name FROM devices
					WHERE chassis_mac IS NOT NULL AND btrim(chassis_mac) <> ''
					  AND lower(replace(replace(chassis_mac, ':', ''), '-', '')) = $1
					  AND ($2::bigint = 0 OR id <> $2)`,
					hex, excludeID,
				).Scan(&id, &name)
				if err == nil {
					return &DuplicateIdentityError{Kind: "mac", Value: mac, ExistingID: id, ExistingName: name}
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
		}
	}
	return nil
}

// FindDeviceByChassisMAC ищет узел по полному MAC (chassis_mac).
func (s *Store) FindDeviceByChassisMAC(ctx context.Context, raw string) (id int64, name string, ok bool, err error) {
	mac, good := FormatFullMAC(raw)
	if !good {
		return 0, "", false, nil
	}
	hex := macHexDigits(mac)
	err = s.pool.QueryRow(ctx, `
		SELECT id, name FROM devices
		WHERE chassis_mac IS NOT NULL AND btrim(chassis_mac) <> ''
		  AND lower(replace(replace(chassis_mac, ':', ''), '-', '')) = $1
		LIMIT 1`, hex).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, name, true, nil
}

// FindDeviceByHost ищет узел по точному host/IP (без учёта регистра).
func (s *Store) FindDeviceByHost(ctx context.Context, host string) (id int64, name string, ok bool, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return 0, "", false, nil
	}
	err = s.pool.QueryRow(ctx, `
		SELECT id, name FROM devices
		WHERE lower(btrim(host)) = lower(btrim($1))
		LIMIT 1`, host).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, name, true, nil
}

func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	c := strings.ToLower(pgErr.ConstraintName)
	switch {
	case strings.Contains(c, "host"):
		return &DuplicateIdentityError{Kind: "host", Value: "?", ExistingName: "?"}
	case strings.Contains(c, "chassis_mac"):
		return &DuplicateIdentityError{Kind: "mac", Value: "?", ExistingName: "?"}
	default:
		return err
	}
}
