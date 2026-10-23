# NetLynx HTTP API

NetLynx serves the API **itself** — the `netlynxd` process listens on the configured port (default **8080**).

**Base URL** (typical install without nginx):

```text
http://<server-ip-or-name>:8080
```

Examples:

- `http://10.0.0.1:8080/api/v1/devices`
- `http://127.0.0.1:8080/health` — from the server itself

**If you installed nginx** ([install-nginx.sh](../install-nginx.sh)) — 80/443 externally, nginx → `127.0.0.1:8080`. A reverse proxy is **not required**.

Authentication: **Bearer** `access_token` in `Authorization`, or **Basic** from the server (`curl -u`). Refresh — an `HttpOnly` cookie issued by `POST /api/v1/auth/login`.

**Languages:** [Русский](../API.md) · English

## Roles (RBAC)

| Role | Access |
|------|--------|
| **viewer** | read: devices, ports, events, topology, discovered, MAC/loops/postmortem, metrics |
| **operator** | + change devices, ports, notifications (incl. incident actions), discovered, manual links, snapshots |
| **admin** | + users, audit, backups, service journal, destructive inventory |

Details: [Roles.md](Roles.md).

---

## Without authentication

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | `{ "status": "ok", "version", … }` |

---

## Auth — `/api/v1/auth`

| Method | Path | Role | Description |
|--------|------|------|-------------|
| POST | `/login` | — | `{ username, password }` → `access_token`, refresh cookie |
| POST | `/refresh` | cookie | new `access_token` |
| POST | `/logout` | cookie | end the session |
| GET | `/me` | auth | current user |
| POST | `/sse-ticket` | auth | ticket for SSE (EventSource without Bearer) |

---

## Devices — viewer+

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/devices` | device list |
| GET | `/api/v1/devices/{id}` | one device |
| GET | `/api/v1/devices/{id}/detail` | device + ports + events + neighbors |
| GET | `/api/v1/devices/{id}/interfaces` | ports |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/clients` | MAC/IP (FDB+ARP) |
| POST | `/api/v1/devices/{id}/interfaces/{ifIndex}/clients/refresh` | operator+: live SNMP FDB+ARP for the whole device (no mac-move events); response is clients for this port |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/settings` | live port settings (SSH / show run cache): admin, PoE, VLAN/trunk, STP, …; `?force=1` skips cache |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/poe-reset-schedule` | PoE reset schedule |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/shut-impact` | shutdown risk preview: clients, LLDP/CDP, **downstream** (multi-hop blast cache), cut_paths |
| GET | `/api/v1/devices/{id}/events` | device events |
| GET | `/api/v1/devices/{id}/metrics` | metrics history (`metric_type`: `cpu_pct`, `page_count`, `toner_*_pct`, `ups_*` — see [UPS.md](UPS.md), `util_*`; `from`/`to` RFC3339; `max_points` downsample). Toner/pages/UPS are kept longer than util (`METRICS_PRINTER_RETENTION_DAYS`) |
| GET | `/api/v1/devices/{id}/traffic-series` | traffic per port |
| GET | `/api/v1/devices/{id}/config/snapshots` | `show run` snapshots |
| GET | `/api/v1/devices/{id}/config/snapshots/{snapId}` | one snapshot |
| GET | `/api/v1/devices/{id}/config/diff?from=&to=` | diff (can be omitted — latest) |
| GET | `/api/v1/devices/{id}/vlans` | VLAN database + port membership (show run / ports); FDB only for already known VLANs (no ghost rows) |
| GET | `/api/v1/devices/{id}/vlans/delete-impact?vlan_ids=` | VLAN deletion blast radius: topology (STP/heuristic), VLANs on trunks, downstream/redundant, FDB; depth `VLAN_BLAST_MAX_DEPTH` (default 32); SVI/mgmt (`mgmt_risks`, severity `critical`); gateways with SVI (`gateways`); `blocks_delete=true` when management SVI = host (delete then returns 409); UPLINK/DOWNLINK override |
| GET | `/api/v1/devices/{id}/fdb/snapshots` | daily FDB snapshots |
| GET | `/api/v1/ports/search?q=` | port search |

### CRUD and diagnostics — operator+

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/devices` | create |
| PATCH | `/api/v1/devices/{id}` | SNMP parameters |
| PATCH | `/api/v1/devices/{id}/name` | name |
| PATCH | `/api/v1/devices/{id}/host` | IP/DNS |
| PATCH | `/api/v1/devices/{id}/chassis-mac` | chassis MAC |
| PATCH | `/api/v1/devices/{id}/location` | location |
| PATCH | `/api/v1/devices/{id}/category` | device type |
| PATCH | `/api/v1/devices/{id}/poll-interval` | interval |
| PATCH | `/api/v1/devices/{id}/monitoring` | utilisation thresholds (`util_high_pct`/`util_ok_pct`), `fdb_poll_interval_seconds` (30–86400), `heavy_poll_interval_seconds` (60–86400; `null` = `POLL_HEAVY_SECONDS`, 300 s by default) |
| PATCH | `/api/v1/devices/{id}/online-override` | reachability mode: `auto` \| `ping` \| `online` \| `offline`. `ping` — ICMP only: SNMP is not polled at all for such a device (see the device card) |
| PATCH | `/api/v1/devices/{id}/trust-link-traps` | flag for link traps |
| PATCH | `/api/v1/devices/{id}/ssh` | device SSH |
| DELETE | `/api/v1/devices/{id}` | delete one |
| POST | `/api/v1/devices/{id}/snmp-test` | SNMP test |
| POST | `/api/v1/devices/{id}/traceroute` | traceroute from the server |
| POST | `/api/v1/devices/{id}/tcp-probe` | TCP connect to a port |
| POST | `/api/v1/devices/{id}/sync-port-roles-from-config` | roles from `show run` |
| POST | `/api/v1/devices/{id}/config/snapshot` | snapshot now |
| DELETE | `/api/v1/devices` | admin — all devices; header `X-Confirm: DELETE-ALL-DEVICES` |

