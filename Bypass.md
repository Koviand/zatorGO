# План усиления обхода DPI (ZatorGO)

Документ описывает **продуктовые и технические улучшения эффективности обхода**. Инфраструктурный план Improvements закрыт и удалён; актуальный backlog — [`Roadmap.md`](Roadmap.md).

**Роль ZatorGO:** ставить, конфигурировать и обслуживать zapret2 (`nfqws2`). Ядро desync / Lua API bol-van **не переписывается** — ZatorGO ускоряет подбор, фиксацию, мониторинг и восстановление вокруг него.

**Цель плана:** меньше ручного перебора стратегий, быстрее реакция на смену DPI по ASN, устойчивее YouTube / Discord / RKN-кейсы, без ослабления auth и цепочки обновлений.

---

## Ограничения (обязательны)

- Не добавлять зависимости в `go.mod` / `package.json` без явного согласования.
- Не менять публичные HTTP/CLI-контракты без обратной совместимости (новые поля/actions — ок; ломать старые — нет).
- Не ослаблять auth, SHA256 fail-closed, подпись `SHA256SUMS.sig`, CSRF, shell-safe config.
- Не вносить «универсальную стратегию на всех» — DPI сегментирован; путь **provider/ASN → рекомендации → lock → verify**.
- Новые `--lua-desync=…` только из апстрима / проверенных ассетов; ZatorGO — доставка и UX.
- `assets/webui/` только через `npm run build` из `webui-src/`.
- Один коммит = одно логическое изменение (Conventional Commits, английский).

---

## Текущий каркас (что уже есть)

> Код BP1–BP12 закрыт в `main` / `v0.1.43+`. Полевой QA устройств — [`Roadmap.md`](Roadmap.md) RM2 / [`docs/PARITY.md`](docs/PARITY.md).

| Возможность | Где |
| --- | --- |
| Профили 1…N + `locked.tsv` / scopes | `internal/orchestra`, `internal/webapi` |
| Стратегии в `NFQWS2_OPT` | `assets/config.default` |
| Списки YT / Discord / RKN / custom | `assets/extra_strats`, `internal/domains` |
| Supersweep + изолированный `NfqwsProbe` | `internal/sweep`, `webapi/supersweep.go` |
| TLS 1.2 / 1.3 check | `internal/netcheck`, `/api/check` |
| Recommendations по провайдеру | `internal/telemetry` |
| Googlevideo seed + `auto_locked.tsv` | `internal/googlevideo`, `internal/app/gv_seed.go` |
| Fake / clone / TLS·WG blob | `orchestra` overrides, settings |
| QUIC443, RST guard, reasm, wssize-ramp | settings toggles + Lua assets |
| Health Overview (`overall`, watchdog, validator) | `webapi/status.go` → `healthPayload` |
| Watchdog / strategy-validator | **done** (`health.validator` ok\|degraded\|off; полевой smoke → Roadmap RM2) |

Узкие места сегодня: **адаптация после смены DPI**, **автовосстановление**, **охват сервисов сверх YT/Discord**, **применение рекомендаций без ручного клика по каждой стратегии**.

---

## Рекомендуемый порядок эпиков

| Волна | ID | Цель |
| --- | --- | --- |
| 1 | BP1 → BP2 → BP3 | «вчера работало» чинится само / в один клик |
| 2 | BP4 → BP5 → BP6 | YouTube / Discord / QUIC без ручной магии |
| 3 | BP7 → BP8 → BP9 | Списки, scopes, blob-fallback |
| 4 | BP10 → BP11 → BP12 | Устойчивость ночью + каталог стратегий |

---

## Волна 1. Автоадаптация

### BP1 — Health → auto-relock профиля

**Проблема:** Overview знает `health.overall`, check умеет TLS 1.2/1.3, но после деградации DPI лок остаётся «вчерашним» до ручного supersweep / смены стратегии.

**Идея:** локальный монитор (в процессе Web UI или лёгкий watchdog-хук):

1. Для профилей с ручным/рекомендованным локом периодически (или по событию) гонять узкий check.
2. N подряд fail (настраиваемо, default 2–3) → пометить профиль `degraded`.
3. Запустить **узкий** supersweep / probe только для этого профиля (не полный прогон всех).
4. При успехе — `SetLock` + запись в локальный журнал; при неудаче — toast/warning, лок не трогать или откат.

