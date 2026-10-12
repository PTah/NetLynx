# Миграция прода Invetor → NetLynx

Пошагово: переименование службы и путей **без** переименования PostgreSQL (`invetor` / `invetor_pg`).

## Что меняется / что остаётся

| Было (Invetor) | Стало (NetLynx) |
|----------------|-----------------|
| git `jdoe/Invetor` | `jdoe/NetLynx` |
| `/opt/Invetor` (клон) | `/opt/NetLynx` |
| `Invetor.service` | `NetLynx.service` |
| `/usr/local/bin/invetord` | `/usr/local/bin/netlynxd` |
| `/etc/invetor/invetor.env` | `/etc/netlynx/netlynx.env` |
| `/var/lib/invetor` | `/var/lib/netlynx` |
| пользователь ОС `invetor` | `netlynx` |
| `INETOR_*` в env | `NETLYNX_*` (старые ключи читаются как fallback) |
| HTTP `:8080` снаружи | nginx `https://netlynx.example.com` → `127.0.0.1:8080` |
| PostgreSQL БД/роль `invetor` | **без изменений** |
| Docker volume `invetor_pg` | **без изменений** |

## Подготовка

1. **DNS:** `netlynx.example.com` → IP сервера (например `10.0.0.1`).
2. **Бэкап:**
   ```bash
   sudo systemctl stop Invetor.service
   cd /opt/Invetor
   docker compose exec -T postgres pg_dump -U invetor invetor | gzip > /tmp/invetor-pre-migrate.sql.gz
   sudo tar czf /tmp/invetor-etc-var.tgz /etc/invetor /var/lib/invetor /var/backups/invetor 2>/dev/null || true
   ```
3. **Клон нового репо** (рядом со старым):
   ```bash
   sudo mv /opt/Invetor /opt/Invetor.bak.$(date +%Y%m%d) 2>/dev/null || true
   sudo git clone https://github.com/PTah/NetLynx.git /opt/NetLynx
   sudo chown -R "$USER":"$USER" /opt/NetLynx
   ```

## Миграция env и данных

```bash
# Пользователь службы
sudo useradd --system --home /var/lib/netlynx --shell /usr/sbin/nologin netlynx 2>/dev/null || true
sudo usermod -aG systemd-journal netlynx 2>/dev/null || true

# Каталоги
sudo install -d -m 0755 /opt/netlynx /etc/netlynx /var/lib/netlynx/web /var/backups/netlynx

# Env: копируем и переименовываем ключи
if [[ -f /etc/invetor/invetor.env ]]; then
  sudo cp /etc/invetor/invetor.env /etc/netlynx/netlynx.env
  sudo sed -i \
    -e 's|^INETOR_|NETLYNX_|g' \
    -e 's|^WEB_DIST=.*|WEB_DIST=/var/lib/netlynx/web|' \
    -e 's|^HTTP_ADDR=.*|HTTP_ADDR=127.0.0.1:8080|' \
    /etc/netlynx/netlynx.env
  grep -q '^NETLYNX_COOKIE_SECURE=' /etc/netlynx/netlynx.env || echo 'NETLYNX_COOKIE_SECURE=true' | sudo tee -a /etc/netlynx/netlynx.env
  grep -q '^NETLYNX_TRUST_PROXY=' /etc/netlynx/netlynx.env || echo 'NETLYNX_TRUST_PROXY=true' | sudo tee -a /etc/netlynx/netlynx.env
  grep -q '^NETLYNX_PUBLIC_URL=' /etc/netlynx/netlynx.env || echo 'NETLYNX_PUBLIC_URL=https://netlynx.example.com' | sudo tee -a /etc/netlynx/netlynx.env
fi
sudo chmod 0640 /etc/netlynx/netlynx.env
sudo chown root:netlynx /etc/netlynx/netlynx.env

# Веб и ssh_known_hosts
sudo rsync -a /var/lib/invetor/ /var/lib/netlynx/
sudo chown -R netlynx:netlynx /var/lib/netlynx /var/backups/netlynx

# Метаданные релиза
sudo rsync -a /opt/invetor/ /opt/netlynx/ 2>/dev/null || true
```

## PostgreSQL

Compose из **нового** клона, но volume старый:

```bash
cd /opt/NetLynx
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U invetor -d invetor
```

`DATABASE_URL` в `netlynx.env` должен остаться:
`postgres://invetor:invetor@localhost:5433/invetor?sslmode=disable`

## Deploy и nginx

```bash
cd /opt/NetLynx
REMOTE=origin BRANCH=main bash docs/deploy.sh
sudo bash docs/install-nginx-tls.sh
```

Скрипт берёт wildcard с шары **`//10.0.0.1/Soft/Certs/2026/Export/apache`** (leaf + intermediate + key). Запасной путь: `Export/pfx-pem` (`public-cert.pem` + `privatekey.pem`).

Если SMB с Linux недоступен — скопируйте три файла apache-экспорта вручную и соберите chain:

```bash
sudo install -d -m 0755 /etc/ssl/example
sudo bash -c 'cat wildcard_kalinamall_ru.crt intermediate_pem_globalsign_ssl_ov_wildcard_1.crt > /etc/ssl/example/fullchain.pem'
sudo install -m 0640 wildcard_kalinamall_ru.key /etc/ssl/example/privkey.pem
sudo bash docs/install-nginx-tls.sh
```

Или укажите готовые файлы:

```bash
CERT_FULLCHAIN=/path/to/fullchain.pem CERT_PRIVKEY=/path/to/privkey.pem sudo bash docs/install-nginx-tls.sh
```

## Отключение старого

```bash
sudo systemctl disable --now Invetor.service
sudo rm -f /usr/local/bin/invetord
# unit можно оставить или удалить:
# sudo rm /etc/systemd/system/Invetor.service && sudo systemctl daemon-reload
```

## Проверка

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS https://netlynx.example.com/health
systemctl status NetLynx.service nginx
```

В UI: узлы, пользователи и история должны быть на месте (та же БД).

## Откат

```bash
sudo systemctl stop NetLynx.service nginx
sudo systemctl start Invetor.service
# env: /etc/invetor/invetor.env, бинарник invetord, клон /opt/Invetor.bak.*
```
