# Polling and the topology cache (blast graph)

How NetLynx **collects** network data and **remembers** the connectivity graph.

In short: there is **no** nightly full-network sweep (no automatic ping sweep / scheduled scan of unknown subnets). The network is learned **continuously** by the SNMP poller of devices that are already in the inventory. Separately, a **manual** SNMP scan of a management subnet exists — Settings → Inventory → "Network scan" (see [Autodiscover.md](../Autodiscover.md)). At night the **graph cache** is rebuilt from already collected LLDP/CDP/manual edges, for blast/path analysis.

See also: [Autodiscover.md](../Autodiscover.md) (SNMP + LLDP on switches), [Config-History.md](Config-History.md) (config snapshots).

**Languages:** [Русский](../Polling-and-Topology-Cache.md) · English

## Layers

### 1. Live collection (same day and night)

The poller scheduler (`POLL_SCHEDULER_SECONDS`, each device has its own `poll_interval_seconds`) polls the devices from the **Devices** list:

| What it reads | Where it writes / why |
|---------------|-----------------------|
| LLDP / CDP neighbours | `port_neighbors` → "Discovered" candidates; topology map edges |
| FDB / MAC | live FDB snapshot, events, FDB→topology links |
| IF-MIB, PoE, CPU, Printer-MIB… | device card, metrics, charts |

New devices **do not enter Devices by themselves** — only "Discovered". They are added to the inventory manually (promote from a port / topology / the discovered list).

Code: `internal/poller`; neighbours are written by `UpsertPortNeighbors` / `SyncDiscoveredFromNeighbors`.

#### How the poller decides a device is "gone"

| Rule | Behaviour |
|---|---|
| "Online" | `auto`: ICMP **or** SNMP; `ping`: ICMP only (SNMP not polled); `online`/`offline` — manual mark |
| `DEVICE_OFFLINE` (normal) | after **2 consecutive** failed polls — a short SNMP blip with a live ICMP does not send an email |
| `DEVICE_OFFLINE` (gone) | **immediately on the first poll** when a device that used to answer ICMP went silent on both ICMP and SNMP (that is no blip) |
| `DEVICE_ONLINE` | on the first successful poll after a recorded offline episode |
| NetLynx restart | the confirmation counter is in memory; if the process restarts mid-outage, `DEVICE_OFFLINE` is caught up once the outage exceeds 2 poll intervals (at least 3 minutes) |

#### Light and heavy polling

Polling is split into two levels, so that a critical device going down is visible immediately while large tables are not fetched every minute.

| Level | What it does | Frequency |
|---|---|---|
| **Light** | ICMP ping, `sysDescr`/`sysUpTime`, IF-MIB (port status, counters, speed), LINK_UP/LINK_DOWN, flap, utilisation, CPU, UPS-MIB, MFP toner | device `poll_interval_seconds` (for critical types — at least `POLL_CRITICAL_INTERVAL_SECONDS`, 15 s by default) |
| **Heavy** | LLDP/CDP neighbours, ARP, PoE (PSE-MIB + SSH fallback + LLDP-PD), STP, chassis MAC, port roles, FDB (`FDB_POLL_INTERVAL_SECONDS`) | `devices.heavy_poll_interval_seconds` (device card "Heavy poll (s)"; empty/`NULL` = `POLL_HEAVY_SECONDS`, 300 s by default) |

Device type priority (affects queue order and the upper bound of the light interval):

| Priority | Types | Interval |
|---|---|---|
| Critical | router, switch, server | at least 15 s (`POLL_CRITICAL_INTERVAL_SECONDS`) |
| Important | UPS, NAS, computer, iLO/IPMI | interval from the device card |
| Normal | SIP phones, cameras, MFPs, TVs, etc. | interval from the device card |

On a light poll, fields that only come with the heavy poll (PoE flags, port roles) are **not overwritten** — `UpsertInterfaces` keeps the previous values (`COALESCE` / role from the snapshot).

util/rate history is written at most every `METRICS_UTIL_INTERVAL_SECONDS` (60 s by default): the light poll may run every 15 s, but the charts do not need that many rows — without throttling `metric_samples` grows to ~19M rows/day instead of ~10M (≈1 sample per port per minute).

Poll queue: the scheduler (`POLL_SCHEDULER_SECONDS`, 10 s by default) takes all overdue devices, **sorts by how overdue they are** (`overdueBy`) and polls up to 40 in parallel. A device whose ICMP is silent is checked over SNMP with a short timeout (2 s, no retries) — a dead device does not stretch the cycle by 10+ seconds, otherwise a reboot inside that "blind window" stays unnoticed.

