#!/usr/bin/env bash
set -euo pipefail
KEY=/tmp/haproxy_deploy_key
chmod 600 "$KEY"
H=jdoe@10.0.0.1
for f in haproxy.cfg netlynx-allowed deploy-haproxy-http-fix.sh; do
  scp -i "$KEY" -o IdentitiesOnly=yes "/tmp/$f" "$H:/tmp/$f"
done
ssh -i "$KEY" -o IdentitiesOnly=yes "$H" "sed -i 's/\r$//' /tmp/deploy-haproxy-http-fix.sh && sudo cp /tmp/netlynx-allowed /etc/haproxy/netlynx-allowed && sudo bash /tmp/deploy-haproxy-http-fix.sh /tmp/haproxy.cfg"
rm -f "$KEY"
echo DEPLOY_OK
