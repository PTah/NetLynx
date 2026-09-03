package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type MetricSample struct {
	IfIndex    *int      `json:"if_index,omitempty"`
	MetricType string    `json:"metric_type"`
	Value      float32   `json:"value"`
	SampledAt  time.Time `json:"sampled_at"`
}

func (s *Store) InsertMetricSample(ctx context.Context, deviceID int64, ifIndex *int, metricType string, value float32, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO metric_samples (device_id, if_index, metric_type, value, sampled_at)
		VALUES ($1, $2, $3, $4, $5)`,
		deviceID, ifIndex, metricType, value, at)
	return err
}

func (s *Store) InsertMetricSamplesBatch(ctx context.Context, deviceID int64, samples []MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, sm := range samples {
		batch.Queue(`
			INSERT INTO metric_samples (device_id, if_index, metric_type, value, sampled_at)
			VALUES ($1, $2, $3, $4, $5)`,
			deviceID, sm.IfIndex, sm.MetricType, sm.Value, sm.SampledAt)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range samples {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMetricSamples(ctx context.Context, deviceID int64, metricType string, ifIndex *int, from, to time.Time) ([]MetricSample, error) {
	q := `
		SELECT if_index, metric_type, value, sampled_at
		FROM metric_samples
		WHERE device_id = $1 AND metric_type = $2 AND sampled_at >= $3 AND sampled_at <= $4`
	args := []interface{}{deviceID, metricType, from, to}
	if ifIndex != nil {
		q += ` AND if_index = $5`
		args = append(args, *ifIndex)
	} else {
		q += ` AND if_index IS NULL`
	}
	q += ` ORDER BY sampled_at ASC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetricSample
	for rows.Next() {
		var sm MetricSample
		sm.MetricType = metricType
		if err := rows.Scan(&sm.IfIndex, &sm.MetricType, &sm.Value, &sm.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// ListPortMetricSamples возвращает метрики портов (if_index IS NOT NULL) для одного или нескольких типов.
func (s *Store) ListPortMetricSamples(ctx context.Context, deviceID int64, metricTypes []string, from, to time.Time) ([]MetricSample, error) {
	if len(metricTypes) == 0 {
		return nil, nil
	}
	q := `
		SELECT if_index, metric_type, value, sampled_at
		FROM metric_samples
		WHERE device_id = $1 AND if_index IS NOT NULL
		  AND metric_type = ANY($2) AND sampled_at >= $3 AND sampled_at <= $4
		ORDER BY if_index ASC, metric_type ASC, sampled_at ASC`
	rows, err := s.pool.Query(ctx, q, deviceID, metricTypes, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetricSample
	for rows.Next() {
		var sm MetricSample
		if err := rows.Scan(&sm.IfIndex, &sm.MetricType, &sm.Value, &sm.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *Store) PruneMetricSamples(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM metric_samples WHERE sampled_at < $1`, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
