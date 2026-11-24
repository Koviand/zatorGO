# ZatorGO HTTP API

Канонические пути: `/api/<name>`. Алиасы `/cgi-bin/<name>.cgi` сохранены для совместимости.

Оба метода `GET` и `POST` принимают одинаковые query/form-параметры там, где ниже не указано иное. Ответы — JSON (`Content-Type: application/json`).

**Envelope ошибок (RM0a):** `{"ok":false,"error":"…","code":"…","detail"?:…}`. Поле `error` сохранено для старых клиентов; `code` — стабильная машинная метка (`unauthorized`, `csrf`, `conflict`, `busy`, `validation`, `not_found`, `rate_limit`, `unavailable`, `timeout`, `internal`, …). У `/api/service` по-прежнему бывает `detail`.

## Авторизация

При `auth_enabled=true`: cookie `zator_session`, `Authorization: Bearer <session_token>`, или HTTP Basic с паролем. Статика SPA (index/js/css) доступна без cookie.

Лимит попыток входа (пароль `/api/login` и HTTP Basic): после 8 неудач с одного `RemoteAddr` — HTTP **429** на ~5 минут. Заголовок `X-Forwarded-For` не учитывается. Валидная session-cookie / Bearer при блокировке IP продолжают работать.

Один глобальный `session_token`: успешный `/api/login` и `/api/logout` ротируют токен для всех клиентов. Cookie живёт **7 дней**; флаг `Secure` ставится только при TLS или `X-Forwarded-Proto: https` (голый HTTP на роутере — без Secure).

## CSRF и клиент

Мутирующие запросы (не GET/HEAD) с заголовком `Origin`, не совпадающим с `Host`, отклоняются (**403**), если нет `X-Zator-Client` / `X-Requested-With`. Web UI всегда шлёт `X-Zator-Client: webui`.

Запись `config_raw` и full-restore проверяют безопасный shell-формат config; перед `config_raw` пишется `config.bak`.

## Обновления и офлайн

Обновление бинарника с GitHub (`ApplyGitHubBinary` / `zator-launcher.sh`) требует ассет `SHA256SUMS` и совпадение контрольной суммы; без суммы или при ошибке её загрузки обновление отклоняется.

`update_apply` / `zator update` требуют успешный pre-update backup (+ снимок `bin/zator`); при ошибке обновление не продолжается. Откат не использует устаревший маркер предыдущего update. При успехе ответ может содержать `restart_required: true` (процесс Web UI перезапускается).

Офлайн-бандл: `ZATOR_BUNDLE_DIR` или `Z2R_OFFLINE=1` + `vendor/`+`zator-*.tar.gz` в cwd/рядом с бинарником. Без флага/каталога бандл из cwd не подхватывается.

## Коды ошибок (общие)

| HTTP | `code` | Когда |
| --- | --- | --- |
| 401 | `unauthorized` | нет/неверная сессия или пароль |
| 403 | `csrf` | CSRF (чужой Origin без `X-Zator-Client`) |
| 409 | `busy` | занят supersweep / auto-relock / repair (повторить позже) |
| 409 | `conflict` | прочий конфликт состояния (не «занятость» прогона) |
| 429 | `rate_limit` | rate-limit входа |
| 400 | `validation` | неверные параметры |
| 404 | `not_found` | неизвестный эндпоинт |
| 503 | `unavailable` | zapret2 не установлен / компонент недоступен |
| 413 | — | слишком большой upload бэкапа (>32 МиБ) |

## Эндпоинты

| Метод | Путь | Назначение |
| --- | --- | --- |
| GET/POST | `/api/status` | статус / профили / локи; `health.validator` = `ok\|degraded\|off`; у профиля: `lock_source` + `lock_origin` |
| GET/POST | `/api/state` | агрегат для UI; `include=capabilities,journal,health` — доп. блоки (RM0c) |
| GET/POST | `/api/capabilities` | версия + флаги фич + список эндпоинтов (RM0a) |
| GET/POST | `/api/events` | long-poll событий: `after=<seq>&timeout=<sec>&type=` (RM0b) |
| POST | `/api/repair` | узкий ремонт: `profile=` + `mode=auto\|relock\|recommendations\|sweep` (RM0c) |
| POST | `/api/set-lock` | фиксация стратегии |
| POST | `/api/clear-lock` | сброс лока |
| POST | `/api/service` | `action=start\|stop\|restart` zapret2; при ошибке `{"error","detail"}` |
| GET/POST | `/api/check` | сетевая проверка; batch: `profiles=1,2,4` → `{ok,checks:[…]}` (RM0c) |
| GET/POST | `/api/domains` | списки доменов (см. ниже) |
| GET/POST | `/api/backups` | бэкапы (см. ниже) |
| GET/POST | `/api/scopes` | client scopes |
| GET/POST | `/api/settings` | настройки (`setting=…`, см. ниже) |
| POST | `/api/blob-upload` | загрузка TLS/WG blob |
| POST | `/api/login` | вход по паролю (ротация session) |
| POST | `/api/logout` | выход (ротация session) |

