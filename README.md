<p align="center">
  <img src=".github/banner.png" alt="ZatorGO — operator panel for zapret2" width="100%">
</p>

<h1 align="center">ZatorGO</h1>

<p align="center">
  <strong>Операторская панель для zapret2</strong><br>
  <em>Один бинарник · Web UI · подписанные релизы · роутер или VPS</em>
</p>

<p align="center">
  <a href="https://github.com/Koviand/zatorGO/releases/latest"><img src="https://img.shields.io/github/v/release/Koviand/zatorGO?style=for-the-badge&label=release&color=1d7a8c" alt="Latest release"></a>
  <a href="https://github.com/Koviand/zatorGO/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Koviand/zatorGO/ci.yml?branch=main&style=for-the-badge&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/Koviand/zatorGO?style=for-the-badge&color=0f172a" alt="License GPL-3.0"></a>
</p>

<p align="center">
  <a href="#быстрый-старт">Старт</a> ·
  <a href="#возможности">Возможности</a> ·
  <a href="#web-ui">Web UI</a> ·
  <a href="#cli">CLI</a> ·
  <a href="docs/API.md">API</a> ·
  <a href=".github/SECURITY.md">Безопасность</a> ·
  <a href="https://github.com/Koviand/zatorGO/releases/latest">Releases</a>
</p>

---

## Быстрый старт

Root + интернет. Сохраните launcher на диск (не `curl | sh`):

```sh
curl -fsSL https://github.com/Koviand/zatorGO/releases/latest/download/zator-launcher.sh -o zator-launcher.sh
chmod +x zator-launcher.sh
./zator-launcher.sh -password 'ваш-секрет'
```

Лаунчер проверит **Ed25519** `SHA256SUMS.sig`, сверит sha256 бинарника, поставит zapret2 и поднимет Web UI на **`:17682`**.

Откройте `http://<IP>:17682/` — вход по паролю **включён по умолчанию**.

<details>
<summary><strong>Без TTY / CI / cloud-init</strong></summary>

```sh
./zator-launcher.sh -password 'ваш-секрет'
# или: ZATOR_PASSWORD='…' ./zator-launcher.sh
# −password виден в ps — предпочтительнее ZATOR_PASSWORD
```

`-no-auth` только осознанно: нужен `-host 127.0.0.1` или `-listen-all`. То же для `zator webui` без auth.
</details>

<details>
<summary><strong>Обновление · переустановка · офлайн</strong></summary>

| Действие | Как |
| --- | --- |
| Обновить | Web UI → Настройки → Обновления, или `zator update` |
| Сменить пароль | `zator auth set-password -password '…'` |
| Переустановить | `zator uninstall -y` → снова launcher (`--purge` сносит данные) |
| Офлайн | `scripts/pack-offline.sh` → `ZATOR_BUNDLE_DIR=…` (нужны `SHA256SUMS` + `.sig`) |

Цепочка доверия: [SECURITY.md](.github/SECURITY.md).
</details>

---

## Зачем ZatorGO

Ядро [bol-van/zapret](https://github.com/bol-van/zapret) (`nfqws2`) **не переписывается** — ZatorGO ставит, конфигурирует и обслуживает его. Go-переписывание bash-[zator](https://github.com/AloofLibra/zator) с UX **Web UI first**.

| | bash-zator | **ZatorGO** |
| --- | --- | --- |
| Управление | Интерактивное CLI-меню | Web UI + короткий CLI |
| Обновления | Вручную | Подписанные GitHub Releases |
| Раскладка | Своя | `/opt/zator` + `/opt/zapret2` |
| Платформы | Ограниченно | 10 Linux-arch, `CGO_ENABLED=0` |

---

## Возможности

| | |
| --- | --- |
| **Web UI** | Health, локи стратегий, домены, бэкапы, firewall / ports / scopes |
| **Keenetic / Entware** | SysV, watchdog’и nfqws2 и Web UI, firewall ensure |
| **Self-contained** | Списки, стратегии, Lua и ASN вшиты в бинарник |
| **Безопасные обновления** | Pre-update snapshot → verify → откат; `SHA256SUMS.sig` |
| **Supersweep** | Подбор стратегий с TLS 1.2 / 1.3 |
| **Архитектуры** | `arm` … `riscv64` (Lexra → `mipsel`) |

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
| Настройки | `#/settings/…` | обновления, бэкапы, TLS/WG, порты, scopes |

**Стратегии** — строки `--lua-desync=…:strategy=N` в `NFQWS2_OPT`. Лок пишет номер в `locked.tsv`; после фиксации UI показывает TLS 1.2 / 1.3.

HTTP API: [`docs/API.md`](docs/API.md) · OpenAPI: [`docs/openapi.yaml`](docs/openapi.yaml) · curl: [`scripts/api-examples.sh`](scripts/api-examples.sh)

---

## CLI

```text
zator install [-port 17682] [-password …] [-no-auth -listen-all|-host 127.0.0.1]
zator webui   [-port …] [-host …] [-listen-all]
zator update | status | version
zator uninstall -y [--purge]
zator auth status|set-password|enable|disable
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

## Офлайн

1. На машине с интернетом: `./scripts/pack-offline.sh`
2. Скопируйте бандл на устройство
3. `ZATOR_BUNDLE_DIR=/path/to/bundle zator install -password '…'`

Бандл обязан содержать `SHA256SUMS` + `SHA256SUMS.sig`. Insecure-режим для zapret **не** расширяется.

---

## Разработка

Go **1.22+** · Node.js **22+**

```sh
go test ./...
go build -o bin/zator ./cmd/zator
cd webui-src && npm ci && npm run check && npm run build   # → assets/webui/
```

После правок UI обязателен `npm run build`. На Windows возможен host drift — сверяйте с Linux / artifact `webui-assets`.  
Паритет: [`docs/PARITY.md`](docs/PARITY.md) · backlog: [`Roadmap.md`](Roadmap.md)

---

## Безопасность

- Порт **17682** по умолчанию на всех интерфейсах — на VPS нужен пароль (или firewall / proxy)
- Не используйте `-no-auth` / `-listen-all` на машинах из интернета
- Обновления только с доверенного `ZATOR_OWNER` / `ZATOR_REPO`
- Install/update **отклоняют** релиз без валидной подписи `SHA256SUMS.sig`

Уязвимости — через [SECURITY.md](.github/SECURITY.md), не в публичные Issues.

---

## Лицензия

[GPL-3.0](LICENSE). Идея bash-zator (AloofLibra), реализация на Go.  
Обход DPI — zapret2 / nfqws2 ([bol-van/zapret](https://github.com/bol-van/zapret)).
