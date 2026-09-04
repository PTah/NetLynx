# Autodiscover устройств на портах (LLDP / CDP)

## Что нужно на коммутаторах

Для **корректного автообнаружения** («Обнаружено», соседи на портах, рёбра топологии по LLDP/CDP) на свитчах должны быть включены:

| Протокол | Зачем | Где на свитче |
|----------|--------|----------------|
| **SNMP** (v2c/v3) | NetLynx читает MIB (в т.ч. LLDP/CDP) только по SNMP | глобально: агент / `snmp-server` + community (хотя бы **RO**) |
| **LLDP** | соседи в `port_neighbors`, кандидаты в «Обнаружено», карта | обычно **на портах** (transmit + receive); без этого walk пустой |
| **CDP** (Cisco и совместимые) | то же для CDP-соседей | на портах / глобально по вендору |

NetLynx **не включает и не выключает** SNMP/LLDP на оборудовании — только опрашивает. Настройка — на свитче (CLI/UI вендора) или вручную.

Community и RO/RW: [SNMP-Community.md](SNMP-Community.md).

### Пример: Ubiquiti EdgeSwitch

SNMP (глобально):

```text
enable
configure
snmp-server community YOUR_RO_COMMUNITY ro
exit
write memory
```

LLDP на диапазоне портов (подставьте свой, напр. `0/1-0/48`):

```text
enable
configure
interface 0/1-0/48
lldp transmit
lldp receive
lldp transmit-tlv port-desc
lldp transmit-tlv sys-name
lldp transmit-tlv sys-desc
lldp transmit-tlv sys-cap
lldp transmit-mgmt
exit
write memory
```

Проверка с сервера NetLynx: `snmpwalk` к LLDP rem (`lldpRemSysName` и др.) — см. раздел «Диагностика» ниже.

## ПК и серверы (Windows / Linux)

Автообнаружение в NetLynx строится так: **опрашивается коммутатор по SNMP**, а в MIB LLDP/CDP свитч отдаёт то, что **услышал от соседей**. Поэтому:

| На чём | SNMP | LLDP | Зачем |
|--------|------|------|--------|
| **Коммутатор** | обязателен | receive (+ обычно transmit) | NetLynx читает соседей с порта |
| **ПК / сервер / телефон** у порта | **не обязателен** для появления в «Обнаружено» | **желателен transmit** | свитч видит sysName / chassis / порт и отдаёт это по SNMP |
| Узел, который сами опрашиваете в NetLynx | нужен агент SNMP | по желанию | карточка устройства, IF-MIB и т.д. |

NetLynx **не ставит** службы на ПК — ниже типовая установка вручную.

### Windows (10 / 11 / Server) — SNMP

> **`Install-WindowsFeature` — только Windows Server.** На Windows 10/11 этой команды нет — используйте блок «Клиент» ниже.

**Windows 10 / 11 (клиент)** — PowerShell **от администратора**:

```powershell
# Проверка: есть ли уже компонент SNMP
Get-WindowsCapability -Online | Where-Object Name -like 'SNMP*'

# Установка
Add-WindowsCapability -Online -Name "SNMP.Client~~~~0.0.1.0"

# Если Add-WindowsCapability недоступен / ошибка — через DISM:
# DISM /Online /Add-Capability /CapabilityName:SNMP.Client~~~~0.0.1.0
```

Либо GUI: **Параметры → Приложения → Дополнительные компоненты → Добавить компонент** → найти **SNMP** / **Simple Network Management Protocol**.

На части сборок SNMP убрали из «Дополнительных компонентов» — тогда ставьте через **Optional Features** в классической панели (`optionalfeatures.exe`) пункт **Простой протокол управления сетью (SNMP)** / **SNMP**, либо через DISM выше.

**Windows Server** (PowerShell от администратора):

```powershell
Install-WindowsFeature SNMP-Service -IncludeManagementTools
# модуль: Import-Module ServerManager
```

Дальше (и клиент, и Server):

1. `services.msc` → служба **SNMP** → тип запуска «Автоматически» → Запустить.
2. Свойства службы → вкладка **Безопасность**: community (например `netlynx-ro`), права **Только чтение**; при необходимости ограничить принимающие узлы IP сервера NetLynx.
3. Вкладка **Ловушки** — только если нужны traps (для автообнаружения не обязательны).
4. Брандмауэр: разрешить **UDP 161** с IP NetLynx (если агент должен отвечать на опрос).

Проверка с сервера NetLynx:

```bash
snmpwalk -v2c -c netlynx-ro HOST_IP system
```

### Windows — LLDP (чтобы свитч видел ПК)

На адаптере должны быть привязки Microsoft LLDP / LLTD (часто уже есть, но выключены).

PowerShell от администратора (имя адаптера своё: `Get-NetAdapter`):

```powershell
Get-NetAdapterBinding -Name "Ethernet" | Where-Object ComponentID -match 'lldp|lltd|rspndr'

Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_lldp
Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_lltdio
Enable-NetAdapterBinding -Name "Ethernet" -ComponentID ms_rspndr
```

Если `ms_lldp` нет в списке — в «Свойства» сетевого подключения включите **Microsoft LLDP Protocol Driver** (и при наличии **Link-Layer Topology Discovery** Mapper/Responder).