**Не делать:** полный supersweep всех профилей на каждый fail; не блокировать UI на минуты без прогресса (уже есть `_pending` / progress у sweep).

**Файлы (ориентир):** `internal/webapi/status.go`, `internal/netcheck`, `internal/sweep`, `internal/app` (опционально daemon-хук), UI Overview / Strategies.

**Критерий готовности:**

- [x] искусственный fail check → через ≤N циклов стартует подбор только затронутого профиля (unit: `relock` / `bypass_test`);
- [x] успешный probe → новый lock + TLS ok;
- [x] неуспех → предыдущий lock сохранён, есть явное предупреждение;
- [x] выключатель в настройках (`auto_relock=0|1`) и в status JSON;
- [x] коммит: `8c90559` `feat(bypass): add recovery suite and wave-0 API surface` (полевая smoke — Roadmap RM2).

---

### BP2 — Apply recommendations с verify + rollback

**Проблема:** `recommendations.tsv` подсказывает стратегии по ASN, но оператор всё равно вручную фиксирует и проверяет.

**Идея:** action `recommendations_apply` (или UI-кнопка «Применить топ»):

1. Взять top-1 (или top-K с остановкой на первом ok) для выбранного профиля.
2. Сохранить предыдущий lock.
3. `SetLock` → короткий TLS check (как после «Зафиксировать»).
4. Fail → rollback lock + сообщение; ok → оставить.

**Файлы:** `internal/telemetry`, `internal/webapi` (settings/recommendations), Strategies UI.

**Критерий готовности:**

- [x] apply без сети рекомендаций → понятная ошибка / unavailable;
- [x] fail check откатывает lock;
- [x] ok оставляет lock и пишет `lock_origin=recommendations` (sidecar → status/UI);
- [x] коммит: `8c90559` (в составе recovery suite).

---

### BP3 — Триггеры supersweep (расписание / смена провайдера)

**Проблема:** один прогон при «поломке руками» не покрывает ночные смены DPI и смену ASN (роуминг, другой uplink).

**Идея:**

- опция «переподобрать сломанное» (только профили с fail health/check);
- хук после успешного `provider_redetect`, если ASN/бренд изменился;
- опциональный редкий schedule (например раз в 24–72 ч), **выключен по умолчанию**, с явным предупреждением о нагрузке на роутер.

**Файлы:** `webapi/settings` (provider), `webapi/supersweep.go`, `internal/provider`, UI Настройки / Стратегии.

**Критерий готовности:**

- [x] смена провайдера с включённой опцией ставит в очередь soft-sweep;
- [x] schedule off by default;
- [x] не запускает второй sweep, если уже `active`;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

## Волна 2. Сервисные контуры

### BP4 — Googlevideo: замкнутый контур качества

**Проблема:** seed + `auto_locked` уже есть; при смене SN / массовом fail оператор не всегда видит, что «качество» и wssize/ipv4 разъехались.

**Шаги:**

1. UI Domains: единый блок «YouTube media CDN» — seed state, auto_locks count, ipv4 freshness, wssize on/off.
2. При росте `MissingHostKeys` или серии fail — повторный seed (rate-limit).
3. Не смешивать GV auto-locks с ручными локами профилей без поля `source` в UI.
4. Кнопка «обновить googlevideo + проверить» (hosts sync + optional refresh_ips + короткий check целевого host).

**Файлы:** `internal/googlevideo`, `internal/app/gv_seed.go`, DomainsView, wssize panel.

**Критерий готовности:**

- [x] статус seed/auto_locks/ipv4/wssize виден в одном месте;
- [x] повторный seed не долбит CPU (backoff);
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP5 — Discord TCP+UDP «починить»

**Проблема:** Discord сидит в отдельном hostlist и голосовом профиле; поломка часто «TCP ок, voice нет» или наоборот — оператор чинит частями.

**Идея:** пресет / action `fix_discord`:

1. Убедиться, что Discord hostlist на месте и не в exclude по ошибке.
2. TCP (профиль 4): изолированный probe → live TLS verify + rollback; voice (6): стратегия + `voicePorts` без фейкового TLS «ok».
3. Один UX-результат: `ok` / `partial` / `fail`; UDP честно: `configured` | `fail` | `skipped` (`udp_verified=false`).

