# Roadmap ZatorGO

Конкретный backlog после закрытия [`Improvments.md`](Improvments.md) и кода [`Bypass.md`](Bypass.md).  
Цель: **сначала** расширить взаимодействие с HTTP API (контракт, события, клиенты), затем довести Bypass до релиза, закрыть полевой QA, укрепить install/CI и UX Overview.

**Ограничения (как в проекте):**
- без новых deps в `go.mod` / `package.json` без явного «да»;
- HTTP/CLI — только обратно совместимые расширения;
- не ослаблять auth, SHA256 fail-closed, Ed25519 `SHA256SUMS.sig`, CSRF, shell-safe;
- `assets/webui/` только через `npm run build` (для CI/release — предпочтительно Linux-сборка);
- один коммит = одно логическое изменение (Conventional Commits, английский).

**Связанные документы**

| Документ | Роль |
| --- | --- |
| [`Improvments.md`](Improvments.md) | Инфра — код закрыт; долг колонки «Коммит» |
| [`Bypass.md`](Bypass.md) | DPI-фичи BP1–BP12 — код в основном ☑, релиз/поле — нет |
| [`docs/PARITY.md`](docs/PARITY.md) | Паритет + ручной QA ☐ |
| [`docs/API.md`](docs/API.md) | Контракт API при новых полях |
| [`.github/SECURITY.md`](.github/SECURITY.md) | Цепочка доверия релизов |

**Легенда статуса:** ☐ todo · ☑ done · ◐ partial  
**Приоритет:** P0 сейчас · P1 скоро · P2 позже

---

## Волны

| Волна | ID | Фокус |
| --- | --- | --- |
| **0** | **RM0a–RM0e** | **Расширенное взаимодействие API (прежде всего)** |
| 1 | RM1–RM4, RM10 | Bypass → релиз; QA; политика webui; долг docs |
| 2 | RM5–RM7, RM11–RM12 | Overview «Починить»; strategies feed; install UX |
| 3 | RM8–RM9, RM13–RM16 | GV mid-stream; zapret trust; offline; Node/CI hygiene |

Не смешивать в одном коммите: API-контракт, нарезку Bypass, правку CI и продуктовую UX-фичу.  
**Порядок:** волна 0 → 1 → 2 → 3 (P2 волны 3 не начинать, пока RM0 и RM1–RM2 не закрыты или явно отложены).

---

## Волна 0 — расширенное взаимодействие API (прежде)

Контракт сегодня: multiplexor `/api/settings?setting=…`, CGI-алиасы, JSON без единой схемы ошибок/событий. Цель волны — сделать API удобным для **Web UI, CLI и внешних скриптов** без ломки существующих путей.

### RM0a — Единый контракт ответов и capabilities P0

**Сделать:**
- [x] задокументировать и соблюдать envelope: успех с полезной нагрузкой; ошибка `{"ok":false,"error":"…","code":"…","detail"?:…}` (старые `{"error"}` без `ok` — ещё принимать в UI);
- [x] `GET /api/capabilities` (или поле в `/api/state`): версия zator, флаги фич (`auto_relock`, `strategies_feed`, `journal`, `blob_fallback`, `media_check`, …);
- [x] стабильные `code` для частых кейсов: `unauthorized`, `csrf`, `conflict`, `busy`, `validation`, `not_found`, `rate_limit`;
- [x] обновить [`docs/API.md`](docs/API.md) + `webui-src/src/api/types.ts` / `client.ts` (без новых npm-deps);
- [x] тесты: capabilities; envelope/`code`; CSRF → `csrf`.

**Критерий:** внешний скрипт по `capabilities` понимает, какие Bypass-actions доступны; UI не парсит «голый» error вразнобой.

**Не делать:** ломать CGI-алиасы и старые успешные формы ответов без `ok:true`.

---

### RM0b — События прогресса (SSE или long-poll) P0

**Проблема:** supersweep / update_apply / strategies_feed / repair крутятся через частый poll `settings?setting=supersweep` и таймаут fetch 120s.

**Сделать:**
- [x] `GET /api/events` — long-poll: типы `supersweep`, `repair`, `relock`, …;
- [x] сохранить текущий poll как fallback (обратная совместимость);
- [x] Web UI: подписка на events для SupersweepPanel (Updates/Overview — позже / RM0e);
- [x] таймаут long-poll (default 25s, max 55s); без auth — 401 как у остальных API;
- [x] `docs/API.md` + тест long-poll wake.

**Критерий:** supersweep в UI обновляется без «слепого» интервала 1–2 с; обрыв SSE не клинит UI (fallback на poll).

---

