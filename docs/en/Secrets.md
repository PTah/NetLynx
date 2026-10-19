# Secrets at rest (encryption in PostgreSQL)

SNMP communities, SNMPv3/SSH passwords, SMTP credentials, the Telegram bot token, the UISP API token and backup passwords are stored in PostgreSQL **in plain text by default**. With `NETLYNX_SECRETS_KEY` set, NetLynx writes them as **AES-256-GCM** (`enc:v1:…`).

UI user passwords (`users.password_hash`) are **bcrypt**, unchanged. Refresh tokens are hashed. The JWT signing key in `app_secrets` is not encrypted at this stage (backlog Phase 2 — [To-Do.md](../../docs/To-Do.md)).

**Languages:** [Русский](../Secrets.md) · English

## What this protects against

Protects against: a database dump, a PostgreSQL backup, read-only SQL without access to the server's env.

Does **not** protect against: full access to the `netlynxd` host (secrets are decrypted in RAM for SNMP/SSH).

## Quick start

1. Generate a key (32 bytes, base64):

```bash
netlynxd secrets-genkey
# or: openssl rand -base64 32
```

2. Add it to `/etc/netlynx/netlynx.env` (next to `DATABASE_URL`, **never in git**):

```bash
NETLYNX_SECRETS_KEY=<output of secrets-genkey>
# or a file containing only the key:
# NETLYNX_SECRETS_KEY_FILE=/etc/netlynx/secrets.key
```

3. Restart the service:

```bash
sudo systemctl restart NetLynx.service
```

The log shows `секреты at-rest: AES-256-GCM включён` ("secrets at rest: AES-256-GCM enabled"). New community/SSH/… records are already encrypted.

4. Encrypt plaintext values that are already stored (idempotent):

```bash
netlynxd secrets-rewrap
```

Or the admin API:

```text
GET  /api/v1/system/secrets          # plaintext count + encryption_enabled
POST /api/v1/system/secrets/rewrap   # migrate
```

5. Once `plaintext.total = 0`, make the key mandatory:

```bash
SECRETS_REQUIRE_KEY=true
```

Without a key the service **does not start** (so plaintext cannot be written again by accident).

## Restore / a second server

A PostgreSQL backup taken after the rewrap contains ciphertext only. On a new machine you need the **same** `NETLYNX_SECRETS_KEY`, otherwise polling/SSH/SMTP break.

## Env

| Variable | Meaning |
|----------|---------|
| `NETLYNX_SECRETS_KEY` | Master key, base64 of 32 bytes |
| `NETLYNX_SECRETS_KEY_FILE` | Path to a file with the same base64 (takes precedence over KEY) |
| `SECRETS_REQUIRE_KEY` | `true` → starting without a key is fatal |

## Format in the database

```text
enc:v1:<base64url-raw(nonce||ciphertext||tag)>
```

A string without the prefix is legacy plaintext (dual-read until rewrapped).

## Column scope

| Table | Columns |
|-------|---------|
| `devices` | `community`, `v3_auth_pass`, `v3_priv_pass`, `ssh_password`, `ssh_enable_password` |
| `notification_settings` | `smtp_password`, `telegram_bot_token` |
| `uisp_settings` | `api_token` |
| `backup_settings` | `share_password`, `ssh_password`, `ssh_enable_password` |

The API still returns only `has_*` flags — never plaintext and never ciphertext.
