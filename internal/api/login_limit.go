package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type loginBucket struct {
	fails    int
	blocked  time.Time
	windowAt time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	byIP    map[string]*loginBucket
	byUser  map[string]*loginBucket
	maxFail int
	window  time.Duration
	blockFor time.Duration
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{
		byIP:     make(map[string]*loginBucket),
		byUser:   make(map[string]*loginBucket),
		maxFail:  8,
		window:   5 * time.Minute,
		blockFor: 2 * time.Minute,
	}
	go l.gcLoop()
	return l
}

func (l *loginLimiter) gcLoop() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		l.gc()
	}
}

func (l *loginLimiter) gc() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	prune := func(m map[string]*loginBucket) {
		for k, b := range m {
			if b == nil {
				delete(m, k)
				continue
			}
			idle := now.Sub(b.windowAt) > l.window*2 && now.After(b.blocked)
			if idle || (b.fails == 0 && now.After(b.blocked) && now.Sub(b.windowAt) > l.window) {
				delete(m, k)
			}
		}
	}
	prune(l.byIP)
	prune(l.byUser)
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func (l *loginLimiter) allow(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	return l.allowBucket(l.byIP, ip, now) && l.allowBucket(l.byUser, strings.ToLower(username), now)
}

func (l *loginLimiter) allowBucket(m map[string]*loginBucket, key string, now time.Time) bool {
	if key == "" {
		return true
	}
	b := m[key]
	if b == nil {
		return true
	}
	if now.Before(b.blocked) {
		return false
	}
	if now.Sub(b.windowAt) > l.window {
		b.fails = 0
		b.windowAt = now
	}
	return true
}

func (l *loginLimiter) fail(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.failBucket(l.byIP, ip, now)
	l.failBucket(l.byUser, strings.ToLower(username), now)
}

func (l *loginLimiter) failBucket(m map[string]*loginBucket, key string, now time.Time) {
	if key == "" {
		return
	}
	b := m[key]
	if b == nil {
		b = &loginBucket{windowAt: now}
		m[key] = b
	}
	if now.Sub(b.windowAt) > l.window {
		b.fails = 0
		b.windowAt = now
	}
	b.fails++
	if b.fails >= l.maxFail {
		b.blocked = now.Add(l.blockFor)
		b.fails = 0
		b.windowAt = now
	}
}

func (l *loginLimiter) success(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byIP, ip)
	delete(l.byUser, strings.ToLower(username))
}
