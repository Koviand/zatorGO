<p align="center">
  <img src=".github/banner.png" alt="ZatorGO" width="100%">
</p>

<h1 align="center">ZatorGO</h1>

<p align="center">
  <strong>Операторская панель для zapret2</strong><br>
  Установка, стратегии DPI, hostlist’ы и Web UI — один бинарник на роутер или VPS.
</p>

<p align="center">
  <a href="https://github.com/Koviand/zatorGO/releases/latest"><img src="https://img.shields.io/github/v/release/Koviand/zatorGO?style=flat-square&label=release&color=1d7a8c" alt="Latest release"></a>
  <a href="https://github.com/Koviand/zatorGO/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Koviand/zatorGO/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/Koviand/zatorGO?style=flat-square&color=0f172a" alt="License GPL-3.0"></a>
  <a href="https://github.com/Koviand/zatorGO/releases"><img src="https://img.shields.io/github/downloads/Koviand/zatorGO/total?style=flat-square&color=1f7a4d" alt="Downloads"></a>
</p>

<p align="center">
  <a href="#установка">Установка</a> ·
  <a href="#что-внутри">Возможности</a> ·
  <a href="#web-ui">Web UI</a> ·
  <a href="#cli">CLI</a> ·
  <a href="docs/API.md">API</a> ·
  <a href=".github/SECURITY.md">Безопасность</a>
</p>

---

## Установка

Root + интернет. Одна команда:

```sh
curl -fsSL https://github.com/Koviand/zatorGO/releases/latest/download/zator-launcher.sh -o zator-launcher.sh
chmod +x zator-launcher.sh
./zator-launcher.sh
```

Лаунчер проверит **Ed25519-подпись** `SHA256SUMS` (нужен OpenSSL 3+), сверит sha256 бинарника, поставит zapret2 и поднимет Web UI на порту **17682**.

### Неинтерактивно (без TTY / CI / cloud-init)

Аргументы после launcher уходят в `zator install`:

```sh
./zator-launcher.sh -password 'ваш-секрет'
# эквивалент: zator install -password '…'
```

Без TTY пароль **нельзя** ввести интерактивно — нужен `-password`.  
**Не** используйте `-no-auth` как «удобный дефолт»: Web UI окажется открыт всей сети. Флаг допустим только осознанно и с предупреждением в логе.

Не ставьте через `curl | sh` без просмотра скрипта; сохраните launcher на диск, как выше.

Откройте `http://<IP>:17682/` — auth **включён по умолчанию**.

| Действие | Команда / место |
| --- | --- |
| Обновить | Web UI → Настройки → Обновления, или `zator update` |
| Переустановить | `zator uninstall -y` → снова launcher (`--purge` сносит и данные) |
| Сменить пароль | `zator auth set-password -password '…'` |
| Офлайн-бандл | `scripts/pack-offline.sh` → `ZATOR_BUNDLE_DIR=…` / `Z2R_OFFLINE=1` (см. ниже) |

Подробности цепочки доверия: [SECURITY.md](.github/SECURITY.md).

---

## Зачем ZatorGO