### RM0c — Операции ремонта и батч-check P0

**Сделать:**
- [x] `POST /api/repair`: `profile=` / `scope=`; стратегия `relock|recommendations|sweep` (default: auto по policy); ответ с `started`, `mode`, `previous_lock`;
- [x] `POST /api/check` batch: `profiles=1,2,4` → массив результатов (обратная совместимость: один profile как сейчас);
- [x] `GET /api/state?include=journal,health,capabilities` — меньше round-trips для Overview;
- [x] API готов для RM5 (кнопка «Починить» → `repairProfile()` в `endpoints.ts`).

**Критерий:** один запрос инициирует узкий repair; batch-check не ломает одиночный вызов.

---

### RM0d — Машиночитаемая схема + примеры клиентов P1

**Сделать:**
- [x] `docs/openapi.yaml` (ручной, без генераторов/deps) ключевых payload’ов (`status`, `state`, `supersweep`, `error`, wave0);
- [x] раздел в API.md: curl-примеры login → state → set-lock → check → supersweep/events;
- [x] скрипт `scripts/api-examples.sh` (CLI `zator api` не добавляли — без раздувания);
- [x] синхронизация Bypass-settings: `bypass_flags`, `strategy_journal` в таблице API.md.

**Критерий:** новый интегратор поднимает сессию и читает state по доке за <15 минут без чтения Go-кода.

---

### RM0e — Паритет UI↔API и «дыры» клиента P1

**Сделать:**
- [x] аудит: таблица паритета в [`docs/API.md`](docs/API.md); закрыты `login`, `scope_copy_locks`, `domains.refresh_ips`, repair/events/capabilities;
- [x] Bypass в UI-клиенте: feed/journal/repair/bypass_flags обёрнуты;
- [x] единый busy-ключ `BUSY_RESTART` / `withBusyRestart` для update_apply и strategies_feed;
- [x] checklist: «новый endpoint → types + endpoints + API.md» (в API.md RM0e + ниже).

**Критерий:** нет «скрытых» Bypass-actions, доступных только через сырой `fetch` в обход `endpoints.ts`.

---

## Волна 1 — доставка и поле

### RM1 — Нарезать Bypass-WIP в коммиты и релиз P0

**Проблема:** BP-код (relock, recommendations_apply, feed, journal, blob_fallback, …) лежит локально / не в tagged release.

**Сделать:**
- [ ] сверить рабочее дерево с критериями [`Bypass.md`](Bypass.md) (особенно BP1 — в сводке ☑, в критериях ещё есть ☐);
- [ ] один BP / один Conventional Commit (`feat(bypass): …`);
- [ ] `go test ./...`, `go vet ./...`, `npm run check` + `npm run build`, закоммитить `assets/webui/`;
- [ ] обновить `docs/API.md` под новые actions/поля;
- [ ] tag + release (подписанный `SHA256SUMS.sig`);
- [ ] в [`Bypass.md`](Bypass.md) проставить реальные коммиты / закрыть оставшиеся ☐ критериев.

**Критерий:** `vX.Y.Z` на GitHub содержит Bypass-фичи; launcher ставит эту версию.

---

### RM2 — Ручной QA по [`docs/PARITY.md`](docs/PARITY.md) P0

**Сделать (чеклист с устройства):**
- [ ] Ubuntu 22/24: launcher → Web UI → start zapret2;
- [ ] Debian/cloud VPS: YouTube + Discord check;
- [ ] OpenWRT: procd + nftables + wssize-ramp;
- [ ] Keenetic Entware: S90, ndm hook, iptables;
- [ ] Merlin: `firewall-start` hook;
- [ ] MIPS или arm: бинарник стартует, UI отвечает, locks атомарны;
- [ ] Lexra (если есть железо): first install + `zator update` не меняет endian (mips BE).

**Критерий:** все пункты в PARITY «Ручной QA» отмечены ☑ или заведены баги с repro.

---

### RM3 — Политика sync `assets/webui` P0

**Проблема:** Vite Windows ≠ Ubuntu → CI embed-check красный; release уже тянет Linux-артефакт.

**Сделать:**
- [x] зафиксировать в `.cursor/rules/30-build-artifacts.mdc` / README: перед коммитом UI либо Linux build, либо артефакт `webui-assets` из CI;
- [x] CI: при fail embed-check — `git diff --ignore-cr-at-eol` + how-to;
- [ ] опционально: job `webui-sync` на PR, который комментирует «нужен sync assets» (отложено).

**Критерий:** два подряд push с UI-изменениями на `main` не валят webui job из‑за host drift.

---

