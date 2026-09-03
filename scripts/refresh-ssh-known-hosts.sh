#!/usr/bin/env bash
# Обновляет ключи в /var/lib/netlynx/ssh_known_hosts для коммутаторов из БД NetLynx.
# Роутеры и прочие категории не трогаем (у них часто другой SSH-порт).
# Один проход по всем свитчам: сначала ssh-keyscan (современные алгоритмы OpenSSH),
# если ключа нет — fetch-ssh-hostkey (modern → transitional → legacy DH-SHA1).
set -euo pipefail

KNOWN_HOSTS="${KNOWN_HOSTS:-/var/lib/netlynx/ssh_known_hosts}"
DEFAULT_SSH_PORT="${DEFAULT_SSH_PORT:-22}"
KEY_TYPES="${KEY_TYPES:-rsa,ecdsa,ed25519}"
SCAN_TIMEOUT="${SCAN_TIMEOUT:-8}"
PARALLEL="${PARALLEL:-16}"
PG_CONTAINER="${PG_CONTAINER:-}"  # auto: netlynx-postgres-1 или invetor-postgres-1
PG_USER="${PG_USER:-invetor}"
PG_DB="${PG_DB:-invetor}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: run as root (sudo)" >&2
  exit 1
fi

if ! command -v ssh-keyscan >/dev/null 2>&1; then
  echo "ERROR: ssh-keyscan not found" >&2
  exit 1
fi

# host<TAB>port — по одной записи на строку.
fetch_targets() {
  if [[ -n "${HOSTS_FILE:-}" && -f "${HOSTS_FILE}" ]]; then
    while IFS= read -r line || [[ -n "${line}" ]]; do
      line="$(echo "${line}" | sed 's/#.*//' | tr -d '\r' | xargs)"
      [[ -z "${line}" ]] && continue
      if [[ "${line}" == *:* && "${line}" != *" "* ]]; then
        host="${line%%:*}"
        port="${line##*:}"
      else
        host="${line%% *}"
        port="${DEFAULT_SSH_PORT}"
      fi
      printf '%s\t%s\n' "${host}" "${port}"
    done < "${HOSTS_FILE}"
    return
  fi
  if [[ -n "${HOSTS:-}" ]]; then
    local port="${SSH_PORT:-${DEFAULT_SSH_PORT}}"
    for h in $(echo "${HOSTS}" | tr ',;' ' '); do
      h="$(echo "${h}" | tr -d '[:space:]')"
      [[ -z "${h}" ]] && continue
      printf '%s\t%s\n' "${h}" "${port}"
    done
    return
  fi
  if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -qx "${PG_CONTAINER}"; then
    # Только онлайн-коммутаторы (как IsOnline в NetLynx: SNMP ok или online_override=true).
    # Оффлайн узлы в known_hosts не трогаем — ssh-keyscan всё равно бесполезен.
    docker exec "${PG_CONTAINER}" psql -U "${PG_USER}" -d "${PG_DB}" -t -A -F $'\t' \
      -c "SELECT DISTINCT host, COALESCE(NULLIF(ssh_port, 0), ${DEFAULT_SSH_PORT})
          FROM devices
          WHERE host IS NOT NULL AND btrim(host) <> ''
            AND lower(btrim(device_category)) IN ('switch', 'коммутатор', 'коммутаторы')
            AND CASE
                  WHEN online_override IS NOT NULL THEN online_override
                  ELSE COALESCE(last_snmp_ok, false)
                END
          ORDER BY 1"
    return
  fi
  echo "ERROR: no targets (docker postgres, HOSTS or HOSTS_FILE)" >&2
  exit 1
}

mapfile -t TARGETS < <(fetch_targets | sed '/^$/d')
if [[ "${#TARGETS[@]}" -eq 0 ]]; then
  echo "ERROR: switch list is empty" >&2
  exit 1
fi

install -d -m 0700 -o netlynx -g netlynx "$(dirname "${KNOWN_HOSTS}")"
touch "${KNOWN_HOSTS}"
chown netlynx:netlynx "${KNOWN_HOSTS}"
chmod 0600 "${KNOWN_HOSTS}"

