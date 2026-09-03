package netutil

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// TraceHop — один хоп traceroute.
type TraceHop struct {
	Hop      int       `json:"hop"`
	Address  string    `json:"address,omitempty"`
	Hostname string    `json:"hostname,omitempty"`
	RTTMs    []float64 `json:"rtt_ms,omitempty"`
	Timeout  bool      `json:"timeout,omitempty"`
}

// TraceResult — результат traceroute с сервера NetLynx.
type TraceResult struct {
	Target string     `json:"target"`
	OK     bool       `json:"ok"`
	Via    string     `json:"via"`
	Hops   []TraceHop `json:"hops"`
	Error  string     `json:"error,omitempty"`
}

var (
	reTracerouteHop = regexp.MustCompile(`^\s*(\d+)\s+([\*\s\d\.a-fA-F:]+(?:\s+[\*\s\d\.a-fA-F:]+)*)\s*(.*)$`)
	reRTTMs         = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*ms`)
	reTracepathHop  = regexp.MustCompile(`^\s*(\d+):\s+(.+)$`)
)

// ValidateProbeTarget — SSRF-safe проверка цели (LAN ok, loopback/link-local/metadata — нет).
func ValidateProbeTarget(ctx context.Context, host string) error {
	host = normalizeProbeHost(host)
	if host == "" {
		return fmt.Errorf("пустой target")
	}
	if ip := net.ParseIP(host); ip != nil {
		return validateIP(ip, ProbeHostPolicy())
	}
	if err := ValidateDeviceHost(host); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return validateHostContext(ctx, host, ProbeHostPolicy())
}

func validateHostContext(ctx context.Context, host string, p OutboundPolicy) error {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("DNS: нет адресов для %s", host)
	}
	for _, ipa := range addrs {
		if err := validateIP(ipa.IP, p); err != nil {
			return err
		}
	}
	return nil
}

func normalizeProbeHost(raw string) string {
	h := strings.TrimSpace(raw)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.Trim(h, "[]")
}

// Traceroute выполняет traceroute/tracepath/tracert с хоста NetLynx.
func Traceroute(ctx context.Context, target string, maxHops int, timeout time.Duration) TraceResult {
	target = normalizeProbeHost(target)
	out := TraceResult{Target: target}
	if target == "" {
		out.Error = "пустой target"
		return out
	}
	if maxHops <= 0 {
		maxHops = 15
	}
	if maxHops > 30 {
		maxHops = 30
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := ValidateProbeTarget(ctx, target); err != nil {
		out.Error = err.Error()
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch runtime.GOOS {
	case "windows":
		return traceWindows(ctx, target, maxHops, timeout, out)
	default:
		res := traceLinux(ctx, target, maxHops, timeout, out, "traceroute")
		if len(res.Hops) > 0 {
			res.OK = true
			return res
		}
		if !strings.Contains(res.Error, "не установлен") && res.Error != "" {
			// traceroute есть, но не разобрали — не fallback
			return res
		}
		fallback := traceLinux(ctx, target, maxHops, timeout, TraceResult{Target: target}, "tracepath")
		if len(fallback.Hops) > 0 {
			fallback.OK = true
		}
		return fallback
	}
}

func traceLinux(ctx context.Context, target string, maxHops int, timeout time.Duration, out TraceResult, tool string) TraceResult {
	var cmd *exec.Cmd
	waitSec := int(timeout/time.Second)/maxHops + 1
	if waitSec < 1 {
		waitSec = 1
	}
	if waitSec > 5 {
		waitSec = 5
	}
	switch tool {
	case "tracepath":
		cmd = exec.CommandContext(ctx, "tracepath", "-n", target)
		out.Via = "tracepath"
	default:
		cmd = exec.CommandContext(ctx, "traceroute", "-n", "-w", strconv.Itoa(waitSec), "-q", "1", "-m", strconv.Itoa(maxHops), target)
		out.Via = "traceroute"
	}
	raw, err := cmd.CombinedOutput()
	text := string(raw)
	if tool == "tracepath" {
		out.Hops = parseTracepathOutput(text)
	} else {
		out.Hops = parseTracerouteOutput(text)
	}
	if len(out.Hops) == 0 {
		if err != nil {
			if strings.Contains(err.Error(), "executable file not found") || strings.Contains(text, "not found") {
				out.Error = tool + " не установлен"
				return out
			}
			out.Error = strings.TrimSpace(err.Error())
			if text != "" {
				out.Error = out.Error + ": " + strings.TrimSpace(lastLines(text, 3))
			}
			return out
		}
		if strings.TrimSpace(text) != "" {
			out.Error = "не удалось разобрать вывод " + tool
		} else {
			out.Error = tool + ": пустой вывод"
		}
		return out
	}
	out.OK = true
	return out
}

func traceWindows(ctx context.Context, target string, maxHops int, _ time.Duration, out TraceResult) TraceResult {
	out.Via = "tracert"
	cmd := exec.CommandContext(ctx, "tracert", "-d", "-h", strconv.Itoa(maxHops), target)
	raw, err := cmd.CombinedOutput()
	text := string(raw)
	out.Hops = parseTracertOutput(text)
	if len(out.Hops) == 0 {
		if err != nil {
			out.Error = strings.TrimSpace(err.Error())
		} else {
			out.Error = "не удалось разобрать вывод tracert"
		}
		return out
	}
	out.OK = true
	return out
}

func parseTracerouteOutput(text string) []TraceHop {
	var hops []TraceHop
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		m := reTracerouteHop.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		hopN, _ := strconv.Atoi(m[1])
		addrPart := strings.TrimSpace(m[2])
		h := TraceHop{Hop: hopN}
		if strings.Contains(addrPart, "*") && !strings.Contains(addrPart, ".") && !strings.Contains(addrPart, ":") {
			h.Timeout = true
			hops = append(hops, h)
			continue
		}
		fields := strings.Fields(addrPart)
		if len(fields) > 0 && fields[0] != "*" {
			h.Address = fields[0]
		}
		rtts := reRTTMs.FindAllStringSubmatch(line, -1)
		for _, r := range rtts {
			if len(r) < 2 {
				continue
			}
			if v, err := strconv.ParseFloat(r[1], 64); err == nil {
				h.RTTMs = append(h.RTTMs, v)
			}
		}
		if h.Address == "" && len(h.RTTMs) == 0 {
			h.Timeout = true
		}
		hops = append(hops, h)
	}
	return hops
}

func parseTracepathOutput(text string) []TraceHop {
	var hops []TraceHop
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		m := reTracepathHop.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		hopN, _ := strconv.Atoi(m[1])
		rest := strings.TrimSpace(m[2])
		h := TraceHop{Hop: hopN}
		if p1 := strings.Index(rest, "("); p1 >= 0 {
			if p2 := strings.Index(rest[p1:], ")"); p2 > 1 {
				h.Address = strings.TrimSpace(rest[p1+1 : p1+p2])
				h.Hostname = strings.TrimSpace(rest[:p1])
			}
		} else {
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				h.Address = parts[0]
			}
		}
		for _, r := range reRTTMs.FindAllStringSubmatch(rest, -1) {
			if len(r) >= 2 {
				if v, err := strconv.ParseFloat(r[1], 64); err == nil {
					h.RTTMs = append(h.RTTMs, v)
				}
			}
		}
		if h.Address == "" {
			h.Timeout = true
		}
		hops = append(hops, h)
	}
	return hops
}

func parseTracertOutput(text string) []TraceHop {
	var hops []TraceHop
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hopN, err := strconv.Atoi(fields[0])
		if err != nil || hopN <= 0 {
			continue
		}
		h := TraceHop{Hop: hopN}
		for _, f := range fields[1:] {
			if f == "*" {
				continue
			}
			if net.ParseIP(f) != nil {
				h.Address = f
				continue
			}
			if strings.HasSuffix(f, "ms") {
				vStr := strings.TrimSuffix(f, "ms")
				if v, err := strconv.ParseFloat(vStr, 64); err == nil {
					h.RTTMs = append(h.RTTMs, v)
				}
			}
		}
		if h.Address == "" && len(h.RTTMs) == 0 {
			h.Timeout = true
		}
		hops = append(hops, h)
	}
	return hops
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
