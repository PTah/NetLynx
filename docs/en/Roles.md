# NetLynx user roles

There are three roles. Permissions **accumulate**: operator can do everything viewer can, admin everything operator can.

Normal sign-in is the **"Sign in to NetLynx"** page. Accounts are created by an admin in **Settings → Users**.

The role is assigned when the user is created. The HTTP Basic fallback from `.env` is no longer stored in the browser.

**Languages:** [Русский](../Roles.md) · English

## In short

| Role | For whom | Essence |
|------|----------|---------|
| **viewer** | Watching, on-call duty | See the dashboard, devices, events, topology, "Discovered", **MAC / Loops / Postmortem**. **Change nothing.** |
| **operator** | Network engineer | Inventory, ports, discovered, manual links, notifications and **incident actions**, config snapshots. **Not** users or backups. |
| **admin** | System administrator | Everything operator can do + users, audit, backups, service journal, full inventory cleanup. |

## viewer — read only

Allowed:

- dashboard, device list, device card (ports, MAC, charts, events, config history — read);
- event feed, topology, "Discovered";
- **MAC**, **Loops**, **Postmortem**;
- GET for category / mac-investigation / inventory preview settings;
- Prometheus metrics `GET /metrics`.

Not allowed: CRUD on devices/ports, promote, ignore, notification settings, users, backups, service journal.

## operator — working with the network

Everything viewer can do, plus:

- CRUD on a single device, UISP, SNMP test, traceroute / tcp-probe;
- ports: admin/PoE/isolate/…, ignore, thresholds, **VLAN on a port** (access / trunk allow / remove), **create / rename / delete / bulk delete VLANs** in the VLAN database (a VLAN cannot be deleted while it is on ports);
- discovered promote/ignore, manual links;
- syncing port roles from `show run`, manual config snapshot;
- MAC investigation status; PATCH mac-investigation;
- **Settings → Notifications** (including incident dry-run / cooldown / admin_down);
- UISP, device types (mutations), clearing stale FDB.

Not allowed: users, audit, backups, "clear all devices", service journal, deleting the offline batch (admin).

## admin — the whole system

Everything operator can do, plus:

- users and roles;
- audit journal;
- backups;
- **service journal** (journalctl SSE);
- delete all devices / offline devices (with `X-Confirm: …`).

The **last active admin** cannot be deleted or demoted.

## Choosing a role

- Just watching — **viewer**.
- Running the network and notifications — **operator**.
- Server, people, backups — **admin**.

## Emergency admin from `.env`

`NETLYNX_ADMIN_USER` / `NETLYNX_ADMIN_PASSWORD` → HTTP Basic = **admin**. The password is never stored in the browser.
