# Switch configuration history (`show run`)

Running-config snapshots over SSH, to answer "**what changed on the switch**" between two points in time.

**Languages:** [Русский](../Config-History.md) · English

## Quick path

1. Device card → **"Config history (show run) — saved when the config changes"** block.
2. **Save snapshot now** (operator), or wait for the scheduler / backup / role sync.
3. **Diff with previous**, or pick two snapshots → **Show diff**.

## Which devices

Snapshots are taken only if:

- the category is **switch**, or
- **MikroTik RouterOS** — category **router** or **ap** (wAP, cAP…) + vendor MikroTik: set explicitly in the card, or auto-detected from sysDescr/name (RouterOS, CCR, RouterBOARD, RBwAP…).

MikroTik capture: SSH `/export compact` (fallback `/export`) → file in the ZIP and a history snapshot.

Example: `sysDescr: RouterOS RBwAPG-5HacT2HnD`, category "access point" — snapshot allowed (since **0.13.6**).

SSH credentials on the device are required (or the shared ones from Settings → Backups), or `SSH_POE_*` in `/etc/netlynx/netlynx.env`.

Since **0.12.28** the ZIP backup and the snapshot scheduler read the password via `GetDevice` (at-rest decryption). Previously `ListDevices` returned ciphertext → `unable to authenticate`, even though the password was in the card and a manual diff worked.

Since **0.13.0** routers with `ssh_vendor=auto` also end up in the ZIP when SNMP/name says RouterOS (previously an explicit MikroTik vendor was required). In Settings → Backups there are two independent checkboxes: switches (`switch_cfg_enabled`) and MikroTik routers (`router_cfg_enabled`). The old single "switches only" checkbox actually enabled the whole SSH capture — the label was wrong. After the migration, if capture was already enabled, both flags stay enabled.

## SSH errors and alerts (0.12.27+)

| Class (`err_class`) | Typical text | What to do |
|---------------------|--------------|------------|
| `auth` | `unable to authenticate … [none password]` | Correct login/password in the card / Backups / `SSH_POE_*`. Passwords are never guessed. |
| `hostkey` | `crypto/rsa: verification error`, "host key changed" | NetLynx walks legacy algorithms and refreshes `/var/lib/netlynx/ssh_known_hosts`. The key probe (since **0.13.1**) stops before userauth — no `login failure` for `_hostkey_probe_` on RouterOS. |
| `timeout` | i/o timeout | Network / ACL / switch offline. |

Events in the feed and Telegram (if enabled and the type filter is empty or contains these types):

- `CONFIG_SSH_FAIL` — capture failure on a device (6 h debounce per error class)
- `BACKUP_SSH_PARTIAL` — ZIP backup summary (how many devices produced no config)

When a device is created / promoted, NetLynx probes the host key and SSH itself; on failure — `ssh_warning` in the API response and `CONFIG_SSH_FAIL`.

If Settings → Notifications has a **non-empty** `telegram_event_types` whitelist, add `CONFIG_SSH_FAIL,BACKUP_SSH_PARTIAL` to it.

## Snapshot sources

| source | When |
|--------|------|
| `scheduled` | background scheduler (every 24 h by default) |
| `backup` | nightly SSH config backup |
| `port_sync` | "Re-read config (SSH)" / sync port roles |
| `manual` | the "Save snapshot now" button |

Duplicates are not written: SHA-256 is computed over the **canonical** text. Before comparing, runtime lines that are not configuration are dropped:

- EdgeSwitch / Ubiquiti: `!System Up Time`, `!Current SNTP Synchronized Time` (and the NTP variant)
- Cisco: `ntp clock-period`
- MikroTik: date/time in the first `# … by RouterOS` line

Old snapshots that still contain uptime do **not** produce a new row on the next poll if nothing else changed. New snapshots no longer contain those lines.

The nightly ZIP backup still stores a fresh `show run` as is (for restore); the history table only gets it when there is a real change.

## Env

```bash
CONFIG_SNAPSHOT_ENABLED=true          # false = no scheduler
CONFIG_SNAPSHOT_INTERVAL_HOURS=24
CONFIG_SNAPSHOT_RETENTION_DAYS=90
```

## API

```bash
GET  /api/v1/devices/{id}/config/snapshots
GET  /api/v1/devices/{id}/config/snapshots/{snapId}
GET  /api/v1/devices/{id}/config/diff?to=123&from=122   # from/to may be omitted — the latest is used
POST /api/v1/devices/{id}/config/snapshot               # operator
```
