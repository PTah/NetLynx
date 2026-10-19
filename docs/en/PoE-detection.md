# Detecting PoE on a port in NetLynx

This document describes **how the backend decides** that a switch port is actively delivering PoE (or something close to it), and **how the web UI** renders the ⚡ indicator. The goal: it should be easy not to lose the meaning of each step when the code changes.

**Languages:** [Русский](../PoE-detection.md) · English

---

## Terms

| Term | Meaning |
|------|---------|
| **PSE** | Power Sourcing Equipment — the switch/injector that **delivers** power over Ethernet. |
| **PD** | Powered Device — a device **consuming** PoE (camera, access point, etc.). |
| **ifIndex** | The numeric SNMP interface identifier; every PoE source is ultimately mapped to it. |
| **`poe_active` in the DB** | The `device_interfaces.poe_active` field: `true` / `false` / `NULL`. `NULL` means "could not determine" or "not updated in this cycle". |

---

## Overall flow (one device poll cycle)

1. SNMP: sysName / sysDescr.
2. IF-MIB: the port table.
3. **`WalkPoEDeliveringByIfIndex`** — SNMP PSE (RFC / Huawei / Cisco / Ubiquiti / SNR) → a map `ifIndex → true/false`.
4. If the map has **no** `true` and the fallback is enabled — **SSH CLI** (`SSH_POE_*` from **env**, not the device card SSH) for Ubiquiti/SNR.
5. If the map is still **empty** — **LLDP-PD** as a last resort (no uplink to a switch).
6. **`ClearPoEOnFiberPorts`** — on SFP/optics `poe_active=false` (server-side heuristic; the UI additionally hides ⚡ on fiber per the port catalogue).
7. Database write: only explicit keys from the map; otherwise the field is left untouched.

Implementation: `internal/poller/poller.go`, `internal/snmp/poe.go`, `internal/snmp/lldp_poe.go`, `internal/snmp/fiber_poe.go`, `internal/poecli/ubnt.go`.

**Important:** SSH for PoE uses only the env `SSH_POE_USER` / `SSH_POE_PASS` / …; SSH credentials in the device card are used for backups / config snapshots / port CLI, **not** for this fallback.

---

## Step 3. SNMP: order of sources

**`WalkPoEDeliveringByIfIndex`** merges several MIBs. When **merging** into the common map, mostly entries with the value **`true`** are kept — that reduces the risk of mass-writing "PoE off" where data is simply missing.

### 3.1. RFC 3621 — POWER-ETHERNET-MIB

- The standard PSE port table.
- **`pethPsePortDetectionStatus`** plus the binding to the physical interface tells that the port is in a **delivering** state (`deliveringPower` and numeric equivalents).
- **Why:** this is the "canonical" way to say "the PSE is feeding a PD right now", if the SNMP agent implements `1.3.6.1.2.1.105`.

### 3.2. Huawei — HUAWEI-POE-MIB

- Its own OIDs for PD consumption/class per port.
- **Why:** on some Huawei switches the standard RFC branch is empty while the vendor branch answers.

### 3.3. Cisco — CISCO-POWER-ETHERNET-EXT-MIB

- A "device detected" flag on a PSE port.
- **Why:** fills in the picture where Cisco exposes the extended MIB instead of RFC only.

### 3.4. Ubiquiti / Broadcom fastPath — `1.3.6.1.4.1.4413.1.1.15`

- Power and current per port (usually mW and mA).
- **Why:** on some EdgeSwitch firmware **this branch is absent** — then this step yields nothing and the fallbacks below matter.

### 3.5. When LLDP is considered

- If after steps **3.1–3.4** (and SNR) the map has **at least one key** (including `false`), **LLDP is not called** in this SNMP step.
- LLDP-PD is called in the **poller later**, only if the map is still **empty/nil** (MIBs are silent) **and** the SSH fallback did not return a port table.

**Why:** "the neighbour claims to be a PD" is not "the PSE is feeding it". If SSH worked, its `true`/`false` wins over LLDP.

### 3.6. Fallback — LLDP-EXT-DOT3-MIB

- Walk of **`lldpXdot3RemPowerPortClass`** (including legacy).
- If the neighbour claims class **PD**, the local port is mapped to its **`ifIndex`**.
- Written to `poe_active` **only as a last resort**, when there is no PSE data and no SSH table. On an EdgeSwitch without 4413 and without SSH this preserves the previous indication.
- **Uplink filter:** if the LLDP neighbour looks like another switch (name `Eltex`, `EdgeSwitch`, … or a remote port like `gi1/0/23`, `te`, `lag`), **`false`** is written for that `ifIndex` instead of `true` — so an uplink to a switch never shows ⚡.

### 3.7. Empty results and SNMP errors

- If **no** source marked any port and some walks failed, the function may return an **empty or "zero" map** without a mass `false`.
- **Why:** the poller must not **overwrite** `poe_active` with false "no PoE" on devices without a PoE MIB (the typical EdgeSwitch case).

---

## Step 4. SSH CLI fallback (optional)

Enabled **only if** all of these hold:

- the SSH PoE fallback flag is enabled in the config;
- an SSH user is set;
- after SNMP there is **no** `ifIndex` with **`true`**;
- **sysDescr** looks like **Ubiquiti / EdgeSwitch** or **SNR**.

