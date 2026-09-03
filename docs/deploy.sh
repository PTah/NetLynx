#!/usr/bin/env bash
set -euo pipefail

# NetLynx production deploy script (single entrypoint).
# Pulls latest code, builds artifacts, installs runtime layout, restarts systemd service.

APP_NAME="NetLynx"
SERVICE_NAME="NetLynx.service"
REMOTE="${REMOTE:-origin}"
BRANCH="${BRANCH:-main}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/health}"

OPT_DIR="/opt/netlynx"
ETC_DIR="/etc/netlynx"
VAR_DIR="/var/lib/netlynx"
WEB_TARGET_DIR="${VAR_DIR}/web"
BIN_TARGET="/usr/local/bin/netlynxd"
ENV_TARGET="${ETC_DIR}/netlynx.env"
SERVICE_TARGET="/etc/systemd/system/${SERVICE_NAME}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

if [[ ! -f VERSION || ! -f go.mod || ! -d cmd/netlynxd ]]; then
  echo "ERROR: run from repository root"
  exit 1
fi

for bin in git go npm sudo; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "ERROR: $bin not found"
    exit 1
  fi
done

STASH_CREATED=0
STASH_NAME="deploy-safe-$(date +%Y%m%d-%H%M%S)"
BUILD_OUT="$(mktemp -d)"
cleanup() { rm -rf "$BUILD_OUT"; }
trap cleanup EXIT

echo "[1/12] Check local changes"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "Local changes found -> stash: ${STASH_NAME}"
  git stash push -u -m "${STASH_NAME}"
  STASH_CREATED=1
else
  echo "Working tree is clean."
fi

echo "[2/12] Update git branch (${REMOTE}/${BRANCH})"
git fetch "${REMOTE}"
git checkout "${BRANCH}"
git pull --rebase "${REMOTE}" "${BRANCH}"

VER="$(tr -d '\r\n' < VERSION | head -1)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "nogit")"
BUILT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "version=${VER} commit=${COMMIT} built_at=${BUILT}"

echo "[3/12] Build web"
cd web
npm install
npm run build
cd "${ROOT_DIR}"

echo "[4/12] Resolve Go modules"
if [[ ! -f go.sum ]]; then
  go mod tidy
else
  go mod download || go mod tidy
fi

echo "[5/12] Build binaries"
go build \
  -ldflags "-X main.version=${VER} -X main.commit=${COMMIT} -X main.builtAt=${BUILT}" \
  -o "${BUILD_OUT}/netlynxd" \
  ./cmd/netlynxd
go build -o "${BUILD_OUT}/fetch-ssh-hostkey" ./cmd/fetch-ssh-hostkey

echo "[6/12] Prepare system user and directories"
if ! id -u netlynx >/dev/null 2>&1; then
  sudo useradd --system --home "${VAR_DIR}" --shell /usr/sbin/nologin netlynx || true
fi
# Журнал в UI читает journalctl от имени службы — нужна группа systemd-journal.
if getent group systemd-journal >/dev/null 2>&1; then
  sudo usermod -aG systemd-journal netlynx || true
fi
sudo install -d -m 0755 "${OPT_DIR}" "${ETC_DIR}" "${VAR_DIR}" "${WEB_TARGET_DIR}"
sudo install -d -m 0750 -o netlynx -g netlynx /var/backups/netlynx
sudo chown -R netlynx:netlynx "${VAR_DIR}"
if ! command -v pg_dump >/dev/null 2>&1; then
  echo "Installing postgresql-client (pg_dump for backups)..."
  sudo apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql-client \
    || echo "WARNING: pg_dump not installed; UI backup will dump via DATABASE_URL (service user cannot use docker.sock)"
fi

echo "[7/12] Install binary and web assets"
sudo install -m 0755 "${BUILD_OUT}/netlynxd" "${BIN_TARGET}"
sudo install -m 0755 "${BUILD_OUT}/fetch-ssh-hostkey" "/usr/local/bin/fetch-ssh-hostkey"
if command -v rsync >/dev/null 2>&1; then
  sudo rsync -a --delete "web/dist/" "${WEB_TARGET_DIR}/"
else
  sudo rm -rf "${WEB_TARGET_DIR:?}/"*
  sudo cp -a web/dist/. "${WEB_TARGET_DIR}/"
fi
sudo chown -R netlynx:netlynx "${WEB_TARGET_DIR}"

