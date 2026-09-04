# NetLynx

SNMP switch monitoring with a web UI.

NetLynx **monitors switches over SNMP**: it polls them and shows status and events (link up/down, port utilization, and so on) in a **web UI** in the browser.

The app started as an in-house office tool — mainly to find devices on the network and to draw a device topology. Then it grew into what you see now :)

Spec (Russian): [docs/TZ-snmp-switch-monitor.md](docs/TZ-snmp-switch-monitor.md). HTTP API: [docs/API.md](docs/API.md).

**Русский:** [README.md](README.md)

**Screenshots:** [SCREENSHOTS.md](SCREENSHOTS.md)

## Fresh install

| Platform | Document |
|----------|----------|
| **Linux** (recommended) | [docs/Runbook-Linux.md](docs/Runbook-Linux.md) — server from scratch; UI login: [step 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) (`:8080` or nginx) |
| **Linux, copy-paste level** | [Checklist](docs/Runbook-Linux.md#краткий-чеклист-копируйте-блоками) — copy the **whole block** (packages → docker → clone → `bash docs/deploy.sh`). [deploy.sh](docs/deploy.sh) **installs everything** (build, Postgres, systemd, health). [Step 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) — open the UI. The runbook covers a missed re-login after `usermod docker` *(spoiler: `permission denied` is not a NetLynx bug)*. |
| **Windows Server** | [docs/Windows-Server-Setup.md](docs/Windows-Server-Setup.md) |

After install: SNMP community — [docs/SNMP-Community.md](docs/SNMP-Community.md); autodiscovery (switches need **SNMP + LLDP**) — [docs/Autodiscover.md](docs/Autodiscover.md); vendors — [docs/Vendors.md](docs/Vendors.md).

Canonical git repo: [https://github.com/PTah/NetLynx](https://github.com/PTah/NetLynx) (`main` branch).

Ops docs (mostly Russian):

- Access roles: [docs/Roles.md](docs/Roles.md)
- PoE and SNMP (troubleshooting): [docs/PoE-detection.md](docs/PoE-detection.md)
- **SNMP RO/RW** (port control): [docs/SNMP-Community.md](docs/SNMP-Community.md) — monitoring works with **RO**; **controlling** the switch (shutdown, alias, incident actions) needs an **RW** community on the device — otherwise NetLynx is read-only.
- **Autodiscovery / LLDP topology:** [docs/Autodiscover.md](docs/Autodiscover.md) — switches must have **SNMP** and **LLDP** enabled (NetLynx does not turn them on for you).
- Switch vendors: [docs/Vendors.md](docs/Vendors.md)
- HTTPS via nginx (optional): [docs/install-nginx.sh](docs/install-nginx.sh)
- Windows desktop: [desktop/README.md](desktop/README.md) — **not ready**, draft against the spec only
- Plan / backlog: [docs/Roadmap.md](docs/Roadmap.md), [docs/To-Do.md](docs/To-Do.md)

---

## What NetLynx is made of

1. **Server (Go)** — SNMP polling, API, PostgreSQL.
2. **Web (React)** — dashboard, devices, events, settings; served from the same **8080** port as the API.

**License:** freeware — free for personal and non-commercial use under [LICENSE](LICENSE). Attribution is required.