### Ports — operator+

| Method | Path | Description |
|--------|------|-------------|
| PATCH | `.../interfaces/{ifIndex}/descr` | description |
| PATCH | `.../interfaces/{ifIndex}/admin` | admin up/down |
| PATCH | `.../interfaces/{ifIndex}/poe` | PoE |
| POST | `.../interfaces/{ifIndex}/poe-reset` | PoE reset (`{ seconds: 1..60 }`, default 10); EdgeSwitch: `poe reset N`; otherwise a power cycle |
| GET | `.../interfaces/{ifIndex}/poe-reset-schedule` | PoE reset schedule (viewer+); server local time; `mode`: `at_time` \| `times_per_day` |
| PUT | `.../interfaces/{ifIndex}/poe-reset-schedule` | `{ enabled, mode, time?: "HH:MM", start_hour?, start_minute?, times_per_day?, reset_seconds? }`; `times_per_day` N from start time → equal step 24h/N (3× from 03:00 → 03:00/11:00/19:00) |
| DELETE | `.../interfaces/{ifIndex}/poe-reset-schedule` | delete schedule |
| POST | `.../interfaces/bulk` | bulk for ≥2 ports: `{ if_indexes, admin_up?, poe_mode?, vlan?: {op,vlan_id}, poe_reset_seconds? }`; on EdgeSwitch sequentially — `interface 0/4-0/7` |
| PATCH | `.../interfaces/{ifIndex}/isolate` | isolation |
| PATCH | `.../interfaces/{ifIndex}/dhcp-snooping` | DHCP snooping |
| PATCH | `.../interfaces/{ifIndex}/flow-control` | flow control |
| PATCH | `.../interfaces/{ifIndex}/stp` | STP |
| PATCH | `.../interfaces/{ifIndex}/vlan` | `{ op, vlan_id?, allowed_mode?, allowed_vlans? }` — `set_access` / `remove` / `trunk_allow` (`add`\|`remove`\|`all`\|`except`\|`set`) / `no_vlan`; legacy `add_tagged`; response right after SSH: `ok`, `message`, `vlans[]`, `source` (`show run` in background). UI: confirmation line + warning if the reply is slow. Warns when VLAN 1 is missing from allowed |
| POST | `/api/v1/devices/{id}/vlans` | `{ "vlan_id": N, "name"? }` — create in the VLAN database; response: `message`, `vlans[]`, `source` |
| DELETE | `/api/v1/devices/{id}/vlans` | `{ "vlan_ids": [167,30,31] }` — bulk delete (`no vlan …`); **409** if still on ports or mgmt SVI; UI calls delete-impact before confirm |
| PATCH | `/api/v1/devices/{id}/vlans/{vlanId}` | `{ "name": "..." }` — name in the VLAN database; response: `message`, `vlans[]`, `source` |
| DELETE | `/api/v1/devices/{id}/vlans/{vlanId}` | delete one VLAN; **409** if on ports or mgmt; response: `message`, `vlans[]`, `source` |
| PATCH | `.../interfaces/{ifIndex}/thresholds` | util |
| PUT/DELETE | `.../interfaces/{ifIndex}/ignore` | ignore |
| GET | `/api/v1/devices/{id}/port-ignores` | list (operator); in detail view viewer sees the flags |
| POST | `.../clients/preview` | SNMP before promote |
| POST | `.../clients/promote` | device from an FDB port |

