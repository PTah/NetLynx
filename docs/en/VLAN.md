# VLAN: database and ports (access / trunk / native / allowed)

NetLynx reads and changes switch VLAN configuration **over SSH** (CLI), not over SNMP. Everything you see on the VLAN tab and in the VLAN column of the ports table comes from `show run` (snapshot) plus the current port polling.

Where it is in the UI: **Devices → device card → "VLAN" tab** and **the same card → "Ports"** (VLAN / mode / allowed columns).

**Languages:** [Русский](../VLAN.md) · English

## Prerequisites

| Requirement | Where to set it |
|---|---|
| SSH access to the switch | device card → `ssh_user` / `ssh_password` / `ssh_port` (+ `enable password` if needed) |
| Vendor is known | `ssh_vendor` field: `auto` (detected from `sysDescr`) or explicitly — `ubiquiti`, `eltex`, `snr`, `mikrotik`, `cisco`, `aruba`, `zyxel`, `huawei`, `hp`, `tplink`, `dlink`, `dahua`, `hikvision`, `hiwatch` |
| An account with write rights | a read-only user will show VLANs but cannot change them |
| Config snapshot | "Save config" in the device card (or scheduled sync) — the VLAN database is taken from the config |

If SSH is not configured, the tab shows data from the last config snapshot and changes are unavailable with a clear error.

## VLAN database

Per-device VLAN table: `id`, name, and where it is used.

- **Create** a VLAN — `POST /api/v1/devices/{id}/vlans` (`vlan_id`, `name`). The VLAN is created in the switch config (`vlan <id>` + `name`).
- **Rename** — `PATCH /api/v1/devices/{id}/vlans/{vlanId}`.
- **Delete** — `DELETE /api/v1/devices/{id}/vlans/{vlanId}` (or as a list: `DELETE /api/v1/devices/{id}/vlans` with `vlan_ids`).

**Important:** you cannot assign a VLAN to a port unless it exists in the device database — the API returns an error. Create the VLAN first, then assign it to ports.

### Deletion: impact check

Before deleting, the UI asks `GET /api/v1/devices/{id}/vlans/delete-impact?vlan_ids=10,20` and shows:

- whether the VLAN exists in the device database,
- whether it is **used on ports** (and in which role),
- whether it appears in the config.

Deleting a VLAN that is in use breaks connectivity: NetLynx shows a warning, but the decision is yours (one VLAN on many ports = a wide blast radius).

## Ports: modes and VLANs

The **VLAN** column in the ports table and the port editor share one model:

| Mode | Meaning | How it is applied |
|---|---|---|
| **Access** | the port is in a single VLAN, untagged traffic | `access vlan <id>` (on Ubiquiti/EdgeSwitch — `switchport` + `vlan`), untagged |
| **Trunk** | tagged trunk plus one **native** VLAN | tagged VLANs + `native vlan <id>` |
| **No VLAN** | no VLAN on the port (e.g. a trunk without native, or a bare uplink) | the VLAN is removed |

Also:

- **Native VLAN** — a separate field for trunks (what leaves untagged).
- **Allowed VLANs** — the allowed list on a trunk; modes: `add`, `remove`, `set` (exactly this list), `all`, `except`.
- Port change: `PATCH /api/v1/devices/{id}/interfaces/{ifIndex}/vlan` with body `{ op, vlan_id?, allowed_mode?, allowed_vlans? }` (`set_access` / `remove` / `trunk_allow` / `no_vlan`). Same from the VLAN tab and from advanced port settings.

After applying, a green confirmation line (and API field `message`) means the switch already accepted the command. The follow-up `show run` runs **in the background** and does not delay the response (on SNR a full dump is often slow). If there is no reply for ~25 s, the UI warns that the write may already have happened — do not click again until you see success or an error.

If a PoE device is connected to the port, the port is not power-cycled: the VLAN is applied on the fly (a PoE reset is a separate button).

### Advanced port settings (live)

The “Advanced settings” dialog reads the port over SSH: `GET …/interfaces/{ifIndex}/settings` (optional `?force=1`). It prefers the config cache / snapshot; otherwise a short `show running-config interface …`. On **SNR** it does not start with a long `show running-config current-mode`; the `--More--` pager is advanced so the UI does not stick on “Reading settings from device…”.

## Related

- **VLANs on the topology.** LLDP/CDP know nothing about VLANs: if the link goes through an SFP or a VLAN cut, define the link manually — [Manual-Topology-Links.md](Manual-Topology-Links.md).
- **VLANs when an address changes.** Changing the IP and "storage" are handled in the device card; moving a device does not change port VLANs automatically.
- **Change history.** Every port/VLAN-database change is a journal event and a new config snapshot: [Config-History.md](../Config-History.md).
- **Port ignore list.** To exclude a port from events (e.g. a test bench) — [Ignore-List.md](../Ignore-List.md).

## Limitations

- Writing is only possible over SSH; SNMP does not change VLANs (Q-BRIDGE is read for FDB, never written).
- Command syntax depends on vendor and firmware: if the vendor is detected incorrectly, set `ssh_vendor` manually.
- VLAN 1 is a special case: on most vendors it cannot be deleted or renamed; "No VLAN" ≠ "VLAN 1".
- If the SSH session is busy (someone is in the terminal), the switch may refuse — retry the operation.
- Bulk changes are applied sequentially; if one port fails, the others are still applied and the error is returned in the response.
- Management SVI = `devices.host`: deleting that VLAN is **blocked** (API 409; delete-impact sets `blocks_delete`).

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/devices/{id}/vlans` | device VLAN database (source: `config_cache` / `snapshot` / `ports`) |
| POST | `/api/v1/devices/{id}/vlans` | create a VLAN; response: `message`, `vlans[]`, `source` |
| PATCH | `/api/v1/devices/{id}/vlans/{vlanId}` | rename; response: `message`, `vlans[]` |
| DELETE | `/api/v1/devices/{id}/vlans/{vlanId}` | delete a VLAN |
| DELETE | `/api/v1/devices/{id}/vlans` | delete a list (`vlan_ids`) |
| GET | `/api/v1/devices/{id}/vlans/delete-impact` | pre-delete impact (`blocks_delete` for mgmt SVI) |
| PATCH | `/api/v1/devices/{id}/interfaces/{ifIndex}/vlan` | VLAN on a port; response: `message`, `vlans[]`, `source` |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/settings` | live port settings (SSH / show run cache) |

Full list — [API.md](API.md). SNR / EdgeSwitch quirks — [Vendors.md](Vendors.md).
