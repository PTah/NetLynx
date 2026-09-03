package netutil

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// TCPProbeResult — результат проверки TCP-порта с сервера NetLynx.
type TCPProbeResult struct {
	Target string `json:"target"`
	Port   int    `json:"port"`
	Open   bool   `json:"open"`
	RTTMs  *int   `json:"rtt_ms,omitempty"`
	Banner string `json:"banner,omitempty"`
	Error  string `json:"error,omitempty"`
}

// TCPProbe — net.DialTimeout с SSRF-safe резолвом (ProbeHostPolicy).
func TCPProbe(ctx context.Context, host string, port int, timeout time.Duration) TCPProbeResult {
	host = normalizeProbeHost(host)
	out := TCPProbeResult{Target: host, Port: port}
	if host == "" {
		out.Error = "пустой target"
		return out
	}
	if port < 1 || port > 65535 {
		out.Error = "port: ожидается 1–65535"
		return out
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	if err := ValidateProbeTarget(ctx, host); err != nil {
		out.Error = err.Error()
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, timeout+500*time.Millisecond)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := dialPinned(ctx, dialer, "tcp", addr, ProbeHostPolicy())
	elapsed := int(time.Since(start) / time.Millisecond)
	if elapsed < 1 {
		elapsed = 1
	}
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			out.Error = "timeout"
			return out
		}
		out.Error = fmt.Sprintf("closed/filtered: %v", err)
		return out
	}
	defer conn.Close()
	out.Open = true
	out.RTTMs = &elapsed

	_ = conn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n > 0 {
		out.Banner = sanitizeBanner(string(buf[:n]))
	}
	return out
}

func sanitizeBanner(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\r' || r == '\t' {
			b.WriteByte(' ')
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 120 {
		out = out[:120] + "…"
	}
	return out
}
