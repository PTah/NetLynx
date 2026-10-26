#!/usr/bin/env bash
# Install NetLynx remote syslog archive receiver (rsyslog + logrotate).
# Safe to re-run. Does not touch device configs — see docs/configure-device-syslog.py
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF_SRC="${ROOT_DIR}/deploy/rsyslog/35-netlynx-remote.conf"
ROTATE_SRC="${ROOT_DIR}/deploy/rsyslog/logrotate-remotelogs"
AA_SRC="${ROOT_DIR}/deploy/rsyslog/apparmor-local-usr.sbin.rsyslogd"
CONF_DST="/etc/rsyslog.d/35-netlynx-remote.conf"
ROTATE_DST="/etc/logrotate.d/netlynx-remotelogs"
AA_DST="/etc/apparmor.d/local/usr.sbin.rsyslogd"
LOG_ROOT="/var/lib/remotelogs"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: run as root (sudo bash docs/setup-remote-syslog.sh)"
  exit 1
fi

if [[ ! -f "$CONF_SRC" || ! -f "$ROTATE_SRC" ]]; then
  echo "ERROR: missing deploy/rsyslog files under ${ROOT_DIR}"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
if ! dpkg -s rsyslog >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq rsyslog
fi

install -d -o syslog -g adm -m 0750 "$LOG_ROOT"
install -m 0644 "$CONF_SRC" "$CONF_DST"
install -m 0644 "$ROTATE_SRC" "$ROTATE_DST"

# AppArmor: stock profile allows /var/log/** but not /var/lib/remotelogs/**
if [[ -f "$AA_SRC" ]]; then
  install -d -m 0755 /etc/apparmor.d/local
  # Merge marker block (idempotent): replace previous NetLynx block or append.
  if [[ -f "$AA_DST" ]] && grep -q 'NetLynx remote syslog archive' "$AA_DST" 2>/dev/null; then
    # rewrite whole local file from our template if it only contains our block — keep simple:
    install -m 0644 "$AA_SRC" "$AA_DST"
  else
    if [[ -s "$AA_DST" ]]; then
      cat "$AA_SRC" >>"$AA_DST"
    else
      install -m 0644 "$AA_SRC" "$AA_DST"
    fi
  fi
  if command -v apparmor_parser >/dev/null 2>&1; then
    apparmor_parser -r /etc/apparmor.d/usr.sbin.rsyslogd 2>/dev/null || true
  fi
fi

# Prefer our drop-in inputs; keep stock imudp/imtcp lines commented in rsyslog.conf.
if grep -qE '^[[:space:]]*module\(load="imudp"' /etc/rsyslog.conf 2>/dev/null; then
  echo "WARN: imudp already loaded in /etc/rsyslog.conf — comment it out if rsyslog fails to start"
fi
if grep -qE '^[[:space:]]*input\(type="imudp"' /etc/rsyslog.conf 2>/dev/null; then
  echo "WARN: imudp input already in /etc/rsyslog.conf — duplicate 514 bind will fail"
fi

# Larger socket buffers help under STP/BPDU storms (best-effort).
if [[ -w /etc/sysctl.d ]]; then
  cat >/etc/sysctl.d/30-netlynx-rsyslog.conf <<'EOF'
net.core.rmem_max = 33554432
net.core.rmem_default = 8388608
EOF
  sysctl --system >/dev/null 2>&1 || sysctl -p /etc/sysctl.d/30-netlynx-rsyslog.conf >/dev/null 2>&1 || true
fi

# Open LAN syslog if ufw is active (idempotent).
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi 'Status: active'; then
  ufw allow from 10.0.0.0/8 to any port 514 proto udp comment 'NetLynx remote syslog' || true
  ufw allow from 10.0.0.0/8 to any port 514 proto tcp comment 'NetLynx remote syslog' || true
  ufw allow from 172.16.0.0/12 to any port 514 proto udp comment 'NetLynx remote syslog' || true
  ufw allow from 172.16.0.0/12 to any port 514 proto tcp comment 'NetLynx remote syslog' || true
  ufw allow from 10.0.0.1/16 to any port 514 proto udp comment 'NetLynx remote syslog' || true
  ufw allow from 10.0.0.1/16 to any port 514 proto tcp comment 'NetLynx remote syslog' || true
fi

echo "[check] rsyslog config"
rsyslogd -N1
systemctl enable rsyslog >/dev/null 2>&1 || true
systemctl restart rsyslog
sleep 1
systemctl --no-pager --full status rsyslog | head -20

echo "[check] listen 514"
ss -ulnp | grep -E ':514\b' || { echo "ERROR: UDP 514 not listening"; exit 1; }
ss -tlnp | grep -E ':514\b' || echo "WARN: TCP 514 not listening (UDP-only devices still OK)"

# Smoke: local UDP sample into archive path
python3 - <<'PY'
import socket, time
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.sendto(b"<14>Oct 24 12:00:00 netlynx-selftest NetLynx remote syslog self-test\n", ("127.0.0.1", 514))
s.close()
time.sleep(0.4)
print("self-test datagram sent to 127.0.0.1:514")
PY

if [[ -f "${LOG_ROOT}/127.0.0.1/syslog.log" ]]; then
  echo "OK: ${LOG_ROOT}/127.0.0.1/syslog.log"
  tail -n 2 "${LOG_ROOT}/127.0.0.1/syslog.log" || true
else
  echo "WARN: self-test file not yet visible (check permissions / SELinux); rsyslog is up"
fi

echo
echo "Remote archive root: ${LOG_ROOT}/<FROMHOST-IP>/syslog.log"
echo "Rotation: daily, keep 60 days (logrotate ${ROTATE_DST})"
echo "App fan-out: UDP 127.0.0.1:9514 (requires NETLYNX_SYSLOG_LISTEN=:9514)"