Типы ответов UI: `webui-src/src/api/types.ts`. Клиент: `webui-src/src/api/endpoints.ts`.

### Волна 0 — расширенное API ([`Roadmap.md`](../Roadmap.md))

| ID | Статус | Содержание |
| --- | --- | --- |
| RM0a | ☑ | envelope + `/api/capabilities` + стабильные `code` |
| RM0b | ☑ | `/api/events` long-poll; UI SupersweepPanel + poll fallback |
| RM0c | ☑ | `/api/repair`, batch `/api/check`, `/api/state?include=` |
| RM0d | ☑ | [`openapi.yaml`](openapi.yaml) + curl [`scripts/api-examples.sh`](../scripts/api-examples.sh) |
| RM0e | ☑ | паритет `endpoints.ts` ↔ actions (см. таблицу ниже) |

### Примеры curl (RM0d)

Скрипт: `BASE=http://IP:17682 PASS=… ./scripts/api-examples.sh` (опц. `START_SWEEP=1`).
Вручную:

```bash
BASE=http://127.0.0.1:17682
JAR=$(mktemp)
H=(-H 'X-Zator-Client: curl' -b "$JAR" -c "$JAR")

# login (если auth_enabled)
curl -sS "${H[@]}" -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'password=SECRET' "$BASE/api/login"

curl -sS "${H[@]}" "$BASE/api/capabilities"
curl -sS "${H[@]}" "$BASE/api/state?scope=default&include=capabilities,journal"
curl -sS "${H[@]}" -d 'profile=1&strategy=1&scope=default' "$BASE/api/set-lock"
curl -sS "${H[@]}" "$BASE/api/check?profile=1&scope=default"
curl -sS "${H[@]}" "$BASE/api/events?timeout=0"
# supersweep: GET status; POST start только осознанно
curl -sS "${H[@]}" "$BASE/api/settings?setting=supersweep"
# curl -sS "${H[@]}" -d 'setting=supersweep&value=start&scope=default' "$BASE/api/settings"
# curl -sS "${H[@]}" "$BASE/api/events?timeout=25&type=supersweep"
```

Машиночитаемая схема ключевых payload’ов: [`docs/openapi.yaml`](openapi.yaml).

#### `/api/capabilities`

`{"ok":true,"version","goos","goarch","auth_required","features":{…},"endpoints":[…]}`.
Фичи включают `repair`, `events`, `batch_check`, `journal`, `auto_relock`, …

#### `/api/events`

Long-poll. Query: `after=<seq>` (события строго после), `timeout=<sec>` (default 25, max 55; `0` = сразу), `type=supersweep|repair|relock|…`.
Ответ: `{"ok":true,"mode":"long-poll","after","seq","events":[{type,state,profile?,message?,ts}]}`.

#### `/api/repair`

POST form: `profile=1..10`, `mode=auto|relock|recommendations|sweep` (default `auto`), `scope=`, опц. `top_k` для recommendations.
Ответ при старте: `{"ok":true,"started":true,"mode","previous_lock",…}`; `relock` — `async:true`.

#### Batch `/api/check`

`profiles=1,2,4` (или повтор `profile=`) → `{"ok":true,"checks":[…]}`. Одиночный `profile=` — прежний плоский ответ без обёртки.

### `/api/domains`

Query/form: `list=<name>`, `scope=…`.

Списки: `netrogat`, `custom_rkn`, `substring`, `netrogat_substring`, `googlevideo`.

| action | Назначение |
| --- | --- |
| *(GET без action)* | содержимое списка; у `custom_rkn` поле `strategy` только если ≥1; у googlevideo — `quality` |
| `explain` (GET, без `list`) | list/kind/role для `domain=` (netrogat exclude, custom/RKN/Discord/YT hostlist, substring, googlevideo, autohostlist) + `hostlist_mode` |
| `add` / `remove` / `rename` / `import` / `clear` | мутации hostlist; `import` опц. `url=` (только публичный http(s); localhost/RFC1918/metadata/CGNAT — deny) |
| `set_strategy` / `clear_strategy` / `check` | только `custom_rkn` |
| `refresh` / `refresh_ips` / `refresh_check` | googlevideo (ipv4 + короткий check профиля 2) |

