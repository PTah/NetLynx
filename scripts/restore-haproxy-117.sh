#!/usr/bin/env bash
# Restore HAProxy on 10.0.0.1 (if NetLynx/nginx was installed there by mistake).
set -euo pipefail

HAPROXY_HOST="jdoe@10.0.0.1"
HAPROXY_PASS="${HAPROXY_PASS:?set HAPROXY_PASS}"
SRC="${1:?usage: restore-haproxy-117.sh /path/to/reverse-proxy}"

SSH=(sshpass -p "$HAPROXY_PASS" ssh -o StrictHostKeyChecking=no -o PreferredAuthentications=password -o PubkeyAuthentication=no "$HAPROXY_HOST")
SCP=(sshpass -p "$HAPROXY_PASS" scp -o StrictHostKeyChecking=no -o PreferredAuthentications=password -o PubkeyAuthentication=no)

for f in haproxy.cfg git-sac-allowed netlynx-allowed zabbix-allowed syno-allowed 1c-allowed stats-allowed rds-allowed-static; do
  "${SCP[@]}" "$SRC/$f" "$HAPROXY_HOST:/tmp/$f"
done

"${SSH[@]}" "echo '$HAPROXY_PASS' | sudo -S bash -s" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

echo "[1/6] stop NetLynx/nginx on HAProxy host"
systemctl stop NetLynx.service nginx 2>/dev/null || true
systemctl disable NetLynx.service nginx 2>/dev/null || true

echo "[2/6] install haproxy"
apt-get update -qq
apt-get install -y -qq haproxy
mkdir -p /etc/haproxy

echo "[3/6] install config + allowlists"
cp /tmp/haproxy.cfg /etc/haproxy/haproxy.cfg
cp /tmp/git-sac-allowed /tmp/netlynx-allowed /tmp/zabbix-allowed /tmp/syno-allowed /tmp/1c-allowed /tmp/stats-allowed /etc/haproxy/
cp /tmp/rds-allowed-static /etc/haproxy/rds-allowed

echo "[4/6] validate"
haproxy -c -f /etc/haproxy/haproxy.cfg

echo "[5/6] enable haproxy"
systemctl enable haproxy
systemctl restart haproxy

echo "[6/6] verify"
systemctl is-active haproxy
ss -tlnp | grep haproxy || true
grep 'Config version' /etc/haproxy/haproxy.cfg
head -5 /etc/haproxy/git-sac-allowed
REMOTE

echo "HAPROXY_RESTORE_OK"
