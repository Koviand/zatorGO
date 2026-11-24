# Roadmap ZatorGO

Единый backlog: бывший `Bypass.md` (DPI BP1–BP12) + бывший Roadmap (RM*).  
Improvements закрыт и удалён ранее.

**Легенда:** ☐ todo · ☑ done · ◐ partial / blocked (нужно железо)  
**Приоритет:** P0 сейчас · P1 скоро · P2 позже

**Ограничения:**
- без новых deps в `go.mod` / `package.json` без явного «да»;
- HTTP/CLI — только обратно совместимые расширения;
- не ослаблять auth, SHA256 fail-closed, Ed25519 `SHA256SUMS.sig`, CSRF, shell-safe;
- `assets/webui/` только через `npm run build` (для CI/release — предпочтительно Linux-сборка);
- один коммит = одно логическое изменение (Conventional Commits, английский);
- ядро desync / Lua API bol-van не переписывается — ZatorGO ставит, конфигурирует, оркестрирует;
- новые `--lua-desync=…` только из апстрима / проверенных ассетов.

**Связанные документы**

| Документ | Роль |
| --- | --- |
| [`docs/PARITY.md`](docs/PARITY.md) | Паритет + ручной QA (железо) — чеклист RM2 |
| [`docs/API.md`](docs/API.md) | Контракт API (в т.ч. Bypass / Wave 0) |
| [`.github/SECURITY.md`](.github/SECURITY.md) | Цепочка доверия релизов / feed |
| [`README.md`](README.md) | Install, возможности |

---

## Открыто

### RM2 — Ручной QA PARITY · P0 · ◐

Единственный незакрытый пункт обоих планов. Код и unit-критерии закрыты; аппаратный smoke **не в CI**.

Чеклист устройств — [`docs/PARITY.md`](docs/PARITY.md) § «Ручной QA»:

- [ ] Ubuntu 22/24: launcher → Web UI → start zapret2
- [ ] Debian cloud VPS: install + check YouTube/Discord
- [ ] OpenWRT: procd, nftables, wssize-ramp
- [ ] Keenetic Entware: S90, ndm hook, iptables
- [ ] Merlin: firewall-start hook
- [ ] MIPS/arm: бинарник стартует, Web UI, атомарные locks
- [ ] Offline bundle без GitHub
- [ ] Update path: latest.json → apply keep-if-exists

Дополнительно (из критериев Bypass, полевая smoke):

- [ ] BP1 auto-relock на устройстве (fail check → узкий подбор → lock / rollback)
- [ ] BP4 Googlevideo quality loop (seed / auto_locks / wssize)
- [ ] BP5 Discord TCP+UDP repair (`ok` / `partial` / UDP honesty)

**Правило:** отмечать ☑ в PARITY только после прогона на железе. Баги → отдельные `fix:` + ссылка на RM2.

---

## Сводка статусов

### Bypass DPI (BP1–BP12) — код ☑

Закрыто в `main` / `v0.1.43+` (коммиты `8c90559`, `85810fc`, …). Полевой smoke → RM2.

| ID | Тема | P | Статус |
| --- | --- | --- | --- |
| BP1 | Health → auto-relock | P0 | ☑ код; поле → RM2 |
| BP2 | Apply recommendations + verify/rollback | P0 | ☑ |
| BP3 | Supersweep triggers (ASN / schedule) | P0 | ☑ |
| BP4 | Googlevideo quality loop | P1 | ☑ код; поле → RM2 |
| BP5 | Discord TCP+UDP repair | P1 | ☑ код; поле → RM2 |
| BP6 | QUIC policy UX | P1 | ☑ |
| BP7 | Domain explain / smart hostlist | P1 | ☑ |
| BP8 | Client-scope presets | P2 | ☑ |
| BP9 | Blob/clone fallback | P2 | ☑ |
| BP10 | Strategy validator / watchdog | P1 | ☑ |
| BP11 | Local success journal | P2 | ☑ |
| BP12 | Signed strategies feed | P2 | ☑ |

### Roadmap (RM*) — почти всё ☑

