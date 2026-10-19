# SNMP community in NetLynx

How to set up polling and switch management over SNMPv1/v2c (and briefly v3).

## Why

NetLynx polls devices over SNMP (interfaces, LLDP/CDP, FDB, PoE, CPU; for MFPs — Printer-MIB: pages and toner).
Some operations **write** to the device via SNMP SET:

- port admin up/down;
- port description (`ifAlias`);
- automatic port lockdown on incidents (if enabled in Settings).

For **reading**, a **read-only (RO)** community is enough.
For **writing**, the switch needs a **read-write (RW)** community — and the same value must be set in the NetLynx device card.

> The device card has a single **Community** field, used for both GET and SET. There is no separate "RW community" field: if you need SET, put the **RW** community there (or use SNMP v3 with write rights).

More on automatic actions: [Incident-Actions-Plan.md](../Incident-Actions-Plan.md).
For some vendors (Ubiquiti / ELTEX / SNR) port control can go over **SSH** — then an RW community is not required for those operations, but an SNMP community (at least RO) is still needed for polling.

**Languages:** [Русский](../SNMP-Community.md) · English

## On the switch

1. Enable SNMP (agent / SNMP server).
2. Create the community:
   - **RO** — monitoring only (safer for "look, don't touch").
   - **RW** — if you plan shutdown / port description / incident admin-down from NetLynx.
3. Restrict access with an ACL / source IP of the NetLynx server (recommended).
4. Do not use `public`/`private` in production without an ACL.
5. For **discovery** and LLDP-based topology, enable **LLDP** on the same switches (on ports; on Cisco possibly CDP too). NetLynx does not enable SNMP/LLDP on hardware — see [Autodiscover.md](../Autodiscover.md).

Examples of intent (syntax depends on the vendor):

| Goal | On the switch | In NetLynx |
|------|---------------|------------|
| Monitoring only | community `netlynx-ro` RO | Community = `netlynx-ro` |
| Monitoring + port control over SNMP | community `netlynx-rw` RW | Community = `netlynx-rw` |
| SNMPv3 | user with auth/priv; write views for SET | Version v3 + user/auth/priv in the card |

## In NetLynx

1. **Devices** → device card (or the add form / "Discovered" → Add).
2. SNMP: **v1** / **v2c** / **v3**.
3. For v1/v2c: the **Community** field = the same name as on the switch (RO or RW — see the table above).
4. Save, wait for a poll (or press "Test SNMP").
5. If SET fails, logs/errors usually say "community without write?" / "write community or SSH needed" — check RW on the switch or configure SSH in the device card (where supported).

The trap community (receiving SNMP traps on the server) is a **different** value, kept in the env (`SNMP_TRAP_COMMUNITY`), not the device community.
Enabling reception and the UDP port: **Settings → Notifications → "Accept traps"** (default port **9162**; stored in the DB + hot reload). The `SNMP_TRAP_LISTEN_ADDR` env does not control the listener. If the port is > 1000, DNAT **162 → that port** if needed.

**Trap logs (test):** the tab is active only when reception is enabled. While "Keep trap logs" is on, **non-link** traps are **not** duplicated into the general journal as `SNMP_TRAP`. For **LINK** traps the `link_trap_events_mode` (`off` / `per_device` / `all`) decides whether `LINK_UP`/`LINK_DOWN` are written straight from the trap; the poller can later set `trap_confirmed`. With `off` — trap journal only, LINK events come from polling.

## Minimal rights (recommendation)

- Fleet monitoring: RO community + ACL.
- Managing selected devices: RW only on the switches that need it, or an SSH account with a restricted command set.
- Incident auto admin-down: only when explicitly enabled in Settings; start with dry-run; SNMP SET requires RW.

## Related documents

- [Runbook-Linux.md](Runbook-Linux.md) — deploying the server
- [Autodiscover.md](../Autodiscover.md) — LLDP/CDP, "Discovered", switch requirements
- [Vendors.md](../Vendors.md) — vendor support
- [Roles.md](Roles.md) — who may change devices and enable actions
- [PoE-detection.md](../PoE-detection.md) — PoE polling
