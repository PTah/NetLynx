# Installing NetLynx on Linux (Ubuntu / Debian)

Step-by-step: from a clean Ubuntu/Debian to switch monitoring working in a browser.

**NetLynx** is a Go server + PostgreSQL + web interface. Switches are polled over **SNMP**; data and settings live in the database.

Two installation paths:

| Path | Who it is for |
|------|---------------|
| **[Fast](#step-4-one-command-install)** — `docs/deploy.sh` | You need a working server in 10–15 minutes |
| **[Manual](#appendix-b-manual-installation)** — step-by-step commands | You want to understand every file and directory |

See also: [SNMP-Community.md](SNMP-Community.md) · [Vendors.md](../Vendors.md) · [Roles.md](Roles.md) · [API.md](API.md) · [Windows-Server-Setup.md](../Windows-Server-Setup.md)

**Languages:** [Русский](../Runbook-Linux.md) · English

---

## What you end up with

| Component | Where |
|-----------|-------|
| Web UI and API | `http://SERVER_IP:8080` (or a URL through nginx) |
| Service | `NetLynx.service` |
| Settings | `/etc/netlynx/netlynx.env` |
| Sources and Docker Compose | the git clone directory, e.g. `/opt/NetLynx` |
| PostgreSQL | Docker, host port **127.0.0.1:5433** (not exposed to the LAN) |

The database is named **`invetor`** for historical reasons; there is no need to change it.

---

## Requirements

- A server or VM: **Ubuntu 22.04** / **Debian 12** (or a close relative).
- A user with **`sudo`** (usually the account you logged in with over SSH).
- Internet access (apt, Docker Hub, npm, Go modules).
- Port **8080/tcp** reachable from your workstation to the server (or 80/443 if you add nginx later).
- Switches must answer over **SNMP** from the network where NetLynx runs.
- For **discovery** and LLDP-based topology the switches must have **LLDP** enabled (and CDP on Cisco) — NetLynx does not enable it for you: [Autodiscover.md](../Autodiscover.md).

---

## Step 1. Update the system and install packages

Log in over SSH and run:

```bash
sudo apt update
sudo apt install -y git curl ca-certificates docker.io docker-compose-v2 \
  nodejs npm golang-go rsync postgresql-client
```

| Package | Why |
|---------|-----|
| **git** | download the NetLynx sources |
| **curl**, **ca-certificates** | health checks and HTTPS |
| **docker.io**, **docker-compose-v2** | PostgreSQL in a container (simpler than a system-wide install) |
| **nodejs**, **npm** | build the web UI (React) once |
| **golang-go** | build the `netlynxd` server |
| **rsync** | copy the built web assets to `/var/lib/netlynx/web` |
| **postgresql-client** | `pg_dump` for backups taken from the UI (optional, but handy) |

Verify everything is in place:

```bash
git --version
docker --version
docker compose version
go version
node -v
npm -v
```

If `go version` reports something older than **Go 1.22+**, install Go from [go.dev/dl](https://go.dev/dl/) or via snap and check `go version` again. Then continue with [step 2](#step-2-docker-without-sudo-important).

---

## Step 2. Docker without `sudo` (important)

The PostgreSQL container is started by **your** user (not root), so the account must be in the **`docker`** group:

```bash
sudo usermod -aG docker "$USER"
```

What this means:

- **`$USER`** — your current login (the same as `whoami`).
- **`-aG docker`** — append to the `docker` group **without** dropping the other groups.
- After that, `docker compose …` works **without** `sudo`.

**Required:** the group only takes effect after a **new login**:

```bash
# option 1 — log out of SSH and log back in
exit

# option 2 — without logging out, once in the current session:
newgrp docker
```

Check it (must work **without** sudo):

```bash
docker run --rm hello-world
```

If you see "permission denied", you have not re-logged in or `usermod` was never run.

> **About the `netlynx` user:** the installer creates it, and the NetLynx **service runs as it**. It does not need to be in the `docker` group: PostgreSQL is started **before** the service, by your own user.

---

## Step 3. Get the sources

Pick a directory, e.g. `/opt/NetLynx`:

```bash
sudo mkdir -p /opt
sudo chown "$USER":"$USER" /opt
cd /opt
```

**Public repository (GitHub):**

```bash
git clone https://github.com/PTah/NetLynx.git
cd /opt/NetLynx
```

**Private git (your own Gitea/GitLab):** substitute your URL.

```bash
# example
git clone https://git.example.com/your-org/NetLynx.git
cd /opt/NetLynx
git checkout main
```

All further commands assume the **clone root** (`/opt/NetLynx`) unless stated otherwise.

---

## Step 4. One-command install

`docs/deploy.sh` builds the program, installs the service, starts PostgreSQL and verifies that everything is alive.

```bash
cd /opt/NetLynx
bash docs/deploy.sh
```

It takes a few minutes (npm and Go download dependencies). At the end you should see:

```text
Done. Installed:
health: {"status":"ok", ...}
```

### What the script does first (git)

Before building, `deploy.sh` tries to **pull the latest code** (`git fetch` + `git pull`). This matters on **re-runs** (upgrades). On a **first** install right after `git clone` there is usually nothing to pull and the script simply continues.

A **git remote** is the **short name of the server** you cloned from (not a "remote desktop" and not the NetLynx SSH host).

To see how yours is named:

```bash
cd /opt/NetLynx
git remote -v
```

A typical output after cloning from GitHub:

```text
origin  https://github.com/PTah/NetLynx.git (fetch)
origin  https://github.com/PTah/NetLynx.git (push)
```

Here **`origin`** is the standard "where it was downloaded from" name. The script uses exactly it by default (`REMOTE=origin`). **In 99% of cases you do not need to pass anything** — `bash docs/deploy.sh` is enough.

`REMOTE=…` is needed **only** if you have several remotes and the pull must come from another name (a corporate mirror, etc.):

```bash
# example: you added a second remote called "company"
git remote add company https://git.example.com/net/NetLynx.git
REMOTE=company BRANCH=main bash docs/deploy.sh
```

If unsure — **do not** set `REMOTE`, just run `git pull` before deploy (see [Upgrade](#upgrade)).

### How to expose the web UI: nginx or port 8080

At the **very end** of `deploy.sh` (after a successful health check) the script may ask:

```text
Install an nginx reverse proxy in front of NetLynx? [y/N]
```

If an nginx site for NetLynx already exists (`/etc/nginx/sites-enabled/netlynx`), the question is **not asked** — neither the config nor `HTTP_ADDR` is touched.

**In plain words:**

- NetLynx itself listens on port **8080**.
- **nginx** is a separate "front door" on ports **80** (HTTP) and **443** (HTTPS). The browser talks to nginx, nginx forwards to NetLynx inside the server.

| Your answer | What you get | When people choose it |
|-------------|--------------|-----------------------|
| **Enter** or **N** | Open **`http://SERVER_IP:8080`** directly | First install, lab, home network, "just try it" |
| **y** | Externally **80/443** via nginx; NetLynx hidden on `127.0.0.1:8080` | You want a "normal" URL without `:8080`, HTTPS later, and a name like `netlynx.company.local` |

**Advice for a first install:** press **Enter** (answer "no"). HTTPS and a pretty hostname can be added later — see [HTTPS via nginx](#https-via-nginx-optional) and `docs/install-nginx.sh`.

Non-interactive runs (automation):

```bash
# explicitly: only :8080 (same as pressing Enter on the nginx question)
INSTALL_NGINX=0 bash docs/deploy.sh

# explicitly: install nginx right away
INSTALL_NGINX=1 bash docs/deploy.sh
```

---

## Step 5. First login in the browser

After `deploy.sh`, open NetLynx **from your workstation** (not through the server's terminal):

| What you chose in step 4 | URL |
|---|---|
| **A** (no nginx) | `http://SERVER_IP:8080` |
| **B** (nginx) | the URL from `/etc/netlynx/netlynx.env` (`NETLYNX_PUBLIC_URL`), e.g. `http://netlynx.example.com/` |

**Default credentials** (unless you changed the env before the first start):

| | |
|--|--|
| Login | `admin` |
| Password | `change-me-to-a-long-secret` |

**Change the password immediately:** **Settings → Users**.

Where the password comes from: `NETLYNX_ADMIN_USER` and `NETLYNX_ADMIN_PASSWORD` in `/etc/netlynx/netlynx.env`. On the **first** service start a user is created in the database; afterwards the password from the database applies.

**Device secrets (SNMP/SSH/SMTP/tokens) in PostgreSQL:** plain text by default. Enable AES-GCM: [Secrets.md](../Secrets.md) (`NETLYNX_SECRETS_KEY`, then `netlynxd secrets-rewrap`).

### Checks on the server

```bash
# NetLynx is alive (on the server — always localhost:8080)
curl -s http://127.0.0.1:8080/health

sudo systemctl status NetLynx.service --no-pager

# if you installed nginx:
sudo systemctl status nginx --no-pager
curl -sI "http://127.0.0.1/" -H "Host: $(grep '^NETLYNX_PUBLIC_URL=' /etc/netlynx/netlynx.env | cut -d/ -f3)" | head -1
```

Expected: `health` → `{"status":"ok",...}`; in the browser — the **"Sign in to NetLynx"** form.

---

## Step 6. Add a switch (SNMP)

1. On the switch enable **SNMP v2c** (a **read-only** community is enough for monitoring).
2. For the **"Discovered"** page and the LLDP topology map, enable **LLDP** on switch ports; on PCs/servers an LLDP agent is desirable (Windows / `lldpd`). NetLynx does not install agents for you: [Autodiscover.md](../Autodiscover.md).
3. In NetLynx: **Devices → Add** — switch IP, community, poll interval.
4. Press **Test SNMP** — sysName / sysDescr must appear.

Details: [SNMP-Community.md](SNMP-Community.md). Supported vendors: [Vendors.md](../Vendors.md).

---

## Upgrade

From the clone directory — **first the new code, then deploy**:

```bash
cd /opt/NetLynx
git pull          # fetch changes from the same server you cloned from (origin)
bash docs/deploy.sh
```

`git pull` = "update the files in `/opt/NetLynx`". You do not need `REMOTE=…` unless you configured several remotes.

`/etc/netlynx/netlynx.env` is **not** overwritten by deploy (passwords and DATABASE_URL survive).

---

## If something goes wrong

### The service does not start

```bash
sudo journalctl -u NetLynx.service -n 80 --no-pager
sudo grep -E '^DATABASE_URL|^HTTP_ADDR' /etc/netlynx/netlynx.env
```

### PostgreSQL does not answer

```bash
cd /opt/NetLynx
docker compose ps
docker compose up -d postgres
docker compose logs --tail=40 postgres
ss -tlnp | grep 5433
```

A common cause: [step 2](#step-2-docker-without-sudo-important) was skipped — `docker compose` failed with "permission denied".

### The page does not open in the browser

**Option A (no nginx):**

- Firewall: `sudo ufw allow 8080/tcp` (if ufw is used).
- Port: `ss -tlnp | grep 8080` — it must listen on `*:8080` or `:8080`.
- Health: `curl http://127.0.0.1:8080/health`.

**Option B (through nginx):**

- Do not open `:8080` from another machine — look at `NETLYNX_PUBLIC_URL` in `/etc/netlynx/netlynx.env`.
- DNS or `/etc/hosts` on the client: the site name must resolve to the server IP.
- Firewall: **80/tcp** and/or **443/tcp**, 8080 is not required externally.
- `sudo nginx -t && sudo systemctl status nginx`
- NetLynx health locally: `curl http://127.0.0.1:8080/health` (must be ok even when only nginx is exposed).

### "Web build" / npm errors

On older Debian a newer Node may be required (LTS 20+). Install it from [nodejs.org](https://nodejs.org/) or via `nvm`, then run `bash docs/deploy.sh` again.

---

## HTTPS via nginx (optional)

If you pressed **Enter** on the nginx question during install (access via `:8080`) and later want **80/443** with a certificate:

```bash
sudo bash /opt/NetLynx/docs/install-nginx.sh
sudo systemctl restart NetLynx.service
```

The script asks for the **site name** (DNS, e.g. `netlynx.example.com`) and the mode: HTTP, HTTPS with your own `.pem`, or self-signed for a lab. Example config: `deploy/nginx/netlynx.example.conf`.

---

## Appendix A. Where things land after installation

| Path | Purpose |
|------|---------|
| `/opt/NetLynx` | git clone, `docker-compose.yml`, `docs/deploy.sh` |
| `/usr/local/bin/netlynxd` | service binary |
| `/etc/netlynx/netlynx.env` | passwords, DATABASE_URL, ports |
| `/var/lib/netlynx/web` | built web UI |
| `/etc/systemd/system/NetLynx.service` | systemd unit |
| Docker volume `invetor_pg` | PostgreSQL data |

The system user **`netlynx`** exists only to run the service (not for your SSH login).

---

## Appendix B. Manual installation

The same steps `deploy.sh` performs, but by hand. Useful for non-standard paths or to learn "how it works inside".

### B1. PostgreSQL

```bash
cd /opt/NetLynx
docker compose up -d postgres

for i in $(seq 1 30); do
  docker compose exec -T postgres pg_isready -U invetor -d invetor && break
  sleep 2
done
```

Connection string:

```text
postgres://invetor:invetor@localhost:5433/invetor?sslmode=disable
```

### B2. Build the web UI

```bash
cd /opt/NetLynx/web
npm install
npm run build
cd /opt/NetLynx
```

### B3. Build the server

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

### B4. Service user and directories

```bash
sudo useradd --system --home /var/lib/netlynx --shell /usr/sbin/nologin netlynx 2>/dev/null || true
# the UI log reader uses journalctl — the service needs the systemd-journal group:
sudo usermod -aG systemd-journal netlynx 2>/dev/null || true

sudo install -d -m 0755 /opt/netlynx /etc/netlynx /var/lib/netlynx /var/lib/netlynx/web
sudo install -d -m 0750 -o netlynx -g netlynx /var/backups/netlynx
sudo chown -R netlynx:netlynx /var/lib/netlynx
```

### B5. Configuration

```bash
sudo cp /opt/NetLynx/.env.example /etc/netlynx/netlynx.env
sudo chmod 0640 /etc/netlynx/netlynx.env
sudo chown root:netlynx /etc/netlynx/netlynx.env
sudo nano /etc/netlynx/netlynx.env
```

Minimum:

```text
DATABASE_URL=postgres://invetor:invetor@localhost:5433/invetor?sslmode=disable
HTTP_ADDR=:8080
WEB_DIST=/var/lib/netlynx/web
NETLYNX_ADMIN_USER=admin
NETLYNX_ADMIN_PASSWORD=<a long secret>
NETLYNX_COOKIE_SECURE=false
```

### B6. Install the files

```bash
sudo install -m 0755 /tmp/netlynx-build/netlynxd /usr/local/bin/netlynxd
sudo install -m 0755 /tmp/netlynx-build/fetch-ssh-hostkey /usr/local/bin/fetch-ssh-hostkey
sudo rsync -a --delete /opt/NetLynx/web/dist/ /var/lib/netlynx/web/
sudo chown -R netlynx:netlynx /var/lib/netlynx/web
sudo install -m 0644 /opt/NetLynx/deploy/NetLynx.service /etc/systemd/system/NetLynx.service
```

### B7. Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now NetLynx.service
curl -s http://127.0.0.1:8080/health
```

---

## Appendix C. Rolling back to a previous version

```bash
cd /opt/NetLynx
git log --oneline -n 10
git checkout <old_commit>
bash docs/deploy.sh
# back to main:
git checkout main && git pull
```

---

## Short checklist (copy the block)

The block below goes from packages to a working NetLynx in one go. Step 4 (`docs/deploy.sh`) performs the **full** install: web/Go build, PostgreSQL, systemd, health check; at the end it asks about nginx.

```bash
# 1. Packages
sudo apt update
sudo apt install -y git curl ca-certificates docker.io docker-compose-v2 \
  nodejs npm golang-go rsync postgresql-client

# 2. Docker without sudo — then LOG OUT of SSH and back in (or: newgrp docker)
sudo usermod -aG docker "$USER"

# 3. Sources
sudo mkdir -p /opt && sudo chown "$USER":"$USER" /opt
cd /opt && git clone https://github.com/PTah/NetLynx.git && cd NetLynx

# 4. Install
bash docs/deploy.sh
#    At the end: Enter/N → option A; y → option B (nginx)

# 5. Browser (pick your variant after step 4):
#    A — no nginx:  http://SERVER_IP:8080
#    B — nginx:     the URL from  grep NETLYNX_PUBLIC_URL /etc/netlynx/netlynx.env
#    Login admin / change-me-to-a-long-secret → change the password immediately
```
