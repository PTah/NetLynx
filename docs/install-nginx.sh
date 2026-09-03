#!/usr/bin/env bash
# Установка nginx reverse proxy для NetLynx (generic).
# Вручную: sudo bash docs/install-nginx.sh
# Из deploy.sh: INSTALL_NGINX=1 sudo -E bash docs/deploy.sh
set -euo pipefail

ENV_TARGET="${ENV_TARGET:-/etc/netlynx/netlynx.env}"
SITE_AVAILABLE="/etc/nginx/sites-available/netlynx"
SITE_ENABLED="/etc/nginx/sites-enabled/netlynx"
SSL_DIR="/etc/ssl/netlynx"

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Запустите с sudo: sudo bash docs/install-nginx.sh"
  exit 1
fi

if ! command -v nginx >/dev/null 2>&1; then
  echo "Установка nginx..."
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y nginx
fi

SERVER_NAME="${NGINX_SERVER_NAME:-}"
if [[ -z "${SERVER_NAME}" && -t 0 ]]; then
  read -r -p "Имя хоста (server_name, например netlynx.example.com): " SERVER_NAME
fi
SERVER_NAME="${SERVER_NAME:-netlynx.local}"

MODE="${NGINX_MODE:-}"
if [[ -z "${MODE}" && -t 0 ]]; then
  echo "Режим:"
  echo "  1) HTTP :80 (без TLS)"
  echo "  2) HTTPS :443 (ваши fullchain.pem + privkey.pem)"
  echo "  3) HTTPS с самоподписанным сертификатом (lab)"
  read -r -p "Выбор [1/2/3] (по умолчанию 1): " MODE
fi
MODE="${MODE:-1}"

write_proxy_location() {
  cat <<'EOF'
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }
EOF
}

PUBLIC_URL="http://${SERVER_NAME}"

case "${MODE}" in
  2|https|HTTPS)
    CERT="${NGINX_CERT:-}"
    KEY="${NGINX_KEY:-}"
    if [[ -z "${CERT}" && -t 0 ]]; then
      read -r -p "Путь к fullchain.pem: " CERT
      read -r -p "Путь к privkey.pem: " KEY
    fi
    if [[ ! -f "${CERT}" || ! -f "${KEY}" ]]; then
      echo "ERROR: укажите NGINX_CERT и NGINX_KEY"
      exit 1
    fi
    install -d -m 0755 "${SSL_DIR}"
    install -m 0644 "${CERT}" "${SSL_DIR}/fullchain.pem"
    install -m 0640 "${KEY}" "${SSL_DIR}/privkey.pem"
    PUBLIC_URL="https://${SERVER_NAME}"
    {
      echo "server {"
      echo "    listen 80;"
      echo "    listen [::]:80;"
      echo "    server_name ${SERVER_NAME};"
      echo "    return 301 https://\$host\$request_uri;"
      echo "}"
      echo ""
      echo "server {"
      echo "    listen 443 ssl;"
      echo "    listen [::]:443 ssl;"
      echo "    server_name ${SERVER_NAME};"
      echo "    ssl_certificate     ${SSL_DIR}/fullchain.pem;"
      echo "    ssl_certificate_key ${SSL_DIR}/privkey.pem;"
      echo "    access_log /var/log/nginx/netlynx.access.log;"
      echo "    error_log  /var/log/nginx/netlynx.error.log;"
      write_proxy_location
      echo "}"
    } > "${SITE_AVAILABLE}"
    ;;
  3|self|selfsigned)
    install -d -m 0755 "${SSL_DIR}"
    if [[ ! -f "${SSL_DIR}/fullchain.pem" ]]; then
      openssl req -x509 -nodes -days 825 -newkey rsa:2048 \
        -keyout "${SSL_DIR}/privkey.pem" \
        -out "${SSL_DIR}/fullchain.pem" \
        -subj "/CN=${SERVER_NAME}"
      chmod 0640 "${SSL_DIR}/privkey.pem"
    fi
    NGINX_MODE=2 NGINX_CERT="${SSL_DIR}/fullchain.pem" NGINX_KEY="${SSL_DIR}/privkey.pem" \
      NGINX_SERVER_NAME="${SERVER_NAME}" bash "$(dirname "$0")/install-nginx.sh"
    exit 0
    ;;
  *)
    {
      echo "server {"
      echo "    listen 80;"
      echo "    listen [::]:80;"
      echo "    server_name ${SERVER_NAME};"
      echo "    access_log /var/log/nginx/netlynx.access.log;"
      echo "    error_log  /var/log/nginx/netlynx.error.log;"
      write_proxy_location
      echo "}"
    } > "${SITE_AVAILABLE}"
    ;;
esac

ln -sf "${SITE_AVAILABLE}" "${SITE_ENABLED}"
rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
nginx -t
systemctl enable --now nginx
systemctl reload nginx

if [[ -f "${ENV_TARGET}" ]]; then
  set_env() {
    local key="$1" val="$2"
    if grep -q "^${key}=" "${ENV_TARGET}"; then
      sed -i "s|^${key}=.*|${key}=${val}|" "${ENV_TARGET}"
    else
      echo "${key}=${val}" >> "${ENV_TARGET}"
    fi
  }
  set_env HTTP_ADDR "127.0.0.1:8080"
  set_env NETLYNX_TRUST_PROXY "true"
  set_env NETLYNX_PUBLIC_URL "${PUBLIC_URL}"
  if [[ "${PUBLIC_URL}" == https://* ]]; then
    set_env NETLYNX_COOKIE_SECURE "true"
  fi
fi

echo "OK: nginx → 127.0.0.1:8080, снаружи ${PUBLIC_URL}"
echo "Перезапустите NetLynx: systemctl restart NetLynx.service"
