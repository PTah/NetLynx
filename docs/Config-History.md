# История конфигурации свитча (show run)

Снимки running-config по SSH для ответа «**что изменилось на свитче**» между двумя моментами.

## Быстрый путь

1. Карточка узла → блок **История конфига (show run) — сохраняется при изменении конфига**.
2. **Сохранить снимок сейчас** (operator) или дождаться планировщика / бэкапа / sync ролей.
3. **Diff с предыдущим** или два снимка → **Показать diff**.

## Для каких узлов

Снимки делаются только если:

- категория **switch**, или
- **MikroTik router** (категория router + SSH-вендор MikroTik).

Нужны SSH-учётные данные на узле (или общие из Настройки → Резервные копии).

## Источники снимков

| source | Когда |
|--------|--------|
| `scheduled` | фоновый scheduler (по умолчанию раз в 24 ч) |
| `backup` | ночной SSH-бэкап конфигов |
| `port_sync` | «Перечитать конфиг (SSH)» / sync port roles |
| `manual` | кнопка «Сохранить снимок сейчас» |

Дубликаты не пишутся: SHA-256 считается по **каноническому** тексту. Перед сравнением выкидываются runtime-строки, которые не являются конфигом:

- EdgeSwitch / Ubiquiti: `!System Up Time`, `!Current SNTP Synchronized Time` (и NTP-вариант)
- Cisco: `ntp clock-period`
- MikroTik: дата/время в первой строке `# … by RouterOS`

Старые снимки с uptime в тексте при следующем опросе **не** порождают новый ряд, если больше ничего не менялось. В новые снимки эти строки уже не кладутся.

Ночной ZIP-бэкап по-прежнему кладёт свежий `show run` как есть (для restore); в таблицу истории — только при реальном изменении.

## Env

```bash
CONFIG_SNAPSHOT_ENABLED=true          # false = выкл scheduler
CONFIG_SNAPSHOT_INTERVAL_HOURS=24
CONFIG_SNAPSHOT_RETENTION_DAYS=90
```

## API

```bash
GET  /api/v1/devices/{id}/config/snapshots
GET  /api/v1/devices/{id}/config/snapshots/{snapId}
GET  /api/v1/devices/{id}/config/diff?to=123&from=122   # from/to можно опустить — берётся последний
POST /api/v1/devices/{id}/config/snapshot               # operator
```