#### Devices without an SNMP agent (backoff)

For devices that answer ICMP but not SNMP (cameras, PCs, TVs, some APs), after **3 consecutive failures** SNMP polling pauses for **15 minutes** — ICMP keeps working and reachability / `DEVICE_OFFLINE` / `DEVICE_ONLINE` are computed as usual. A successful SNMP poll resets the counter. This saves poll slots: without backoff such devices produced ~70 timeouts per minute, 5–10 seconds each.

If a device does have SNMP but it disappeared temporarily (agent reboot), the backoff lifts itself after 15 minutes and polling resumes.

#### FDB on devices without bridge-MIB

If an FDB poll fails ("bridge port map empty" — servers, some APs, devices without `BRIDGE-MIB`), the attempt is still recorded in `last_fdb_poll_at`: such a device is not retried on every heavy poll, it waits for its own `FDB_POLL_INTERVAL_SECONDS`. The device card shows "FDB unavailable".

### 2. Nightly (and periodic) blast-cache rebuild

This is **not** a new SNMP sweep. The job takes edges already known from the database and rebuilds a compact graph for impact analysis (VLAN delete, shut impact, L2 path, loops, path A→B).

Implementation: `internal/topologycache` (`Hub.Run`, `Rebuild`).

Triggers:

| Trigger | When |
|---------|------|
| `startup` | `netlynxd` start |
| `hourly` | every `TOPOLOGY_BLAST_CACHE_INTERVAL_MINUTES` (60 min by default) |
| `night` | wall-clock hour `TOPOLOGY_BLAST_NIGHT_HOUR` (**3:00** by default), once a day |
| `event:…` | after the poller / manual links, debounce ~45 s (`NotifyDirty`) |

`Rebuild` steps:

1. `BuildTopologyGraphFiltered` — inventory nodes + edges from `port_neighbors` / manual (no stale).
2. Root selection (STP preferred / heuristic).
3. BFS distances, VLANs on edges.
4. Atomic cache write + a history row.

Tables: `topology_blast_meta`, `topology_blast_edges`, `topology_blast_dist`, `topology_blast_edge_vlans`, `topology_blast_history` (kept ~`TOPOLOGY_BLAST_HISTORY_DAYS`, 30 by default).

The UI map ("Topology") is built from live `port_neighbors` and related data; the blast cache is the skeleton for "what gets cut off / where does the path go" calculations.

### 3. Other "once a day" background jobs

| Job | Schedule | What it remembers |
|-----|----------|-------------------|
| **FDB daily snapshots** | after a successful FDB poll, not more often than ~20–24 h (`FDB_SNAPSHOT_*`) | the history of "where a MAC was N days ago" |
| **Config snapshots** | interval ~24 h (`CONFIG_SNAPSHOT_*`) | `show run` / diff on the device card |
| **SSH backup** | wall-clock from the UI (hour/minute in backup settings) | switch config archives |

FDB and config are **interval-based**, not tied to 3:00. The backup is closer to "nightly", but it is a config backup, not topology.

## Data flow

```text
SNMP poll (continuous, inventory)
  → port_neighbors / FDB / metrics / discovered
  → (debounce) NotifyDirty
       ↘
TopologyCache Hub: startup | hourly | night@03:00 | event
  → Rebuild from the DB (no SNMP)
  → topology_blast_* + history
```

## Environment variables

See `.env.example`:

| Variable | Meaning | Default |
|----------|---------|---------|
| `TOPOLOGY_BLAST_CACHE_ENABLED` | enable/disable the cache | `true` |
| `TOPOLOGY_BLAST_CACHE_INTERVAL_MINUTES` | periodic rebuild | `60` |
| `TOPOLOGY_BLAST_NIGHT_HOUR` | nightly rebuild hour (0–23) | `3` |
| `TOPOLOGY_BLAST_HISTORY_DAYS` | cache snapshot history retention | `30` |
| `FDB_SNAPSHOT_ENABLED` / `_INTERVAL_HOURS` / `_RETENTION_DAYS` | daily FDB snapshots | see `.env.example` |
| `CONFIG_SNAPSHOT_ENABLED` / `_INTERVAL_HOURS` / `_RETENTION_DAYS` | config snapshots | see `.env.example` |

## Summary

- **Devices / neighbours / MAC** — continuous SNMP polling + the database.
- **The "nightly sweep" in code** — a rebuild of the connectivity cache at `TOPOLOGY_BLAST_NIGHT_HOUR` (plus hourly and on events), **without** rescanning the LAN.
- **A full nightly discovery of unknown IPs is not implemented.**
