#!/usr/bin/env bash
# Финализация прода после Migrate-Prod-Invetor-to-NetLynx: убрать legacy Invetor, оставить NetLynx.
# Запуск на сервере NetLynx (10.0.0.1): sudo bash scripts/finalize-server-netlynx.sh
set -euo pipefail

TS="$(date +%Y%m%d)"
BAK="/root/legacy-invetor-${TS}"

echo "[1/8] Проверка NetLynx"
systemctl is-active --quiet NetLynx.service
curl -fsS http://127.0.0.1:8080/health >/dev/null
curl -fsSk https://netlynx.example.com/health >/dev/null
echo "  NetLynx OK"

echo "[2/8] Invetor.service / invetord"
systemctl disable --now Invetor.service 2>/dev/null || true
rm -f /usr/local/bin/invetord
rm -f /etc/systemd/system/Invetor.service
systemctl daemon-reload

echo "[3/8] Архив legacy-каталогов в ${BAK}"
install -d -m 0700 "${BAK}"
move_legacy() {
  local src="$1"
  local dest_name="$2"
  if [[ -e "$src" ]]; then
    mv "$src" "${BAK}/${dest_name}"
    echo "  moved $src -> ${BAK}/${dest_name}"
  fi
}
move_legacy /root/Invetor root-Invetor
move_legacy /etc/invetor etc-invetor
move_legacy /opt/invetor opt-invetor-metadata
move_legacy /var/lib/invetor var-lib-invetor
if [[ -d /var/backups/invetor ]]; then
  install -d -m 0750 -o netlynx -g netlynx /var/backups/netlynx
  rsync -a /var/backups/invetor/ /var/backups/netlynx/ 2>/dev/null || true
  mv /var/backups/invetor "${BAK}/backups-invetor"
fi

echo "[4/8] PostgreSQL (контейнер invetor-postgres-1 — не трогаем)"
docker ps --format '{{.Names}}' | grep -qx invetor-postgres-1
docker exec invetor-postgres-1 pg_isready -U invetor -d invetor

echo "[5/8] backup_settings: пути invetor -> netlynx"
docker exec invetor-postgres-1 psql -U invetor -d invetor -v ON_ERROR_STOP=1 -c "
UPDATE backup_settings SET
  local_dir = '/var/backups/netlynx',
  updated_at = now()
WHERE id = 1 AND local_dir LIKE '%invetor%';
"

echo "[6/8] git-клон NetLynx"
if [[ ! -d /root/NetLynx/.git ]]; then
  echo "ERROR: /root/NetLynx missing — clone NetLynx first"
  exit 1
fi
grep -q 'jdoe/NetLynx' /root/NetLynx/.git/config

echo "[7/8] каталоги netlynx"
install -d -m 0755 /opt/netlynx /etc/netlynx /var/lib/netlynx/web /var/backups/netlynx
chown -R netlynx:netlynx /var/lib/netlynx /var/backups/netlynx

echo "[8/8] итог"
systemctl is-active NetLynx.service nginx
echo "Legacy backup: ${BAK}"
echo "URL: https://netlynx.example.com"
echo "Deploy updates: cd /root/NetLynx && bash docs/deploy.sh"
echo "Done."