echo "[8/12] Install env + service"
if [[ ! -f "${ENV_TARGET}" ]]; then
  sudo cp .env.example "${ENV_TARGET}"
  sudo chmod 0640 "${ENV_TARGET}"
  sudo chown root:netlynx "${ENV_TARGET}"
  sudo sed -i 's|^WEB_DIST=.*|WEB_DIST=/var/lib/netlynx/web|' "${ENV_TARGET}" || true
  echo "Created ${ENV_TARGET} from .env.example (edit secrets before production use)."
fi
sudo install -m 0644 "deploy/NetLynx.service" "${SERVICE_TARGET}"
if [[ -d deploy/ssh_config.d ]]; then
  sudo install -d -m 0755 /etc/ssh/ssh_config.d
  sudo install -m 0644 deploy/ssh_config.d/*.conf /etc/ssh/ssh_config.d/
fi

echo "[9/12] Write release metadata"
echo "${VER}" | sudo tee "${OPT_DIR}/VERSION" >/dev/null
echo "${ROOT_DIR}" | sudo tee "${OPT_DIR}/repo.path" >/dev/null
{
  echo "version=${VER}"
  echo "commit=${COMMIT}"
  echo "built_at=${BUILT}"
  echo "repo=${ROOT_DIR}"
} | sudo tee "${OPT_DIR}/release.env" >/dev/null

echo "[10/12] PostgreSQL (docker compose)"
if ! command -v docker >/dev/null 2>&1; then
  echo "WARNING: docker not found; PostgreSQL not started."
elif [[ ! -f docker-compose.yml ]]; then
  echo "WARNING: docker-compose.yml missing in ${ROOT_DIR}; PostgreSQL not started."
else
  PG_OK=0
  if docker ps --format '{{.Names}}' | grep -qx 'invetor-postgres-1'; then
    echo "postgres: existing container invetor-postgres-1 (keep volume invetor_pg)"
  fi
  docker compose up -d postgres
  for i in $(seq 1 30); do
    if docker compose exec -T postgres pg_isready -U invetor -d invetor >/dev/null 2>&1; then
      PG_OK=1
      break
    fi
    echo "postgres not ready (try ${i}/30), wait 2s..."
    sleep 2
  done
  if [[ "${PG_OK}" -ne 1 ]]; then
    echo "ERROR: PostgreSQL did not become ready."
    docker compose logs --tail=80 postgres || true
    exit 1
  fi
  echo "postgres ready."
fi

echo "[11/12] Restart service + health"
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}"
sudo systemctl restart "${SERVICE_NAME}"
sudo systemctl --no-pager --full status "${SERVICE_NAME}" | sed -n '1,20p'

OK=0
for i in 1 2 3 4 5; do
  if BODY="$(curl -fsS --max-time 5 "${HEALTH_URL}" 2>/dev/null)"; then
    echo "health: ${BODY}"
    OK=1
    break
  fi
  echo "health not ready (try ${i}/5), wait 2s..."
  sleep 2
done
if [[ "${OK}" -ne 1 ]]; then
  echo "ERROR: ${HEALTH_URL} did not respond after retries."
  echo "Recent service logs:"
  sudo journalctl -u "${SERVICE_NAME}" -n 80 --no-pager || true
  exit 1
fi

echo "[12/13] Restore stash (if created)"
if [[ "${STASH_CREATED}" -eq 1 ]]; then
  # Build may touch dependency lock files. Restore them before stash pop to avoid overwrite errors.
  git restore go.mod 2>/dev/null || git checkout HEAD -- go.mod 2>/dev/null || true
  if git ls-files --error-unmatch go.sum >/dev/null 2>&1; then
    git restore go.sum 2>/dev/null || git checkout HEAD -- go.sum 2>/dev/null || true
  else
    rm -f go.sum
  fi
  if git ls-files --error-unmatch web/package-lock.json >/dev/null 2>&1; then
    git restore web/package-lock.json 2>/dev/null || git checkout HEAD -- web/package-lock.json 2>/dev/null || true
  else
    rm -f web/package-lock.json
  fi
  STASH_REF="$(git stash list | awk -v n="${STASH_NAME}" 'index($0,n){ref=$1; sub(/:$/,"",ref); print ref; exit}')"
  if [[ -n "${STASH_REF:-}" ]]; then
    if git stash pop "${STASH_REF}"; then
      echo "stash restored."
    else
      echo "WARNING: stash pop conflict. Check git status."
      exit 2
    fi
  fi
fi

echo "[13/13] Доступ к UI: nginx или напрямую :8080"
# Уже настроенный сайт — не спрашивать снова и не трогать HTTP_ADDR (часто 127.0.0.1:8080 за nginx).
NGINX_SITE_EXISTS=0
if [[ -e /etc/nginx/sites-enabled/netlynx || -e /etc/nginx/sites-available/netlynx ]]; then
  NGINX_SITE_EXISTS=1
fi

USE_NGINX=0
SKIP_HTTP_ADDR_REWRITE=0
if [[ "${INSTALL_NGINX:-}" == "1" || "${INSTALL_NGINX:-}" == "yes" ]]; then
  USE_NGINX=1
elif [[ "${INSTALL_NGINX:-}" == "0" || "${INSTALL_NGINX:-}" == "no" ]]; then
  USE_NGINX=0
  if [[ "${NGINX_SITE_EXISTS}" -eq 1 ]]; then
    SKIP_HTTP_ADDR_REWRITE=1
  fi
elif [[ "${NGINX_SITE_EXISTS}" -eq 1 ]]; then
  USE_NGINX=0
  SKIP_HTTP_ADDR_REWRITE=1
  PUB="$(sudo grep -E '^NETLYNX_PUBLIC_URL=' "${ENV_TARGET}" 2>/dev/null | head -1 | cut -d= -f2- || true)"
  echo "nginx для NetLynx уже есть — шаг установки пропущен."
  echo "UI: ${PUB:-http://<hostname>/} (backend обычно 127.0.0.1:8080)"
elif [[ -t 0 && -z "${INSTALL_NGINX:-}" ]]; then
  read -r -p "Установить nginx reverse proxy перед NetLynx? [y/N] " NGINX_ANS
  case "${NGINX_ANS}" in
    y|Y|yes|Yes|YES) USE_NGINX=1 ;;
  esac
fi

if [[ "${USE_NGINX}" -eq 1 ]]; then
  echo "Запуск docs/install-nginx.sh ..."
  ENV_TARGET="${ENV_TARGET}" sudo bash "${ROOT_DIR}/docs/install-nginx.sh"
  sudo systemctl restart "${SERVICE_NAME}" || true
  PUB="$(sudo grep -E '^NETLYNX_PUBLIC_URL=' "${ENV_TARGET}" 2>/dev/null | head -1 | cut -d= -f2- || true)"
  echo "UI через nginx: ${PUB:-http(s)://<hostname>}"
elif [[ "${SKIP_HTTP_ADDR_REWRITE}" -eq 1 ]]; then
  : # конфиг nginx/HTTP_ADDR уже на месте
else
  if [[ -f "${ENV_TARGET}" ]]; then
    if grep -q '^HTTP_ADDR=' "${ENV_TARGET}"; then
      sudo sed -i 's|^HTTP_ADDR=.*|HTTP_ADDR=:8080|' "${ENV_TARGET}"
    else
      echo 'HTTP_ADDR=:8080' | sudo tee -a "${ENV_TARGET}" >/dev/null
    fi
    grep -q '^NETLYNX_COOKIE_SECURE=' "${ENV_TARGET}" || echo 'NETLYNX_COOKIE_SECURE=false' | sudo tee -a "${ENV_TARGET}" >/dev/null
  fi
  sudo systemctl restart "${SERVICE_NAME}" || true
  echo "UI напрямую: http://<server-ip>:8080"
fi

echo "Done. Installed:"
echo "  binary:  ${BIN_TARGET}"
echo "  config:  ${ENV_TARGET}"
echo "  web:     ${WEB_TARGET_DIR}"
echo "  service: ${SERVICE_TARGET}"
echo "  repo:    ${ROOT_DIR} (docker compose postgres — отсюда)"
echo ""
ADMIN_U="$(sudo grep -E '^NETLYNX_ADMIN_USER=' "${ENV_TARGET}" 2>/dev/null | head -1 | cut -d= -f2- || true)"
ADMIN_U="${ADMIN_U:-admin}"
echo "UI login (bootstrap admin):"
if [[ "${USE_NGINX:-0}" -eq 1 || "${SKIP_HTTP_ADDR_REWRITE:-0}" -eq 1 ]]; then
  PUB_URL="$(sudo grep -E '^NETLYNX_PUBLIC_URL=' "${ENV_TARGET}" 2>/dev/null | head -1 | cut -d= -f2- || true)"
  echo "  URL:   ${PUB_URL:-http://<hostname>/}"
else
  echo "  URL:   http://<server-ip>:8080"
fi
echo "  user:  ${ADMIN_U}"
echo "  pass:  see NETLYNX_ADMIN_PASSWORD in ${ENV_TARGET}"
echo "  default if env from .env.example untouched: change-me-to-a-long-secret"
echo "  (seeded into DB only on first user create; later change via UI)"
