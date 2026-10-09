# История конфигурации свитча (show run)

Снимки running-config по SSH для ответа «**что изменилось на свитче**» между двумя моментами.

## Быстрый путь

1. Карточка узла → блок **История конфига (show run) — сохраняется при изменении конфига**.
2. **Сохранить снимок сейчас** (operator) или дождаться планировщика / бэкапа / sync ролей.
3. **Diff с предыдущим** или два снимка → **Показать diff**.

## Для каких узлов

Снимки делаются только если:

- категория **switch**, или
- **MikroTik router** (категория router + вендор MikroTik: явно в карточке или автодетект по sysDescr/имени — RouterOS, CCR, RouterBOARD…).

Съём MikroTik: SSH `/export compact` (fallback `/export`) → файл в ZIP и снимок истории.

Нужны SSH-учётные данные на узле (или общие из Настройки → Резервные копии), либо `SSH_POE_*` в `/etc/netlynx/netlynx.env`.

С **0.12.28** ZIP-бэкап и scheduler снимков читают пароль через `GetDevice` (расшифровка at-rest). Раньше `ListDevices` отдавал ciphertext → `unable to authenticate`, хотя в карточке пароль был и ручной diff работал.

С **0.13.0** роутеры с `ssh_vendor=auto` тоже попадают в ZIP, если SNMP/имя говорят RouterOS (раньше нужен был явный вендор MikroTik). В Настройки → Резервные копии — два независимых флажка: коммутаторы (`switch_cfg_enabled`) и роутеры MikroTik (`router_cfg_enabled`). Старая одна галочка «только у коммутаторов» на деле включала весь SSH-съём — подпись была неверной. После миграции, если съём уже был включён, оба флага остаются включёнными.

## Ошибки SSH и оповещения (0.12.27+)

| Класс (`err_class`) | Типичный текст | Что делать |
|---------------------|----------------|------------|
| `auth` | `unable to authenticate … [none password]` | Верный логин/пароль в карточке / Резервные копии / `SSH_POE_*`. Автоматически пароль не угадывается. |
| `hostkey` | `crypto/rsa: verification error`, «ключ хоста изменился» | NetLynx перебирает legacy-алгоритмы и обновляет `/var/lib/netlynx/ssh_known_hosts`. Probe ключа (с **0.13.1**) обрывается до userauth — без `login failure` для `_hostkey_probe_` на RouterOS. |
| `timeout` | i/o timeout | Сеть / ACL / свитч offline. |

События в ленте и Telegram (если включён и фильтр типов пуст или содержит эти типы):

- `CONFIG_SSH_FAIL` — сбой съёма у узла (debounce 6 ч на класс ошибки)
- `BACKUP_SSH_PARTIAL` — сводка ZIP-бэкапа (сколько узлов без конфига)

При создании / promote узла NetLynx сам пробует host key и SSH; при ошибке — `ssh_warning` в ответе API и `CONFIG_SSH_FAIL`.

Если в Настройки → Уведомления задан **непустой** whitelist `telegram_event_types`, добавьте туда `CONFIG_SSH_FAIL,BACKUP_SSH_PARTIAL`.

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
