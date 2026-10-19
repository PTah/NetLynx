# Device autodiscovery on ports (LLDP / CDP)

How NetLynx **polls** switches and **rebuilds** the graph cache (including the "nightly" rebuild at 3:00) is covered separately: [Polling-and-Topology-Cache.md](Polling-and-Topology-Cache.md).

**Languages:** [Русский](../Autodiscover.md) · English

## What the switches need

For **correct autodiscovery** ("Discovered", port neighbours, LLDP/CDP topology edges) the switches must have:

| Protocol | Why | Where on the switch |
|----------|-----|---------------------|
| **SNMP** (v2c/v3) | NetLynx reads MIBs (LLDP/CDP included) over SNMP only | globally: agent / `snmp-server` + community (at least **RO**) |
| **LLDP** | neighbours in `port_neighbors`, "Discovered" candidates, the map | usually **on ports** (transmit + receive); otherwise the walk is empty |
| **CDP** (Cisco and compatible) | the same for CDP neighbours | on ports / globally, vendor-dependent |

NetLynx **does not enable or disable** SNMP/LLDP on hardware — it only polls. Configuration happens on the switch (vendor CLI/UI) or manually.

Community and RO/RW: [SNMP-Community.md](SNMP-Community.md).

### Example: Ubiquiti EdgeSwitch

SNMP (global):

```text
enable
configure
snmp-server community YOUR_RO_COMMUNITY ro
exit
write memory
```

LLDP on a port range (use your own, e.g. `0/1-0/48`):

```text
enable
configure
interface 0/1-0/48
lldp transmit
lldp receive
lldp transmit-tlv port-desc
lldp transmit-tlv sys-name
lldp transmit-tlv sys-desc
lldp transmit-tlv sys-cap
lldp transmit-mgmt
exit
write memory
```

Check from the NetLynx server: `snmpwalk` against LLDP rem (`lldpRemSysName` etc.) — see "Diagnostics" below.

## PCs and servers (Windows / Linux)

Autodiscovery in NetLynx works like this: **the switch is polled over SNMP**, and in its LLDP/CDP MIBs the switch returns what it **heard from neighbours**. Therefore:

| On | SNMP | LLDP | Why |
|----|------|------|-----|
| **Switch** | required | receive (+ usually transmit) | NetLynx reads port neighbours |
| **PC / server / phone** at a port | **not required** to appear in "Discovered" | **transmit is desirable** | the switch sees sysName / chassis / port and returns it over SNMP |
| A node you poll in NetLynx itself | an SNMP agent is needed | optional | device card, IF-MIB, etc. |

NetLynx **does not install** anything on PCs — below is a typical manual setup.

### Windows (10 / 11 / Server) — SNMP

> **`Install-WindowsFeature` is Windows Server only.** On Windows 10/11 that command does not exist — use the "client" block below.

**Windows 10 / 11 (client)** — PowerShell **as Administrator**:

```powershell
# Check whether the SNMP component is already there
Get-WindowsCapability -Online | Where-Object Name -like 'SNMP*'

# Install
Add-WindowsCapability -Online -Name "SNMP.Client~~~~0.0.1.0"

# If Add-WindowsCapability is unavailable / fails — use DISM:
# DISM /Online /Add-Capability /CapabilityName:SNMP.Client~~~~0.0.1.0
```

Or the GUI: **Settings → Apps → Optional features → Add a feature** → find **SNMP** / **Simple Network Management Protocol**.

On some builds SNMP was removed from "Optional features" — then install it via **Optional Features** in the classic panel (`optionalfeatures.exe`) → **Simple Network Management Protocol (SNMP)**, or through DISM above.

**Windows Server** (PowerShell as Administrator):

```powershell
Install-WindowsFeature SNMP-Service -IncludeManagementTools
# module: Import-Module ServerManager
```

Then (both client and Server):

