package netutil

import (
	"testing"
	"time"
)

func TestTCPProbe_invalidPort(t *testing.T) {
	res := TCPProbe(nil, "192.168.1.1", 0, time.Second)
	if res.Error == "" {
		t.Fatal("expected port error")
	}
}

func TestTCPProbe_blocksLoopback(t *testing.T) {
	res := TCPProbe(nil, "127.0.0.1", 22, time.Second)
	if res.Open || res.Error == "" {
		t.Fatalf("expected block: %+v", res)
	}
}
