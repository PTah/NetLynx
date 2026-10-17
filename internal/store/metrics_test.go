package store

import (
	"testing"
	"time"
)

func TestDownsampleMetricSamples(t *testing.T) {
	now := time.Now()
	in := make([]MetricSample, 0, 100)
	for i := 0; i < 100; i++ {
		in = append(in, MetricSample{MetricType: "page_count", Value: float32(i), SampledAt: now.Add(time.Duration(i) * time.Minute)})
	}
	out := DownsampleMetricSamples(in, 10)
	if len(out) != 10 {
		t.Fatalf("len=%d", len(out))
	}
	same := DownsampleMetricSamples(in, 200)
	if len(same) != 100 {
		t.Fatalf("no downsample: %d", len(same))
	}
}
