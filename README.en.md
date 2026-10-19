# NetLynx

SNMP monitoring for switches, with a web UI.

The program **watches your switches over SNMP**: it polls them and shows their state and events (link up/down, port utilisation, and more) in a **web interface** right in the browser.

It started as a personal office tool — mostly to find devices on the network and to draw the topology — and grew from there.

Technical specification: [docs/TZ-snmp-switch-monitor.md](docs/TZ-snmp-switch-monitor.md). HTTP API: [docs/API.md](docs/API.md).

**Русский:** [README.md](README.md)

**Screenshots:** [SCREENSHOTS.md](SCREENSHOTS.md)

> Detailed guides under `docs/` are written in Russian — that is the primary language of the project. This page is the English overview: install, first run, configuration, upgrade.

**Sections:** [Installation](#installation-from-scratch) · [First run](#first-run-after-installation) · [What it does](#what-netlynx-does) · [Environment](#environment-settings-env) · [Upgrade & operations](#upgrade-and-operations) · [Recent releases](#recent-releases)

## What NetLynx does

| Feature | In short | Doc |
|---|---|---|
| SNMP polling | ports, speed, utilisation, CPU, Link Up/Down events, flaps, broadcast storms; light poll every 15–60 s, heavy poll (LLDP/CDP/ARP/PoE/STP/FDB) every 5 min | [Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md) |
| Topology | LLDP/CDP + FDB, map, manual links, path finder, blast radius (VLAN / port shutdown) | [Manual-Topology-Links.md](docs/Manual-Topology-Links.md) |
| Discovery & inventory | subnet scan over SNMP/ICMP, "Discovered" → "Devices", device types, UISP | [Autodiscover.md](docs/Autodiscover.md) |
| Ports & VLANs | access/trunk/native/allowed, VLAN database, PoE (detect, watts, reset), port description, mass edit | [VLAN.md](docs/VLAN.md), [PoE-detection.md](docs/PoE-detection.md) |
| Investigations | where a MAC lives, why a loop happened, who is behind a port | [MAC-Investigation.md](docs/MAC-Investigation.md), [Loop-Investigation.md](docs/Loop-Investigation.md) |
| MFPs / printers | page counter, toner left, charts | [Printer-MIB.md](docs/Printer-MIB.md) |
| UPS | UPS-MIB: charge, runtime, input/output, load, temperature, alarms; events and history | [UPS.md](docs/UPS.md) |
| Events & notifications | event journal, Telegram / email / webhook, filters by type and severity, offline and UPS statistics | [Notifications.md](docs/Notifications.md) |
| Roles & access | viewer / operator / admin, JWT + sessions, Basic auth as emergency login | [Roles.md](docs/Roles.md) |
| Secrets & backups | AES-GCM for communities/SSH/SMTP/tokens; ZIP backup of DB + env; `show run` and MikroTik `/export` snapshots | [Secrets.md](docs/Secrets.md), [Config-History.md](docs/Config-History.md) |

## Environment settings (env)

File: `/etc/netlynx/netlynx.env` on Linux (template — [.env.example](.env.example)). After editing: `sudo systemctl restart NetLynx`. Secrets (communities, SMTP, tokens) are stored in the database encrypted — see [Secrets.md](docs/Secrets.md).

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | `postgres://user:password@127.0.0.1:5433/invetor` | PostgreSQL (the database is named `invetor` for historical reasons) |
| `HTTP_ADDR` | `:8080` | API + web UI address (nginx optional) |
| `NETLYNX_ADMIN_USER` / `NETLYNX_ADMIN_PASSWORD` | `admin` / password from the file | first login; after that — users in the UI |
| `NETLYNX_SECRETS_KEY` | — | AES-GCM key for secrets in the DB (`secrets-genkey`) |
| `POLL_SCHEDULER_SECONDS` | `10` | poll scheduler tick |
| `POLL_CRITICAL_INTERVAL_SECONDS` | `15` | upper bound of the light poll for routers/switches/servers |
| `POLL_HEAVY_SECONDS` | `300` | heavy poll (LLDP/CDP/ARP/PoE/STP/FDB); can be overridden per device |
| `FDB_POLL_INTERVAL_SECONDS` | `900` | FDB polling (inside the heavy poll) |
| `METRICS_ENABLED` / `METRICS_RETENTION_DAYS` | `true` / `7` | metrics history: on/off and retention |
| `METRICS_UTIL_INTERVAL_SECONDS` | `60` | how often util/rate samples are written |
| `METRICS_PRINTER_RETENTION_DAYS` | `365` | retention of toner/pages/UPS series (`page_count`, `toner_*`, `ups_*`) |
| `WEB_DIST` | `web/dist` | built web assets directory |
| `NETLYNX_COOKIE_SECURE` | `false` | set `true` when serving over HTTPS only |

The full annotated list is in [.env.example](.env.example).

## Upgrade and operations

- **Upgrade:** [Runbook-Linux.md → "Обновление версии"](docs/Runbook-Linux.md#обновление-версии) (`git pull` + `bash docs/deploy.sh`; DB migrations run automatically on start).
- **Roll back:** [Appendix C](docs/Runbook-Linux.md#приложение-c-откат-на-старую-версию).
- **Check it is alive:** `curl -s http://127.0.0.1:8080/health` → `{"status":"ok","version":"…"}`.
- **Service logs:** `journalctl -u NetLynx -f` or **Settings → Log** in the UI.
- **Backups:** **Settings → Backups** plus [Config-History.md](docs/Config-History.md).
- **If something is broken:** [Runbook → "Если что-то пошло не так"](docs/Runbook-Linux.md#если-что-то-пошло-не-так) (service, Postgres, port, web build).

## First run after installation

Fifteen minutes to make the system useful (everything in the UI, admin role):

1. **Change the admin password** — "Users" → `admin` → password (or create personal accounts right away: [Roles.md](docs/Roles.md)).
2. **Mind the secrets.** Communities, SSH passwords, SMTP credentials and tokens are encrypted with `NETLYNX_SECRETS_KEY`, which the installer generates. Keep it safe: without it the secrets in the DB cannot be decrypted ([Secrets.md](docs/Secrets.md)).
3. **Configure SNMP on the switches**: an RO community is enough for monitoring (RW only if you want VLAN/PoE changes), and `snmp-server host <NetLynx IP>` for traps ([SNMP-Community.md](docs/SNMP-Community.md)).
4. **Enable LLDP/CDP on the switches** — otherwise there is no topology and no discovery ([Autodiscover.md](docs/Autodiscover.md)).
5. **Add devices**: "Discovered" → select → "To devices", or add manually (name, IP, type, community). The device type sets the polling priority: routers/switches/servers are polled every 15 s.
6. **Open one switch card**: ports, VLANs, PoE, topology. If it is empty, check the vendor/SSH settings and the community ([Vendors.md](docs/Vendors.md)).
7. **Configure notifications**: Telegram/email/webhook, tick the event types you need ([Notifications.md](docs/Notifications.md)).
8. **Configure backups** (Settings → Backups) and make sure a file is actually created.

## Installation from scratch

| Platform | Document |
|---|---|
| **Linux** (recommended) | [docs/Runbook-Linux.md](docs/Runbook-Linux.md) — a server from scratch; UI login: [step 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) (`:8080` or nginx) |
| **Linux, copy-paste level** | [Checklist](docs/Runbook-Linux.md#краткий-чеклист-копируйте-блоками) — copy the **whole block** (packages → docker → clone → `bash docs/deploy.sh`). [deploy.sh](docs/deploy.sh) **does everything** (build, Postgres, systemd, health). Then [step 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) to log in. Read the runbook if you skipped re-login after `usermod docker` *(spoiler: `permission denied` is not a NetLynx bug)*. |
| **Windows Server** | [docs/Windows-Server-Setup.md](docs/Windows-Server-Setup.md) |

After installation: SNMP community — [docs/SNMP-Community.md](docs/SNMP-Community.md); discovery (needs **SNMP + LLDP** on switches) — [docs/Autodiscover.md](docs/Autodiscover.md); polling and topology cache — [docs/Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md); vendors — [docs/Vendors.md](docs/Vendors.md); MFPs/printers — [docs/Printer-MIB.md](docs/Printer-MIB.md); UPS over SNMP — [docs/UPS.md](docs/UPS.md); notifications — [docs/Notifications.md](docs/Notifications.md).

Reference git repository: [https://github.com/PTah/NetLynx](https://github.com/PTah/NetLynx) (branch `main`).

Operations documents:
- Access roles: [docs/Roles.md](docs/Roles.md)
- PoE and SNMP diagnostics: [docs/PoE-detection.md](docs/PoE-detection.md)
- **SNMP RO/RW**: monitoring works with **RO**; **managing** a switch (shutdown, alias, incident actions) requires an **RW** community on the device ([docs/SNMP-Community.md](docs/SNMP-Community.md)).
- Discovery / LLDP topology: [docs/Autodiscover.md](docs/Autodiscover.md)
- Polling and topology cache: [docs/Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md)
- Secrets at rest: [docs/Secrets.md](docs/Secrets.md)
- Config history and backups: [docs/Config-History.md](docs/Config-History.md)
- Switch vendors: [docs/Vendors.md](docs/Vendors.md)
- MAC / flapping investigation: [docs/MAC-Investigation.md](docs/MAC-Investigation.md)
- Notifications: [docs/Notifications.md](docs/Notifications.md)
- LLDP loops: [docs/Loop-Investigation.md](docs/Loop-Investigation.md)
- Postmortem: [docs/Postmortem.md](docs/Postmortem.md)
- HTTPS via nginx (optional): [docs/install-nginx.sh](docs/install-nginx.sh)
- Windows desktop client: [desktop/README.md](desktop/README.md) — **not ready**, draft only
- Roadmap / backlog: [docs/Roadmap.md](docs/Roadmap.md), [docs/To-Do.md](docs/To-Do.md)

### Versioning (since 0.2.0)

Format: **`0.<minor>.<patch>`** (the `VERSION` file in the repository root).

| Rule | How |
|---|---|
| Normal change | **patch** +1: `0.2.0` → `0.2.1` → … → `0.2.9` |
| After `.9` | next **minor**, patch resets: `0.2.9` → **`0.3.0`**, then `0.3.1`…`0.3.9` → **`0.4.0`**, and so on |
| Major shift | a **major** bump (`1.0.0`) when the meaning calls for it |

History before the reset (the `0.13.x` line and older) is kept in the journal — [docs/Changelog.md](docs/Changelog.md); the current line starts at **0.2.0**.

## Recent releases

Full journal of every version — [docs/Changelog.md](docs/Changelog.md) (Russian). Latest releases:

**0.4.8** — docs: version journal moved to [docs/Changelog.md](docs/Changelog.md), added the **"First run"** checklist, a new [docs/VLAN.md](docs/VLAN.md) (VLAN database, access/trunk/native/allowed, delete-impact checks, limitations) and this full English README.

**0.4.7** — admin-facing docs: README gained **"What it does"**, **"Environment settings (env)"**, **"Upgrade and operations"** and a table of contents; new [docs/Notifications.md](docs/Notifications.md) (Telegram/email/webhook, filters, full event-type list, statistics, "why nothing arrives" checklist); broken links in `desktop/README.md` fixed.

**0.4.6** — Notification statistics: periods from **1 hour** to **365 days** (1–12/24/48/72 h, 7…365 d); the API accepts `hours` (up to 8760).

**0.4.5** — Settings → Notifications → **Statistics**: per-device table for a period — how many times a device went offline (`DEVICE_OFFLINE`/`DEVICE_ONLINE`) or how many times a UPS lost mains (`UPS_ON_BATTERY`/`UPS_MAINS_OK`); API `GET /api/v1/settings/notifications/stats`.

**0.4.4** — journal noise: repeated SNMP failures within one episode (devices without an agent) are logged at debug level, warn only on the first failure; FDB attempts are recorded even when they fail, so "bridge port map empty" now appears at most once per `FDB_POLL_INTERVAL_SECONDS` per device (was every 5 minutes).

**0.4.3** — the heavy poll interval is configurable per device ("Heavy poll (s)", `PATCH /devices/{id}/monitoring`, `NULL` in the DB = `POLL_HEAVY_SECONDS`). Failed FDB polls are recorded in `last_fdb_poll_at`, so devices without a bridge MIB are no longer probed every 5 minutes.

**0.4.2** — util/rate samples are written at most every `METRICS_UTIL_INTERVAL_SECONDS` (60 s by default): the 15-second light poll pushed ~19M rows/day into `metric_samples`, now ~10M (the pre-change level). Port statuses, LINK_UP/DOWN and flap detection still run every 15–20 s.

**0.4.1** — SNMP backoff for devices without an SNMP agent (cameras, PCs, TVs): after 3 consecutive failures SNMP is not polled for 15 minutes while ICMP keeps working — fewer timeouts in the journal, polling slots are not wasted (~70 failed SNMP polls per minute before).

**0.4.0** — light and heavy polling split: critical types (router, switch, server) are polled at least every **15 s** (`POLL_CRITICAL_INTERVAL_SECONDS`), others at their own interval; heavy tables (LLDP/CDP/ARP/PoE/STP/chassis/FDB, port roles) are read every `POLL_HEAVY_SECONDS` (300 s by default) instead of on every poll. Queue order: critical types first, then the most overdue. PoE flags and port roles are not overwritten by light polls. Effect: a switch going down is visible within ~15–30 s, with less load on the network and the switches.

## What NetLynx is made of

1. **Server (Go)** — SNMP polling, API, PostgreSQL.
2. **Web (React)** — dashboard, devices, events, settings; served from the same port **8080** as the API.

**License:** freeware — free for personal and non-commercial use under the terms of [LICENSE](LICENSE). Attribution is required.
