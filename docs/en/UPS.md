# UPS over SNMP

NetLynx polls UPS units via the standard **UPS-MIB (RFC 1628, `1.3.6.1.2.1.33`)** — which is what most network cards implement: APC/Schneider, Ippon (EPPC), Eaton, Powercom, CyberPower, Winpower-compatible NMCs.

Verified on: **Ippon RT II 6000** with the `Network Management Card for UPS` card (`sysObjectID = 1.3.6.1.4.1.935.10.1`), address `10.0.0.1`, community `public`.

**Languages:** [Русский](../UPS.md) · English

## Quick SNMP check

```bash
# is it answering?
snmpget -v2c -c public 10.0.0.1 1.3.6.1.2.1.1.1.0 1.3.6.1.2.1.33.1.1.5.0
# model / status / charge / runtime
snmpget -v2c -c public 10.0.0.1 \
  1.3.6.1.2.1.33.1.1.2.0 1.3.6.1.2.1.33.1.2.1.0 \
  1.3.6.1.2.1.33.1.2.4.0 1.3.6.1.2.1.33.1.2.3.0
```

From Windows without `snmpget` — use the probe from the repository:

```bash
UPS_TARGET=10.0.0.1 go test ./internal/snmp/ -run TestReadUPSLive -v
```

If `UPS_TARGET` is not set, the test is skipped (live devices are not touched by default).

## Adding a device

1. **Devices → Add**: name (e.g. `UPS-Server-Room`), UPS IP, type — **UPS** (`ups`).
2. SNMP: v2c + community (on Ippon/APC cards usually `public`), or v3.
3. Poll interval: 60 s is enough, the card answers a GET batch in ~50 ms.

Polling switches on automatically when the device type is `ups` **or** `sysDescr` looks like a UPS (`ups`, `smart-ups`, `winpower`, `ippon`, `eaton`, `powercom`, `network management card for ups`). For UPS units NetLynx does **not** touch IF-MIB/FDB/LLDP/PoE — it reads UPS-MIB only.

## OID map and decoding rules

Scalar values are read in **one** SNMP GET batch (the card is slow: a full walk of group 33 takes about 18 seconds).

| OID | NetLynx field | Rule |
|---|---|---|
| `1.3.6.1.2.1.1.5.0` | sysName | string |
| `33.1.1.1.0` | manufacturer | string (`EPPC`) |
| `33.1.1.2.0` | model | string (`ON-LINE`) |
| `33.1.1.4.0` | agent_software | card agent version string |
| `33.1.1.5.0` | ident_name | UPS name (`Ippon RT II 6000`) |
| `33.1.2.1.0` | battery_status | 1 unknown, 2 normal, 3 low, 4 depleted |
| `33.1.2.2.0` | seconds_on_battery | integer, seconds |
| `33.1.2.3.0` | runtime_min | integer, minutes |
| `33.1.2.4.0` | battery_charge_pct | integer, % |
| `33.1.2.5.0` | battery_voltage_v | raw / 10 (2187 → 218.7 V) |
| `33.1.2.7.0` | battery_temp_c | integer, °C |
| `33.1.3.2.0` | input_lines | number of input lines |
| `33.1.3.3.1.2.1` | input_frequency_hz | raw / 10 (499 → 49.9 Hz) |
| `33.1.3.3.1.3.1` | input_voltage_v | integer, V (238) |
| `33.1.3.3.1.4.1` | input_current_a | raw / 10; `-1` = not supported |
| `33.1.4.1.0` | output_source | 3 normal, 5 battery, 4 bypass, 6 booster, 7 reducer |
| `33.1.4.2.0` | output_frequency_hz | raw / 10 |
| `33.1.4.4.1.2.1` | output_voltage_v | integer, V (220) |
| `33.1.4.4.1.3.1` | output_current_a | raw / 10 (56 → 5.6 A) |
| `33.1.4.4.1.4.1` | output_power_w | integer, W (1100) |
| `33.1.4.4.1.5.1` | output_load_pct | integer, % (20) |
| `33.1.6.1.0` | alarms_present | number of active alarms |
| `33.1.9.1.0` | config_input_voltage_v | nominal input, V |
| `33.1.9.7.0` | low_battery_runtime_min | "low battery" threshold, minutes |

Not supported on the Ippon RT II 6000: bypass (`33.1.5`), `upsTest` (`33.1.7`), shutdown delays (`33.1.8`). Such fields come back as `-1`/`noSuchObject` and are not stored in the snapshot.

**Running on battery** is detected as `upsSecondsOnBattery > 0` **or** `upsOutputSource == 5`.

## What is stored in the database

| Where | What |
|---|---|
| `devices.last_ups` (JSONB) | current snapshot of every field in the table above |
| `devices.last_ups_at` | when the snapshot was updated |
| `metric_samples` (`ups_*`) | history for charts, one sample every 5 minutes, kept up to a year |