BACKUP="${KNOWN_HOSTS}.bak.$(date +%Y%m%d%H%M%S)"
cp -a "${KNOWN_HOSTS}" "${BACKUP}"
echo "backup: ${BACKUP} switches=${#TARGETS[@]}"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

remove_host_keys() {
  local file="$1" host="$2" port="$3"
  ssh-keygen -R "${host}" -f "${file}" >/dev/null 2>&1 || true
  if [[ "${port}" != "22" ]]; then
    ssh-keygen -R "[${host}]:${port}" -f "${file}" >/dev/null 2>&1 || true
  fi
}

keys_file_has_entries() {
  local f="$1"
  [[ -s "${f}" ]] && grep -qvE '^\s*#' "${f}"
}

scan_host_keys() {
  local host="$1" port="$2" out="$3"
  rm -f "${out}"
  # 1) OpenSSH defaults (curve25519/ecdh/…)
  if ssh-keyscan -T "${SCAN_TIMEOUT}" -p "${port}" -t "${KEY_TYPES}" "${host}" >"${out}" 2>/dev/null \
    && keys_file_has_entries "${out}"; then
    grep -vE '^\s*#' "${out}" > "${out}.clean"
    mv "${out}.clean" "${out}"
    return 0
  fi
  # 2–4) fetch-ssh-hostkey сам перебирает modern → transitional → legacy
  if command -v fetch-ssh-hostkey >/dev/null 2>&1; then
    if fetch-ssh-hostkey "${host}" "${port}" >"${out}" 2>/dev/null && keys_file_has_entries "${out}"; then
      return 0
    fi
  fi
  rm -f "${out}"
  return 1
}

RUNNING=0
for row in "${TARGETS[@]}"; do
  host="$(echo "${row%%$'\t'*}" | tr -d '[:space:]')"
  port="$(echo "${row##*$'\t'}" | tr -d '[:space:]')"
  [[ -z "${host}" ]] && continue
  [[ -z "${port}" || "${port}" == "${host}" ]] && port="${DEFAULT_SSH_PORT}"
  (
    safe="${host//\//_}_${port}"
    out="${TMP}/${safe}.keys"
    if scan_host_keys "${host}" "${port}" "${out}"; then
      printf 'ok\t%s\t%s\n' "${host}" "${port}"
    else
      rm -f "${out}"
      printf 'skip\t%s\t%s\n' "${host}" "${port}" >&2
    fi
  ) &
  RUNNING=$((RUNNING + 1))
  if [[ "${RUNNING}" -ge "${PARALLEL}" ]]; then
    wait -n 2>/dev/null || wait
    RUNNING=$((RUNNING - 1))
  fi
done
wait

shopt -s nullglob
KEY_FILES=("${TMP}"/*.keys)
if [[ "${#KEY_FILES[@]}" -eq 0 ]]; then
  echo "ERROR: ssh-keyscan got no keys from switches" >&2
  exit 1
fi

WORK="${TMP}/known_hosts"
cp -a "${KNOWN_HOSTS}" "${WORK}"
chown netlynx:netlynx "${WORK}"
chmod 0600 "${WORK}"

UPDATED=0
for f in "${KEY_FILES[@]}"; do
  base="$(basename "${f}" .keys)"
  port="${base##*_}"
  host="${base%_*}"
  host="${host//_//}"
  remove_host_keys "${WORK}" "${host}" "${port}"
  cat "${f}" >> "${WORK}"
  UPDATED=$((UPDATED + 1))
done

awk '!seen[$0]++' "${WORK}" > "${WORK}.uniq"
install -m 0600 -o netlynx -g netlynx "${WORK}.uniq" "${KNOWN_HOSTS}"
SKIP=$((${#TARGETS[@]} - UPDATED))
echo "done: ${KNOWN_HOSTS} updated=${UPDATED} skipped=${SKIP} lines=$(wc -l < "${KNOWN_HOSTS}")"
