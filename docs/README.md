# Documentation

NetLynx documentation. **Russian is the primary language**; English versions live in [`en/`](en/) and are being added in batches — the table below shows what is available right now.

Основная документация — на русском. Английские версии — в каталоге [`en/`](en/), добавляются партиями (см. таблицу).

| Document | RU | EN | What it is about |
|---|:--:|:--:|---|
| [Runbook-Linux.md](Runbook-Linux.md) | ✅ | ✅ [en](en/Runbook-Linux.md) | installing on Ubuntu/Debian, upgrade, rollback, troubleshooting, manual path |
| [API.md](API.md) | ✅ | ✅ [en](en/API.md) | every HTTP endpoint, roles, destructive confirms |
| [Roles.md](Roles.md) | ✅ | ✅ [en](en/Roles.md) | viewer / operator / admin |
| [SNMP-Community.md](SNMP-Community.md) | ✅ | ✅ [en](en/SNMP-Community.md) | RO vs RW community, traps, minimal rights |
| [UPS.md](UPS.md) | ✅ | ✅ [en](en/UPS.md) | UPS-MIB polling, OID map, alerts, card traps |
| [VLAN.md](VLAN.md) | ✅ | ✅ [en](en/VLAN.md) | VLAN database, access/trunk/native/allowed, delete impact |
| [Notifications.md](Notifications.md) | ✅ | ✅ [en](en/Notifications.md) | Telegram/email/webhook, filters, event types, statistics |
| [Autodiscover.md](Autodiscover.md) | ✅ | ⏳ | discovery: LLDP/CDP, SNMP scan, promotion to inventory |
| [Polling-and-Topology-Cache.md](Polling-and-Topology-Cache.md) | ✅ | ⏳ | light/heavy polling, priorities, blast cache |
| [Vendors.md](Vendors.md) | ✅ | ⏳ | supported vendors and their quirks |
| [PoE-detection.md](PoE-detection.md) | ✅ | ⏳ | PoE detection, watts, reset, SSH fallback |
| [Printer-MIB.md](Printer-MIB.md) | ✅ | ✅ [en](en/Printer-MIB.md) | MFPs: pages, toner, alerts |
| [Config-History.md](Config-History.md) | ✅ | ⏳ | `show run` snapshots, diffs, backups |
| [MAC-Investigation.md](MAC-Investigation.md) | ✅ | ⏳ | where a MAC lives, flapping |
| [Loop-Investigation.md](Loop-Investigation.md) | ✅ | ✅ [en](en/Loop-Investigation.md) | LLDP/CDP loops |
| [Manual-Topology-Links.md](Manual-Topology-Links.md) | ✅ | ✅ [en](en/Manual-Topology-Links.md) | manual links (SFP, VLAN cuts) |
| [Ignore-List.md](Ignore-List.md) | ✅ | ⏳ | ignoring ports and events |
| [Secrets.md](Secrets.md) | ✅ | ✅ [en](en/Secrets.md) | secrets at rest (`NETLYNX_SECRETS_KEY`) |
| [Windows-Server-Setup.md](Windows-Server-Setup.md) | ✅ | ⏳ | Windows Server installation |
| [Port-Model-Catalog.md](Port-Model-Catalog.md) | ✅ | ⏳ | port model catalogue |
| [ELTEX-Aging.md](ELTEX-Aging.md) | ✅ | ⏳ | Eltex aging specifics |
| [Postmortem.md](Postmortem.md) | ✅ | ⏳ | incident timeline |
| [Incident-Actions-Plan.md](Incident-Actions-Plan.md) | ✅ | ⏳ | automatic incident actions |
| [Migrate-Prod-Invetor-to-NetLynx.md](Migrate-Prod-Invetor-to-NetLynx.md) | ✅ | ⏳ | migrating from the legacy Invetor install |
| [Monday-Readiness.md](Monday-Readiness.md) | ✅ | ⏳ | production readiness checklist |
| [Roadmap.md](Roadmap.md) | ✅ | ⏳ | product roadmap |
| [To-Do.md](To-Do.md) | ✅ | ⏳ | backlog |
| [TZ-snmp-switch-monitor.md](TZ-snmp-switch-monitor.md) | ✅ | ⏳ | technical specification |
| [Changelog.md](Changelog.md) | ✅ | ⏳ | full version history (RU only by design) |
| [MIBs/OIDs-2026-04-15.md](MIBs/OIDs-2026-04-15.md) | ✅ | ⏳ | vendor OID notes |

Legend: ✅ available · ⏳ pending · Documents marked RU-only are internal history/specs.

## Where to start

- **Installing:** [Runbook-Linux.md](Runbook-Linux.md) (or the English [en/Runbook-Linux.md](en/Runbook-Linux.md)).
- **First run:** README → "Первый запуск после установки" / README.en.md → "First run after installation".
- **Integrating:** [API.md](API.md).
- **Everyday operations:** [Notifications.md](Notifications.md), [VLAN.md](VLAN.md), [UPS.md](UPS.md).
