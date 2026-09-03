# NetLynx: готовность к понедельнику

Чеклист после миграции Invetor → NetLynx на **10.0.0.1**.

## Для пользователей

| | |
|--|--|
| URL | **https://netlynx.example.com** |
| Логин | как раньше (мигрирован из Invetor) |
| Старый URL | `http://10.0.0.1:8080` снаружи **не** использовать |

## Для админа (один раз на сервере)

```bash
cd /root/NetLynx
git pull origin main
sudo bash scripts/finalize-server-netlynx.sh
bash docs/deploy.sh   # последняя версия из git
```

Финализация убирает legacy Invetor (unit, бинарник, старые каталоги в `/root/legacy-invetor-*`).

## Что остаётся с именем invetor (намеренно)

| Компонент | Почему |
|-----------|--------|
| PostgreSQL `invetor` / порт 5433 | данные без переименования |
| Docker `invetor-postgres-1`, volume `invetor_pg` | тот же том с БД |
| `DATABASE_URL=.../invetor` | см. выше |

## Проверка перед понедельником

```bash
systemctl status NetLynx.service nginx --no-pager
curl -fsS http://127.0.0.1:8080/health
curl -fsSk https://netlynx.example.com/health
sudo docker exec invetor-postgres-1 psql -U invetor -d invetor -tAc 'select count(*) from devices'
```

В UI: **Узлы**, **Топология**, **Настройки → Резервные копии** (локальный каталог `/var/backups/netlynx`).

## Разработка

Эталонный git: [github.com/PTah/NetLynx](https://github.com/PTah/NetLynx) (`main`).  
Репозиторий **Invetor** — архив, не коммитить.

Периметр (HAProxy): [reverse-proxy](https://github.com/PTah/reverse-proxy), SNI `netlynx.example.com`.
