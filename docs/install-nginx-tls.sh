#!/usr/bin/env bash
# Установка nginx + wildcard TLS перед NetLynx.
# Сертификаты на шаре (предпочтительно Apache-экспорт):
#   //10.0.0.1/Soft/Certs/2026/Export/apache
#     wildcard_kalinamall_ru.crt + intermediate_pem_globalsign_ssl_ov_wildcard_1.crt → fullchain
#     wildcard_kalinamall_ru.key → privkey
# Запасной вариант (pfx-pem): public-cert.pem + privatekey.pem
set -euo pipefail

CERT_APACHE="${CERT_APACHE:-//10.0.0.1/Soft/Certs/2026/Export/apache}"
CERT_PFX_PEM="${CERT_PFX_PEM:-//10.0.0.1/Soft/Certs/2026/Export/pfx-pem}"
SSL_DIR="${SSL_DIR:-/etc/ssl/example}"
NGINX_SITE="${NGINX_SITE:-/etc/nginx/sites-available/netlynx}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Запускайте от root: sudo bash docs/install-nginx-tls.sh"
  exit 1
fi

echo "[1/6] nginx"
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y nginx smbclient

echo "[2/6] Каталог TLS ${SSL_DIR}"
install -d -m 0755 "${SSL_DIR}"
WORK="$(mktemp -d)"
cleanup() { rm -rf "${WORK}"; }
trap cleanup EXIT

# //host/share/path → host, share, path (для smbclient)
smb_parse() {
  local uri="$1"
  uri="${uri#//}"
  SMB_HOST="${uri%%/*}"
  local rest="${uri#*/}"
  SMB_SHARE="${rest%%/*}"
  SMB_SUBPATH="${rest#*/}"
  if [[ "${SMB_SUBPATH}" == "${SMB_SHARE}" ]]; then
    SMB_SUBPATH=""
  fi
}

smb_fetch() {
  local uri="$1"
  local remote_name="$2"
  local local_path="$3"
  smb_parse "${uri}"
  local cd_cmd=""
  if [[ -n "${SMB_SUBPATH}" ]]; then
    cd_cmd="cd ${SMB_SUBPATH}; "
  fi
  if smbclient "//${SMB_HOST}/${SMB_SHARE}" -N -c "${cd_cmd}get \"${remote_name}\" \"${local_path}\"" >/dev/null 2>&1; then
    [[ -s "${local_path}" ]]
    return 0
  fi
  return 1
}

copy_local() {
  local src_dir="$1"
  local name="$2"
  local dest="$3"
  if [[ -f "${src_dir}/${name}" ]]; then
    install -m 0644 "${src_dir}/${name}" "${dest}"
    return 0
  fi
  return 1
}

install_from_apache() {
  local src="${1}"
  local leaf="${WORK}/wildcard_kalinamall_ru.crt"
  local inter="${WORK}/intermediate_pem_globalsign_ssl_ov_wildcard_1.crt"
  local key="${WORK}/wildcard_kalinamall_ru.key"
  local got=0

  if [[ -d "${src}" ]]; then
    copy_local "${src}" "wildcard_kalinamall_ru.crt" "${leaf}" && got=$((got + 1)) || true
    copy_local "${src}" "intermediate_pem_globalsign_ssl_ov_wildcard_1.crt" "${inter}" && got=$((got + 1)) || true
    copy_local "${src}" "wildcard_kalinamall_ru.key" "${key}" && got=$((got + 1)) || true
  else
    smb_fetch "${src}" "wildcard_kalinamall_ru.crt" "${leaf}" && got=$((got + 1)) || true
    smb_fetch "${src}" "intermediate_pem_globalsign_ssl_ov_wildcard_1.crt" "${inter}" && got=$((got + 1)) || true
    smb_fetch "${src}" "wildcard_kalinamall_ru.key" "${key}" && got=$((got + 1)) || true
  fi

  if [[ "${got}" -lt 3 ]]; then
    return 1
  fi

  cat "${leaf}" "${inter}" > "${SSL_DIR}/fullchain.pem"
  install -m 0640 "${key}" "${SSL_DIR}/privkey.pem"
  echo "Источник: Apache export (${src})"
  return 0
}