### RM4 — Закрыть долг колонки «Коммит» в Improvements P1

**Сделать:**
- [ ] для каждого ID в [`Improvments.md`](Improvments.md) либо проставить хеш/тег, либо `n/a (squashed)` / ссылка на PR;
- [ ] одна строка в конце: «план архивный, новый backlog — Roadmap.md».

**Критерий:** нет «код ☑ / коммит ☐» без пояснения.

---

## Волна 2 — операторский контур

### RM5 — Overview: «Что сломалось» + «Починить» P0

**Зависимость:** RM0c (`/api/repair` или эквивалент).

**Сделать:**
- [x] блок на Overview: degraded profiles, `lock_origin` / lock / source, validator, health warning;
- [x] CTA «Починить» → `POST /api/repair` (`mode=auto`);
- [x] не запускать полный supersweep всех профилей с этой кнопки;
- [x] подписка на `type=repair` events (long-poll) + toast на done/fail.

**Критерий:** на degraded-профиле один клик инициирует узкий repair; fail не молчит (toast + journal).

---

### RM6 — Signed `strategies-extra.tsv` в каждом release P1

**Сделать:**
- [ ] в `release.yml` всегда публиковать `strategies-extra.tsv` + `.sig` (не только «если файл есть»);
- [ ] процесс обновления каталога (кто правит TSV, как тестировать);
- [ ] Web UI / `strategies_feed`: понятная ошибка при отсутствии/битой подписи;
- [ ] ссылка из SECURITY.md.

**Критерий:** latest release всегда содержит подписанный feed; install/update может его применить.

---

### RM7 — BP1 auto-relock: добить полевые критерии P0

**Сделать (даже если код ☑):**
- [ ] искусственный fail check → ≤N циклов → узкий подбор только этого профиля;
- [ ] успех → новый lock + TLS ok; неуспех → старый lock + warning;
- [ ] `auto_relock=0|1` в settings и status JSON;
- [ ] rate-limit / один профиль за раз (не «дёргать» роутер).

**Критерий:** все ☐ в секции BP1 [`Bypass.md`](Bypass.md) закрыты; Keenetic или VPS smoke.

---

### RM10 — Неинтерактивный install в README P1

**Сделать:**
- [ ] в README: `./zator-launcher.sh -password '…'` и запрет pipe без TTY без флага;
- [ ] не предлагать `-no-auth` как «удобный дефолт» (только с предупреждением рисков).

**Критерий:** новый пользователь без TTY ставит с первого раза по доке.

---

### RM11 — Install: regression на формат апстрим `sha256sum.txt` P1

**Сделать:**
- [ ] тест/job: разбор **реального** (или зафиксированного fixture) `sha256sum.txt` fork/official без строки `*.tar.gz`;
- [ ] опционально nightly: fetch latest sums + mock archive → `DownloadAndInstall`;
- [ ] не ослаблять fail-closed.

**Критерий:** регресс «сумма только на binaries/» ловится в CI, не в поле.

---

### RM12 — Session после `zator auth set-password` P2

**Сделать:**
- [ ] тест: смена пароля CLI → старый cookie/Bearer отвергается без рестарта webui (mtime reload);
- [ ] при необходимости явная ротация `session_token` в `SetPassword`.

**Критерий:** один интеграционный тест в `app`/`webapi` зелёный.

---

## Волна 3 — качество обхода и платформа

### RM8 — Googlevideo mid-stream в UI P1

**Опора:** [`test_googlevideo.com.md`](test_googlevideo.com.md).

**Сделать:**
- [ ] Overview/check: не только TLS к корню, но 16K/64K / намёк на WSSIZE lite;
- [ ] связать с `check?profile=2&media_url=…` (уже в API);
- [ ] короткий copy в UI: «TLS ok ≠ видео ok».

**Критерий:** оператор видит mid-stream fail без чтения markdown runbook.

---

### RM9 — Усиление доверия к zapret-архиву P2

**Сделать (выбрать один путь, без лишних deps):**
- [ ] доп. якорь: GitHub Releases asset `digest` **или**
- [ ] vendor-бандл zapret с вашей Ed25519-подписью в offline pack;
- [ ] документ в SECURITY: «zapret sums ≠ signed Zator SUMS».

**Критерий:** MITM, подменивший и tar, и `sha256sum.txt`, не проходит хотя бы на одном из якорей.

---

### RM13 — Offline-бандл first-class P2

**Сделать:**
- [ ] README: сценарий «роутер без GitHub» (`pack-offline.sh`, `ZATOR_BUNDLE_DIR` / `Z2R_OFFLINE`);
- [ ] проверка SHA256 vendor zapret в офлайне (уже требует SHA или AllowInsecure — не расширять insecure);
- [ ] smoke: ApplyBundle на чистом дереве.

