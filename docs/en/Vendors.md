# Switch vendors

The SNMP/SSH support plan for different brands in **NetLynx**.

Cameras (Dahua / Hikvision / Trassir and others) can already appear in FDB with an OUI label; below we talk about **managed switches** from the same and other vendors (Dahua, Hikvision, HiWatch and Trassir make L2/L3 managed switches too — they must be supported as switches, not just as "cameras behind a port").

**Languages:** [Русский](../Vendors.md) · English

## Support levels

| Level | What NetLynx can do |
|-------|---------------------|
| **L0 — generic** | IF-MIB (link, speed, util), often LLDP/FDB/ARP with the standard agent. No special CPU/PoE/CLI. |
| **L1 — poll+** | + CPU profile, PoE MIB/SSH read, stable LLDP/CDP |
| **L2 — control** | + SNMP SET (admin/ifAlias) and/or SSH CLI: shutdown, description, PoE, isolate |
| **L3 — ops** | + running-config backup over SSH, vendor quirks (pager, enable, XP/BusyBox, etc.) |

**Port persist (since 0.6.4):** UI admin/description prefers SSH, then saves startup: SNR/ELTEX `write`, Cisco-like / EdgeSwitch `write memory`, Huawei `save`. Without SSH credentials on the device card, a SNMP-only change stays in runtime and is lost after reboot.

Current state (reference **0.6.0**, the level table keeps its meaning). **W3** (Keenetic/Cudy/…) — not started, **no ETA** (after polishing W1/W2 and getting real hardware):

| Vendor | Level | Notes |
|--------|-------|-------|
| Ubiquiti EdgeSwitch | **L2–L3** | Main focus; XP has CLI limitations |
| ELTEX (MES) | **L2–L3** | CLI + aging docs |
| SNR / NAG | **L2–L3** | Private PoE MIB + CLI; FDB SNMP: ifIndex 5001+ (see below) |
| Cisco | **L2** (W1) | CPU; SSH IOS-like; `show run` backup |
| Huawei | **L2** (W1) | CPU; VRP system-view; `display current-configuration` |
| MikroTik | **L2** (W1) | CPU; SSH disable/comment/PoE; `/export` |
| Aruba | **L2** (W1) | CPU; SSH AOS-like; PoE; isolate — later |
| Zyxel | **L2** (W1) | CPU; SSH Cisco-like; poe mode; isolate — later |
| HP ProCurve | **L2** (W2) | Separate from Aruba; CPU; SSH; PoE power-over-ethernet |
| TP-Link JetStream | **L2** (W2) | CPU; SSH Cisco-like; power inline supply |
| D-Link DGS/DES | **L2** (W2) | CPU; SSH; power inline |
| Dahua / Hikvision / HiWatch / Trassir **switches** | **L2** (W2) | switch ≠ camera detection; best-effort SSH/PoE; `show run` backup |
| Juniper | **L0–L1** | CPU profile |
| The rest of the list below | **L0** or none | Profiles are needed |

## Target list

| Vendor | Wave priority | Comment |
|--------|---------------|---------|
| **Cisco** | W1 | Enterprise; IF-MIB SET is often fine already; needs `swcfg` + PoE polish |
| **Huawei** | W1 | PoE present; CPU + CLI port |
| **MikroTik** | W1 | RouterOS switch/CRS/CCR; CLI/API or SSH |
| **Aruba** (HPE) | W1 | Common in campus; LLDP/PoE |
| **Zyxel** | W1 | SMB/campus |
| **HP / HPE ProCurve** | W2 | Related to Aruba; separate MIBs |
| **TP-Link** | W2 | Easy Smart / JetStream; see `docs/MIBs/` |
| **D-Link** | W2 | DGS/DES |
| **Dahua** (switches) | W2 | Managed switches (not just cameras) |
| **Hikvision** / **HiWatch** (switches) | W2 | Managed switch line-up |
| **Trassir** (switches) | W2 | Managed switches for video surveillance |
| **Keenetic** | W3 | Often SOHO; limited SNMP |
| **Cudy** | W3 | |
| **Tenda** | W3 | |
| **Origo** | W3 | |
| **ExeGate** | W3 | |
| **ZyXEL** | see Zyxel W1 | |

Ubiquiti / ELTEX / SNR are already in the product; improvements happen as bugs are found, not "from scratch".

### VLAN database (create / name / delete) — per-vendor CLI

Written over SSH (`POST …/vlans`, `PATCH/DELETE …/vlans/{id}`). The order of attempts depends on vendor detection; on `Invalid input` the next style is tried.

| Vendor | Primary CLI (per docs) | Fallback |
|--------|------------------------|----------|
| **Ubiquiti EdgeSwitch** | `# vlan database` → `vlan N` / `vlan name N "…"`; `no vlan 167,30,31` | vlan database from `(Config)#` |
| **ELTEX MES23xx/33xx** | `(config)# vlan database` → `vlan N` / `vlan N name …` | IOS `vlan N` / `name`; Fastpath |
| **ELTEX MES14xx/24xx** | `(config)# vlan N` → `name …` | eltex-db / Fastpath |
| **SNR** | `(config)# vlan N` → `name …` | Fastpath `#` / config; eltex-db |
| **Cisco / Aruba / Zyxel / HP / TP-Link / D-Link / video-LAN** | `(config)# vlan N` → `name …` / `no vlan N` | eltex-db; Fastpath |
| **Huawei VRP** | `system-view` → `vlan N` → `name` / `undo name`; `undo vlan N` | — |
| **MikroTik** | **next** (RouterOS `/interface bridge vlan`, not the IOS vlan database) | — |

