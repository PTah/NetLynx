# NetLynx на Windows Server

Инструкция для запуска **сервера** NetLynx (`netlynxd.exe`) на Windows Server или Windows 10/11 Pro. Веб-интерфейс — в браузере (`http://<сервер>:8080`), как на Linux.

> **Desktop-клиент** (`desktop/`) — отдельная история, **не готов**; см. [desktop/README.md](../desktop/README.md).

См. также: [Runbook-Linux.md](Runbook-Linux.md) (основной путь) · [SNMP-Community.md](SNMP-Community.md) · [Vendors.md](Vendors.md) · [API.md](API.md)

---

## Что такое NSSM и зачем он здесь

**NSSM** — **N**on-**S**ucking **S**ervice **M**anager, бесплатная утилита: [nssm.cc](https://nssm.cc/download).

| Вопрос | Ответ |
|--------|--------|
| **Что делает?** | Превращает обычную программу (`.exe`) в **службу Windows** — старт при загрузке, перезапуск при падении, логи, работа в фоне без открытого окна консоли. |
| **Почему не «просто exe»?** | `netlynxd.exe` — обычный Go-бинарник. Сам по себе он **не** регистрируется в «Службах Windows» (в отличие от `.NET` с встроенным Service). NSSM — простой способ обернуть exe в службу **без** переписывания кода. |
| **Где лежит?** | После установки — файл **`nssm.exe`** (часто `C:\Tools\nssm\nssm.exe` или рядом с проектом). В **Пуск → Службы** (`services.msc`) появится служба с именем, которое вы зададите (ниже — `netlynxd`). |
| **Альтернатива** | Запуск вручную в консоли (`.\bin\netlynxd.exe`) — только для теста. Для production на Windows — служба через NSSM или свой wrapper; в этой инструкции — **NSSM**. |

### Установка NSSM (один раз)

1. Скачайте архив с [nssm.cc/download](https://nssm.cc/download) (под вашу разрядность: **win64** для 64-bit Windows).
2. Распакуйте, например в `C:\Tools\nssm\`.
3. Добавьте каталог с `nssm.exe` в **PATH** или вызывайте полным путём.

Проверка в PowerShell **от имени администратора**:

```powershell
nssm version
# или: C:\Tools\nssm\win64\nssm.exe version
```

Должна вывестись версия NSSM (например `NSSM 2.24`).

---

## 1) Что ещё установить

| Компонент | Зачем |
|-----------|--------|
| **Git for Windows** | скачать исходники |
| **Go 1.22+** | собрать `netlynxd.exe` |
| **Node.js LTS** (npm) | собрать веб-интерфейс |
| **Docker Desktop** или **PostgreSQL** | база данных (как в Linux: роль/БД `invetor`, порт **5433** при docker-compose) |
| **NSSM** | служба Windows (см. выше) |

---

## 2) Размещение проекта

```powershell
cd C:\
git clone https://github.com/PTah/NetLynx.git NetLynx
cd C:\NetLynx
copy .env.example .env
```

Проверьте `DATABASE_URL` в `.env` (обычно `127.0.0.1:5433` при Postgres из `docker-compose.yml`).

Поднять Postgres (если через Docker):

```powershell
cd C:\NetLynx
docker compose up -d postgres
```

---

## 3) Первая сборка

```powershell
cd C:\NetLynx\web
npm install
npm run build
cd C:\NetLynx
go mod tidy
New-Item -ItemType Directory -Force -Path .\bin | Out-Null
go build -o .\bin\netlynxd.exe .\cmd\netlynxd
```

Пробный запуск **без** службы (окно консоли, для проверки):

```powershell
$env:DATABASE_URL = "postgres://invetor:invetor@127.0.0.1:5433/invetor?sslmode=disable"
$env:HTTP_ADDR = ":8080"
$env:WEB_DIST = "web\dist"
.\bin\netlynxd.exe
# в другом окне: curl http://127.0.0.1:8080/health
```

Остановите Ctrl+C, дальше — постоянная служба через NSSM.

---

## 4) Установка службы Windows (NSSM)

PowerShell **от администратора**. Создайте каталог для логов:

```powershell
New-Item -ItemType Directory -Force -Path C:\NetLynx\logs | Out-Null
```

Регистрация службы **`netlynxd`**:

```powershell
nssm install netlynxd C:\NetLynx\bin\netlynxd.exe
nssm set netlynxd AppDirectory C:\NetLynx
nssm set netlynxd AppEnvironmentExtra DATABASE_URL=postgres://invetor:invetor@127.0.0.1:5433/invetor?sslmode=disable
nssm set netlynxd AppEnvironmentExtra HTTP_ADDR=:8080
nssm set netlynxd AppEnvironmentExtra WEB_DIST=C:\NetLynx\web\dist
nssm set netlynxd AppStdout C:\NetLynx\logs\netlynxd.out.log
nssm set netlynxd AppStderr C:\NetLynx\logs\netlynxd.err.log
nssm start netlynxd
```

Проверить в GUI: `Win+R` → `services.msc` → служба **netlynxd** → состояние «Выполняется».

Если переменных окружения много — wrapper-скрипт `.cmd`/`.ps1`, который читает `.env`, и в NSSM укажите его как `Application` вместо прямого вызова exe.

---

## 5) Обновление версии

```powershell
cd C:\NetLynx
git pull
cd web
npm install
npm run build
cd ..
go mod tidy
go build -o .\bin\netlynxd.exe .\cmd\netlynxd
nssm restart netlynxd
```

---

## 6) Проверка

```powershell
curl http://127.0.0.1:8080/health
```

Ожидаемо: `"status":"ok"`. В браузере: `http://IP_СЕРВЕРА:8080` (логин по умолчанию — как в [Runbook-Linux.md](Runbook-Linux.md#шаг-5-первый-вход-в-браузере)).

---

## 7) Firewall

Откройте порт **8080** только для доверенной сети:

```powershell
New-NetFirewallRule -DisplayName "NetLynx 8080" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8080
```

---

## 8) Эксплуатация

- Запускать службу под **отдельной** учётной записью (не Administrator), если политика компании требует.
- Секреты (SMTP, пароли) — не в открытом `.env` на shared-хосте; по возможности — хранилище секретов ОС.
- Резервные копии PostgreSQL — отдельно от git-клона.
- Reverse proxy (IIS/nginx на Windows) — по желанию; по умолчанию NetLynx слушает **:8080** напрямую, как на Linux без nginx.

---

## Удалить службу (если нужно)

```powershell
nssm stop netlynxd
nssm remove netlynxd confirm
```
