# ZatorGO HTTP API

Канонические пути: `/api/<name>`. Алиасы `/cgi-bin/<name>.cgi` сохранены для совместимости.

Оба метода `GET` и `POST` принимают одинаковые query/form-параметры там, где ниже не указано иное. Ответы — JSON (`Content-Type: application/json`), ошибки обычно вида `{"error":"…"}`; у `/api/service` дополнительно бывает `detail`.

## Авторизация

При `auth_enabled=true`: cookie `zator_session`, `Authorization: Bearer <session_token>`, или HTTP Basic с паролем. Статика SPA (index/js/css) доступна без cookie.

Лимит попыток входа (пароль `/api/login` и HTTP Basic): после 8 неудач с одного `RemoteAddr` — HTTP **429** на ~5 минут. Заголовок `X-Forwarded-For` не учитывается. Валидная session-cookie / Bearer при блокировке IP продолжают работать.

Один глобальный `session_token`: успешный `/api/login` и `/api/logout` ротируют токен для всех клиентов. Cookie живёт 30 дней, флаг `Secure` не выставляется (типичный HTTP на роутере).

## CSRF и клиент

Мутирующие запросы (не GET/HEAD) с заголовком `Origin`, не совпадающим с `Host`, отклоняются (**403**), если нет `X-Zator-Client` / `X-Requested-With`. Web UI всегда шлёт `X-Zator-Client: webui`.

Запись `config_raw` и full-restore проверяют безопасный shell-формат config; перед `config_raw` пишется `config.bak`.

## Обновления и офлайн

Обновление бинарника с GitHub (`ApplyGitHubBinary` / `zator-launcher.sh`) требует ассет `SHA256SUMS` и совпадение контрольной суммы; без суммы или при ошибке её загрузки обновление отклоняется.

`update_apply` / `zator update` требуют успешный pre-update backup (+ снимок `bin/zator`); при ошибке обновление не продолжается. Откат не использует устаревший маркер предыдущего update. При успехе ответ может содержать `restart_required: true` (процесс Web UI перезапускается).

Офлайн-бандл: `ZATOR_BUNDLE_DIR` или `Z2R_OFFLINE=1` + `vendor/`+`zator-*.tar.gz` в cwd/рядом с бинарником. Без флага/каталога бандл из cwd не подхватывается.

## Коды ошибок (общие)

| Код | Когда |
| --- | --- |
| 401 | нет/неверная сессия или пароль |
| 403 | CSRF |
| 409 | конфликт состояния (занят supersweep, конфликт режимов fallback↔auto_mode, …) |
| 429 | rate-limit входа |
| 413 | слишком большой upload бэкапа (>32 МиБ) |

## Эндпоинты

| Метод | Путь | Назначение |
| --- | --- | --- |
| GET/POST | `/api/status` | статус / профили / локи (`service_warnings`, `nfqws2_errors`, `post_update_verify`, `block_full_restore`) |
| GET/POST | `/api/state` | агрегат для UI (status + settings) |
| POST | `/api/set-lock` | фиксация стратегии |
| POST | `/api/clear-lock` | сброс лока |
| POST | `/api/service` | `action=start\|stop\|restart` zapret2; при ошибке `{"error","detail"}` |
| GET/POST | `/api/check` | сетевая проверка (`profile=2&media_url=` — Range 16K/64K googlevideo) |
| GET/POST | `/api/domains` | списки доменов (см. ниже) |
| GET/POST | `/api/backups` | бэкапы (см. ниже) |
| GET/POST | `/api/scopes` | client scopes |
| GET/POST | `/api/settings` | настройки (`setting=…`, см. ниже) |
| POST | `/api/blob-upload` | загрузка TLS/WG blob |
| POST | `/api/login` | вход по паролю (ротация session) |
| POST | `/api/logout` | выход (ротация session) |

Типы ответов UI: `webui-src/src/api/types.ts`. Клиент: `webui-src/src/api/endpoints.ts`.