Names with hyphens/spaces are quoted. VLAN 1 cannot be deleted. The live models in the office do not cover the whole list — verification is done against vendor CLI guides plus unit tests on command generation.

### Trunk allowed VLANs on a port

`PATCH …/interfaces/{ifIndex}/vlan` with `op=trunk_allow`, `allowed_mode=add|remove|all|except`, `allowed_vlans` (a list like `10,20-22` or SNR `10;20`).

| Vendor / style | CLI |
|----------------|-----|
| **Cisco / EdgeSwitch / SNR / ELTEX MES** | `switchport mode trunk` + `switchport trunk allowed vlan add\|remove\|all\|except …` |
| **ELTEX** | usually `add` / `remove` / `all`; `except` may return Invalid input → fallback |
| **SNR** | like Cisco; lists are often `;`-separated — NetLynx normalises to `,` |
| **Fastpath IEEE (no allowed list)** | `vlan participation include` + `vlan tagging` per ID; `all`/`except` are not emitted in this style |

In the UI, `add` without VLAN 1 (and `remove`/`except` touching management) shows a warning about losing SSH/Web access (native/VLAN 1).

### SNR: FDB and live port settings

| Topic | NetLynx behaviour |
|-------|-------------------|
| **FDB (SNMP)** | In `dot1dTpFdbPort` / Q-BRIDGE SNR often returns an **ifIndex already** (`5001`=ge1 … `5009`=ge9 / SFP uplink), not bridge-port `1…9`. NetLynx accepts both; otherwise the UI MAC table stays empty while `show mac` is full. See [Polling-and-Topology-Cache.md](Polling-and-Topology-Cache.md). |
| **Advanced port settings** | Read with `show running-config interface …` (one port); the `--More--` pager is advanced. Do not start with a long `current-mode` from config. |
| **VLAN writes** | API / green “done” right after CLI; `show run` in the background (full dump on SNR is slow). |

EdgeSwitch: classic `dot1dTpFdbPort` is often empty — MACs come from Q-BRIDGE (as before). On a downlink to SNR, FDB usually has few MACs (source learning on ingress); on SNR ge9 toward the core — many; that is switch behaviour, not a UI filter.

## Work waves

### W1 — enterprise / network core

**Done (0.35.29–0.35.30):** MikroTik → Cisco → Aruba → Zyxel → Huawei — detection, SSH port CLI (admin/description/PoE where applicable), running/export backup, CPU profiles. An SSH account in the device card is required; for SNMP SET an RW community is still needed ([SNMP-Community.md](SNMP-Community.md)).

Next for W1 (polish):
1. PoE SSH-print → ifIndex parsing (MikroTik/Aruba/Zyxel).
2. Isolate for Aruba/Zyxel; AOS-CX vs ProVision nuances; Huawei model quirks.
3. Tests on live devices / sysDescr snapshots.

### W2 — SMB + video-LAN switches

**Done (0.35.34):** HP ProCurve (separate from Aruba) → TP-Link JetStream → D-Link → Dahua / Hikvision / HiWatch / Trassir switches — detection (DS-2CD/IPC cameras are not confused with DS-3E/PFS switches), SSH port CLI (admin/description/PoE), `show running-config` backup, CPU profiles, "SSH vendor" items in the UI. Isolate for W2 — later.

Next for W2 (polish):
1. Vendor PoE MIBs (TP-Link/D-Link) from the walks in `docs/MIBs/`.
2. Live CLI tests on SMB/video switches (PoE syntax drifts between revisions).
3. SNMP SET verification with an RW community.

### W3 — SOHO / long tail

1. Keenetic, Cudy, Tenda, Origo, ExeGate — start with L0 + an IF-MIB test; profiles only when there is SNMP and a reproducible sysDescr.
2. Honestly mark "limited support" in the UI when there is no SET/PoE.

## Architecture (where the code lives)

| Layer | Directory / place |
|-------|-------------------|
| CPU | `internal/snmp/cpu.go` |
| PoE | `internal/snmp/poe.go`, `internal/poecli/` |
| Device category | `internal/store/device_category.go` + UI categories |
| Port CLI / PoE / isolate | `internal/swcfg/vendor*.go` |
| SSH backup | `internal/backup/`, `internal/swcfg/fetch.go` |
| LLDP uplink hints | `internal/snmp/lldp_*.go` |

Rule: **generic IF-MIB first**; vendor code only where the standard breaks or CLI is required.

## Definition of Done ("vendor supported")

- [ ] Recognised by `sysDescr` / name / enterprise OID → correct **category** (switch/router/…).
- [ ] Stable IF polling plus at least one of: LLDP, FDB.
- [ ] The required SNMP community (RO/RW) and limitations are documented.
- [ ] (L1+) CPU and/or PoE without false zeros.
- [ ] (L2+) shutdown or description via SNMP SET **or** SSH.
- [ ] (L3) config capture into a backup without breaking the session (pager/enable).

## Related documents

- [SNMP-Community.md](SNMP-Community.md)
- [PoE-detection.md](PoE-detection.md)
- [Autodiscover.md](Autodiscover.md)
