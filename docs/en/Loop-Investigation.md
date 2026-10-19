# Topology loops (blast cache / LLDP)

A separate report about **cycles in the neighbour graph** of the inventory. Do not confuse it with MAC flapping ([MAC-Investigation.md](../MAC-Investigation.md)): here we look for rings of **devices**.

Since **0.9.0** the DFS runs on the **topology blast cache** by default (the same skeleton as the VLAN blast: LLDP+CDP+manual). If the cache is empty it falls back to live LLDP.

Since **0.10.0** the same cache is the single skeleton for VLAN delete-impact (depth `VLAN_BLAST_MAX_DEPTH`, default **32**), MAC L2-path, loops, multi-hop shut-impact and `GET /api/v1/topology/path`.

**Languages:** [Русский](../Loop-Investigation.md) · English

## Quick path

1. Menu **Loops** (`/investigate/loops`).
2. Press **Refresh** — DFS over the blast cache (or live LLDP).
3. Inspect the list of cycles (length, device names, port→port hops).

Parallel uplinks (two links between the same pair) are reported too.

## API

```bash
curl -sS -H "Authorization: *** $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/loops"
```

| Parameter | Value |
|-----------|-------|
| `protocol` | empty / `lldp` — prefer the blast cache; `live` / `cdp` — live graph without the cache |

Role: **viewer+**.

Response: `source` = `blast_cache` \| `live`, `protocol`, `cycles[]` (with `cycle_key`).

## Events (0.9.0)

| Type | When |
|------|------|
| `L2_LOOP_APPEARED` | The periodic loop-watch (~15 min) saw a **new** cycle key (a stable ring does not spam) |
| `PORT_FLAP` | N LINK_UP/DOWN on a port within the window (see `PORT_FLAP_*` in `.env.example`) |

In the MAC report, loop hypotheses add `loops_touching` plus a `likely_cause` line (`l2_loop` / `port_flap_cable` / `stp_reconvergence` / …).

## Expected result

- `cycles[]` — the rings found, with `hops` (from/to device + ifIndex/ifName).
- `node_count` / `edge_count` — graph size.
- An empty list means no loop is visible (too few neighbours, a star, or an empty cache).

## False loops (offline / zombie LLDP)

Since **0.12.26** the loop graph **excludes edges with an offline device** (the same online criterion as in "Devices"). An offline switch is not polled, so its `port_neighbors` used to stay `stale=false` forever and closed a phantom triangle.

Additionally, during a blast-cache rebuild and before loop-watch, a **global age-out** removes neighbours older than `NeighborStaleTTL` (2 h).

| Symptom | Cause | What was done |
|---------|-------|---------------|
| A cycle through a powered-off switch | zombie LLDP of an offline device | edges with offline devices are dropped |
| One port in two hops of a cycle | one edge is stale, the other is live | age-out + offline filter |

## Empty result but "the loop is there"

| Symptom | What to check |
|---------|----------------|
| The loop goes through an unmanaged switch | LLDP will not see it — see [MAC-Investigation.md](../MAC-Investigation.md) |
| No neighbours on the map | LLDP on ports; [Autodiscover.md](../Autodiscover.md); rebuild the blast cache |
| Need a timeline | [Postmortem.md](../Postmortem.md) around the incident |
| A false loop with an offline device | upgrade to 0.12.26+; check "Devices" online state |