Ответы мутаций с проверкой содержат `check` (сетевая проба выполняется **вне** блокировки списка).

### `/api/backups`

| action | Назначение |
| --- | --- |
| *(GET)* | список архивов |
| `create` | создать бэкап |
| `delete` | удалить (`name=…`) |
| `restore` | восстановить (`name=…`, `mode=smart\|lists\|full`) |
| `upload` | загрузить `.tar` (multipart; `name=` опционально) |
| `download` | скачать (`name=…`) |

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
| `recommendations_apply` | применить top-K рек. с TLS verify + rollback (`profile=`, `top_k=`) |
| `auto_relock` | монитор TLS→relock (`value=0\|1`, опц. `fails=`); GET того же имени — флаги |
| `bypass_flags` | GET-алиас пакета флагов: `auto_relock`, `auto_relock_fails`, `sweep_on_provider_change`, `sweep_schedule_hours`, `blob_fallback`, `journal_enabled` |
| `strategy_journal` | GET лента JSONL (`enabled`, `entries[]`); запись только при `journal_enabled=1` |
| `sweep_on_provider_change`, `sweep_schedule_hours`, `sweep_broken` | триггеры supersweep |
| `fix_discord` | Discord TCP (4) + voice (6): `tcp=ok\|fail`, `udp=configured\|fail\|skipped`, `udp_verified=false`, `result=ok\|partial\|fail` |
| `blob_fallback`, `journal_enabled` | fallback blob/clone; журнал JSONL (opt-in) |
| `strategies_feed` | подписанный каталог: verify → регистрация с dedupe; `registered`/`skipped`/`restarted` |
| `scope_preset`, `scope_copy_locks` | пресеты client-scope; copy: `scope=mark:N`, `undo=1` откатывает |
| `strategy_register` | ручная регистрация стратегии |
| `supersweep` | суперавтопрогон (`workers=` — yt\|gv\|ds\|rkn; пусто = все; мусор → 400, не полный прогон) |

Многие POST-ответы содержат `restarted` / `restart_required` и иногда `warning`.

### Паритет UI client ↔ API (RM0e)

Правило: новый endpoint / `setting=` → `types.ts` + `endpoints.ts` + эта таблица / OpenAPI в том же изменении.

| API | Клиент (`endpoints.ts`) | Заметки |
| --- | --- | --- |
| `/api/login` | `login()` | |
| `/api/logout` | `logout()` | |
| `/api/status` | `fetchStatus` | |
| `/api/state` | `fetchState(scope, include?)` | |
| `/api/capabilities` | `fetchCapabilities` | |
| `/api/events` | `fetchEvents` | |
| `/api/repair` | `repairProfile` | для RM5 |
| `/api/check` | `runCheck` / `profileCheck` / `batchCheck` | |
| `/api/set-lock` / `clear-lock` | `setLock` / `clearLock` | |
| `/api/service` | `serviceAction` | |
| `/api/scopes` | `fetchScopes` | |
| `/api/domains` | `domains.*` | `refresh_ips` → `domains.refresh_ips` |
| `/api/backups` | `backups.*` / `fetchBackups` | |
| `/api/blob-upload` | `uploadBlob` | |
| `setting=bypass_flags` GET | `fetchBypassFlags` | |
| `setting=strategy_journal` GET | `fetchStrategyJournal` | |
| `setting=recommendations` | `fetchRecommendations` | |
| `recommendations_apply` | `applySetting.recommendations_apply` | |
| `auto_relock` / journal / sweep_* / blob_fallback | `applySetting.*` | |
| `strategies_feed` | `applySetting.strategies_feed` | UI busy-slot: `BUSY_RESTART` (ожидание рестарта) |
| `fix_discord` / `sweep_broken` | `applySetting.*` | HTTP 409 `code=busy`, если supersweep/relock уже идёт |
| `scope_preset` / `scope_copy_locks` | `applySetting.scope_preset` / `scope_copy_locks` | |
| `supersweep` | `fetchSupersweep` / `supersweepStart` / `supersweepCancel` | |
| `strategy_register` | `registerStrategy` | |
| прочие toggles / blobs / ports / … | `fetchSetting.*` / `applySetting.*` | |

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
