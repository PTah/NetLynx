# Производители коммутаторов (Vendors)

План поддержки SNMP/SSH в **NetLynx** для разных брендов.

Камеры (Dahua / Hikvision / Trassir и др.) уже могут появляться в FDB с подписью OUI; ниже речь про **управляемые коммутаторы** тех же и других вендоров (у Dahua, Hikvision, HiWatch, Trassir есть L2/L3 managed switches — их тоже нужно поддерживать как свитчи, не только как «камеры за портом»).

## Уровни поддержки

| Уровень | Что умеет NetLynx |
|---------|-------------------|
| **L0 — generic** | IF-MIB (линк, скорость, util), часто LLDP/FDB/ARP при стандартном агенте. Без спец. CPU/PoE/CLI. |
| **L1 — poll+** | + CPU-профиль, PoE MIB/SSH-read, стабильный LLDP/CDP |
| **L2 — control** | + SNMP SET (admin/ifAlias) и/или SSH CLI: shutdown, description, PoE, isolate |
| **L3 — ops** | + бэкап running-config по SSH, вендорные нюансы (пейджер, enable, XP/BusyBox и т.п.) |

Сейчас (ориентир **0.6.0**, таблица уровней без изменений смысла). **W3** (Keenetic/Cudy/…) — не начата, **срока нет** (после polish W1/W2 и живого железа):

| Вендор | Уровень | Заметки |
|--------|---------|---------|
| Ubiquiti EdgeSwitch | **L2–L3** | Основной фокус; XP — ограничения CLI |
| ELTEX (MES) | **L2–L3** | CLI + aging docs |
| SNR / NAG | **L2–L3** | Private PoE MIB + CLI |
| Cisco | **L2** (W1) | CPU; SSH IOS-like; бэкап show run |
| Huawei | **L2** (W1) | CPU; VRP system-view; display current-configuration |
| MikroTik | **L2** (W1) | CPU; SSH disable/comment/PoE; `/export` |
| Aruba | **L2** (W1) | CPU; SSH AOS-like; PoE; isolate — позже |
| Zyxel | **L2** (W1) | CPU; SSH Cisco-like; poe mode; isolate — позже |
| HP ProCurve | **L2** (W2) | Отдельно от Aruba; CPU; SSH; PoE power-over-ethernet |
| TP-Link JetStream | **L2** (W2) | CPU; SSH Cisco-like; power inline supply |
| D-Link DGS/DES | **L2** (W2) | CPU; SSH; power inline |
| Dahua / Hikvision / HiWatch / Trassir **switches** | **L2** (W2) | Детект switch≠camera; SSH/PoE best-effort; бэкап show run |
| Juniper | **L0–L1** | CPU-профиль |
| Остальные из списка ниже | **L0** или нет | Нужны профили |

## Целевой список

| Производитель | Приоритет волны | Комментарий |
|---------------|-----------------|-------------|
| **Cisco** | W1 | Enterprise; IF-MIB SET часто уже ок; нужен `swcfg` + PoE polish |
| **Huawei** | W1 | PoE есть; CPU + CLI port |
| **MikroTik** | W1 | RouterOS switch/CRS/CCR; CLI/API или SSH |
| **Aruba** (HPE) | W1 | Частый campus; LLDP/PoE |
| **Zyxel** | W1 | SMB/campus |
| **HP / HPE ProCurve** | W2 | Родственно Aruba; отдельные MIB |
| **TP-Link** | W2 | Easy Smart / JetStream; см. `docs/MIBs/` |
| **D-Link** | W2 | DGS/DES |
| **Dahua** (switches) | W2 | Управляемые свитчи (не только камеры) |
| **Hikvision** / **HiWatch** (switches) | W2 | Managed switch линейки |
| **Trassir** (switches) | W2 | Управляемые свитчи под видеонаблюдение |
| **Keenetic** | W3 | Часто SOHO; SNMP ограничен |
| **Cudy** | W3 | |
| **Tenda** | W3 | |
| **Origo** | W3 | |
| **ExeGate** | W3 | |
| **ZyXEL** | см. Zyxel W1 | |

Ubiquiti / ELTEX / SNR — уже в продукте; доработки по мере багов, не «с нуля».

### VLAN database (создание / имя / удаление) — CLI по вендору

Запись через SSH (`POST …/vlans`, `PATCH/DELETE …/vlans/{id}`). Порядок попыток зависит от детекта вендора; при `Invalid input` — следующий стиль.