**Критерий:** установка по доке без исходящего GitHub (кроме заранее скачанного бандла).

---

### RM14 — Node 22 LTS в Actions P2

**Сделать:**
- [ ] `ci.yml` / `release.yml`: Node 20 → 22;
- [ ] `npm ci` + `check` + `build` зелёные.

**Критерий:** нет annotation «Node.js 20 is deprecated» на release.

---

### RM15 — Подсказка про firewall порта 17682 P2

**Сделать:**
- [ ] Overview или Settings → Updates/Security: «UI слушает все интерфейсы; ограничьте доступ»;
- [ ] ссылка на SECURITY.md;
- [ ] опционально (отдельный коммит): bind/LAN-only setting — только с обратной совместимостью и QA Entware.

**Критерий:** новый install видит предупреждение при auth on + публичный интерфейс.

---

### RM16 — Settings: группировка панелей P2

**Сделать:**
- [ ] группы «Безопасность / Сеть / Обход / Служебное» в SettingsView (без ломки `/settings/:panel` deep links);
- [ ] не раздувать первый viewport Overview статистикой.

**Критерий:** deep links панелей живы; `npm run check` ок.

---

## P2 / later (не терять)

| ID | Тема | Заметка |
| --- | --- | --- |
| RM17 | Busy/timeout UX | унифицировать toast для update_apply / strategies_feed / supersweep (client 120s) |
| RM18 | webapi boundaries | не плодить пакеты; держать settings / bypass / domains границами |
| RM19 | VPS panels 3x-ui/marzban | out of scope ядра (PARITY); только если отдельный epic |
| RM20 | Redis stats | out of scope |
| RM21 | ANSI-меню | removed — не возвращать |

---

## Сводка

| ID | Тема | P | Статус |
| --- | --- | --- | --- |
| RM0a | Envelope + `/api/capabilities` | P0 | ☑ |
| RM0b | Events long-poll | P0 | ☑ |
| RM0c | `/api/repair` + batch check | P0 | ☑ |
| RM0d | OpenAPI/схема + curl/CLI examples | P1 | ☑ |
| RM0e | Паритет UI client ↔ API | P1 | ☑ |
| RM1 | Bypass-WIP → коммиты + release | P0 | ☐ |
| RM2 | Ручной QA PARITY | P0 | ☐ |
| RM3 | Политика assets/webui | P0 | ☑ |
| RM4 | Долг коммитов Improvements | P1 | ☐ |
| RM5 | Overview «Починить» | P0 | ☑ |
| RM6 | strategies feed в каждом release | P1 | ☐ |
| RM7 | BP1 полевые критерии | P0 | ☐ |
| RM8 | GV mid-stream UI | P1 | ☐ |
| RM9 | Zapret trust anchor | P2 | ☐ |
| RM10 | README non-interactive install | P1 | ☐ |
| RM11 | sha256sum.txt regression | P1 | ☐ |
| RM12 | Session после set-password | P2 | ☐ |
| RM13 | Offline bundle docs | P2 | ☐ |
| RM14 | Node 22 в Actions | P2 | ☐ |
| RM15 | Firewall hint :17682 | P2 | ☐ |
| RM16 | Settings grouping | P2 | ☐ |
| RM17–RM21 | later / out of scope | — | ☐ |

---

## Как вести работу

1. **Волна 0 (RM0a–RM0e) закрыта** — далее P0 волны 1 (RM1–RM2); потом RM5 на базе RM0c.
2. Перед merge: `go test ./...`, `go vet ./...`; UI → `npm run check` + `npm run build` (+ sync `assets/webui/`).
3. Новый endpoint / `setting=` → `types.ts` + `endpoints.ts` + `docs/API.md` (+ при необходимости `openapi.yaml`) в том же изменении.
4. Обновлять колонку **Статус** в сводке (`☐` → `◐` → `☑`).
5. Issues на GitHub (опционально): title = `RMN: …`, body = секция из этого файла; лейблы `P0|P1|P2`, `roadmap`.
6. Баги с полевого QA → отдельные `fix:` коммиты, в Roadmap только ссылка в RM2.

### Шаблон GitHub Issue

```markdown
## RMN — <title>

**Приоритет:** P0|P1|P2  
**Волна:** 1|2|3  
**Документ:** https://github.com/Koviand/zatorGO/blob/main/Roadmap.md

### Сделать
- [ ] …

### Критерий готовности
…

### Не делать
- новые deps без согласования
- ломка API/CLI
```
