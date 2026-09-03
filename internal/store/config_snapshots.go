package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ConfigSnapshot struct {
	ID         int64     `json:"id"`
	DeviceID   int64     `json:"device_id"`
	FetchedAt  time.Time `json:"fetched_at"`
	ConfigHash string    `json:"config_hash"`
	Source     string    `json:"source"`
	ByteSize   int       `json:"byte_size"`
}

type ConfigSnapshotFull struct {
	ConfigSnapshot
	ConfigText string `json:"config_text"`
}

var ErrConfigSnapshotNotFound = errors.New("config snapshot not found")

// Волатильные строки в show run / export: uptime, часы, ntp clock-period — не функциональный конфиг.
var volatileConfigLine = regexp.MustCompile(`(?i)^\s*(?:` +
	`[!#]\s*System Up Time\b|` +
	`[!#]\s*Current SNTP Synchronized Time\b|` +
	`[!#]\s*Current NTP Synchronized Time\b|` +
	`[!#]\s*System Time\b|` +
	`ntp clock-period\b|` +
	`#\s+\w{3}/\d{1,2}/\d{4}\s+\d{1,2}:\d{2}:\d{2}\s+by\s+RouterOS\b|` +
	`#\s+\d{4}-\d{2}-\d{2}\s+\d{1,2}:\d{2}:\d{2}\s+by\s+RouterOS\b` +
	`)`)

// CanonicalizeConfigText убирает runtime-шум (uptime, SNTP/NTP time, Cisco ntp clock-period, дата в шапке RouterOS).
func CanonicalizeConfigText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var b strings.Builder
	b.Grow(len(text))
	first := true
	for _, line := range strings.Split(text, "\n") {
		if volatileConfigLine.MatchString(line) {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}

func configHash(text string) string {
	sum := sha256.Sum256([]byte(CanonicalizeConfigText(text)))
	return hex.EncodeToString(sum[:])
}

// SaveConfigSnapshotIfChanged сохраняет снимок, если канонический hash отличается от последнего.
func (s *Store) SaveConfigSnapshotIfChanged(ctx context.Context, deviceID int64, text, source string) (saved bool, id int64, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, 0, fmt.Errorf("пустой конфиг")
	}
	src, err := ValidateConfigSnapshotSource(source)
	if err != nil {
		return false, 0, err
	}
	stored := CanonicalizeConfigText(text)
	if stored == "" {
		return false, 0, fmt.Errorf("пустой конфиг")
	}
	hash := configHash(stored)
	prev, prevErr := s.GetLatestConfigSnapshot(ctx, deviceID)
	if prevErr != nil && !errors.Is(prevErr, ErrConfigSnapshotNotFound) {
		return false, 0, prevErr
	}
	if prevErr == nil && prev != nil {
		if prev.ConfigHash == hash || configHash(prev.ConfigText) == hash {
			return false, 0, nil
		}
	}
	id, err = s.InsertConfigSnapshot(ctx, deviceID, stored, hash, src)
	if err != nil {
		return false, 0, err
	}
	return true, id, nil
}

func (s *Store) InsertConfigSnapshot(ctx context.Context, deviceID int64, text, hash, source string) (int64, error) {
	if source == "" {
		source = "scheduled"
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO device_config_snapshots (device_id, config_text, config_hash, source)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, deviceID, text, hash, source).Scan(&id)
	return id, err
}

func (s *Store) GetLatestConfigSnapshot(ctx context.Context, deviceID int64) (*ConfigSnapshotFull, error) {
	var snap ConfigSnapshotFull
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text), config_text
		FROM device_config_snapshots
		WHERE device_id = $1
		ORDER BY fetched_at DESC, id DESC
		LIMIT 1
	`, deviceID).Scan(
		&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize, &snap.ConfigText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConfigSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) GetLatestConfigSnapshotHash(ctx context.Context, deviceID int64) (hash string, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT config_hash FROM device_config_snapshots
		WHERE device_id = $1
		ORDER BY fetched_at DESC, id DESC
		LIMIT 1
	`, deviceID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

func (s *Store) ListConfigSnapshots(ctx context.Context, deviceID int64, limit int) ([]ConfigSnapshot, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text)
		FROM device_config_snapshots
		WHERE device_id = $1
		ORDER BY fetched_at DESC, id DESC
		LIMIT $2
	`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConfigSnapshot
	for rows.Next() {
		var snap ConfigSnapshot
		if err := rows.Scan(&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) GetConfigSnapshot(ctx context.Context, deviceID, snapID int64) (*ConfigSnapshotFull, error) {
	var snap ConfigSnapshotFull
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text), config_text
		FROM device_config_snapshots
		WHERE id = $1 AND device_id = $2
	`, snapID, deviceID).Scan(
		&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize, &snap.ConfigText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConfigSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) GetPreviousConfigSnapshot(ctx context.Context, deviceID, afterSnapID int64) (*ConfigSnapshotFull, error) {
	var snap ConfigSnapshotFull
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text), config_text
		FROM device_config_snapshots
		WHERE device_id = $1 AND id < $2
		ORDER BY id DESC
		LIMIT 1
	`, deviceID, afterSnapID).Scan(
		&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize, &snap.ConfigText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConfigSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) GetConfigSnapshotNear(ctx context.Context, deviceID int64, before time.Time) (*ConfigSnapshotFull, error) {
	var snap ConfigSnapshotFull
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text), config_text
		FROM device_config_snapshots
		WHERE device_id = $1 AND fetched_at <= $2
		ORDER BY fetched_at DESC, id DESC
		LIMIT 1
	`, deviceID, before).Scan(
		&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize, &snap.ConfigText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConfigSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) PruneConfigSnapshots(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM device_config_snapshots WHERE fetched_at < $1
	`, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) CountConfigSnapshots(ctx context.Context, deviceID int64) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM device_config_snapshots WHERE device_id = $1
	`, deviceID).Scan(&n)
	return n, err
}

func ValidateConfigSnapshotSource(src string) (string, error) {
	switch src {
	case "", "scheduled", "backup", "manual", "port_sync":
		if src == "" {
			return "scheduled", nil
		}
		return src, nil
	default:
		return "", fmt.Errorf("unknown source %q", src)
	}
}
