# Printer-MIB: pages and toner (MFPs / printers)

Kyocera and Sharp (and most networked MFPs) expose data through the **IETF Printer-MIB, RFC 3805** (`1.3.6.1.2.1.43`). Vendor branches (Kyocera `1347`, Sharp `2385`) are a fallback only, when the standard says nothing.

NetLynx polls Printer-MIB for devices of type **MFP** (`mfu`; aliases `принтер` / `printer`) and when `sysDescr` looks like printing hardware.

**Languages:** [Русский](../Printer-MIB.md) · English

## Total pages

Primary OID: **`1.3.6.1.2.1.43.10.2.1.4`** (`prtMarkerLifeCount`).
Typical instance: `1.3.6.1.2.1.43.10.2.1.4.1.1`. The maximum non-negative value from the walk is used (several markers).

## Toner level

Table **`prtMarkerSuppliesTable`** (`1.3.6.1.2.1.43.11.1`):

| Field | OID |
|------|-----|
| Colorant index | `…43.11.1.1.3` |
| Type | `…43.11.1.1.5` (3 = toner, 21 = tonerCartridge; 4 = waste — skipped) |
| Description | `…43.11.1.1.6` |
| Unit | `…43.11.1.1.7` (`18` = percent per RFC 3805) |
| Max | `…43.11.1.1.8` |
| Current | `…43.11.1.1.9` |

Colour is **not** hard-wired to indices 1–4. `prtMarkerColorantValue` (`1.3.6.1.2.1.43.12.1.1.4`) is used first, otherwise Black/Cyan/Magenta/Yellow substrings in the description. The CMYK order differs between devices (sometimes waste sits at index 1).

### Percentage

- Unit = percent → take current (if max ≠ 100 — `current/max*100`).
- Otherwise **`% = current / max * 100`**.
- Current **−2** — unknown; **−3** — "some left", no exact number.

Kyocera, if IETF gives no percentage: `1.3.6.1.4.1.1347.40.10.1.1.3.{1–4}` (already 0–100).

## History in the database

Every successful poll writes to `metric_samples`:

| metric_type | meaning |
|-------------|---------|
| `page_count` | `prtMarkerLifeCount` |
| `toner_black_pct` / `_cyan_` / `_magenta_` / `_yellow_` | remaining % |

Toner/pages retention is **separate** from util/CPU: `METRICS_PRINTER_RETENTION_DAYS` (**365** by default). The general `METRICS_RETENTION_DAYS` (often 3–7) only trims util/rate/CPU, not MFP history.

API: `GET /api/v1/devices/{id}/metrics?metric_type=…&from=&to=` (RFC3339), optionally `max_points` (downsample).

## UI

- **Devices → List** and **Dashboard:** "Total pages" and "Toner left" columns appear only when the type filter contains **MFPs only** (on the dashboard it replaces CPU). A colour MFP shows all cartridges.
- **Device card:** pages and toner are charts with a period selector (1 / 7 / 30 / 90 / 365 days). CPU is not polled for MFPs.
- When the toner level drops to **≤5%** — event `TONER_LOW` (Telegram/email per the notification filter); at **≤2%** — `TONER_CRITICAL` and the device row blinks red on the dashboard and in the device list.

## Alerts

The `TONER_LOW` / `TONER_CRITICAL` types are added to the Telegram/email/webhook whitelist by migration (if a whitelist is already set). A repeat alert happens only on a new threshold crossing (after recovering above the threshold and dropping again).