| Вендор | Основной CLI (по докам) | Запасной |
|--------|-------------------------|----------|
| **Ubiquiti EdgeSwitch** | `# vlan database` → `vlan N` / `vlan name N "…"`; `no vlan 167,30,31` | vlan database из `(Config)#` |
| **ELTEX MES23xx/33xx** | `(config)# vlan database` → `vlan N` / `vlan N name …` | IOS `vlan N` / `name`; Fastpath |
| **ELTEX MES14xx/24xx** | `(config)# vlan N` → `name …` | eltex-db / Fastpath |
| **SNR** | `(config)# vlan N` → `name …` | Fastpath `#` / config; eltex-db |
| **Cisco / Aruba / Zyxel / HP / TP-Link / D-Link / video-LAN** | `(config)# vlan N` → `name …` / `no vlan N` | eltex-db; Fastpath |
| **Huawei VRP** | `system-view` → `vlan N` → `name` / `undo name`; `undo vlan N` | — |
| **MikroTik** | **next** (RouterOS `/interface bridge vlan`, не IOS vlan database) | — |

Имена с дефисами/пробелами уходят в кавычки. VLAN 1 удалить нельзя. Живые модели в офисе не покрывают весь список — сверка по CLI guides вендоров + unit-тесты генерации команд.

### Trunk allowed VLAN на порту

`PATCH …/interfaces/{ifIndex}/vlan` с `op=trunk_allow`, `allowed_mode=add|remove|all|except`, `allowed_vlans` (список `10,20-22` или SNR `10;20`).

| Вендор / стиль | CLI |
|----------------|-----|
| **Cisco / EdgeSwitch / SNR / ELTEX MES** | `switchport mode trunk` + `switchport trunk allowed vlan add\|remove\|all\|except …` |
| **ELTEX** | обычно `add` / `remove` / `all`; `except` может дать Invalid input → fallback |
| **SNR** | как Cisco; списки часто через `;` — NetLynx нормализует в `,` |
| **Fastpath IEEE (нет allowed list)** | `vlan participation include` + `vlan tagging` по ID; `all`/`except` на этом стиле не эмитятся |

В UI при `add` без VLAN 1 (и при `remove`/`except`, затрагивающих управление) — предупреждение о риске потери SSH/Web (native/VLAN 1).

## Волны работ

### W1 — enterprise / ядро сети

**Сделано (0.35.29–0.35.30):** MikroTik → Cisco → Aruba → Zyxel → Huawei — детект, SSH port CLI (admin/description/PoE где применимо), бэкап running/export, CPU-профили. Нужна SSH-учётка в карточке узла; для SNMP SET по-прежнему RW community ([SNMP-Community.md](SNMP-Community.md)).

Дальше по W1 (polish):
1. Парсинг PoE SSH-print → ifIndex (MikroTik/Aruba/Zyxel).
2. Isolate для Aruba/Zyxel; нюансы AOS-CX vs ProVision; Huawei model quirks.
3. Тесты на живых устройствах / сниффеты sysDescr.

### W2 — SMB + video-LAN switches

**Сделано (0.35.34):** HP ProCurve (отдельно от Aruba) → TP-Link JetStream → D-Link → свитчи Dahua / Hikvision / HiWatch / Trassir — детект (камеры по DS-2CD/IPC не путаем со свитчами DS-3E/PFS), SSH port CLI (admin/description/PoE), бэкап `show running-config`, CPU-профили, пункты в UI «SSH вендор». Isolate для W2 — позже.

Дальше по W2 (polish):
1. Vendor PoE MIB (TP-Link/D-Link) по walk’ам из `docs/MIBs/`.
2. Живые тесты CLI на SMB/video свитчах (синтаксис PoE гуляет между ревизиями).
3. SNMP SET проверка на RW community.

### W3 — SOHO / long tail

1. Keenetic, Cudy, Tenda, Origo, ExeGate — сначала L0+тест IF-MIB; профили только если есть SNMP и воспроизводимый sysDescr.
2. Честно помечать в UI «ограниченная поддержка», если нет SET/PoE.

## Архитектура (куда класть код)

| Слой | Каталог / место |
|------|-----------------|
| CPU | `internal/snmp/cpu.go` |
| PoE | `internal/snmp/poe.go`, `internal/poecli/` |
| Категория узла | `internal/store/device_category.go` + UI categories |
| CLI порт / PoE / isolate | `internal/swcfg/vendor*.go` |
| Бэкап SSH | `internal/backup/`, `internal/swcfg/fetch.go` |
| LLDP uplink hints | `internal/snmp/lldp_*.go` |

Правило: **generic IF-MIB сначала**; вендорный код — только там, где стандарт ломается или нужен CLI.

## Критерии «вендор поддержан» (Definition of Done)

- [ ] Узнаётся по `sysDescr` / name / enterprise OID → корректная **категория** (switch/router/…).
- [ ] Стабильный опрос IF + хотя бы один из: LLDP, FDB.
- [ ] Документирован нужный SNMP community (RO/RW) и ограничения.
- [ ] (L1+) CPU и/или PoE без ложных нулей.
- [ ] (L2+) shutdown или description через SNMP SET **или** SSH.
- [ ] (L3) съём конфига в бэкап без порчи сессии (пейджер/enable).

## Связанные документы

- [SNMP-Community.md](SNMP-Community.md)
- [PoE-detection.md](PoE-detection.md)
- [Roadmap.md](Roadmap.md)
- [Autodiscover.md](Autodiscover.md)
