#!/usr/bin/env bash
set -euo pipefail
export GIT_SSH_COMMAND='ssh -i /root/.ssh/id_ed25519_invetor_deploy -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new'

awk '!seen[$0]++' /etc/netlynx/netlynx.env > /tmp/netlynx.env.new
install -m 0640 -o root -g netlynx /tmp/netlynx.env.new /etc/netlynx/netlynx.env
rm -f /tmp/netlynx.env.new

cd /root/NetLynx
echo "=== deploy NetLynx ==="
env GIT_SSH_COMMAND="$GIT_SSH_COMMAND" REMOTE=origin BRANCH=main bash docs/deploy.sh

echo "=== nginx + TLS ==="
bash docs/install-nginx-tls.sh

echo "=== disable Invetor ==="
systemctl disable --now Invetor.service 2>/dev/null || true

echo "=== verify ==="
curl -fsS http://127.0.0.1:8080/health
echo
curl -fsSk --resolve netlynx.example.com:443:127.0.0.1 https://netlynx.example.com/health
echo
systemctl is-active NetLynx.service nginx