install_from_pfx_pem() {
  local src="${1}"
  local pub="${WORK}/public-cert.pem"
  local key="${WORK}/privatekey.pem"

  if [[ -d "${src}" ]]; then
    copy_local "${src}" "public-cert.pem" "${pub}" || return 1
    copy_local "${src}" "privatekey.pem" "${key}" || return 1
  else
    smb_fetch "${src}" "public-cert.pem" "${pub}" || return 1
    smb_fetch "${src}" "privatekey.pem" "${key}" || return 1
  fi

  install -m 0644 "${pub}" "${SSL_DIR}/fullchain.pem"
  install -m 0640 "${key}" "${SSL_DIR}/privkey.pem"
  echo "Источник: pfx-pem export (${src})"
  return 0
}

echo "[3/6] Копирование сертификата"
if [[ -n "${CERT_FULLCHAIN:-}" && -n "${CERT_PRIVKEY:-}" ]]; then
  install -m 0644 "${CERT_FULLCHAIN}" "${SSL_DIR}/fullchain.pem"
  install -m 0640 "${CERT_PRIVKEY}" "${SSL_DIR}/privkey.pem"
  echo "Источник: CERT_FULLCHAIN / CERT_PRIVKEY (env)"
elif install_from_apache "${CERT_APACHE}"; then
  :
elif install_from_pfx_pem "${CERT_PFX_PEM}"; then
  :
else
  echo "Не удалось получить сертификат."
  echo "Apache (предпочтительно): ${CERT_APACHE}"
  echo "  wildcard_kalinamall_ru.crt"
  echo "  intermediate_pem_globalsign_ssl_ov_wildcard_1.crt"
  echo "  wildcard_kalinamall_ru.key"
  echo "Запасной pfx-pem: ${CERT_PFX_PEM} (public-cert.pem + privatekey.pem)"
  echo ""
  echo "Смонтируйте SMB или скопируйте вручную:"
  echo "  sudo mkdir -p ${SSL_DIR}"
  echo "  sudo bash -c 'cat leaf.crt intermediate.crt > ${SSL_DIR}/fullchain.pem'"
  echo "  sudo install -m 0640 wildcard.key ${SSL_DIR}/privkey.pem"
  exit 1
fi

chmod 0644 "${SSL_DIR}/fullchain.pem"
chmod 0640 "${SSL_DIR}/privkey.pem"
chown root:root "${SSL_DIR}/fullchain.pem" "${SSL_DIR}/privkey.pem"

echo "[4/6] Конфиг nginx"
install -m 0644 "${ROOT_DIR}/deploy/nginx/netlynx.conf" "${NGINX_SITE}"
ln -sf "${NGINX_SITE}" /etc/nginx/sites-enabled/netlynx
if [[ -f /etc/nginx/sites-enabled/default ]]; then
  rm -f /etc/nginx/sites-enabled/default
fi
nginx -t

echo "[5/6] netlynx.env — HTTPS за прокси"
ENV="/etc/netlynx/netlynx.env"
if [[ -f "${ENV}" ]]; then
  grep -q '^HTTP_ADDR=' "${ENV}" && sed -i 's|^HTTP_ADDR=.*|HTTP_ADDR=127.0.0.1:8080|' "${ENV}" || echo 'HTTP_ADDR=127.0.0.1:8080' >> "${ENV}"
  grep -q '^NETLYNX_COOKIE_SECURE=' "${ENV}" && sed -i 's|^NETLYNX_COOKIE_SECURE=.*|NETLYNX_COOKIE_SECURE=true|' "${ENV}" || echo 'NETLYNX_COOKIE_SECURE=true' >> "${ENV}"
  grep -q '^NETLYNX_TRUST_PROXY=' "${ENV}" && sed -i 's|^NETLYNX_TRUST_PROXY=.*|NETLYNX_TRUST_PROXY=true|' "${ENV}" || echo 'NETLYNX_TRUST_PROXY=true' >> "${ENV}"
  grep -q '^NETLYNX_PUBLIC_URL=' "${ENV}" && sed -i 's|^NETLYNX_PUBLIC_URL=.*|NETLYNX_PUBLIC_URL=https://netlynx.example.com|' "${ENV}" || echo 'NETLYNX_PUBLIC_URL=https://netlynx.example.com' >> "${ENV}"
else
  echo "WARNING: ${ENV} не найден — настройте HTTP_ADDR и NETLYNX_* вручную после deploy.sh"
fi

echo "[6/6] Перезапуск"
systemctl enable nginx
systemctl reload nginx
if systemctl is-active --quiet NetLynx.service; then
  systemctl restart NetLynx.service
fi

echo "Готово. Проверка:"
echo "  curl -fsS https://netlynx.example.com/health"
echo "DNS: A netlynx.example.com → IP этого сервера"
