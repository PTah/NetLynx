# NetLynx Desktop (Windows) — не готов

**Статус: черновик / заготовка под будущий клиент. Полноценного desktop-приложения пока нет.**

В каталоге `desktop/` лежит ранний каркас на Electron (эксперимент для шага «Windows-клиент» из [TZ-snmp-switch-monitor.md](TZ-snmp-switch-monitor.md)). Это **не** автономная версия NetLynx и **не** замена серверу или веб-интерфейсу.

## Что планировалось (ТЗ)

- Клиент Windows поверх того же REST API, что и веб-морда.
- Стабильный контракт API — см. [API.md](API.md).

## Что есть сейчас (не для production)

- Черновик Electron с частичным UI (логин, список узлов, события).
- Код может не собираться или не соответствовать текущему API без доработки.

## Как пользоваться NetLynx на Windows сегодня

1. **Сервер** — [Windows-Server-Setup.md](Windows-Server-Setup.md) (Go + PostgreSQL + NSSM) **или** Linux-сервер + браузер.
2. **Интерфейс** — веб в браузере (`http://<host>:8080` или через nginx).

Сборка черновика (если нужна для разработки):

```bash
cd desktop
npm install
npm start
```

Portable `.exe` (`npm run dist:win`) — только для проверки каркаса, не релиз.