1. `services.msc` → **SNMP** service → startup type "Automatic" → Start.
2. Service properties → **Security** tab: community (e.g. `netlynx-ro`), rights **Read Only**; optionally restrict accepted hosts to the NetLynx IP.
3. **Traps** tab — only if you need traps (not required for discovery).
4. Firewall: allow **UDP 161** from the NetLynx IP (if the agent must answer polling).

Check from the NetLynx server:

```bash
snmpwalk -v2c -c netlynx-ro HOST_IP system
```

### Windows — LLDP (so the switch sees the PC)

The adapter needs Microsoft LLDP / LLTD bindings (often present but disabled).

PowerShell as Administrator (use your adapter name: `Get-NetAdapter`):

```powershell
Get-NetAdapterBinding -Name "Ethernet" | Where-Object ComponentID -match 'lldp|lltd|rspndr'

Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_lldp
Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_lltdio
Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_rspndr
```

If `ms_lldp` is not in the list — enable **Microsoft LLDP Protocol Driver** in the adapter properties (and **Link-Layer Topology Discovery** Mapper/Responder if available).

On the **switch**, `show lldp remote-device` (and the neighbour column in NetLynx) should then show the host (Windows name / chassis). Without LLDP on the PC the switch often shows only the MAC in FDB, with no proper sysName in LLDP.

**Important (Windows):** the Microsoft LLDP driver often sends only the chassis MAC **without sysName and without a management address**. Since **0.12.6** NetLynx fills the device `chassis_mac` from SNMP `ifPhysAddress` (by host IP) and/or from the switch ARP, and substitutes an empty LLDP sysName from the inventory — otherwise the link does not connect on the map.

### Linux — SNMP (`snmpd`)

Debian / Ubuntu:

```bash
sudo apt update
sudo apt install -y snmpd snmp
sudo cp -a /etc/snmp/snmpd.conf /etc/snmp/snmpd.conf.bak
```

Minimal RO community (example; adjust community and ACL):

```bash
sudo tee /etc/snmp/snmpd.conf >/dev/null <<'EOF'
agentAddress udp:161
rocommunity netlynx-ro default
sysLocation office
sysContact admin@example.com
sysName $(hostname -f)
EOF
# better to set sysName explicitly:
sudo sed -i "s/sysName .*/sysName $(hostname -f)/" /etc/snmp/snmpd.conf

sudo systemctl enable --now snmpd
sudo systemctl status snmpd --no-pager
```

Allow UDP/161 in the firewall (`ufw allow from NETLYNX_IP to any port 161 proto udp` with ufw).

Check:

```bash
snmpwalk -v2c -c netlynx-ro 127.0.0.1 system
```

### Linux — LLDP (`lldpd`)

```bash
sudo apt update
sudo apt install -y lldpd
sudo systemctl enable --now lldpd
sudo lldpcli show configuration
sudo lldpcli show neighbors
```

By default `lldpd` transmits and receives LLDP on interfaces. To limit interfaces (example):

```bash
# /etc/lldpd.d/local.conf — lldpd syntax; or:
sudo lldpcli configure system interface pattern eth*,en*
```

RHEL / Rocky / Alma:

```bash
sudo dnf install -y net-snmp net-snmp-utils lldpd
sudo systemctl enable --now snmpd lldpd
```

SNMP config: `/etc/snmp/snmpd.conf` (same `rocommunity`).

### What to expect in NetLynx

1. The switch is in the inventory with working SNMP.
2. On the port to a PC/server the switch sees an LLDP neighbour → after polling there is a row in **"Discovered"** / the "LLDP neighbour" column.
3. SNMP on the PC itself is only needed if you **add that host** as a polled device; to make the neighbour appear from the switch port, **LLDP on the host + SNMP+LLDP on the switch** is enough.

## What is implemented

On every successful IF-MIB poll, LLDP and CDP are read as well; neighbours are written to `port_neighbors`. Separately, FDB can create `protocol=fdb` neighbours (AP/edge heuristics) for the topology map.

