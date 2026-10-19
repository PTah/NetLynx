# MAC anomaly investigation

This module helps to analyse **MAC flapping**, roaming addresses and "one MAC on several access ports" situations, when the LLDP/CDP topology shows no ring.

See also: [Loop-Investigation.md](Loop-Investigation.md) (device loops), [Postmortem.md](../Postmortem.md) (timeline around the moment).

**Languages:** [Русский](../MAC-Investigation.md) · English

## Quick path

1. Menu **MAC** (`/investigate/mac`).
2. **Hot MACs**, or type an address manually.
3. Look at: the multi-access banner (if any), the **L2 path**, the **movement map**, the footprint, **hypotheses**, the timeline, **FDB history**.
4. Status: **open** / **resolved** / **ignored** (operator).
5. If needed — a **shut-impact** preview and port admin-down. WiFi clients are not shown in hot MACs and reports while "Track WiFi…" is off (Settings → MAC).

Or "investigate" from a `MAC_FLAPPING` / `MAC_MOVED` / `MAC_MULTI_ACCESS` event, or click a MAC in the port search.

## API

```bash
# report
curl -sS -H "Authorization: *** $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac?mac=52:54:4c:83:09:e0"

# hot MACs (UI: hours=24, min_moves=2 — do not confuse with the MAC_FLAP_MIN_MOVES event threshold)
curl -sS -H "Authorization: *** $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac/flappers?hours=24&min_moves=2"

# history from FDB snapshots
curl -sS -H "Authorization: *** $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac/fdb-history?mac=52:54:4c:83:09:e0"
```

| Method | Path | Role |
|--------|------|------|
| GET | `/investigate/mac` | viewer |
| GET | `/investigate/mac/flappers` | viewer |
| GET | `/investigate/mac/fdb-history` | viewer |
| PATCH | `/investigate/mac/status` | operator |
| GET/PATCH | `/settings/mac-investigation` | get viewer / patch operator |

A WiFi MAC from the "do not track" list → **404** `mac_not_tracked` (no report and no suggestion to enable WiFi tracking).

## Hypotheses (examples)

`unmanaged_loop`, `kvm_dual_uplink`, `core_loop_broadcast`, `misclassified_uplink`, `virtualization_mac`, `dual_homed_or_clone`, `ap_roaming`, `rogue_mac`, `duplicate_mac`, `port_storm`, `insufficient_data`, …

`kvm_dual_uplink` may be suppressed by a strong `core_loop_broadcast`.

## Data sources

| Source | How | Why |
|--------|-----|-----|
| FDB poll + `mac_fdb_moves` | always | slow movements |
| Daily `fdb_snapshots` | `FDB_SNAPSHOT_*` | "where it was N days ago" |
| Syslog UDP | `NETLYNX_SYSLOG_LISTEN=:9514` | realtime flap: Eltex `%BRG_MACNTFY…`, Cisco `%SW_MATM-4-MACFLAP_NOTIF` / `MACFLAP` |

Without syslog, sub-second flapping often never reaches an FDB snapshot.

> **0.9.1:** QEMU/KVM MACs (`52:54:…`, locally administered) are no longer dropped just because of a missing ARP entry — otherwise syslog `MAC_FLAPPING` from Eltex never became an event.

### Syslog (Eltex example)

```bash
# /etc/netlynx/netlynx.env
NETLYNX_SYSLOG_LISTEN=:9514
```

On the switch — send UDP to the NetLynx IP. A flapping message → `MAC_FLAPPING` + history.

## Thresholds (env)

| Variable | Default | Meaning |
|----------|---------|---------|
| `MAC_FLAP_MIN_MOVES` | 3 | port changes per window → `MAC_FLAPPING` event |
| `MAC_FLAP_WINDOW_SECONDS` | 3600 | window |
| `MAC_FLAP_DEBOUNCE_SECONDS` | 900 | anti-spam |
| `MAC_MOVES_RETENTION_DAYS` | 14 | moves retention |
| `NETLYNX_SYSLOG_LISTEN` | empty | UDP; `off` = disabled |
| `FDB_SNAPSHOT_ENABLED` | true | daily snapshots |
| `FDB_SNAPSHOT_INTERVAL_HOURS` | 24 | |
| `FDB_SNAPSHOT_RETENTION_DAYS` | 30 | |
| `FDB_STALE_CLEAR_DAYS` | 60 | auto-cleanup of live FDB (0 = off) |

## If it does not work

| Symptom | What to do |
|---------|------------|
| Timeline is empty | 2+ FDB polls; check the device FDB status |
| Syslog is not received | UDP firewall; host = `devices.host`; port names ≈ `if_name` |
| False flapping on an uplink | trunk role / sync from SSH |
| No ring on the topology | normal for unmanaged / KVM dual-uplink — check the hypotheses |
| MAC `52:54:…` between two access ports | `kvm_dual_uplink`: try admin-down on one port; check STP/bond on the hypervisor |
