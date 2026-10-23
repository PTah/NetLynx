# Manual topology links

When LLDP/CDP does not see a link yet, or **lies** (for example several VLANs/neighbours on one MikroTik SFP), you can define a **port ↔ port** connection manually. It appears on the **Topology** map and in the device card.

**Languages:** [Русский](../Manual-Topology-Links.md) · English

## Model

The `manual_topology_links` table (separate from `port_neighbors`):

- ends: `a_device_id`/`a_if_index` and `b_device_id`/`b_if_index` (both ports are required);
- the pair is stored in canonical order (`a_device_id < b_device_id`);
- `status`: `active` | `superseded`;
- on supersede: `superseded_at`, `superseded_by` (`lldp` / `cdp`) — manual or admin tooling only (never from the poller).

Manual rows are **not** written to `port_neighbors`: the poller marks unseen entries `stale` and cleans them up by TTL.

## Priority over LLDP/CDP

While a link is **`active`** it is **pinned**:

1. The poller does **not** move it to `superseded` when LLDP/CDP shows up.
2. On the topology map, LLDP/CDP edges from the local ports of both ends of a manual link are **hidden** — the manual link stays.

To unpin: delete the manual record (or mark it superseded through the API if needed).

## Graph building

`BuildTopologyGraph` adds `active` manual edges with `protocol: "manual"` and a `manual_link_id` field. The `GET /api/v1/topology?protocol=manual` filter is supported.

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/manual-links?device_id=&status=&limit=` | list (`status` empty or `all` — everything; otherwise filtered) |
| POST | `/api/v1/manual-links` | create |
| PATCH | `/api/v1/manual-links/{id}` | ports / note / restore |
| DELETE | `/api/v1/manual-links/{id}` | delete |

Create body: `{a_device_id, a_if_index, b_device_id, b_if_index, note?}`.

## UI

- **Topology**: "Link" mode, purple dashed line for pure-manual links, inspector (note / delete).
- **Device card**: for non-switch/router devices — **“Connected to…”** (auto LLDP/CDP/FDB to switch/port); below that, the “Manual links” section — table, add / edit / restore / delete.

See also [Autodiscover.md](../Autodiscover.md).
