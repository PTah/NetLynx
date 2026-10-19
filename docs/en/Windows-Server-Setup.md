# NetLynx on Windows Server

How to run the NetLynx **server** (`netlynxd.exe`) on Windows Server or Windows 10/11 Pro. The web UI is in the browser (`http://<server>:8080`), exactly as on Linux.

> The **desktop client** (`desktop/`) is a separate story and is **not ready**; see [desktop/README.md](../../desktop/README.md).

See also: [Runbook-Linux.md](Runbook-Linux.md) (the main path) · [SNMP-Community.md](SNMP-Community.md) · [Vendors.md](../Vendors.md) · [API.md](API.md)

**Languages:** [Русский](../Windows-Server-Setup.md) · English

---

## What NSSM is and why it is here

**NSSM** — **N**on-**S**ucking **S**ervice **M**anager, a free utility: [nssm.cc](https://nssm.cc/download).

| Question | Answer |
|----------|--------|
| **What does it do?** | Turns an ordinary program (`.exe`) into a **Windows service** — start at boot, restart on crash, logs, background operation without a console window. |
| **Why not "just an exe"?** | `netlynxd.exe` is a plain Go binary. On its own it does **not** register in "Windows Services" (unlike `.NET` with a built-in Service). NSSM is the simple way to wrap an exe into a service **without** rewriting code. |
| **Where is it?** | After installation — the **`nssm.exe`** file (often `C:\Tools\nssm\nssm.exe`, or next to the project). In **Start → Services** (`services.msc`) a service appears under the name you choose (below — `netlynxd`). |
| **Alternative** | Running manually in a console (`.\bin\netlynxd.exe`) — for tests only. For production on Windows use a service via NSSM or your own wrapper; this guide uses **NSSM**. |

### Installing NSSM (once)

1. Download the archive from [nssm.cc/download](https://nssm.cc/download) (match your architecture: **win64** for 64-bit Windows).
2. Unpack it, e.g. into `C:\Tools\nssm\`.
3. Add the directory with `nssm.exe` to **PATH**, or call it by full path.

Check in PowerShell **as Administrator**:

```powershell
nssm version
# or: C:\Tools\nssm\win64\nssm.exe version
```

It must print the NSSM version (e.g. `NSSM 2.24`).

---

## 1) What else to install

| Component | Why |
|-----------|-----|
| **Git for Windows** | download the sources |
| **Go 1.22+** | build `netlynxd.exe` |
| **Node.js LTS** (npm) | build the web UI |
| **Docker Desktop** or **PostgreSQL** | the database (as on Linux: role/database `invetor`, port **5433** with docker-compose) |
| **NSSM** | the Windows service (see above) |

---

## 2) Where the project lives

```powershell
cd C:\
git clone https://github.com/PTah/NetLynx.git NetLynx
cd C:\NetLynx
copy .env.example .env
```

Check `DATABASE_URL` in `.env` (usually `127.0.0.1:5433` when Postgres comes from `docker-compose.yml`).

Start Postgres (if you use Docker):

```powershell
cd C:\NetLynx
docker compose up -d postgres
```

---

## 3) First build

```powershell
cd C:\NetLynx\web
npm install
npm run build
cd C:\NetLynx
go mod tidy
New-Item -ItemType Directory -Force -Path .\bin | Out-Null
go build -o .\bin\netlynxd.exe .\cmd\netlynxd
```

A test run **without** the service (console window, just to check):

```powershell
$env:DATABASE_URL = "postgres://invetor:invetor@127.0.0.1:5433/invetor?sslmode=disable"
$env:HTTP_ADDR = ":8080"
$env:WEB_DIST = "web\dist"
.\bin\netlynxd.exe
# in another window: curl http://127.0.0.1:8080/health
```

Stop it with Ctrl+C, then set up the permanent service via NSSM.

---

## 4) Installing the Windows service (NSSM)

PowerShell **as Administrator**. Create a directory for logs:

```powershell
New-Item -ItemType Directory -Force -Path C:\NetLynx\logs | Out-Null
```

Register the **`netlynxd`** service:

```powershell
nssm install netlynxd C:\NetLynx\bin\netlynxd.exe
nssm set netlynxd AppDirectory C:\NetLynx
nssm set netlynxd AppEnvironmentExtra DATABASE_URL=postgres://invetor:invetor@127.0.0.1:5433/invetor?sslmode=disable
nssm set netlynxd AppEnvironmentExtra HTTP_ADDR=:8080
nssm set netlynxd AppEnvironmentExtra WEB_DIST=C:\NetLynx\web\dist
nssm set netlynxd AppStdout C:\NetLynx\logs\netlynxd.out.log
nssm set netlynxd AppStderr C:\NetLynx\logs\netlynxd.err.log
nssm start netlynxd
```

Check in the GUI: `Win+R` → `services.msc` → service **netlynxd** → state "Running".

If there are many environment variables, use a `.cmd`/`.ps1` wrapper that reads `.env` and set it as the NSSM `Application` instead of calling the exe directly.

---

## 5) Upgrade

```powershell
cd C:\NetLynx
git pull
cd web
npm install
npm run build
cd ..
go mod tidy
go build -o .\bin\netlynxd.exe .\cmd\netlynxd
nssm restart netlynxd
```

---

## 6) Check

```powershell
curl http://127.0.0.1:8080/health
```

Expected: `"status":"ok"`. In the browser: `http://SERVER_IP:8080` (default login — as in [Runbook-Linux.md](Runbook-Linux.md#step-5-first-login-in-the-browser)).

---

## 7) Firewall

Open port **8080** for the trusted network only:

```powershell
New-NetFirewallRule -DisplayName "NetLynx 8080" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080
```

---

## 8) Operations

- Run the service under a **separate** account (not Administrator) if company policy requires it.
- Secrets (SMTP, passwords) — not in a plain `.env` on a shared host; use the OS secret store where possible.
- Back up PostgreSQL separately from the git clone.
- A reverse proxy (IIS/nginx on Windows) is optional; by default NetLynx listens on **:8080** directly, as on Linux without nginx.

---

## Removing the service (if needed)

```powershell
nssm stop netlynxd
nssm remove netlynxd confirm
```
