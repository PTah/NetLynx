# Ignore list — disabling reactions on a port

## Purpose

For a **device + ifIndex** pair, define rules under which NetLynx **does not react** to events as usual: no notifications, no incident actions, optionally no row in `events`.

Typical cases:

- An uplink / trunk with legitimate MAC changes.
- A port with known "noisy" hardware.
- Temporary maintenance (a note in `comment`).

**Languages:** [Русский](../Ignore-List.md) · English

## Model (`port_event_ignore`)

| Field | Default | Meaning |
|-------|---------|---------|
| `event_types` | NULL / `*` | Comma-separated types; empty = all types |
| `block_events` | false | Do not write to the event journal |
| `block_notify` | true | No webhook / Telegram / email |
| `block_actions` | true | No SNMP admin-down and other actions |
| `comment` | — | A note for the admin |

## API

- `GET /api/v1/devices/{id}/port-ignores` — list the rules.
- `PUT /api/v1/devices/{id}/interfaces/{ifIndex}/ignore` — create/update.
- `DELETE /api/v1/devices/{id}/interfaces/{ifIndex}/ignore` — remove.

### UI modes (the button cycles)

| mode | Button | Behaviour |
|------|--------|-----------|
| `off` | white **Mon.** | the rule is deleted |
| `soft` | yellow **Quiet** | no notify/actions for link, util, speed, `MAC_REMOVED`, `ACCESS_PORT_LONG_IDLE_DEVICE`, `ACCESS_PORT_MAC_SUBSTITUTED`; **UNKNOWN_MAC**, **MAC_MOVED**, **MAC_FLAPPING**, **MAC_MULTI_ACCESS** — as usual; events still go to the journal |
| `all` | red **Off** | `block_events` + notify + actions for the listed types; **exception:** some emits (`MAC_MULTI_ACCESS`, syslog `MAC_FLAPPING`) are sent with `ignore=nil` and do **not** consult port-ignore |

`PUT` body with a preset:

```json
{ "mode": "soft" }
```

`PUT` body manually (as before):

```json
{
  "event_types": "UNKNOWN_MAC_ON_ACCESS_PORT,MAC_MOVED",
  "block_events": false,
  "block_notify": true,
  "block_actions": true,
  "comment": "Uplink to the core"
}
```

The interface detail contains `ignore_mode` — `off` | `soft` | `all`.

## Poller behaviour

On `emit()`:

1. If a rule exists for `if_index` and the event type matches `event_types`:
   - `block_events` → the event is not created.
   - otherwise the event is written to the DB, but without notify/actions according to the flags.

## UI

On the device card, in the ports table, the **Mon.** column cycles:

| Button | mode | When to use |
|--------|------|-------------|
| **Mon.** | `off` | normal monitoring |
| **Quiet** (yellow) | `soft` | no Telegram for link/util/speed/idle/substituted; UNKNOWN_MAC / MAC_MOVED / FLAPPING / MULTI_ACCESS stay |
| **Off** (red) | `all` | almost full ignore of the port; see the syslog/MULTI_ACCESS exception above |

Port search (description / MAC / IP): `GET /api/v1/ports/search?q=...` (the "Devices" section and search on "Topology").
MAC/IP: first ports with **LLDP** pointing at that address (a physical neighbour), then **FDB** entries (a MAC may be visible on the uplinks of the whole VLAN — that does not mean "connected to every switch").

In the API detail: `port_ignores[]`; the interface has an `event_ignored` flag.

## Relation to item 6 (incidents)

The ignore list is a **mandatory safety net** before widening auto-actions: first put ignore on uplinks, then enable `incident_action_enabled` on access ports only.
