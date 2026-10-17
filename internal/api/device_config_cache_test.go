package api

import (
	"testing"
	"time"
)

func TestInvalidateMarksRunningConfigDirty(t *testing.T) {
	s := &Server{}
	const id int64 = 42
	s.putDeviceRunningConfig(id, []byte("interface 0/1\npoe opmode auto\n"))
	if _, ok := s.getDeviceRunningConfig(id, time.Hour); !ok {
		t.Fatal("expected memory cache after put")
	}
	if _, dirty := s.runningConfigDirtySince(id); dirty {
		t.Fatal("put must clear dirty")
	}

	s.invalidateDeviceRunningConfig(id)
	if _, ok := s.getDeviceRunningConfig(id, time.Hour); ok {
		t.Fatal("invalidate must drop memory cache")
	}
	dirtyAt, dirty := s.runningConfigDirtySince(id)
	if !dirty || dirtyAt.IsZero() {
		t.Fatal("invalidate must mark dirty for snapshot skip")
	}
}

func TestRunningConfigDirtySinceExpiry(t *testing.T) {
	s := &Server{}
	const id int64 = 7
	s.runningConfigDirtyAt.Store(id, time.Now().Add(-CardCLISyncMaxAge-time.Minute))
	if _, dirty := s.runningConfigDirtySince(id); dirty {
		t.Fatal("dirty older than CardCLISyncMaxAge must be ignored")
	}
}
