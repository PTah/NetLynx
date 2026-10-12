package notify

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestTryAddSendDuringShutdown(t *testing.T) {
	h := NewEventHook(slog.Default(), nil, nil, HookOptions{})
	h.mu.Lock()
	h.stopping = true
	h.mu.Unlock()
	if h.tryAdd() {
		t.Fatal("DispatchEvent after stopping")
	}
	if !h.tryAddSend() {
		t.Fatal("email flush during shutdown")
	}
	h.wg.Done()
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	if h.tryAddSend() {
		t.Fatal("Add after Wait closed")
	}
}

func TestWithRetryHonorsRetryAfter(t *testing.T) {
	var n atomic.Int32
	start := time.Now()
	err := withRetry(context.Background(), 2, time.Second, func(context.Context) error {
		if n.Add(1) == 1 {
			return &retryAfterError{after: 25 * time.Millisecond, msg: "429"}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("attempts %d", n.Load())
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("did not wait Retry-After")
	}
	if time.Since(start) > 800*time.Millisecond {
		t.Fatal("used exponential backoff instead of Retry-After")
	}
}

func TestFireEmailBatchAfterStopping(t *testing.T) {
	h := NewEventHook(slog.Default(), nil, nil, HookOptions{})
	h.mu.Lock()
	h.stopping = true
	h.mu.Unlock()
	h.emailBatches["DEVICE_ONLINE"] = &emailBatchBuf{
		items: []EmailDeviceCard{{DeviceID: 1, Title: "sw"}},
		ns:    store.NotificationSettings{},
	}
	h.fireEmailBatch("DEVICE_ONLINE")
	if _, ok := h.emailBatches["DEVICE_ONLINE"]; ok {
		t.Fatal("batch not taken")
	}
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("send goroutine not started (tryAdd blocked flush)")
	}
}

func TestShouldCancelPendingOffline(t *testing.T) {
	if !shouldCancelPendingOffline(0) {
		t.Fatal("unknown duration: flap")
	}
	if !shouldCancelPendingOffline(10) {
		t.Fatal("10s flap")
	}
	if shouldCancelPendingOffline(530) {
		t.Fatal("8m50s must keep OFFLINE email")
	}
}