SSH also writes **`false`** for the parsed CLI lines — to clear a stuck bolt when a camera was removed.

Then:

1. The **vendor profile** is selected by **sysDescr** (privileged-mode command: `en`, `enable`, etc.; the `show poe …` command set).
2. Ubiquiti EdgeSwitch usually needs a **second password** (enable); it is set by a separate env variable with a fallback to the main SSH password when the enable password is not set.
3. An interactive session runs **in a single SSH channel** (to avoid "cannot open a second channel" errors after leaving privileged mode).
4. Command output (e.g. **`show poe status all`**) is parsed: lines with a port name (`0/3` etc.) are mapped to **`ifIndex`** through the same interface table as SNMP. Activity is determined from **Detection** and the numeric columns (consumption, voltage, current).
5. If SSH closes the session **without an exit status** but **the output text is already there**, that is **not a fatal error** — the parser still processes the output (a quirk of some firmware).

Environment variables (see `.env.example`): `SSH_POE_FALLBACK_ENABLED`, `SSH_POE_USER`, `SSH_POE_PASS`, `SSH_POE_ENABLE_PASS`, `SSH_POE_KNOWN_HOSTS` (default `/var/lib/netlynx/ssh_known_hosts`), port, timeout.

Fallback errors are logged at **WARN** so they are visible in `journalctl`.

After a firmware upgrade on **switches** the SSH keys may change (`knownhosts: key mismatch`). A single script over **online** switches (`device_category=switch`, `last_snmp_ok` or `online_override=true`; offline devices are not probed): first modern algorithms, if the handshake fails — transitional, then legacy (Eltex OpenSSH 5.x). Port from `devices.ssh_port` or 22:

```bash
sudo /usr/local/sbin/refresh-ssh-known-hosts.sh
# or a single switch: sudo HOSTS=10.0.0.1 /usr/local/sbin/refresh-ssh-known-hosts.sh
```

The script's log is wherever you redirect stdout/stderr (e.g. `tee`); it does **not** create `/tmp/invetor-refresh-keys.log` by itself. The old known_hosts is kept as `ssh_known_hosts.bak.*`.

---

## Step 5. Writing to the database

For each port during an interface upsert:

- if the resulting PoE map is **not `nil`** and has a key for this `ifIndex` — **`poe_active`** is written with that value (`true` or `false`);
- if there is **no key**, or the whole PoE walk returned **`nil`** (MIBs are silent) — the field is **not touched** (`COALESCE` keeps the old value / `NULL`).

An explicit `false` is written when a source **listed the port** (RFC detection ≠ delivering, 0 mW, SSH "not delivering"). So the bolt goes out after a PD is removed, but is **not** erased because of a whole-walk timeout.

---

## Web UI: when ⚡ is shown

1. Not a fiber port (model catalogue + name heuristic) — otherwise no bolt.
2. If **`poe_active === true`** — ⚡.
3. Otherwise if **`poe_power_w > 0`** — ⚡.
4. Otherwise no.

The server-side PoE reset on SFP (`ClearPoEOnFiberPorts`) and the UI port catalogue may differ slightly in the name heuristic — see [Port-Model-Catalog.md](../Port-Model-Catalog.md).

---

## Short flowchart

```text
SNMP poll → IF-MIB
         → PoE MIB (RFC / Huawei / Cisco / 4413 / SNR)
         → if no true + SSH_POE_* env → SSH show poe
         → if the map is empty → LLDP DOT3 PD (last resort)
         → ClearPoEOnFiberPorts (SFP)
         → write explicit keys to the DB

UI: not fiber AND (poe_active || poe_power_w > 0)
```

---

## Maintenance

Update this file when the logic changes. A short version journal is in `README.md` (internal git only).

---

## Diagnostics: RFC 3621 (105) and EdgeSwitch

**No Such Object** on `1.3.6.1.2.1.105` is normal: **RFC 3621** is often not implemented by the agent. NetLynx then tries **Huawei**, **Cisco**, **Ubiquiti EdgeSwitch** (Broadcom **fastPath** `1.3.6.1.4.1.4413.1.1.15`). Manual check for EdgeSwitch:

```bash
snmpwalk -v2c -c public HOST 1.3.6.1.4.1.4413.1.1.15.1.1.1
```

**EdgeSwitch *-lite* (example: ES-16-150W, 1.9.3-lite):** the **EdgeSwitch-POWER-ETHERNET-MIB** declares `agentPethOutputPower` / `agentPethOutputCurrent`, but on some builds the agent answers **No Such Object** for the whole **1.1.15** branch — then **`poe_active` is not filled over SNMP** (stays `NULL`).

**EdgeSwitch without `105` and without `4413.1.1.15`:** the poller tries **LLDP-EXT-DOT3** — `lldpXdot3RemPowerPortClass`, value **2** = PD. This requires **LLDP** and the power TLV from the neighbour.

**SSH fallback:** if the map still has no `true` after SNMP, the poller tries CLI via the **env** `SSH_POE_*` (before LLDP-PD). LLDP is only the last resort when both MIBs and SSH gave nothing.
