package syslogrecv

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// FlapHandler обрабатывает разобранный MAC flap (обычно poller.EmitMACFlappingFromSyslog).
type FlapHandler func(ctx context.Context, deviceID int64, mac string, portA, portB int, vlan *int, rawMsg string)

// Receiver — UDP syslog listener.
type Receiver struct {
	log    *slog.Logger
	st     *store.Store
	listen string
	onFlap FlapHandler
}

func New(log *slog.Logger, st *store.Store, listenAddr string, onFlap FlapHandler) *Receiver {
	if log == nil {
		log = slog.Default()
	}
	return &Receiver{
		log:    log,
		st:     st,
		listen: strings.TrimSpace(listenAddr),
		onFlap: onFlap,
	}
}

func (r *Receiver) Enabled() bool {
	return r.listen != "" && r.onFlap != nil
}

func (r *Receiver) Run(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp", r.listen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	r.log.Info("syslog receiver listen", "addr", r.listen)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, 65535)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			r.log.Warn("syslog read", "err", err)
			continue
		}
		if n == 0 || remote == nil {
			continue
		}
		raw := string(buf[:n])
		r.handle(ctx, remote.IP.String(), raw)
	}
}

func (r *Receiver) handle(ctx context.Context, sourceIP, raw string) {
	body := StripSyslogHeader(raw)
	flap, ok := ParseMACFlapping(body)
	if !ok {
		return
	}
	deviceID, found, err := r.st.FindDeviceIDByHost(ctx, sourceIP)
	if err != nil {
		r.log.Warn("syslog device lookup", "ip", sourceIP, "err", err)
		return
	}
	if !found {
		r.log.Debug("syslog from unknown host", "ip", sourceIP, "mac", flap.MAC)
		return
	}
	portA, okA, err := r.st.FindIfIndexByPortName(ctx, deviceID, flap.PortA)
	if err != nil {
		r.log.Warn("syslog port A", "err", err)
		return
	}
	portB, okB, err := r.st.FindIfIndexByPortName(ctx, deviceID, flap.PortB)
	if err != nil {
		r.log.Warn("syslog port B", "err", err)
		return
	}
	if !okA || !okB {
		r.log.Warn("syslog ports unresolved", "device_id", deviceID, "a", flap.PortA, "b", flap.PortB, "okA", okA, "okB", okB)
		// всё равно эмитим с 0-индексами нельзя — нужен хотя бы один
		if !okA && !okB {
			return
		}
		if !okA {
			portA = portB
		}
		if !okB {
			portB = portA
		}
	}
	hctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r.onFlap(hctx, deviceID, flap.MAC, portA, portB, flap.VLAN, flap.Raw)
}