На **коммутаторе** после этого в `show lldp remote-device` / в NetLynx в колонке соседа должен появиться хост (имя Windows / chassis). Без LLDP на ПК свитч часто показывает только MAC в FDB, без нормального sysName в LLDP.

### Linux — SNMP (`snmpd`)

Debian / Ubuntu:

```bash
sudo apt update
sudo apt install -y snmpd snmp
sudo cp -a /etc/snmp/snmpd.conf /etc/snmp/snmpd.conf.bak
```

Минимальный RO community (пример; поправьте community и ACL):

```bash
sudo tee /etc/snmp/snmpd.conf >/dev/null <<'EOF'
agentAddress udp:161
rocommunity netlynx-ro default
sysLocation office
sysContact admin@example.com
sysName $(hostname -f)
EOF
# sysName лучше задать явно:
sudo sed -i "s/sysName .*/sysName $(hostname -f)/" /etc/snmp/snmpd.conf

sudo systemctl enable --now snmpd
sudo systemctl status snmpd --no-pager
```

Разрешите UDP/161 в firewall (`ufw allow from NETLYNX_IP to any port 161 proto udp` при ufw).

Проверка:

```bash
snmpwalk -v2c -c netlynx-ro 127.0.0.1 system
```

### Linux — LLDP (`lldpd`)

```bash
sudo apt update
sudo apt install -y lldpd
sudo systemctl enable --now lldpd
sudo lldpcli show configuration
sudo lldpcli show neighbors
```

По умолчанию `lldpd` шлёт и принимает LLDP на интерфейсах. Ограничить интерфейсы (пример):

```bash
# /etc/lldpd.d/local.conf — синтаксис lldpd; либо:
sudo lldpcli configure system interface pattern eth*,en*
```

RHEL / Rocky / Alma:

```bash
sudo dnf install -y net-snmp net-snmp-utils lldpd
sudo systemctl enable --now snmpd lldpd
```

Конфиг SNMP: `/etc/snmp/snmpd.conf` (аналогично `rocommunity`).

### Что ожидать в NetLynx

1. Свитч в inventory с рабочим SNMP.
2. На порту к ПК/серверу свитч видит LLDP-соседа → после опроса строка в **«Обнаружено»** / колонка «Сосед LLDP».
3. SNMP на самом ПК нужен только если вы **добавляете этот хост** как узел для опроса; для появления соседа с порта свитча достаточно **LLDP на хосте + SNMP+LLDP на свитче**.

## Реализовано

При каждом успешном опросе IF-MIB дополнительно читаются LLDP и CDP; соседи пишутся в `port_neighbors`. Отдельно FDB может создавать соседей `protocol=fdb` (AP/edge-эвристики) для карты топологии.

### LLDP

- `lldpRemSysName`, `lldpRemPortId`, `lldpRemChassisId`
- subtype chassis (в т.ч. networkAddress → `remote_mgmt_addr`)
- сопоставление local port → ifIndex через `lldpLocPortTable`, иначе ifIndex / ifName `0/N`

### CDP (Cisco)

- `cdpCacheDeviceId` / `cdpCacheDevicePort` / address
- индекс `ifIndex.deviceIndex`

### Хранение

Таблица `port_neighbors`:

- PK `(device_id, if_index, protocol, rem_index)`
- `protocol`: `lldp` | `cdp` | `fdb`
- `stale` + TTL ~**2 ч**
- При **ошибке** walk данные **не очищаются**
- Пустой успешный walk: соседи помечаются stale только после **двух подряд** пустых опросов (`neighborEmptyConfirmPolls = 2`), чтобы не мигать при одноразовом сбое LLDP

## API / UI

| Что | Где |
|-----|-----|
| Соседи на портах | `GET /devices/{id}/detail` → `neighbors`; колонка **«Сосед LLDP»** |
| Кандидаты в inventory | страница **Обнаружено** (`/discovered`): ignore / reopen / preview / promote |
| Карта | **Топология** (`/topology`): фильтры protocol (lldp/cdp/fdb/manual), VLAN, локация, depth, layout |
| Настройки карты | `GET/PATCH /api/v1/settings/topology` |

Ручные связи — [Manual-Topology-Links.md](Manual-Topology-Links.md).

## Резолв в известный узел

1. **MAC** — chassis / MAC в Port ID  
2. **mgmt IP** — `remote_mgmt_addr` / host  
3. **sysName** — только если уникален среди узлов  

При poll пишется `devices.chassis_mac`; при promote MAC из discovered сразу в `chassis_mac`.

### Ловушка EdgeSwitch ifName `0/10+`

OctetString `0/24` нельзя трактовать как IPv4 (`48.47.50.52`).

## Статус фич

| Фича | Статус |
|------|--------|
| discovered_devices + promote/ignore | **готово** (исторически 0.16) |
| FDB → topology neighbors | **готово** (`protocol=fdb`) |
| Фильтры/layout топологии | **готово** в UI |

## Диагностика

- На свитче включены **SNMP** и **LLDP** (и CDP, если нужны Cisco-соседи)? Без этого автообнаружение и топология по LLDP/CDP не заработают.
- `snmpwalk` LLDP rem sysName / CDP deviceId с хоста NetLynx
- «Показывать все интерфейсы» — сосед может быть на нефизическом ifIndex
- Логи: `lldp neighbors walk` / `cdp …` / `… write`
