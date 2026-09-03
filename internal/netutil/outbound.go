package netutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OutboundPolicy — правила для исходящих HTTP (анти-SSRF).
type OutboundPolicy struct {
	// RequireHTTPS — только https (webhook).
	RequireHTTPS bool
	// AllowPrivate — разрешить RFC1918 / ULA (нужно для UISP в LAN).
	AllowPrivate bool
	// AllowLoopback — разрешить 127.0.0.0/8 и ::1 (обычно нет).
	AllowLoopback bool
}

// WebhookPolicy: https-only, без private/loopback/link-local.
func WebhookPolicy() OutboundPolicy {
	return OutboundPolicy{RequireHTTPS: true, AllowPrivate: false, AllowLoopback: false}
}

// UISPPolicy: http/https, private LAN ок; loopback и link-local (metadata) — нет.
func UISPPolicy() OutboundPolicy {
	return OutboundPolicy{RequireHTTPS: false, AllowPrivate: true, AllowLoopback: false}
}

// ValidateOutboundURL проверяет URL и резолвит хост: блокирует опасные цели.
func ValidateOutboundURL(raw string, p OutboundPolicy) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("пустой URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("некорректный URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if p.RequireHTTPS {
		if scheme != "https" {
			return fmt.Errorf("допустим только https")
		}
	} else if scheme != "https" && scheme != "http" {
		return fmt.Errorf("допустимы только http/https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("пустой хост")
	}
	if err := validateHost(host, p); err != nil {
		return err
	}
	return nil
}

// ProbeHostPolicy: LAN RFC1918 можно (NMS); loopback и link-local (AWS metadata) — нет.
func ProbeHostPolicy() OutboundPolicy {
	return OutboundPolicy{RequireHTTPS: false, AllowPrivate: true, AllowLoopback: false}
}

// ValidateDeviceHost блокирует loopback/link-local IP и имя localhost.
// Имя хоста без IP не резолвим при сохранении (оффлайн-DNS / DNS-SSRF).
func ValidateDeviceHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	low := strings.ToLower(host)
	if low == "localhost" || strings.HasSuffix(low, ".localhost") {
		return fmt.Errorf("запрещён loopback-хост")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return validateIP(ip, ProbeHostPolicy())
}

func validateHost(host string, p OutboundPolicy) error {
	if ip := net.ParseIP(host); ip != nil {
		return validateIP(ip, p)
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("DNS: нет адресов для %s", host)
	}
	for _, ip := range addrs {
		if err := validateIP(ip, p); err != nil {
			return err
		}
	}
	return nil
}

func validateIP(ip net.IP, p OutboundPolicy) error {
	if ip == nil {
		return fmt.Errorf("пустой IP")
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("запрещён unspecified IP")
	}
	if ip.IsLoopback() && !p.AllowLoopback {
		return fmt.Errorf("запрещён loopback адрес")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("запрещён link-local адрес")
	}
	if ip.IsMulticast() {
		return fmt.Errorf("запрещён multicast адрес")
	}
	if !p.AllowPrivate && ip.IsPrivate() {
		return fmt.Errorf("запрещён private/RFC1918 адрес")
	}
	return nil
}

// dialPinned резолвит host один раз, проверяет IP по политике и dial'ит IP:port
// (без повторного DNS в default dialer → защита от DNS rebinding).
func dialPinned(ctx context.Context, dialer *net.Dialer, network, addr string, p OutboundPolicy) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := validateIP(ip, p); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}
	ipas, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS: %w", err)
	}
	var lastErr error
	for _, ipa := range ipas {
		if err := validateIP(ipa.IP, p); err != nil {
			lastErr = err
			continue
		}
		c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("нет разрешённых адресов для %s", host)
}

// SafeHTTPClient — клиент с таймаутом, DialContext pin IP и проверкой redirect.
func SafeHTTPClient(timeout time.Duration, p OutboundPolicy) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialPinned(ctx, dialer, network, addr, p)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("слишком много redirect")
			}
			return ValidateOutboundURL(req.URL.String(), p)
		},
	}
}
