# Чеклист паритета: bash zator → Go zator

Референс: ветка `feat/googlevideo-fix` репозитория AloofLibra/zator.
Цель: весь управляющий функционал доступен через bootstrap CLI + Web UI.

## Bootstrap / лаунчер

| Функция | Статус | Где |
| --- | --- | --- |
| Определение arch + скачивание бинарника | done | `scripts/zator-launcher.sh` |
| `zator install` (каталоги, assets, password auth) | done | `cmd/zator`, `internal/app` |
| Установка zapret2 без `install_easy.sh` | done | `internal/zapret`, `-zapret` |
| Service hooks systemd/procd/Entware/Merlin/Keenetic | done | `internal/service` |
| `zator webui` + URL/token | done | `cmd/zator`, `internal/webapi` |
| `zator update` / self-update tar | done | `internal/deploy` |
| Offline bundle (`Z2R_OFFLINE`, vendor/) | done | `internal/deploy` |
| Cross-build arm/arm64/mips*/ppc/x86/x86_64 | done | `scripts/crossbuild.sh`, CI |

## Web UI / API (контракт CGI)

| Эндпоинт | Статус |
| --- | --- |
| status / state | done |
| set-lock / clear-lock | done |
| service start/stop/restart | done |
| check (TLS 1.2/1.3) | done |
| domains (netrogat/custom/substring) | done |
| backups create/list/download/delete/upload | done |
| scopes | done |
| settings (~35) | done |
| supersweep start/status/cancel | done (`NfqwsProbe`; `NoopProbe` в тестах) |
| update_check | done (`internal/deploy`) |
| recommendations | done (`internal/telemetry`) |

## Стратегии / orchestra

| Функция | Статус |
| --- | --- |
| locked.tsv / locked.manual.tsv / scopes | done |
| blob/sni/mode/clonesize overrides | done |
| auto-sweep | done (`internal/sweep`) |
| supersweep coordinator + progress files | done |
| Изолированный nfqws2 probe | done (`sweep.NewNfqwsProbe` в webapi supersweep) |

## Сеть / платформа

| Функция | Статус |
| --- | --- |
| Platform detect VPS/OpenWRT/Entware | done |
| Firewall client-scope | done |
| wssize-ramp rules + custom.d | done |
| SuperSweep marks/chains | done |
| Googlevideo central list + collector | done |
| DNS sniffer C binary orchestration | done (режим sniffer/ubus) |
| Provider ASN detect | done |
| Telemetry consent + send | done (токен из env) |
| Watchdog / strategy-validator daemon | done (unit `zator-strategy-validator`; `health.validator` = `ok\|degraded\|off` — unit alive + relock degraded / queue FAIL; Entware nfqws2 respawn — `zator-nfqws-watchdog`; Web UI auto-relock) |
| Pre-update backup + post-update verify | done (`CreatePreUpdateBackup`, `VerifyAfterUpdate`) |
| Config inter-process write-lock | done (`fsutil.WithDirLock` in `Config.SaveTo`) |
| Transactional backup restore | done (snapshot undo in `RestoreFile`) |
| VPS panels 3x-ui/marzban/wg/3proxy | out of scope для ядра; можно добавить later |
| Интерактивное ANSI-меню | removed (breaking) |

## Данные / Lua (не на Go)

| Компонент | Статус |
| --- | --- |
| Lua zator (`locked.lua`, wssize-ramp, …) | shipped as assets |
| bol-van / nfqws2 / ip2net / mdig | external download |
| Upstream Lua desync API | untouched |
| Server stats Redis (`tools/stats`) | out of scope |

## Ручной QA (роутер / VPS)

- [ ] Ubuntu 22/24: `curl …/zator-launcher.sh | sh`, открыть Web UI, start zapret2
- [ ] Debian cloud VPS: install + check YouTube/Discord
- [ ] OpenWRT: procd unit, nftables, wssize-ramp
- [ ] Keenetic Entware: S90, ndm hook, iptables
- [ ] Merlin: firewall-start hook
- [ ] MIPS/arm бинарник стартует, Web UI отвечает, locks пишутся атомарно
- [ ] Offline bundle на устройстве без GitHub
- [ ] Update path: latest.json → apply keep-if-exists