---

## Investigation — viewer+ / operator+

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/investigate/mac?mac=` | viewer | MAC report |
| GET | `/api/v1/investigate/mac/flappers` | viewer | top "hot" MACs |
| GET | `/api/v1/investigate/mac/fdb-history` | viewer | history from FDB snapshots |
| PATCH | `/api/v1/investigate/mac/status` | operator | open/resolved/ignored |
| GET | `/api/v1/investigate/loops?protocol=` | viewer | LLDP/CDP loops |
| GET | `/api/v1/postmortem?device_id=&around=&window=&hops=` | viewer | incident timeline |

See [MAC-Investigation.md](../MAC-Investigation.md), [Loop-Investigation.md](../Loop-Investigation.md), [Postmortem.md](../Postmortem.md).

---

## Events and live

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/events` | viewer | feed |
| GET | `/api/v1/events/stream?ticket=` | ticket/Bearer | SSE |

---

## Topology and discovered

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/topology` | LLDP/CDP/FDB/manual graph; `vlan_id` — highlighting: `vlan_filter_id` + `vlan_match_device_ids` (VLANs in the show run VLAN database), the graph is not trimmed |
| GET | `/api/v1/topology/path?from=&to=` | shortest path A→B over the **blast cache** (viewer+); `404` if the cache is empty or there is no path |
| GET | `/api/v1/topology/reachability?from=&max_depth=` | BFS slice from a device (chain debugging); default `max_depth=32` |
| GET/PATCH | `/api/v1/settings/topology` | map settings (viewer get / operator patch) |
| GET | `/api/v1/discovered` | LLDP/CDP candidates / snmp scan; heals MAC/IP and FDB on the same port; for weak LLDP — `likely_device_id` |
| POST | `/api/v1/discovered/{id}/preview\|promote\|ignore\|reopen\|link` | operator; `link` — bind to an existing device (`device_id`) without CreateDevice |
| POST | `/api/v1/devices/scan-snmp` | operator — scan a CIDR/list of IPs (v1/v2c, ≤256 hosts); hits → discovered |
| POST | `/api/v1/devices/scan-snmp/add` | operator — bulk CreateDevice for the selected hits |
| GET | `/api/v1/manual-links` | viewer |
| POST/PATCH/DELETE | `/api/v1/manual-links[/{id}]` | operator |

---

## Settings

| Method | Path | Role | Description |
|-------|------|------|-------------|
| GET/PATCH | `/api/v1/settings/notifications` | operator | webhook, TG, email, **incident_action_*** (enabled, event_types, dry_run, cooldown_seconds) |
| GET | `/api/v1/settings/notifications/stats` | viewer | summary: `kind=reachability\|ups`, `hours` (1–8760; UI: 1 h…365 d) or `days` (compat = N×24 h); per-device table |
| POST | `/api/v1/settings/notifications/email-test` | operator | SMTP test |
| GET/PATCH | `/api/v1/settings/uisp` | operator | UISP |
| GET | `/api/v1/settings/device-categories` | viewer | device types |
| POST/PATCH/DELETE | `/api/v1/settings/device-categories[/{id}]` | operator | device type CRUD |
| GET/PATCH | `/api/v1/settings/mac-investigation` | get viewer / patch operator | WiFi OUI etc. |
| GET | `/api/v1/settings/inventory/stale-fdb` | viewer | stale live FDB preview |
| POST | `/api/v1/settings/inventory/stale-fdb/clear` | operator | `X-Confirm: CLEAR-STALE-FDB` |
| GET | `/api/v1/settings/inventory/offline-devices` | viewer | offline devices |
| POST | `/api/v1/settings/inventory/offline-devices/delete` | admin | `X-Confirm: DELETE-OFFLINE-DEVICES` |
| POST | `/api/v1/devices/import-uisp` | operator | UISP import |
| POST | `/api/v1/devices/scan-snmp` | operator | SNMP scan of a subnet/IP list |
| POST | `/api/v1/devices/scan-snmp/add` | operator | add the selected hits to Devices |

### Backups — admin

| Method | Path | Description |
|--------|------|-------------|
| GET/PATCH | `/api/v1/settings/backup` | schedule, SMB, retention |
| POST | `/api/v1/backup/run` | run now |
| GET | `/api/v1/backup/archives` | list |
| POST | `/api/v1/backup/verify` | verify the ZIP |
| POST | `/api/v1/backup/import` | import into a clean database |

### Service journal — admin

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/settings/journal` | metadata |
| GET | `/api/v1/settings/journal/lines` | lines |
| GET | `/api/v1/settings/journal/stream?ticket=` | SSE |