Ядро [bol-van/zapret](https://github.com/bol-van/zapret) (`nfqws2`) **не переписывается** — ZatorGO ставит, конфигурирует и обслуживает его. Это Go-переписывание bash-[zator](https://github.com/AloofLibra/zator) с UX **Web UI first**.

| | bash-zator | ZatorGO |
| --- | --- | --- |
| Управление | Интерактивное CLI-меню | Web UI + короткий CLI |
| Обновления | Вручную | Подписанные GitHub Releases |
| Раскладка | Своя | `/opt/zator` + `/opt/zapret2` |

Релизы: **[Koviand/zatorGO](https://github.com/Koviand/zatorGO/releases)**.

---

## Что внутри

- **Web UI** — health, локи стратегий, домены, бэкапы, firewall/ports/scopes
- **Keenetic / Entware** — SysV, watchdog’и nfqws2 и Web UI, firewall ensure
- **Self-contained** — списки, стратегии, Lua и ASN вшиты в бинарник
- **Безопасные обновления** — pre-update snapshot, verify, откат; подпись `SHA256SUMS.sig`
- **Supersweep** — подбор стратегий с TLS 1.2 / 1.3
- **10 архитектур** — `arm` … `riscv64` (Lexra → `mipsel`), `CGO_ENABLED=0`

```sh
make cross    # → dist/zator-linux-*
make test
```

---

## Web UI

| Раздел | Путь | Назначение |
| --- | --- | --- |
| Обзор | `#/` | health, TLS-check, старт / стоп / рестарт |
| Стратегии | `#/strategies` | локи профилей 1–10 |
| Вручную | `#/manual` | desync, blob, supersweep |
| Домены | `#/domains` | hostlist, custom_rkn, googlevideo, исключения |
| Настройки | `#/settings/…` | обновления, бэкапы, TLS/WG, порты, scopes, конфиг |

**Стратегии** — строки `--lua-desync=…:strategy=N` в `NFQWS2_OPT`. Лок пишет номер в `locked.tsv`; после фиксации UI показывает TLS 1.2 / 1.3.

HTTP API: [`docs/API.md`](docs/API.md) (`/api/*`; `/cgi-bin/*.cgi` — совместимый алиас). Схема: [`docs/openapi.yaml`](docs/openapi.yaml); curl: [`scripts/api-examples.sh`](scripts/api-examples.sh).

---

## CLI

```text
zator install [-port 17682] [-password …] [-no-auth] [-source fork|official]
zator update
zator uninstall -y [--purge]
zator auth status|set-password|enable|disable
zator webui [-port 17682]
zator status
zator version
```

Повседневная работа — в Web UI. CLI — install / update / uninstall / auth и аварийный запуск панели.

---

## Раскладка

| Путь | Содержимое |
| --- | --- |
| `/opt/zator/bin/zator` | бинарник |
| `/opt/zator/webui` | статика UI |
| `/opt/zator/lists`, `extra_strats` | hostlist’ы и стратегии |
| `/opt/zator/state/orchestra` | локи и overrides |
| `/opt/zator/config/zator.json` | порт, auth, password_hash |
| `/opt/zapret2` | runtime zapret2 |

---

## Офлайн (роутер без GitHub)

1. На машине с интернетом: `./scripts/pack-offline.sh` (или подготовьте `vendor/` + `zator-*.tar.gz`).
2. Скопируйте бандл на устройство.
3. Установка: `ZATOR_BUNDLE_DIR=/path/to/bundle zator install -password '…'`  
   либо `Z2R_OFFLINE=1` при наличии `vendor/` рядом с бинарником.

Офлайн **не** расширяет insecure-режим: для zapret по-прежнему нужна контрольная сумма (или явный AllowInsecure только в отладке). Подпись самого бинарника Zator — через подписанный `SHA256SUMS` при online update; offline bundle доверяет заранее скачанному артефакту.

---

## Разработка

Go **1.22+**, Node.js **22+**.

```sh
go test ./...
go build -o bin/zator ./cmd/zator

cd webui-src && npm ci && npm run build   # → assets/webui/
```

После правок в `webui-src` обязателен `npm run build` — иначе в релиз уйдёт старая статика. На Windows возможен host drift CRLF/бандла: перед push сверяйте с Linux-сборкой или артефактом `webui-assets` из CI (см. Roadmap RM3). Паритет с bash-zator: [`docs/PARITY.md`](docs/PARITY.md). Актуальный backlog: [`Roadmap.md`](Roadmap.md).

Структура: `cmd/zator`, `internal/{app,webapi,deploy,domains,orchestra,…}`, `webui-src` → `assets/webui`, `docs/`.

---

## Безопасность

- Порт **17682** слушает все интерфейсы — на VPS нужен пароль (или firewall / proxy).
- Не используйте `-no-auth` на машинах из интернета.
- Обновления только с доверенного `ZATOR_OWNER` / `ZATOR_REPO`.
- Install/update **отклоняют** релиз без валидной подписи `SHA256SUMS.sig`.

Уязвимости — через [SECURITY.md](.github/SECURITY.md), не в публичные Issues.

---

## Лицензия

[GPL-3.0](LICENSE). Идея bash-zator (AloofLibra), реализация на Go.  
Обход DPI — zapret2 / nfqws2 ([bol-van/zapret](https://github.com/bol-van/zapret)).