| ID | Тема | P | Статус |
| --- | --- | --- | --- |
| RM0a–RM0e | Wave 0 API | P0/P1 | ☑ |
| RM1 | Bypass → commits + release `v0.1.43` | P0 | ☑ |
| **RM2** | **Ручной QA PARITY** | **P0** | **◐ железо** |
| RM3 | Политика `assets/webui` | P0 | ☑ |
| RM4 | Improvements archive | P1 | ☑ (файл удалён) |
| RM5 | Overview «Починить» | P0 | ☑ |
| RM6 | strategies feed в каждом release | P1 | ☑ |
| RM7 | BP1 критерии (unit) | P0 | ☑ · поле → RM2 |
| RM8 | GV mid-stream UI | P1 | ☑ |
| RM9 | Zapret trust docs | P2 | ☑ |
| RM10 | README non-interactive install | P1 | ☑ |
| RM11 | sha256sum.txt upstream fixture | P1 | ☑ |
| RM12 | Session после set-password | P2 | ☑ |
| RM13 | Offline bundle docs | P2 | ☑ |
| RM14 | Node 22 LTS в Actions | P2 | ☑ |
| RM15 | Firewall hint :17682 | P2 | ☑ |
| RM16 | Settings grouping | P2 | ☑ |
| RM17 | Busy/timeout UX | — | ☑ |
| RM18 | webapi boundaries | — | ☑ (`.cursor/rules`) |
| RM19 | VPS panels 3x-ui/marzban | — | out of scope |
| RM20 | Redis stats | — | out of scope |
| RM21 | ANSI-меню | — | removed |

---

## Архив: Bypass (что сделано)

Роль ZatorGO: ставить/конфигурировать zapret2 (`nfqws2`). Цель плана была: меньше ручного перебора, быстрее реакция на смену DPI по ASN, устойчивее YT/Discord/RKN — без ослабления auth и цепочки обновлений.

**Каркас в коде:** профили + locks (`orchestra`/`webapi`), стратегии `NFQWS2_OPT`, списки YT/Discord/RKN, supersweep + `NfqwsProbe`, TLS check, recommendations, googlevideo seed, fake/clone/blob, QUIC443/RST/reasm/wssize, health Overview, watchdog/validator.

### Волна 1 — автоадаптация

- **BP1** — монитор health → N fail → узкий supersweep профиля → `SetLock` или сохранение старого lock; `auto_relock=0|1`.
- **BP2** — `recommendations_apply`: top-K → check → rollback; `lock_origin=recommendations`.
- **BP3** — soft-sweep при смене провайдера; schedule off by default; не второй sweep если `active`.

### Волна 2 — сервисные контуры

- **BP4** — UI Domains: seed / auto_locks / ipv4 / wssize; backoff re-seed; кнопка refresh+check.
- **BP5** — `fix_discord`: TCP+voice согласованно; `ok`/`partial`/`fail`; UDP `configured|fail|skipped`.
- **BP6** — политика QUIC в status/settings; предупреждение про Discord/голос.

### Волна 3 — точность

- **BP7** — `domains?action=explain`; импорт URL с Normalize/autoGuard/SSRF deny.
- **BP8** — пресеты scopes без ручного hex; copy locks default→scope.
- **BP9** — blob/clone fallback в relock/sweep, opt-in, откат override.

### Волна 4 — устойчивость

- **BP10** — strategy-validator daemon; `health.validator` = `ok|degraded|off`; PARITY → done.
- **BP11** — локальный JSONL-журнал (opt-in, без password/session).
- **BP12** — signed `strategies-extra.tsv` (Ed25519 fail-closed); CLI + Web единый путь.

**Риски (актуальны для RM2):** auto-relock нагрузка на роутер → rate-limit/opt-in; ложные TLS fail → N подряд; устаревшие recommendations → verify+rollback; feed → Ed25519.

---

## Архив: Roadmap волны (что сделано)

| Волна | ID | Фокус | Статус |
| --- | --- | --- | --- |
| 0 | RM0a–RM0e | envelope/`code`, capabilities, events, repair, batch check, OpenAPI, паритет `endpoints.ts` | ☑ |
| 1 | RM1–RM4, RM10 | Bypass release; QA; webui policy; Improvements | ☑ / **RM2 ◐** |
| 2 | RM5–RM7, RM11–RM12 | Overview «Починить»; feed; BP1 unit; sha256sum; session | ☑ |
| 3 | RM8–RM9, RM13–RM16 | GV mid-stream; trust docs; offline; Node 22; firewall hint; settings | ☑ |

---

## Вне скоупа (явно)

- Переписывание Lua desync / nfqws2 / bol-van API.
- VPN/прокси как замена DPI-обхода.
- Панели 3x-ui / marzban (RM19).
- Redis stats (RM20).
- Интерактивное ANSI-меню (RM21 — removed, не возвращать).

---

## Как вести работу

1. Новый endpoint / `setting=` → `types.ts` + `endpoints.ts` + `docs/API.md` (+ `openapi.yaml` при необходимости).
2. Перед merge: `go test ./...`, `go vet ./...`; UI → `npm run check` + `npm run build`.
3. Полевой QA (RM2) — только на устройстве; не отмечать ☑ без repro/прогона.
4. Issues: title = `RMN:` / `BPn:`, body = секция; лейблы `P0|P1|P2`, `roadmap`.
5. Баги с железа → `fix:` + ссылка на RM2 / PARITY.
