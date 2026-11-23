# План улучшений ZatorGO

Документ фиксирует поэтапный план рефакторинга и усиления проекта без смены публичного HTTP/CLI-контракта и без новых зависимостей (исключения явно отмечены и требуют согласования).

**Статус:** все этапы кода **выполнены**. Коммиты в git — по отдельному запросу (сейчас всё в working tree).

**Ограничения проекта (обязательны на всех этапах):**
- не добавлять зависимости в `go.mod` / `package.json` без явного «да»;
- не менять сигнатуры API/CLI без обратной совместимости;
- не ослаблять auth, SHA256 fail-closed, CSRF, shell-safe config;
- `assets/webui/` только через `npm run build` из `webui-src/`;
- один коммит = одно логическое изменение (Conventional Commits, английский).

**Рекомендуемый порядок эпиков:**

| Неделя | Этапы | Цель |
| --- | --- | --- |
| 1 | B4 → W2 → UI1 → S1 → A5 → CI1 | быстрые победы, низкий риск |
| 2 | D1 → C2 → U3 | целостность данных + fuzz |
| 3 | B1 → UI2 → W1 | крупный рефактор без смены контракта |
| 4+ | U1 | подпись релизов |

---

## Этап 1. D1 + B1 + B4 — домены и бэкапы

### B4 — безопасный extract tar ☑

- [x] symlink в backup-tar отклоняется;
- [x] `go test ./internal/webapi ./internal/fetch` зелёный;
- [ ] коммит: `fix(backup): reject non-regular tar entries on restore`.

### D1 — один источник правды для доменов ☑

- [x] единственная нормализация домена в кодовой базе;
- [x] query `list=` / `action=` без изменений;
- [x] `go test ./internal/domains ./internal/webapi` (в т.ч. `TestDomainsCustomRKN`);
- [ ] коммит(ы): `refactor(domains): use internal/domains from webapi`.

**Сделано:** `Server.domains` → `domains.Manager` (общий `Locks` store); CRUD/import/clear/strategies
через Manager; `NormalizeSubstring` сохраняет регистр; googlevideo и `check` остались в webapi.

### B1 — единый `backup.Manager` для UI и update ☑

- [x] create UI и pre-update идут через один пакет;
- [x] restore старого webui-`.tar` и manager-`.tar` работает;
- [x] `full` при `BlockFull` отклоняется; `smart` проходит;
- [ ] коммит(ы): `refactor(backup): unify webui and manager backup paths`.

---

## Этап 2. W2 + UI1

### W2 — reload auth по mtime/ttl ☑

- [x] число чтений `zator.json` ≪ числа HTTP-запросов под нагрузкой;
- [x] смена пароля CLI подхватывается без рестарта Web UI;
- [ ] коммит: `perf(webapi): reload auth on mtime/ttl instead of every request`.

### UI1 — разорвать цикл ES-модулей ☑

- [x] ациклический граф импортов;
- [x] mid-session 401 → login + сброс кэша доменов;
- [x] `npm run check && npm run build`, обновить `assets/webui/`;
- [ ] коммит: `fix(webui): break api/client ↔ stores import cycle`.

---

## Этап 3. U1 / U3 + C2

### C2 — fuzz `ValidateSourceSafe` ☑

- [x] fuzz проходит локально и в CI (короткий прогон);
- [ ] коммит: `test(config): fuzz ValidateSourceSafe`.

### U3 — SHA256 для установки zapret2 ☑

- [x] install без валидной суммы не проходит (или только с явным insecure-флагом);
- [ ] коммит: `fix(zapret): require SHA256 for release archives`.

### U1 — подпись релизов ☑

**Схема (без новых deps):** Ed25519 (stdlib `crypto/ed25519`).

- CI: `release.yml` подписывает `SHA256SUMS` → `SHA256SUMS.sig` (secret `ZATOR_RELEASE_SIGNING_KEY`).
- Launcher: verify подписи (OpenSSL 3+) → затем sha256 бинарника.
- In-app: `deploy.FindGitHubBinaryURL` fail-closed без валидной подписи.
- Документация: `.github/SECURITY.md`, README.

