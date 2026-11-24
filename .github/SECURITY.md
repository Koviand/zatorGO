# Security Policy

## Поддерживаемые версии

Исправления безопасности принимаются для **последнего релиза** на ветке `main` / теге `v*`.

| Версия | Поддержка |
| --- | --- |
| latest release | да |
| более старые теги | по возможности |

## Как сообщить об уязвимости

Не открывайте публичный Issue с деталями эксплойта.

1. Напишите maintainer’у через GitHub: [@Koviand](https://github.com/Koviand)
2. Или откройте [private security advisory](https://github.com/Koviand/zatorGO/security/advisories/new), если доступно.

В сообщении укажите:

- затронутую версию / commit;
- шаги воспроизведения;
- ожидаемое vs фактическое поведение;
- влияние (RCE, auth bypass, DoS, утечка данных и т.п.).

Мы подтвердим получение и сообщим о плане фикса.

## Рекомендации по развёртыванию

- Держите **auth включённым** на устройствах, доступных из сети.
- Не используйте `zator install -no-auth` на публичных VPS.
- Ограничивайте порт `17682` firewall’ом / reverse proxy при необходимости.
- Проверяйте, что `ZATOR_OWNER` / `ZATOR_REPO` указывают на доверенный репозиторий обновлений.

## Подпись релизов (SHA256SUMS)

Цепочка доверия для install/update:

1. CI подписывает `SHA256SUMS` ключом Ed25519 → ассет `SHA256SUMS.sig`.
2. `zator-launcher.sh` и in-app update (`internal/deploy`) **fail-closed**: без валидной подписи установка/обновление отклоняется.
3. Затем сверяется sha256 бинарника с записью в подписанном `SHA256SUMS`.

Публичный ключ вшит в бинарник (`deploy.ReleasePublicKeyB64`) и в launcher (`scripts/release-public.pem`).

Приватный ключ хранится **только** в GitHub Actions secret `ZATOR_RELEASE_SIGNING_KEY` (base64 raw 64-байтного Go `ed25519.PrivateKey`). Не коммитьте его в репозиторий.

Локальная подпись (smoke):

```bash
go run ./scripts/sign-sha256sums.go -key-file .secrets/release.ed25519.b64 \
  -in out/SHA256SUMS -out out/SHA256SUMS.sig
```

Ротация ключа: сгенерировать новую пару, обновить secret, `ReleasePublicKeyB64`, PEM в launcher/`scripts/release-public.pem`, выпустить новый тег.

## Каталог стратегий (strategies-extra.tsv)

Тот же Ed25519-ключ подписывает ассет `strategies-extra.tsv` → `strategies-extra.tsv.sig` в **каждом** GitHub Release (CI всегда публикует пару, даже placeholder).

- `zator update -strategies` и Web UI `setting=strategies_feed` **fail-closed** без валидной подписи / при отсутствии ассета.
- После verify файл сохраняется в `state/strategies-extra.tsv`, затем строки `profile\tdesync` регистрируются в конфиг через `EnsureProfileStrategy` (идемпотентный dedupe). Повторный apply не плодит дубликаты; при новых стратегиях нужен restart zapret2.

Процесс обновления каталога: правьте `assets/strategies-extra.tsv` → CI на tag подписывает → оператор жмёт «Каталог стратегий» / `zator update -strategies`. Локальный smoke подписи: тот же `sign-sha256sums.go` с `-in strategies-extra.tsv`.

## Zapret-архив ≠ подписанный Zator SUMS

- **Zator** binary / launcher / strategies feed: Ed25519 `SHA256SUMS.sig` (этот документ выше).
- **zapret2** upstream `sha256sum.txt`: обычно **только per-file** суммы `binaries/<arch>/nfqws2` **без** подписи Zator и часто **без** строки на `.tar.gz`. ZatorGO проверяет суммы файлов после распаковки (fail-closed); это **не** замена Ed25519-якоря.
- MITM, подменивший и tar, и `sha256sum.txt` upstream, теоретически проходит zapret-якорь; бинарник **Zator** при online update всё равно требует валидный `SHA256SUMS.sig`. Для офлайна используйте заранее проверенный `pack-offline` бандл.