History metrics: `ups_battery_charge_pct`, `ups_runtime_min`, `ups_battery_voltage_v`, `ups_battery_temp_c`, `ups_input_voltage_v`, `ups_output_voltage_v`, `ups_output_load_pct`, `ups_output_w`, `ups_on_battery` (0/1).

Charts: device card → **State** → metric chips (a click opens the chart), period 1/7/30/90/365 days.

## Events and alerts

| Event type | Severity | When |
|---|---|---|
| `UPS_ON_BATTERY` | error | switched to battery (mains lost) |
| `UPS_MAINS_OK` | info | mains is back (output is `normal` again) |
| `UPS_BATTERY_LOW` | warning | `upsBatteryStatus = 3` |
| `UPS_BATTERY_DEPLETED` | error | `upsBatteryStatus = 4` |
| `UPS_BATTERY_OK` | info | battery status back to `normal` |
| `UPS_TEMP_HIGH` | warning | battery temperature ≥ 45 °C |
| `UPS_TEMP_CRITICAL` | error | battery temperature ≥ 55 °C |
| `UPS_ALARM` | warning | `upsAlarmsPresent` rose from 0 |
| `UPS_ALARM_CLEAR` | info | alarms cleared (`upsAlarmsPresent = 0`) |
| `UPS_LOAD_HIGH` | warning | output load ≥ 90 % |
| `UPS_LOAD_OK` | info | load dropped below 80 % |
| `UPS_CHARGE_LOW` | warning | battery charge ≤ 50 % (clears above 60 %) |
| `UPS_CHARGE_CRITICAL` | error | battery charge ≤ 20 % |
| `UPS_CHARGE_OK` | info | charge back above 60 % |
| `UPS_CHARGE_DROP` | warning | charge falls ≥ 10 % between polls while the UPS is **on mains** (battery not charging) |
| `UPS_DISCHARGE_FAST` | warning | discharge ≥ 2 %/min with a ≥ 3 % drop per poll (on battery); 10-minute anti-flap |
| `UPS_RUNTIME_LOW` | warning | estimated runtime ≤ 10 min while on battery |
| `UPS_ON_BATTERY_LONG` | error | mains did not return: on battery for more than 10 minutes |

Thresholds: battery temperature 45/55 °C, load 90 % (clear at 80 %), charge 50/20 % (clear at 60 %), discharge 2 %/min.

Charge events are compared with the previous poll, so there are no "same state" repeats: `UPS_CHARGE_LOW` fires once when a threshold is crossed, not every minute.

Events appear in the journal and in Telegram/email/webhook when their types are enabled in notification settings (UPS types are added to the Telegram/email/webhook lists by migration `077_ups.sql`).

The first poll after adding a device produces no alerts — it only establishes the baseline (same as MFP toner).

## Traps from UPS cards

Polling alerts arrive with a delay of at most one poll interval (60 s). To get an instant alarm, the card can send SNMP traps to NetLynx.

The NetLynx side is already prepared:

| What | Value |
|---|---|
| Trap reception | Settings → Notifications → "Accept traps", port **9162** (`netlynxd` listens on UDP on all interfaces) |
| Standard port forwarding | on the server `iptables -t nat`: `REDIRECT --dport 162 --to-ports 9162` — the card may target the standard **162** |
| Community | `SNMP_TRAP_COMMUNITY` is not set → traps with any community are accepted |
| Log filter | `trap_include_labels`; migration 078 adds `upsTrapEppc`, `upsTrapApc` (UPS alarms show up in "Trap logs") |

Card configuration (EPPC/Ippon, APC-compatible NMC). The receiver lives in the private table `1.3.6.1.4.1.935.10.1.1.9` — slots 1…8:

| OID | Meaning |
|---|---|
| `…9.1.1.2.N` | NMS receiver address (string, `10.0.0.1`) |
| `…9.1.1.3.N` | trap community (`public`) |
| `…9.1.1.4.N` | version/type (2 — same as the working slot) |
| `…9.1.1.5.N` | extra slot flag |

Writing is possible over SNMP **SET** (community `public` has write rights on the card we tested). It is usually simpler to set it in the card's web UI: *SNMP → Trap receivers / NMS list* → add `10.0.0.1`, community `public`, port `162`.

Check: press "Send test trap" in the card's web UI (or wait for a real event) and look at **Trap logs** in the UI — the UPS alarm appears with the `upsTrapEppc` / `upsTrapApc` label.

## Winpower Manager

Neither the card's web UI nor Winpower Manager on a separate PC is **required** — NetLynx reads the same data directly over SNMP. Winpower can be removed.

## Limitations

- Polling is SNMP-based; if the UPS card only exposes a web UI without SNMP (`161/udp` closed), set the device's reachability mode to "Auto (Ping)" in the device card — then only ICMP is polled, without UPS-MIB.
- A full `walk` of group `33` is slow (≈18 s) — NetLynx issues one GET batch of a fixed OID set instead of a walk.
- Numeric fields marked `-1` by the vendor are not stored in the JSON (shown as `—` in the UI).