**Файлы:** `assets/extra_strats/TCP_Discord.txt`, `config.default` (Discord + voice blocks), `webapi` locks/voicePorts, Strategies / Manual UI.

**Критерий готовности:**

- [x] один action трогает согласованно TCP и voice;
- [x] partial / configured / skipped различимы в API/UI;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP6 — QUIC / HTTP3 политика по сервису

**Проблема:** глобальный `quic443` не объясняет, для каких сервисов лучше «ломать QUIC→TCP», а где иметь UDP/QUIC fakes (YT).

**Идея:**

- в status/settings: явная политика `quic_mode=disable|fake|inherit` (или сохранить toggle + описание);
- привязка к профилям YT/UDP_YT в документации UI;
- check/health учитывает «ожидался TCP из‑за quic off».

**Файлы:** `settings_post_modes.go` (quic443), status/UI, `docs/API.md` при новых полях.

**Критерий готовности:**

- [x] оператор видит текущую политику QUIC и к чему она применяется;
- [x] переключение не ломает Discord/голос без предупреждения;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

## Волна 3. Точность и мультиклиент

### BP7 — Умный hostlist / объяснение «почему в обходе»

**Проблема:** домен может попасть в обработку через RKN list, substring, custom_rkn, autohostlist, googlevideo — отладка неочевидна.

**Идея:**

- API `domains?action=explain&domain=` → какой list/kind сработал (или none);
- UI: поле «проверить домен» на вкладке Домены;
- опционально: импорт URL-списка с теми же normalize/валидаторами, что Manager;
- периодическая подсказка «custom_rkn не открывался N дней» (локальный last-ok, без облака).

**Файлы:** `internal/domains`, `webapi/domains.go`, DomainsView.

**Критерий готовности:**

- [x] explain для known/unknown домена стабилен и без смены CRUD-контракта (exclude/RKN/Discord/YT/autohostlist/googlevideo + role);
- [x] импорт URL не обходит Normalize / autoGuard (+ SSRF deny);
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP8 — Wizard client scopes

**Проблема:** scopes API есть, UX сложный (mask/shift/max).

**Идея:** пресеты «все устройства / отдельно TV / гостевая сеть» с подсказками mark и копированием локов default→scope.

**Файлы:** `webapi` scopes/settings, ScopesPanel, Strategies (scope selector).

**Критерий готовности:**

- [x] пресет включает scope без ручного hex, если платформа позволяет;
- [x] копирование локов default→scope опционально и обратимо;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP9 — Blob / clone fallback при fail TLS

**Проблема:** смена только `strategy=N` иногда недостаточна; помогает `fake_default_tls` ↔ maxru / clone_size.

**Идея:** в auto-relock (BP1) или supersweep: при fail после смены strategy попробовать ограниченный набор blob/mode (1–2 варианта), с откатом overrides.

**Файлы:** `orchestra` overrides, `settings_post_blobs.go`, sweep coordinator.

**Критерий готовности:**

- [x] blob fallback только при включённой опции;
- [x] откат override при неуспехе;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

## Волна 4. Устойчивость и каталог

### BP10 — Strategy-validator / ночной watchdog → done

**Проблема:** в `docs/PARITY.md` watchdog / strategy-validator — **partial**. На Entware процесс nfqws2 может жить, а обход — нет.

**Идея:** довести daemon/хук:

- периодический check ключевых профилей;
- интеграция с BP1 (relock) без дублирования логики;
- статус в `health` (`validator: ok|degraded|off`).

**Файлы:** `internal/app`, service scripts / watchdog assets, `status.go`, PARITY.md.

**Критерий готовности:**

- [x] PARITY: validator daemon = done (unit + `health.validator` ok|degraded|off, wired to relock/queue FAIL);
- [x] health отражает validator (`degraded` при relock-профилях или свежих FAIL очереди);
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP11 — Локальный журнал успеха (opt-in)

**Проблема:** телеметрия наружу есть; локальной истории «какой lock держался сутки» нет.

**Идея:** кольцевой лог в `state/` (JSONL): timestamp, profile, strategy, check result, source. Overview: sparkline / последние fails. Без сети.

**Файлы:** `internal/app` или `internal/telemetry` (local-only), Overview UI.

**Критерий готовности:**

