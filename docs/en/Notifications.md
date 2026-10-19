# Notifications: Telegram, email, webhook

NetLynx writes **all** events to the journal (UI: "Journal"). Additionally they can be delivered to channels: **Telegram**, **email**, **webhook**. Setup: **Settings → Notifications** (operator role and above).

In short: event → filter by type and severity → channel → retry on failure.

**Languages:** [Русский](../Notifications.md) · English

## Channels

| Channel | What to set | Where to look on failure |
|---|---|---|
| **Telegram** | enable the channel, `bot_token` (create a bot via @BotFather), `chat_id` (the chat/group ID; the bot must be a member) | service journal: `journalctl -u NetLynx \| grep telegram` |
| **Email (SMTP)** | enable, `smtp_host`/`smtp_port` (587 by default), login/password, `from`, `to` (several, comma-separated); for a self-signed certificate — "skip TLS verification" | `journalctl -u NetLynx \| grep -i smtp`; the **"Test email"** button (`POST /api/v1/settings/notifications/email-test`) |
| **Webhook** | enable, URL (the outbound policy / SSRF protection is enforced), filters | `journalctl -u NetLynx \| grep webhook`; the receiver must answer 2xx |

**DEVICE_OFFLINE / DEVICE_ONLINE** emails are batched into a single message; other events are sent one by one.

Secrets (bot token, SMTP password) are stored encrypted in the database — see [Secrets.md](Secrets.md).

## Filters

Each channel has its own:

- **Event types** (`event_types`, comma-separated) — an **empty value means all types**.
- **Severity** (`severities`: `info`, `warning`, `error`) — empty means all.

Retries: `notify_max_retries` (2 by default) and `notify_retry_backoff_ms` (500 ms). After the attempts are exhausted the event stays in the journal and the error goes to the service journal (Warn).

## Event types

| Group | Types |
|---|---|
| Ports and connectivity | `LINK_UP`, `LINK_DOWN`, `PORT_FLAP`, `PORT_SPEED_DOWN`, `PORT_SPEED_OK`, `PORT_UTILIZATION_HIGH`, `PORT_UTILIZATION_OK` |
| Device | `DEVICE_OFFLINE`, `DEVICE_ONLINE`, `SERVICE_STARTED`, `CONFIG_EDIT`, `SNMP_TRAP` |
| MAC / L2 | `UNKNOWN_MAC_ON_ACCESS_PORT`, `MAC_MOVED`, `MAC_FLAPPING`, `MAC_MULTI_ACCESS`, `MAC_REMOVED`, `ACCESS_PORT_MAC_SUBSTITUTED`, `ACCESS_PORT_LONG_IDLE_DEVICE` |
| Loops and STP | `L2_LOOP_APPEARED`, `STP_TOPOLOGY_CHANGE`, `STP_ROOT_CHANGED`, `BROADCAST_STORM_SUSPECTED` |
| Topology | `MANUAL_LINK_SUPERSEDED` |
| MFPs | `TONER_LOW`, `TONER_CRITICAL` |
| UPS | `UPS_ON_BATTERY`, `UPS_MAINS_OK`, `UPS_ON_BATTERY_LONG`, `UPS_BATTERY_LOW`, `UPS_BATTERY_DEPLETED`, `UPS_BATTERY_OK`, `UPS_CHARGE_LOW`, `UPS_CHARGE_CRITICAL`, `UPS_CHARGE_OK`, `UPS_CHARGE_DROP`, `UPS_DISCHARGE_FAST`, `UPS_RUNTIME_LOW`, `UPS_TEMP_HIGH`, `UPS_TEMP_CRITICAL`, `UPS_ALARM`, `UPS_ALARM_CLEAR`, `UPS_LOAD_HIGH`, `UPS_LOAD_OK` |

UPS event semantics and thresholds — [UPS.md](UPS.md). Port and MAC events — [Polling-and-Topology-Cache.md](../Polling-and-Topology-Cache.md), [MAC-Investigation.md](../MAC-Investigation.md).

## Statistics (since 0.4.5)

**Settings → Notifications → Statistics** — a per-device table for a period: how many times a device went offline, or how many times a UPS lost mains.

- Periods: from **1 hour** to **365 days** (1–12/24/48/72 h, 7…365 d).
- API: `GET /api/v1/settings/notifications/stats?kind=reachability|ups&hours=N` (`hours` up to 8760; the legacy `days` is still accepted).

## If notifications do not arrive

1. Is the channel enabled? (`telegram_enabled` / `email_enabled` / `webhook_enabled`).
2. Does the type filter include the event type? An empty list means all, but `,UPS_ON_BATTERY,` with a typo does not.
3. Does the severity pass the filter (e.g. `UPS_MAINS_OK` is `info`)?
4. Did the event occur at all? Check the journal in the UI by type.
5. Telegram: the bot is a member of the chat, `chat_id` is the chat ID (not a username), the bot is not blocked.
6. Email: the SMTP port is not blocked by the provider; if "Test email" returns an error, read its text.
7. Service journal: `journalctl -u NetLynx -n 200 | grep -iE "telegram|smtp|email|webhook"`.

## Instant events from hardware (traps)

For switches and UPS units you can receive SNMP traps — then some events arrive immediately instead of on the next poll (see [UPS.md](UPS.md#traps-from-ups-cards) and `GET/PATCH /api/v1/settings/snmp-traps`). Receiver: UDP **9162**; forwarding the standard port 162 → 9162 is described in the Runbook.
