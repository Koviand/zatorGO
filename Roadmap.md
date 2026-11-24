# Roadmap ZatorGO

Конкретный backlog после закрытия Improvements и кода [`Bypass.md`](Bypass.md).  
Цель: расширить HTTP API, довести Bypass до релиза, закрыть полевой QA, укрепить install/CI и UX.

**Ограничения (как в проекте):**
- без новых deps в `go.mod` / `package.json` без явного «да»;
- HTTP/CLI — только обратно совместимые расширения;
- не ослаблять auth, SHA256 fail-closed, Ed25519 `SHA256SUMS.sig`, CSRF, shell-safe;
- `assets/webui/` только через `npm run build` (для CI/release — предпочтительно Linux-сборка);
- один коммит = одно логическое изменение (Conventional Commits, английский).

**Связанные документы**

| Документ | Роль |
| --- | --- |
| [`Bypass.md`](Bypass.md) | DPI-фичи BP1–BP12 — код ☑; полевой smoke → RM2 |
| [`docs/PARITY.md`](docs/PARITY.md) | Паритет + ручной QA (железо) |
| [`docs/API.md`](docs/API.md) | Контракт API |
| [`.github/SECURITY.md`](.github/SECURITY.md) | Цепочка доверия релизов |

**Легенда статуса:** ☐ todo · ☑ done · ◐ partial / blocked (нужно железо)  
**Приоритет:** P0 сейчас · P1 скоро · P2 позже

---

## Волны

| Волна | ID | Фокус | Статус |
| --- | --- | --- | --- |
| **0** | **RM0a–RM0e** | Расширенное взаимодействие API | ☑ |
| 1 | RM1–RM4, RM10 | Bypass → релиз; QA; политика webui | ☑ / RM2 ◐ |
| 2 | RM5–RM7, RM11–RM12 | Overview; feed; install UX | ☑ |
| 3 | RM8–RM9, RM13–RM16 | GV mid-stream; trust; offline; Node; hint; settings | ☑ |

---

## Волна 0 — API ☑

Все RM0a–RM0e закрыты: envelope/`code`, `/api/capabilities`, `/api/events`, `/api/repair`, batch check, OpenAPI, паритет `endpoints.ts`.

---

## Волна 1 — доставка и поле

### RM1 — Bypass → коммиты и релиз P0 ☑

- [x] код Bypass в `main` (`8c90559`, `85810fc`, …);
- [x] `docs/API.md` / Bypass commit hashes;
- [x] tag `v0.1.43` + signed release (SHA256SUMS.sig + strategies-extra.tsv.sig).

### RM2 — Ручной QA PARITY P0 ◐

Полевой чеклист требует устройств (Ubuntu/OpenWRT/Keenetic/Merlin/MIPS/Lexra).  
**Статус в репозитории:** критерии кода закрыты; аппаратный smoke **не выполняется в CI** — отмечать ☑ в [`docs/PARITY.md`](docs/PARITY.md) только после прогона на железе. Баги → отдельные `fix:` + ссылка сюда.

### RM3 — Политика `assets/webui` ☑

### RM4 — Долг Improvements ☑ (файл удалён)

### RM10 — Неинтерактивный install в README ☑

---

## Волна 2

### RM5 — Overview «Починить» ☑
### RM6 — Signed strategies feed в каждом release ☑
### RM7 — BP1 auto-relock критерии (unit) ☑ · полевая smoke → RM2
### RM11 — sha256sum.txt upstream fixture ☑
### RM12 — Session после set-password ☑

---

## Волна 3

### RM8 — Googlevideo mid-stream в UI ☑

Методика Range-проб: [`docs/test_googlevideo.com.md`](docs/test_googlevideo.com.md).
### RM9 — Zapret trust docs (sums ≠ Zator Ed25519) ☑
### RM13 — Offline-бандл first-class docs ☑
### RM14 — Node 22 LTS в Actions ☑
### RM15 — Подсказка firewall :17682 ☑
### RM16 — Settings группировка ☑

---

## P2 / later

| ID | Тема | Статус |
| --- | --- | --- |
| RM17 | Busy/timeout UX | ☑ (`BUSY_RESTART` / long-poll) |
| RM18 | webapi boundaries | ☑ (политика в `.cursor/rules`) |
| RM19 | VPS panels 3x-ui/marzban | out of scope |
| RM20 | Redis stats | out of scope |
| RM21 | ANSI-меню | removed — не возвращать |

---

## Сводка

| ID | Тема | P | Статус |
| --- | --- | --- | --- |
| RM0a–RM0e | Wave 0 API | P0/P1 | ☑ |
| RM1 | Bypass → release | P0 | ☑ |
| RM2 | Ручной QA PARITY | P0 | ◐ (железо) |
| RM3 | Политика assets/webui | P0 | ☑ |
| RM4 | Improvements archive | P1 | ☑ |
| RM5 | Overview «Починить» | P0 | ☑ |
| RM6 | strategies feed в release | P1 | ☑ |
| RM7 | BP1 критерии | P0 | ☑ |
| RM8 | GV mid-stream UI | P1 | ☑ |
| RM9 | Zapret trust docs | P2 | ☑ |
| RM10 | README non-interactive | P1 | ☑ |
| RM11 | sha256sum fixture | P1 | ☑ |
| RM12 | Session после set-password | P2 | ☑ |
| RM13 | Offline bundle docs | P2 | ☑ |
| RM14 | Node 22 | P2 | ☑ |
| RM15 | Firewall hint :17682 | P2 | ☑ |
| RM16 | Settings grouping | P2 | ☑ |
| RM17–RM21 | later / oos | — | ☑ / oos |

---

## Как вести работу

1. Новый endpoint / `setting=` → `types.ts` + `endpoints.ts` + `docs/API.md` (+ `openapi.yaml` при необходимости).
2. Перед merge: `go test ./...`, `go vet ./...`; UI → `npm run check` + `npm run build`.
3. Полевой QA (RM2) — только на устройстве; не отмечать ☑ без repro/прогона.
4. Issues: title = `RMN: …`, body = секция; лейблы `P0|P1|P2`, `roadmap`.
