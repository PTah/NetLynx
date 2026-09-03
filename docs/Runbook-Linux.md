# Установка NetLynx на Linux (Ubuntu / Debian)

Пошаговая инструкция: от чистой Ubuntu/Debian до работающего мониторинга коммутаторов в браузере.

**NetLynx** — сервер на Go + PostgreSQL + веб-интерфейс. Опрос свитчей идёт по **SNMP**; данные и настройки хранятся в БД.

Два пути установки:

| Путь | Кому подходит |
|------|----------------|
| **[Быстрый](#шаг-4-установка-одной-командой)** — `docs/deploy.sh` | Нужен рабочий сервер за 10–15 минут |
| **[Ручной](#приложение-б-ручная-установка)** — команды по шагам | Хотите понять каждый файл и каталог |

См. также: [SNMP-Community.md](SNMP-Community.md) · [Vendors.md](Vendors.md) · [Roles.md](Roles.md) · [API.md](API.md) · [Windows-Server-Setup.md](Windows-Server-Setup.md)

---

## Что получится в итоге

| Компонент | Где |
|-----------|-----|
| Веб и API | `http://IP_СЕРВЕРА:8080` (или URL через nginx) |
| Служба | `NetLynx.service` |
| Настройки | `/etc/netlynx/netlynx.env` |
| Исходники и Docker Compose | каталог git-клона, например `/opt/NetLynx` |
| PostgreSQL | Docker, порт хоста **127.0.0.1:5433** (не в LAN) |

Имя БД **`invetor`** — историческое, менять не обязательно.

---

## Что понадобится

- Сервер или VM: **Ubuntu 22.04** / **Debian 12** (или близкий дистрибутив).
- Пользователь с **`sudo`** (обычно тот, под кем вы зашли по SSH).
- Доступ в интернет (apt, Docker Hub, npm, Go modules).
- С вашего ПК до сервера открыт порт **8080/tcp** (или 80/443, если позже поставите nginx).
- Свитчи должны отвечать по **SNMP** из сети, где стоит NetLynx.

---

## Шаг 1. Обновить систему и поставить пакеты

Подключитесь по SSH и выполните:

```bash
sudo apt update
sudo apt install -y git curl ca-certificates docker.io docker-compose-v2 \
  nodejs npm golang-go rsync postgresql-client
```

| Пакет | Зачем |
|-------|--------|
| **git** | скачать исходники NetLynx |
| **curl**, **ca-certificates** | проверки и HTTPS |
| **docker.io**, **docker-compose-v2** | PostgreSQL в контейнере (проще, чем ставить Postgres в систему) |
| **nodejs**, **npm** | один раз собрать веб-интерфейс (React) |
| **golang-go** | собрать сервер `netlynxd` |
| **rsync** | скопировать собранный веб в `/var/lib/netlynx/web` |
| **postgresql-client** | утилита `pg_dump` для резервных копий из UI (необязательно, но удобно) |

Проверка, что всё встало:

```bash
git --version
docker --version
docker compose version
go version
node -v
npm -v
```

Если `go version` показывает слишком старую версию (нужен **Go 1.22+**), установите Go с [go.dev/dl](https://go.dev/dl/) или через snap — и снова проверьте `go version`. Дальше — [шаг 2](#шаг-2-docker-без-sudo-важно).

---

## Шаг 2. Docker без `sudo` (важно)

Контейнер с PostgreSQL запускает **ваш** пользователь (не root). Его нужно добавить в группу **`docker`**:

```bash
sudo usermod -aG docker "$USER"
```

Что это значит:

- **`$USER`** — ваш текущий логин (тот же, что в `whoami`).
- **`-aG docker`** — добавить в группу `docker`, **не** удаляя из других групп.
- После этого команды `docker compose …` можно выполнять **без** `sudo`.

**Обязательно:** группа применится только после **нового входа** в SSH-сессию:

```bash
# вариант 1 — выйти из SSH и зайти снова
exit

# вариант 2 — не выходя, один раз в текущей сессии:
newgrp docker
```

Проверка (должно работать **без** sudo):

```bash
docker run --rm hello-world
```

Если видите «permission denied» — вы не перeloginились или `usermod` не выполнялся.

> **Про пользователя `netlynx`:** его создаёт скрипт установки — под ним **работает служба** NetLynx. В группу `docker` его добавлять не нужно: Postgres вы поднимаете **до** запуска службы, от своего пользователя.

---

## Шаг 3. Скачать исходники

Каталог для проекта — например `/opt/NetLynx`:

```bash
sudo mkdir -p /opt
sudo chown "$USER":"$USER" /opt
cd /opt
```

**Публичный репозиторий (GitHub):**

```bash
git clone https://github.com/PTah/NetLynx.git
cd /opt/NetLynx
```

**Приватный git (если у вас свой Gitea/GitLab):** подставьте свой URL.

```bash
# пример
git clone https://git.example.com/your-org/NetLynx.git
cd /opt/NetLynx
git checkout main
```

Дальше все команды — из **корня клона** (`/opt/NetLynx`), если не указано иное.

---

## Шаг 4. Установка одной командой

Скрипт `docs/deploy.sh` собирает программу, ставит службу, поднимает PostgreSQL и проверяет, что всё живо.

```bash
cd /opt/NetLynx
bash docs/deploy.sh
```

Установка занимает несколько минут (npm и Go качают зависимости). В конце должно быть:

```text
Done. Installed:
health: {"status":"ok", ...}
```

### Что делает скрипт в начале (git)

Перед сборкой `deploy.sh` пытается **подтянуть свежий код** из git (`git fetch` + `git pull`). Это нужно при **повторном** запуске (обновление версии). При **первой** установке сразу после `git clone` обычно нечего качать — скрипт просто продолжит.

**Git remote** — это **короткое имя сервера**, откуда вы клонировали репозиторий (не «удалённый рабочий стол» и не SSH-хост NetLynx).

Посмотреть, как у вас названо:

```bash
cd /opt/NetLynx
git remote -v
```

Типичный вывод после clone с GitHub:

```text
origin  https://github.com/PTah/NetLynx.git (fetch)
origin  https://github.com/PTah/NetLynx.git (push)
```

Здесь **`origin`** — стандартное имя «откуда скачали». Скрипт по умолчанию использует именно его (`REMOTE=origin`). **В 99% случаев ничего дополнительно указывать не нужно** — достаточно `bash docs/deploy.sh`.

Переменная `REMOTE=…` нужна **только** если у вас несколько remotes и pull должен идти не с `origin`, а с другого имени (корпоративное зеркало и т.п.):

```bash
# пример: второй remote вы сами добавили как «company»
git remote add company https://git.example.com/net/NetLynx.git
REMOTE=company BRANCH=main bash docs/deploy.sh
```

Если не уверены — **не задавайте** `REMOTE`, используйте обычный `git pull` перед deploy (см. [Обновление версии](#обновление-версии)).

### Как открыть веб: nginx или порт 8080

В **самом конце** `deploy.sh` (после успешного health) скрипт может спросить:

```text
Установить nginx reverse proxy перед NetLynx? [y/N]
```

Если сайт nginx для NetLynx уже есть (`/etc/nginx/sites-enabled/netlynx`), вопрос **не задаётся** — конфиг и `HTTP_ADDR` не трогают.

**Простыми словами:**

- NetLynx сам по себе слушает порт **8080**.
- **nginx** — отдельная программа-«входная дверь» на портах **80** (HTTP) и **443** (HTTPS). Браузер стучится в nginx, nginx передаёт запрос на NetLynx внутри сервера.

| Ваш ответ | Что получите | Когда так делают |
|-----------|--------------|------------------|
| **Enter** или **N** | Открываете **`http://IP_СЕРВЕРА:8080`** напрямую | Первая установка, lab, домашняя сеть, «просто попробовать» |
| **y** | Снаружи — **80/443** через nginx; NetLynx спрятан на `127.0.0.1:8080` | Нужен «нормальный» URL без `:8080`, позже HTTPS и имя вида `netlynx.company.local` |

**Рекомендация новичку:** на первой установке жмите **Enter** (ответ «нет»). HTTPS и красивый hostname потом — раздел [HTTPS через nginx](#https-через-nginx-опционально) и `docs/install-nginx.sh`.

Если скрипт запускается **без интерактива** (автоматизация):

```bash
# явно: только :8080 (как Enter на вопросе про nginx)
INSTALL_NGINX=0 bash docs/deploy.sh

# явно: поставить nginx сразу
INSTALL_NGINX=1 bash docs/deploy.sh
```

---

## Шаг 5. Первый вход в браузере

После `deploy.sh` откройте NetLynx **с рабочего ПК** (не только с самого сервера). Какой URL — зависит от ответа на вопрос про nginx в [шаге 4](#как-открыть-веб-nginx-или-порт-8080).

### Вариант A — напрямую (по умолчанию)

Если на вопрос «Установить nginx?» нажали **Enter** или **N** (или `INSTALL_NGINX=0`):

```text
http://IP_СЕРВЕРА:8080
```

Пример: `http://10.0.0.1:8080`

NetLynx слушает **8080 на всех интерфейсах**. Порт **8080/tcp** должен быть открыт в firewall до сервера.

### Вариант B — через nginx

Если ответили **y** (или `INSTALL_NGINX=1`), снаружи **не** `:8080` — вход через nginx на **80** или **443**:

```text
http://ИМЯ_САЙТА
```
или (если при установке nginx выбрали HTTPS):
```text
https://ИМЯ_САЙТА
```

**Имя сайта** — то, что вводили в `install-nginx.sh` (`server_name`, например `netlynx.company.local`).  
Узнать сохранённый URL на сервере:

```bash
grep '^NETLYNX_PUBLIC_URL=' /etc/netlynx/netlynx.env
```

NetLynx при этом слушает только **`127.0.0.1:8080`** — с других машин `:8080` может **не открыться**, это нормально: снаружи ходят в nginx.

| | Вариант A (без nginx) | Вариант B (nginx) |
|--|------------------------|-------------------|
| Адрес в браузере | `http://IP:8080` | `http(s)://имя-сайта` |
| Порт снаружи | **8080** | **80** / **443** |
| Где крутится UI | сам NetLynx | nginx → NetLynx внутри сервера |

### Логин (одинаково для A и B)

Учётные данные **по умолчанию** (если не меняли env до первого запуска):

| | |
|--|--|
| Логин | `admin` |
| Пароль | `change-me-to-a-long-secret` |

**Сразу смените пароль:** **Настройки → Пользователи**.

Откуда берётся пароль: в `/etc/netlynx/netlynx.env` — `NETLYNX_ADMIN_USER` и `NETLYNX_ADMIN_PASSWORD`. При **первом** старте службы создаётся пользователь в БД; дальше действует пароль из БД.

### Проверка на сервере

```bash
# NetLynx жив (с сервера — всегда localhost:8080)
curl -s http://127.0.0.1:8080/health

sudo systemctl status NetLynx.service --no-pager

# если ставили nginx:
sudo systemctl status nginx --no-pager
curl -sI "http://127.0.0.1/" -H "Host: $(grep '^NETLYNX_PUBLIC_URL=' /etc/netlynx/netlynx.env | cut -d/ -f3)" | head -1
```

Ожидаемо: `health` → `{"status":"ok",...}`; в браузере — форма **«Вход в NetLynx»**.

---

## Шаг 6. Добавить коммутатор (SNMP)

1. На свитче включите **SNMP v2c** (community **read-only** для мониторинга).
2. В NetLynx: **Узлы → Добавить** — IP свитча, community, интервал опроса.
3. Кнопка **Test SNMP** — должны появиться sysName / sysDescr.

Подробно: [SNMP-Community.md](SNMP-Community.md). Поддерживаемые вендоры: [Vendors.md](Vendors.md).

---

## Обновление версии

Из каталога клона — **сначала новый код, потом deploy**:

```bash
cd /opt/NetLynx
git pull          # скачать изменения с того же сервера, откуда делали clone (origin)
bash docs/deploy.sh
```

`git pull` = «обновить файлы в `/opt/NetLynx`». Отдельно `REMOTE=…` указывать не нужно, если вы не настраивали несколько remotes.

Файл `/etc/netlynx/netlynx.env` при deploy **не перезаписывается** (пароли и DATABASE_URL сохраняются).

---

## Если что-то пошло не так

### Служба не стартует

```bash
sudo journalctl -u NetLynx.service -n 80 --no-pager
sudo grep -E '^DATABASE_URL|^HTTP_ADDR' /etc/netlynx/netlynx.env
```

### PostgreSQL не отвечает

```bash
cd /opt/NetLynx
docker compose ps
docker compose up -d postgres
docker compose logs --tail=40 postgres
ss -tlnp | grep 5433
```

Частая причина: не выполнен [шаг 2](#шаг-2-docker-без-sudo) — `docker compose` падал с «permission denied».

### Страница в браузере не открывается

**Вариант A (без nginx):**

- Firewall: `sudo ufw allow 8080/tcp` (если ufw).
- Порт: `ss -tlnp | grep 8080` — должно слушать `*:8080` или `:8080`.
- Health: `curl http://127.0.0.1:8080/health`.

**Вариант B (через nginx):**

- Не открывайте `:8080` с другого ПК — смотрите `NETLYNX_PUBLIC_URL` в `/etc/netlynx/netlynx.env`.
- DNS или `/etc/hosts` на клиенте: имя сайта должно указывать на IP сервера.
- Firewall: **80/tcp** и/или **443/tcp**, не обязательно 8080 снаружи.
- `sudo nginx -t && sudo systemctl status nginx`
- Health NetLynx локально: `curl http://127.0.0.1:8080/health` (должен быть ok, даже если снаружи только nginx).

### «Сборка web» / npm ошибки

На старых Debian иногда нужен более новый Node (LTS 20+). Поставьте с [nodejs.org](https://nodejs.org/) или через `nvm`, затем снова `bash docs/deploy.sh`.

---

## HTTPS через nginx (опционально)

Если при install вы нажали **Enter** на вопросе про nginx (доступ через `:8080`), а позже нужны **80/443** и сертификат:

```bash
sudo bash /opt/NetLynx/docs/install-nginx.sh
sudo systemctl restart NetLynx.service
```

Скрипт спросит **имя сайта** (DNS, например `netlynx.example.com`) и режим: HTTP, HTTPS со своими `.pem` или self-signed для lab. Пример конфига: `deploy/nginx/netlynx.example.conf`.

---

## Приложение A. Куда что ложится после установки

| Путь | Назначение |
|------|------------|
| `/opt/NetLynx` | git-клон, `docker-compose.yml`, `docs/deploy.sh` |
| `/usr/local/bin/netlynxd` | бинарник службы |
| `/etc/netlynx/netlynx.env` | пароли, DATABASE_URL, порты |
| `/var/lib/netlynx/web` | собранный веб-интерфейс |
| `/etc/systemd/system/NetLynx.service` | unit systemd |
| Docker volume `invetor_pg` | данные PostgreSQL |

Системный пользователь **`netlynx`** — только для работы службы (не для вашего SSH-логина).

---

## Приложение B. Ручная установка

Те же шаги, что внутри `deploy.sh`, но вручную. Нужно, если хотите нестандартные пути или учитесь «как устроено внутри».

### B1. PostgreSQL

```bash
cd /opt/NetLynx
docker compose up -d postgres

for i in $(seq 1 30); do
  docker compose exec -T postgres pg_isready -U invetor -d invetor && break
  sleep 2
done
```

Строка подключения:

```text
postgres://invetor:invetor@localhost:5433/invetor?sslmode=disable
```

### B2. Сборка веба

```bash
cd /opt/NetLynx/web
npm install
npm run build
cd /opt/NetLynx
```

### B3. Сборка сервера

```bash
cd /opt/NetLynx
VER="$(tr -d '\r\n' < VERSION | head -1)"
COMMIT="$(git rev-parse --short HEAD)"
BUILT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
mkdir -p /tmp/netlynx-build

go build \
  -ldflags "-X main.version=${VER} -X main.commit=${COMMIT} -X main.builtAt=${BUILT}" \
  -o /tmp/netlynx-build/netlynxd ./cmd/netlynxd
go build -o /tmp/netlynx-build/fetch-ssh-hostkey ./cmd/fetch-ssh-hostkey
```

### B4. Пользователь службы и каталоги

```bash
sudo useradd --system --home /var/lib/netlynx --shell /usr/sbin/nologin netlynx 2>/dev/null || true
# журнал в UI читает journalctl — службе нужна группа systemd-journal:
sudo usermod -aG systemd-journal netlynx 2>/dev/null || true

sudo install -d -m 0755 /opt/netlynx /etc/netlynx /var/lib/netlynx /var/lib/netlynx/web
sudo install -d -m 0750 -o netlynx -g netlynx /var/backups/netlynx
sudo chown -R netlynx:netlynx /var/lib/netlynx
```

### B5. Конфиг

```bash
sudo cp /opt/NetLynx/.env.example /etc/netlynx/netlynx.env
sudo chmod 0640 /etc/netlynx/netlynx.env
sudo chown root:netlynx /etc/netlynx/netlynx.env
sudo nano /etc/netlynx/netlynx.env
```

Минимум:

```text
DATABASE_URL=postgres://invetor:invetor@localhost:5433/invetor?sslmode=disable
HTTP_ADDR=:8080
WEB_DIST=/var/lib/netlynx/web
NETLYNX_ADMIN_USER=admin
NETLYNX_ADMIN_PASSWORD=ВАШ_ДЛИННЫЙ_СЕКРЕТ
NETLYNX_COOKIE_SECURE=false
```

### B6. Установка файлов

```bash
sudo install -m 0755 /tmp/netlynx-build/netlynxd /usr/local/bin/netlynxd
sudo install -m 0755 /tmp/netlynx-build/fetch-ssh-hostkey /usr/local/bin/fetch-ssh-hostkey
sudo rsync -a --delete /opt/NetLynx/web/dist/ /var/lib/netlynx/web/
sudo chown -R netlynx:netlynx /var/lib/netlynx/web
sudo install -m 0644 /opt/NetLynx/deploy/NetLynx.service /etc/systemd/system/NetLynx.service
```

### B7. Запуск

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now NetLynx.service
curl -s http://127.0.0.1:8080/health
```

---

## Приложение C. Откат на старую версию

```bash
cd /opt/NetLynx
git log --oneline -n 10
git checkout <старый_commit>
bash docs/deploy.sh
# вернуться на main:
git checkout main && git pull
```

---

## Краткий чеклист (копируйте блоками)

Блок **целиком** — от пакетов до работающего NetLynx. Шаг 4 (`docs/deploy.sh`) делает **полную** установку: сборка web/Go, Postgres, systemd, проверка health; в конце — вопрос про nginx.

```bash
# 1. Пакеты
sudo apt update
sudo apt install -y git curl ca-certificates docker.io docker-compose-v2 \
  nodejs npm golang-go rsync postgresql-client

# 2. Docker без sudo — затем ВЫЙТИ из SSH и зайти снова (или: newgrp docker)
sudo usermod -aG docker "$USER"

# 3. Исходники
sudo mkdir -p /opt && sudo chown "$USER":"$USER" /opt
cd /opt && git clone https://github.com/PTah/NetLynx.git && cd NetLynx

# 4. Установка
bash docs/deploy.sh
#    В конце: Enter/N → вариант A; y → вариант B (nginx)

# 5. Браузер (выберите свой вариант после шага 4):
#    A — без nginx:  http://IP_СЕРВЕРА:8080
#    B — nginx:      URL из  grep NETLYNX_PUBLIC_URL /etc/netlynx/netlynx.env
#    Логин admin / change-me-to-a-long-secret → сразу сменить пароль
```