### LLDP

- `lldpRemSysName`, `lldpRemPortId`, `lldpRemChassisId`
- chassis subtype (including networkAddress → `remote_mgmt_addr`)
- local port → ifIndex mapping via `lldpLocPortTable`, otherwise ifIndex / ifName `0/N`

### CDP (Cisco)

- `cdpCacheDeviceId` / `cdpCacheDevicePort` / address
- index `ifIndex.deviceIndex`

### Storage

`port_neighbors` table:

- PK `(device_id, if_index, protocol, rem_index)`
- `protocol`: `lldp` | `cdp` | `fdb`
- `stale` + TTL ~**2 h**
- On a walk **error** the data is **not cleared**
- An empty successful walk: neighbours are marked stale only after **two consecutive** empty polls (`neighborEmptyConfirmPolls = 2`), to avoid flapping on a one-off LLDP glitch

## API / UI

| What | Where |
|------|-------|
| Port neighbours | `GET /devices/{id}/detail` → `neighbors`; the **"LLDP neighbour"** column |
| Inventory candidates | **Discovered** page (`/discovered`): ignore / reopen / preview / promote / **link**. **Ignore** hides the node from the topology and keeps it out of "New" on LLDP/scan (visible only under the "Ignored"/"All" filter). A weak LLDP entry (no MAC/IP) on a port where FDB has exactly one inventory device is merged automatically (`added`) or by the "This is device …" button |
| Map | **Topology** (`/topology`): protocol filters (lldp/cdp/fdb/manual), VLAN, location, depth, layout |
| Map settings | `GET/PATCH /api/v1/settings/topology` |

Manual links — [Manual-Topology-Links.md](Manual-Topology-Links.md).

## Resolving to a known device

1. **MAC** — chassis / MAC in Port ID
2. **mgmt IP** — `remote_mgmt_addr` / host
3. **sysName** — only if unique among devices

During polling `devices.chassis_mac` is written; on promote the MAC from discovered goes straight into `chassis_mac`.

## Network scan (SNMP subnet scan)

Separate from LLDP/CDP: a **manual** sweep of a management subnet or IP list from **Settings → Inventory → Network scan**.

| Parameter | Value |
|-----------|-------|
| Protocol | SNMP **v1 / v2c** (community); v3 is not supported by the scan |
| Limit | ≤ **256** addresses (`/24` or a list) |
| Probe | `sysName` / `sysDescr` (up to 20 in parallel) |
| Staging | successful hits → `discovered_devices` (`last_protocol=snmp-scan`, `identity_key=addr:<ip>`) |
| Into Devices | only explicitly: "Add selected" or promote on `/discovered` |

Do not confuse it with the nightly blast rebuild: this is not a background daemon and not a scheduled ping sweep — [Polling-and-Topology-Cache.md](Polling-and-Topology-Cache.md).

API: `POST /api/v1/devices/scan-snmp`, `POST /api/v1/devices/scan-snmp/add`.

### EdgeSwitch ifName `0/10+` trap

The OctetString `0/24` must not be interpreted as IPv4 (`48.47.50.52`).

## Feature status

| Feature | Status |
|---------|--------|
| discovered_devices + promote/ignore | **done** (historically 0.16) |
| FDB → topology neighbours | **done** (`protocol=fdb`) |
| Topology filters/layout | **done** in the UI |
| SNMP subnet scan (Settings) | **done** (0.12.11) |

## Diagnostics

- Are **SNMP** and **LLDP** enabled on the switch (and CDP if you need Cisco neighbours)? Without them autodiscovery and LLDP/CDP topology will not work.
- `snmpwalk` LLDP rem sysName / CDP deviceId from the NetLynx host
- "Show all interfaces" — the neighbour may live on a non-physical ifIndex
- Logs: `lldp neighbors walk` / `cdp …` / `… write`
