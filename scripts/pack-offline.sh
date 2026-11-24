#!/usr/bin/env bash
# Собирает офлайн-бандл: vendor/ (zapret2 архивы) + zator-full.tar.gz + launcher.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist/offline}"
VER="${VER:-$(cat "$ROOT/VERSION" 2>/dev/null || echo dev)}"
mkdir -p "$OUT/vendor"

if [[ ! -d "$ROOT/dist" ]] || ! ls "$ROOT/dist"/zator-linux-* >/dev/null 2>&1; then
  OUT="$ROOT/dist" bash "$ROOT/scripts/crossbuild.sh"
fi

# Минимальный full-архив из assets + бинарник хост-arch (для применения deploy.Apply).
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
mkdir -p "$STAGE/extra_strats/cache/deploy" "$STAGE/webui" "$STAGE/lua" "$STAGE/lists" "$STAGE/bin"
cp -a "$ROOT/assets/webui/." "$STAGE/webui/"
cp -a "$ROOT/assets/lua/." "$STAGE/lua/" 2>/dev/null || true
cp -a "$ROOT/assets/lists/." "$STAGE/lists/" 2>/dev/null || true
cp "$ROOT/assets/config.default" "$STAGE/config.default"
cp "$ROOT/dist/zator-linux-x86_64" "$STAGE/bin/zator" 2>/dev/null || true

# Манифест: path|dest|class|sha256|size|exec
MAN="$STAGE/extra_strats/cache/deploy/manifest.tsv"
: > "$MAN"
while IFS= read -r -d '' f; do
  rel=${f#"$STAGE/"}
  [[ "$rel" == extra_strats/cache/deploy/* ]] && continue
  sum=$(sha256sum "$f" | awk '{print $1}')
  size=$(wc -c <"$f" | tr -d ' ')
  exec=0
  [[ -x "$f" ]] && exec=1
  printf '%s|/opt/zator/%s|auto|%s|%s|%s\n' "$rel" "$rel" "$sum" "$size" "$exec" >>"$MAN"
done < <(find "$STAGE" -type f -print0)

cat >"$STAGE/extra_strats/cache/deploy/version.env" <<EOF
ZATOR_VERSION="$VER"
ZATOR_DATE="$(date -u +%Y-%m-%d)"
ZATOR_COMMIT=""
ZATOR_SHA="$(sha256sum "$MAN" | awk '{print $1}')"
WEBUI_VERSION="$VER"
WEBUI_DATE="$(date -u +%Y-%m-%d)"
WEBUI_SHA=""
TRACKING="offline"
EOF

tar -C "$STAGE" -czf "$OUT/zator-full.tar.gz" .
cp "$ROOT/scripts/zator-launcher.sh" "$OUT/zator-launcher.sh"
cp "$ROOT/dist"/zator-linux-* "$OUT/vendor/" 2>/dev/null || true

# SHA256SUMS + Ed25519.sig — ApplyBundle fail-closed без них.
(cd "$OUT" && sha256sum zator-full.tar.gz > SHA256SUMS)
if [[ -n "${ZATOR_RELEASE_SIGNING_KEY:-}" ]]; then
  (cd "$ROOT" && go run ./scripts/sign-sha256sums.go -in "$OUT/SHA256SUMS" -out "$OUT/SHA256SUMS.sig")
elif [[ -f "$ROOT/dist/SHA256SUMS.sig" && -f "$ROOT/dist/SHA256SUMS" ]]; then
  # Если dist уже содержит подпись релиза — не подменяем SUMS архива без ключа.
  echo "warn: ZATOR_RELEASE_SIGNING_KEY не задан; положите SHA256SUMS.sig вручную рядом с бандлом" >&2
else
  echo "warn: ZATOR_RELEASE_SIGNING_KEY не задан — бандл без SHA256SUMS.sig; ApplyBundle отклонит" >&2
fi

# zapret2 в vendor/ — установка без GitHub (ZATOR_VENDOR / LocalArchive).
ZAPRET_VER="${ZAPRET2_VERSION:-1.0.7-reasm-fix}"
ZAPRET_ASSET="zapret2-v${ZAPRET_VER}.tar.gz"
ZAPRET_URL="${ZAPRET2_ARCHIVE_URL:-https://github.com/MarkinAlexander/zapret2/releases/download/v${ZAPRET_VER}/${ZAPRET_ASSET}}"
if [[ ! -f "$OUT/vendor/$ZAPRET_ASSET" ]]; then
  echo "скачивание $ZAPRET_URL → vendor/"
  curl -fsSL -o "$OUT/vendor/$ZAPRET_ASSET" "$ZAPRET_URL" || {
    echo "warn: не удалось скачать zapret2; положите $ZAPRET_ASSET в vendor/ вручную" >&2
  }
fi
# Копия рядом с репозиторием для офлайн-разработки.
mkdir -p "$ROOT/vendor"
if [[ -f "$OUT/vendor/$ZAPRET_ASSET" && ! -f "$ROOT/vendor/$ZAPRET_ASSET" ]]; then
  cp "$OUT/vendor/$ZAPRET_ASSET" "$ROOT/vendor/$ZAPRET_ASSET"
fi

echo "offline bundle → $OUT"
ls -lh "$OUT" "$OUT/vendor"