### SNMP traps — operator+

| Method | Path | Description |
|--------|------|-------------|
| GET/PATCH | `/api/v1/settings/snmp-traps` | listener, log, include labels, `link_trap_events_mode` (`off`/`per_device`/`all`), `link_trap_effects` |
| GET/DELETE | `/api/v1/settings/snmp-traps/logs` | journal / purge |

---

## Users and audit — admin

| Method | Path | Description |
|--------|------|-------------|
| GET/POST/PATCH/DELETE | `/api/v1/users[/{id}]` | CRUD |
| GET | `/api/v1/audit` | audit log |

---

## System — viewer+

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/system/stats` | host CPU/RAM/disk |
| GET | `/api/v1/system/secrets` | admin: at-rest encryption status + plaintext counter |
| POST | `/api/v1/system/secrets/rewrap` | admin: plaintext → `enc:v1:` (idempotent) |
| GET | `/metrics` | Prometheus (Bearer viewer+) |

---

## Destructive confirm

Not `X-Confirm: 1` but an **exact token**:

| Operation | Header |
|-----------|--------|
| Delete all devices | `X-Confirm: DELETE-ALL-DEVICES` |
| Clear stale live FDB | `X-Confirm: CLEAR-STALE-FDB` |
| Delete offline devices | `X-Confirm: DELETE-OFFLINE-DEVICES` |

---

## SNMP scheduler (env)

- `POLL_SCHEDULER_SECONDS` — scheduler tick (default 10 s).
- `poll_interval_seconds` on a device — light poll interval (ping + sysUpTime + IF-MIB statuses/counters). For routers/switches/servers it is capped by `POLL_CRITICAL_INTERVAL_SECONDS` (15 s).
- `heavy_poll_interval_seconds` on a device — heavy poll interval (LLDP/CDP, ARP, PoE, STP, chassis MAC, port roles, FDB). `NULL` = `POLL_HEAVY_SECONDS` (300 s). Editable in the device card ("Heavy poll (s)") or via `PATCH /devices/{id}/monitoring`.
- `FDB_POLL_INTERVAL_SECONDS` — FDB polling (inside the heavy poll, default 900 s).

See [`.env.example`](../../.env.example).

## Errors

`{ "error": "text" }`, HTTP 4xx/5xx.
