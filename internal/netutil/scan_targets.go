package netutil

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

const MaxScanHosts = 256

// ExpandScanTargets разворачивает CIDR и/или список IP в уникальный список адресов.
// Для IPv4 /N с host bits пропускаются network и broadcast (кроме /31 и /32).
// Лимит — MaxScanHosts; пустой результат или переполнение — ошибка.
func ExpandScanTargets(cidr string, hosts []string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string

	add := func(raw string) error {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			return fmt.Errorf("некорректный IP: %s", raw)
		}
		if err := ValidateDeviceHost(ip.String()); err != nil {
			return fmt.Errorf("%s: %w", ip.String(), err)
		}
		key := normalizeIPKey(ip)
		if _, ok := seen[key]; ok {
			return nil
		}
		if len(out) >= MaxScanHosts {
			return fmt.Errorf("слишком много адресов (макс. %d)", MaxScanHosts)
		}
		seen[key] = struct{}{}
		out = append(out, key)
		return nil
	}

	cidr = strings.TrimSpace(cidr)
	if cidr != "" {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("некорректный CIDR: %w", err)
		}
		ones, bits := network.Mask.Size()
		hostBits := bits - ones
		skipNetBroadcast := hostBits >= 2 // /30 и шире: skip network+broadcast

		for ip := cloneIP(network.IP.Mask(network.Mask)); network.Contains(ip); incIP(ip) {
			cur := cloneIP(ip)
			isFirst := cur.Equal(network.IP.Mask(network.Mask))
			next := cloneIP(ip)
			incIP(next)
			isLast := !network.Contains(next)
			if skipNetBroadcast && (isFirst || isLast) {
				continue
			}
			if err := add(cur.String()); err != nil {
				return nil, err
			}
		}
	}

	for _, h := range hosts {
		for _, part := range strings.FieldsFunc(h, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			if err := add(part); err != nil {
				return nil, err
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("укажите CIDR или список IP")
	}
	sort.Strings(out)
	return out, nil
}

func normalizeIPKey(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}