- [x] поддельный SUMS без валидной подписи → отказ install/update;
- [ ] коммит(ы): `ci(release): sign SHA256SUMS`, `fix(deploy): verify release signature`.

**Перед первым подписанным релизом:** добавить GitHub Actions secret `ZATOR_RELEASE_SIGNING_KEY`
(base64 raw 64-байтного private key). Публичный ключ уже вшит в `deploy.ReleasePublicKeyB64`
и `scripts/release-public.pem` / launcher. Приватный ключ — в личном agent store и `.secrets/` (gitignored).

---

## Этап 4. S1 + A5 + CI1

### S1 — `docs/PARITY.md` ☑

- [x] PARITY не противоречит коду;
- [ ] коммит: `docs: sync PARITY with NfqwsProbe`.

### A5 — убрать утечки hostlist ☑

- [x] в `internal/app/` нет случайных `.txt` hostlist;
- [x] тесты не загрязняют cwd;
- [ ] коммит: `chore: remove leaked hostlist fixtures from internal/app`.

### CI1 — все arch в CI ☑

- [x] PR CI падает, если любая из 10 arch не собирается;
- [ ] коммит: `ci: build all release architectures on PR`.

---

## Этап 5. UI2 + W1

### UI2 — распил `SettingsView.vue` ☑

- [x] UX и API-вызовы без регрессий;
- [x] файлы панелей обозримого размера;
- [ ] коммит(ы): `refactor(webui): split SettingsView into panels`.

**Сделано:** оболочка + `components/settings/*`; эндпоинты в `endpoints.ts`.

### W1 — разнести `settings_post.go` ☑

- [x] публичный контракт идентичен;
- [x] новый setting добавляется в один файл-домен + строка регистрации;
- [ ] коммит: `refactor(webapi): split settings_post by domain`.

**Сделано:** dispatcher + `modes` / `blobs` / `network`; update/config уже в `extras.go`.

---

## Сводка коммитов (чеклист прогресса)

| ID | Тема | Приоритет | Код | Коммит |
| --- | --- | --- | --- | --- |
| B4 | Safe tar extract | P0 | ☑ | ☐ |
| W2 | Auth reload mtime/ttl | P0 | ☑ | ☐ |
| UI1 | Import cycle break | P0 | ☑ | ☐ |
| S1 | PARITY.md sync | P0 | ☑ | ☐ |
| A5 | Remove leaked txt | P0 | ☑ | ☐ |
| CI1 | All arch in CI | P0 | ☑ | ☐ |
| D1 | Use internal/domains | P0 | ☑ | ☐ |
| C2 | Fuzz shell-safe | P0 | ☑ | ☐ |
| U3 | Zapret SHA256 | P1 | ☑ | ☐ |
| B1 | Unify backup Manager | P0 | ☑ | ☐ |
| UI2 | Split SettingsView | P1 | ☑ | ☐ |
| W1 | Split settings_post | P1 | ☑ | ☐ |
| U1 | Release signatures | P0 | ☑ | ☐ |

---

## Риски и правила снижения

| Риск | Как снизить |
| --- | --- |
| Сломать restore старых `.tar` | golden обоих layout в `testdata/` |
| Сломать `list=custom_rkn` и имена UI | только адаптер Kind↔list, без смены query |
| Подпись релизов без ключа в CI | secret `ZATOR_RELEASE_SIGNING_KEY` до первого tag-release |
| Большой PR «всё сразу» | один этап / один Conventional Commit |
| Новые Go/npm deps | U1 на stdlib Ed25519 — deps не добавлялись |
| Регрессия Web UI | после правок `webui-src`: `npm run check`, `npm run build`, закоммитить `assets/webui/` |

---

## Как вести работу

1. Код плана закрыт; дальше — нарезка коммитов по ID из сводки.
2. Перед merge: `go test ./...`, `go vet ./...`; при UI — `npm run check` + `npm run build`.
3. Обновлять колонку **Коммит** в этом файле после `git commit`.
4. При смене поведения API/CLI — править `docs/API.md` (и при необходимости `docs/PARITY.md`) в том же логическом изменении или сразу следом.