- [x] выключено по умолчанию; запись на диск только при `journal_enabled` (+ лимит размера при записи);
- [x] не пишет password/session;
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

### BP12 — Signed feed стратегий (без полного update бинарника)

**Проблема:** новые рецепты появляются быстрее релизов ZatorGO; manual `strategy_register` масштабируется плохо.

**Идея:**

1. Ассет `strategies-extra.tsv` (или аналог) на GitHub Releases, подписанный тем же Ed25519, что `SHA256SUMS.sig` (или отдельный ключ — согласовать).
2. `zator update -strategies` / Web UI «обновить каталог стратегий» → verify подписи → append в конфиг / extra через существующий register path.
3. Fail-closed без подписи.

**Зависимость:** U1 (подпись релизов) уже в коде; секрет CI должен быть настроен.

**Файлы:** `internal/deploy`, `internal/webapi/manual.go` / strategy_register, release.yml, SECURITY.md.

**Критерий готовности:**

- [x] без валидной подписи каталог не применяется;
- [x] CLI и Web — единый путь (verify → register с dedupe → restart hint/maybeRestart);
- [x] после apply новые N видны в UI как стратегии профиля (кнопка «Каталог стратегий»);
- [x] коммит: `8c90559` / `85810fc` (Bypass suite + webui).

---

## Сводка (чеклист)

| ID | Тема | Приоритет | Статус |
| --- | --- | --- | --- |
| BP1 | Health → auto-relock | P0 | ☑ |
| BP2 | Apply recommendations + verify | P0 | ☑ |
| BP3 | Supersweep triggers | P0 | ☑ |
| BP4 | Googlevideo quality loop | P1 | ☑ |
| BP5 | Discord TCP+UDP repair | P1 | ☑ |
| BP6 | QUIC policy UX | P1 | ☑ |
| BP7 | Domain explain / smart hostlist | P1 | ☑ |
| BP8 | Client-scope presets | P2 | ☑ |
| BP9 | Blob/clone fallback | P2 | ☑ |
| BP10 | Strategy validator done | P1 | ☑ |
| BP11 | Local success journal | P2 | ☑ |
| BP12 | Signed strategies feed | P2 | ☑ |

---

## Риски и как снизить

| Риск | Снижение |
| --- | --- |
| Auto-relock «дёргает» роутер | rate-limit, один профиль за раз, opt-in, не полный supersweep |
| Ложные TLS fail (временный glitch) | N подряд, backoff, не трогать lock с первого fail |
| Recommendations устарели | verify+rollback (BP2); не apply без check |
| Strategy feed скомпрометирован | та же Ed25519-цепочка, что у релизов (BP12) |
| Конфликт autohostlist / manual RKN | сохранить текущий `autoGuard` |
| Раздувание UI | пресеты (BP5/BP8), детали — в explain/journal |

---

## Вне скоупа (явно)

- Переписывание Lua desync / nfqws2 / bol-van API.
- VPN/прокси как замена DPI-обхода (можно позже как optional guide, не ядро).
- Панели 3x-ui / marzban (уже out of scope в PARITY).
- Интерактивное ANSI-меню (removed).

---

## Как вести работу

1. Брать ID сверху волны; не смешивать BP с несвязанным рефакторингом в одном коммите без нужды.
2. Перед merge: `go test ./...`, `go vet ./...`; при UI — `npm run check` + `npm run build`.
3. При новых API-полях — обновить `docs/API.md` в том же логическом изменении.
4. Обновлять колонку **Статус** в этом файле (`☐` → `☑`).
5. Ручной QA на устройстве (Keenetic/OpenWRT/VPS) обязателен для BP1/BP4/BP5 — unit-тестов недостаточно.

---

## Связь с другими документами

| Документ | Связь |
| --- | --- |
| [`Roadmap.md`](Roadmap.md) | Актуальный backlog: релиз Bypass, QA, Overview, CI/webui |
| [`docs/PARITY.md`](docs/PARITY.md) | BP10 закрывает partial watchdog/validator |
| [`docs/API.md`](docs/API.md) | Новые actions/поля BP1–BP12 |
| [`.github/SECURITY.md`](.github/SECURITY.md) | Подпись feed’ов (BP12) |
| [`README.md`](README.md) | После BP4/BP5 — коротко упомянуть пресеты в возможностях |
