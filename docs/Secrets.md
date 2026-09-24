# Секреты at-rest (шифрование в Postgres)

SNMP community, пароли SNMPv3/SSH, SMTP, Telegram bot token, UISP API token и пароли backup **по умолчанию** могут храниться в Postgres открытым текстом. С ключом `NETLYNX_SECRETS_KEY` NetLynx пишет их как **AES-256-GCM** (`enc:v1:…`).

Пароли пользователей UI (`users.password_hash`) — **bcrypt**, без изменений. Refresh-токены — хэш. JWT signing key в `app_secrets` на первом этапе не шифруется (backlog Phase 2 — [To-Do.md](To-Do.md#секреты-at-rest-aes-gcm)).

## Угроза

Закрывает: дамп БД, бэкап Postgres, read-only SQL без доступа к env сервера.

Не закрывает: полный доступ к хосту `netlynxd` (в RAM секреты расшифрованы для SNMP/SSH).

## Быстрый старт

1. Сгенерировать ключ (32 байта, base64):

```bash
netlynxd secrets-genkey
# или: openssl rand -base64 32
```

2. Добавить в `/etc/netlynx/netlynx.env` (рядом с `DATABASE_URL`, **не в git**):

```bash
NETLYNX_SECRETS_KEY=<вывод secrets-genkey>
# либо файл только с ключом:
# NETLYNX_SECRETS_KEY_FILE=/etc/netlynx/secrets.key
```

3. Перезапуск службы:

```bash
sudo systemctl restart NetLynx.service
```

В логе: `секреты at-rest: AES-256-GCM включён`. Новые записи community/SSH/… уже шифруются.

4. Зашифровать уже лежащие plaintext-значения (идемпотентно):

```bash
netlynxd secrets-rewrap
```

Или admin API:

```text
GET  /api/v1/system/secrets          # plaintext count + encryption_enabled
POST /api/v1/system/secrets/rewrap   # migrate
```

5. После `plaintext.total = 0` зафиксировать обязательный ключ:

```bash
SECRETS_REQUIRE_KEY=true
```

Без ключа служба **не стартует** (нельзя случайно снова писать plaintext).

## Restore / второй сервер

Бэкап Postgres после rewrap содержит только ciphertext. На новой машине нужен **тот же** `NETLYNX_SECRETS_KEY`, иначе опрос/SSH/SMTP сломаются.

## Env

| Переменная | Смысл |
|------------|--------|
| `NETLYNX_SECRETS_KEY` | Master key, base64 от 32 байт |
| `NETLYNX_SECRETS_KEY_FILE` | Путь к файлу с тем же base64 (приоритетнее KEY) |
| `SECRETS_REQUIRE_KEY` | `true` → старт без ключа = fatal |

## Формат в БД

```text
enc:v1:<base64url-raw(nonce||ciphertext||tag)>
```

Строка без префикса — legacy plaintext (dual-read до rewrap).

## Scope колонок

| Таблица | Колонки |
|--------|---------|
| `devices` | `community`, `v3_auth_pass`, `v3_priv_pass`, `ssh_password`, `ssh_enable_password` |
| `notification_settings` | `smtp_password`, `telegram_bot_token` |
| `uisp_settings` | `api_token` |
| `backup_settings` | `share_password`, `ssh_password`, `ssh_enable_password` |

API по-прежнему отдаёт только `has_*`, не plaintext и не ciphertext.
