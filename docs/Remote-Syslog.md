# Remote syslog (архив логов устройств)

NetLynx принимает syslog с коммутаторов и роутеров, пишет архив на диск и
параллельно отдаёт копию в приложение для realtime `MAC_FLAPPING`.

## Зачем rsyslog, а не только `:9514`

| Канал | Порт | Назначение |
|---|---|---|
| **rsyslog** | UDP/TCP **514** | долговременный архив, разбор инцидентов |
| **netlynxd** | UDP **9514** | только парсер MAC-flap (см. [MAC-Investigation.md](MAC-Investigation.md)) |

Устройствам достаточно одного назначения — **IP NetLynx:514**.
rsyslog кладёт файл и сам делает fan-out на `127.0.0.1:9514`
(в сообщении сохраняется реальный IP отправителя).

Альтернативы (syslog-ng, Vector, Loki) возможны, но для классического
device-syslog на LAN **rsyslog** — стандарт: уже в Ubuntu, UDP/TCP 514,
dynaFile по IP, предсказуемый logrotate.

## Установка одной командой

На сервере NetLynx (root):

```bash
cd /root/NetLynx
sudo bash docs/setup-remote-syslog.sh
```

### Что получится

| Параметр | Значение |
|---|---|
| Корень архива | `/var/lib/remotelogs/<FROMHOST-IP>/syslog.log` |
| Формат строки | `RFC3339-время-приёма SOURCE-IP <исходное тело>` |
| Ротация | ежедневно, **60** дней, gzip, `dateext` |
| Слушает | UDP+TCP **514** |
| Fan-out | UDP `127.0.0.1:9514` |

Проверка:

```bash
ss -ulnp | grep ':514'
logger -n 127.0.0.1 -P 514 -t selftest "hello from logger"
sudo tail -n 5 /var/lib/remotelogs/127.0.0.1/syslog.log
```

Нужен `NETLYNX_SYSLOG_LISTEN=:9514` в `/etc/netlynx/netlynx.env`
(иначе fan-out только в архив, без событий MAC).

## Настройка устройств

Инвентарь TSV (`host`, `vendor`, `user`, `password`, `enable`, `port`):

```bash
python3 docs/configure-device-syslog.py \
  --target 10.0.0.1 \
  --inventory /path/to/devices-syslog.tsv
```

Поддерживаемые `vendor`: `edgeswitch`, `snr`, `eltex`, `mikrotik`.

### Уровни (кратко)

| Vendor | Что включаем |
|---|---|
| EdgeSwitch | **`logging syslog`** (иначе Relayed=0!), `logging host "IP" ipv4 514 debug`, `logging traps 7` |
| SNR | `info-center loghost`, channel loghost **debug** |
| Eltex MES | `logging host <IP>` (+ optional `port 514`); **не** `logging host ipv4` (ломается на DNS) |
| MikroTik | action `netlynx` + topics: critical/error/warning/info/interface/system/bridge/stp/dhcp/firewall |

Полный `debug` на MikroTik намеренно **не** льём (шум firewall connection-tracking).

После настройки на устройстве через минуту должен появиться каталог:

```bash
sudo ls /var/lib/remotelogs/10.0.0.1/
sudo tail -n 20 /var/lib/remotelogs/10.0.0.1/syslog.log
```

## Отличия от типичного «просто accepted.log»

1. Путь в `/var/lib` (данные сервиса), не свалка в `/var/log`.
2. Время **приёма** на сервере — часы устройств часто врут после cold boot.
3. Без rate-limit на remote path (чтобы не потерять STP/BPDU storm).
4. Большой UDP receive buffer + `maxMessageSize=64k`.
5. Одна схема ротации на всех хостов (60 дней), без зоопарка per-host logrotate.
6. Fan-out в приложение без потери source IP.

## Если не работает

| Симптом | Что проверить |
|---|---|
| Нет каталога IP | `tcpdump -ni any udp port 514`, ACL/firewall на пути, `logging host` на устройстве |
| rsyslog не стартует | `rsyslogd -N1`, нет ли второго `input … 514` в `/etc/rsyslog.conf` |
| Пустой fan-out / нет MAC событий | `NETLYNX_SYSLOG_LISTEN=:9514`, `ss -ulnp \| grep 9514` |
| Диск растёт | `logrotate -d /etc/logrotate.d/netlynx-remotelogs`, `du -sh /var/lib/remotelogs` |
