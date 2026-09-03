// Package pinghost — ICMP-проверка хоста (raw ICMP или системный ping).
package pinghost

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Probe шлёт один ICMP echo. ok=false при таймауте/ошибке/недопустимом host.
func Probe(ctx context.Context, host string, timeout time.Duration) (ok bool, rttMs *int) {
	h, err := sanitizeHost(host)
	if err != nil {
		return false, nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if ok, rtt := probeICMPv4(ctx, h, timeout); ok {
		return true, rtt
	}
	return probeExec(ctx, h, timeout)
}

func probeICMPv4(ctx context.Context, host string, timeout time.Duration) (bool, *int) {
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil || len(ips) == 0 {
			return false, nil
		}
		ip = ips[0]
	}
	ip = ip.To4()
	if ip == nil {
		return false, nil
	}

	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return false, nil
	}
	defer c.Close()

	_ = c.SetDeadline(time.Now().Add(timeout))
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}

	id := (time.Now().UnixNano() & 0xffff)
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: int(id), Seq: 1, Data: []byte("invetor")},
	}
	b, err := msg.Marshal(nil)
	if err != nil {
		return false, nil
	}
	dst := &net.IPAddr{IP: ip}
	start := time.Now()
	if _, err := c.WriteTo(b, dst); err != nil {
		return false, nil
	}

	reply := make([]byte, 1500)
	for {
		n, peer, err := c.ReadFrom(reply)
		if err != nil {
			return false, nil
		}
		if peer == nil || peer.String() != ip.String() {
			// другой хост — продолжаем ждать до дедлайна
			continue
		}
		rm, err := icmp.ParseMessage(1, reply[:n]) // 1 = ICMP IPv4
		if err != nil {
			continue
		}
		if rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := rm.Body.(*icmp.Echo)
		if !ok || echo.ID != int(id) {
			continue
		}
		elapsed := int(time.Since(start) / time.Millisecond)
		if elapsed < 1 {
			elapsed = 1
		}
		return true, &elapsed
	}
}

func probeExec(ctx context.Context, host string, timeout time.Duration) (ok bool, rttMs *int) {
	ctx, cancel := context.WithTimeout(ctx, timeout+500*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		ms := int(timeout / time.Millisecond)
		if ms < 200 {
			ms = 200
		}
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", fmt.Sprintf("%d", ms), host)
	default:
		sec := int(timeout / time.Second)
		if sec < 1 {
			sec = 1
		}
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", fmt.Sprintf("%d", sec), host)
	}
	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := int(time.Since(start) / time.Millisecond)
	if err != nil {
		return false, nil
	}
	text := strings.ToLower(string(out))
	if strings.Contains(text, "100% packet loss") || strings.Contains(text, "100% loss") {
		return false, nil
	}
	if strings.Contains(text, "ttl=") || strings.Contains(text, "time=") ||
		strings.Contains(text, "время=") || strings.Contains(text, "bytes from") ||
		strings.Contains(text, "ответ от") ||
		strings.Contains(text, "received = 1") || strings.Contains(text, "received =1") {
		if elapsed < 1 {
			elapsed = 1
		}
		return true, &elapsed
	}
	return false, nil
}

func sanitizeHost(raw string) (string, error) {
	h := strings.TrimSpace(raw)
	if h == "" {
		return "", fmt.Errorf("empty host")
	}
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h, "]") && net.ParseIP(h) == nil {
		if _, err := net.LookupPort("tcp", h[i+1:]); err == nil {
			h = h[:i]
		}
	}
	h = strings.Trim(h, "[]")
	if net.ParseIP(h) != nil {
		return h, nil
	}
	for _, r := range h {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			continue
		}
		return "", fmt.Errorf("invalid host")
	}
	if len(h) > 253 {
		return "", fmt.Errorf("host too long")
	}
	return h, nil
}