### `/api/domains`

Query/form: `list=<name>`, `scope=…`.

Списки: `netrogat`, `custom_rkn`, `substring`, `netrogat_substring`, `googlevideo`.

| action | Назначение |
| --- | --- |
| *(GET без action)* | содержимое списка; у `custom_rkn` поле `strategy` только если ≥1 |
| `add` / `remove` / `rename` / `import` / `clear` | мутации hostlist |
| `set_strategy` / `clear_strategy` / `check` | только `custom_rkn` |
| `refresh` / `refresh_ips` | googlevideo (в т.ч. ipv4 для WSSIZE) |

Ответы мутаций с проверкой содержат `check` (сетевая проба выполняется **вне** блокировки списка).

### `/api/backups`

| action | Назначение |
| --- | --- |
| *(GET)* | список архивов |
| `create` | создать бэкап |
| `delete` | удалить (`file=…`) |
| `restore` | восстановить (`file=…`, `mode=smart\|lists\|full`) |
| `upload` | загрузить `.tar` (multipart) |
| `download` | скачать (`file=…`) |

`mode`: по умолчанию `smart` (списки и локи; живой config zapret2 не затирается целиком). `full` может быть заблокирован после обновления (`block_full_restore`). Ответ restore: `files`, `restarted` / `restart_required`.

### `/api/settings`

GET: `setting=<name>` — чтение. POST: `setting=<name>&value=…` (+ доп. поля).

Частые `setting` (чтение и/или запись):

| setting | Заметки |
| --- | --- |
| `auto_mode` / `auto_mode_state`, `hostlist` / `hostlist_state`, `fallback` / `fallback_state`, `udp_games` / `udp_games_state`, `rst_guard` / `rst_guard_state`, `reasm` / `reasm_state`, `quic443` / `quic443_state`, `dns_desync` / `dns_desync_state` | переключатели `value=0\|1`; конфликт fallback↔auto → **409** |
| `wg_state`, `wg_blob`, `wg_repeats` | WireGuard |
| `tls_blob`, `tls_blob_profile`, `fake_mode`, `clone_size` | TLS / профили |
| `ports`, `ports_add`, `ports_remove` | NFQUEUE-порты |
| `provider`, `provider_set`, `provider_redetect` | провайдер |
| `fwtype`, `flowoffload` | firewall |
| `ifaces` / `iface` / `interfaces` | WAN/LAN; `action=redetect` |
| `client_scope` | scopes |
| `wssize_ramp` | WSSIZE |
| `config_raw` / `config` | сырой конфиг (перед записью — `config.bak`) |
| `nfqws2_errors` / `errors` | журнал ошибок |
| `update_check`, `update_apply` | обновления |
| `recommendations` | рекомендации |
| `strategy_register` | ручная регистрация стратегии |
| `supersweep` | суперавтопрогон (см. ниже) |

Многие POST-ответы содержат `restarted` / `restart_required` и иногда `warning`.

### `setting=supersweep`

GET — снимок прогресса (формат `internal/sweep.Report`):

```json
{
  "available": true,
  "state": "idle|starting|running|applying|done|cancelled|interrupted",
  "active": false,
  "source": "none|live|saved",
  "started": 0,
  "updated": 0,
  "scope": "default",
  "phase": "",
  "completed": 0,
  "total": 0,
  "workers": [],
  "applied": [],
  "domains": []
}
```

`workers[]`: `name`, `label`, `profile`, `completed`, `total`, `ok`, `warn`, `fail`, `current_strategy`, `current_target`, `greens`, `warns`, `applied`.

`applied[]`: `{ "kind": "profile|domain", "target": "…", "strategy": N }`.

`domains[]`: `{ "domain", "best_strategy", "verdict": "ok|warn|fail", "applied" }`.

POST: `value=start|status|cancel`; для `start` — `scope`, опционально `domain`, `tls_pref`.
