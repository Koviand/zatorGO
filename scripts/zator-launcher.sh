#!/bin/sh
# zator-launcher — минимальный bootstrap: определяет arch, качает бинарник, запускает.
set -eu

ZATOR_OWNER="${ZATOR_OWNER:-Koviand}"
ZATOR_REPO="${ZATOR_REPO:-zatorGO}"
ZATOR_RELEASE="${ZATOR_RELEASE:-latest}"
ZATOR_ROOT="${ZATOR_ROOT:-/opt/zator}"
ZATOR_BIN_DIR="${ZATOR_BIN_DIR:-$ZATOR_ROOT/bin}"
ZATOR_MIRROR="${ZATOR_MIRROR:-}"

umask 022

die() { echo "zator-launcher: $*" >&2; exit 1; }

# Имена owner/repo/tag — только безопасный набор символов (без injection в URL).
ok_token() {
  case "$1" in
    ''|*[!A-Za-z0-9._-]*) return 1 ;;
    *) return 0 ;;
  esac
}

ok_token "$ZATOR_OWNER" || die "некорректный ZATOR_OWNER"
ok_token "$ZATOR_REPO" || die "некорректный ZATOR_REPO"
ok_token "$ZATOR_RELEASE" || die "некорректный ZATOR_RELEASE"

detect_arch() {
  m=$(uname -m 2>/dev/null || echo unknown)
  case "$m" in
    x86_64|amd64) echo x86_64 ;;
    i386|i686|x86) echo x86 ;;
    aarch64|arm64) echo arm64 ;;
    armv7*|armv6*|arm) echo arm ;;
    mips64el|mips64le) echo mipsel64 ;;
    mips64) echo mips64 ;;
    mipsel|mipsle) echo mipsel ;;
    mips) echo mips ;;
    ppc|ppc32) echo ppc ;;
    ppc64|ppc64le) echo ppc ;;
    riscv64) echo riscv64 ;;
    *)
      if grep -qiE 'lexra|rlx' /proc/cpuinfo 2>/dev/null; then
        echo lexra
      else
        die "неизвестная архитектура: $m"
      fi
      ;;
  esac
}

require_https() {
  case "$1" in
    https://*) return 0 ;;
    *) die "разрешены только https URL (получено: $1)" ;;
  esac
}

download() {
  url="$1"
  dest="$2"
  require_https "$url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --proto '=https' --tlsv1.2 --connect-timeout 10 --retry 3 -o "$dest" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    die "нужен curl или wget"
  fi
}

file_sha256() {
  f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk '{print tolower($1)}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk '{print tolower($1)}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$f" | awk '{print tolower($NF)}'
  else
    die "нужен sha256sum, shasum или openssl для проверки контрольной суммы"
  fi
}

# Ищет hex-сумму для имени ассета в файле SHA256SUMS.
sum_for_asset() {
  sums_file="$1"
  want="$2"
  awk -v want="$want" '
    {
      n = NF
      if (n < 2) next
      name = $n
      sub(/^\*/, "", name)
      if (name == want) {
        sum = tolower($1)
        if (sum ~ /^[0-9a-f]{64}$/) { print sum; exit }
      }
    }
  ' "$sums_file"
}

# GitHub: latest → /releases/latest/download/<file>
#         tag    → /releases/download/<tag>/<file>
asset_url() {
  asset="$1"
  if [ -n "$ZATOR_MIRROR" ]; then
    printf '%s\n' "${ZATOR_MIRROR%/}/${asset}"
    return
  fi
  if [ -n "${ZATOR_BASE_URL:-}" ]; then
    printf '%s\n' "${ZATOR_BASE_URL%/}/${asset}"
    return
  fi
  base="https://github.com/${ZATOR_OWNER}/${ZATOR_REPO}"
  if [ "$ZATOR_RELEASE" = "latest" ]; then
    printf '%s\n' "${base}/releases/latest/download/${asset}"
  else
    printf '%s\n' "${base}/releases/download/${ZATOR_RELEASE}/${asset}"
  fi
}

arch=$(detect_arch)
# Lexra (RLX) — нет отдельного Go-таргета. Upstream linux-lexra = big-endian (EB),
# поэтому ассет zator = mips softfloat (не mipsel): иначе update/zapret endian mismatch.
asset_arch=$arch
if [ "$arch" = lexra ]; then
  asset_arch=mips
  echo "zator-launcher: архитектура Lexra — скачиваем zator-linux-mips" >&2
fi
asset="zator-linux-${asset_arch}"
install_dir="$ZATOR_BIN_DIR"
mkdir -p "$install_dir"
tmp="$install_dir/.${asset}.$$"
sums_tmp="$install_dir/.SHA256SUMS.$$"
sig_tmp="$install_dir/.SHA256SUMS.sig.$$"
sig_bin="$install_dir/.SHA256SUMS.sig.bin.$$"
pub_tmp="$install_dir/.release-pub.$$"
trap 'rm -f "$tmp" "$sums_tmp" "$sig_tmp" "$sig_bin" "$pub_tmp"' EXIT INT HUP

# Ed25519 SPKI public key (должен совпадать с deploy.ReleasePublicKeyB64).
write_release_pubkey() {
  cat >"$1" <<'EOF'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAzb7FN0Y688ClMPSQeFVgNUqjXfVAASuIgUdzbQ/UtmE=
-----END PUBLIC KEY-----
EOF
}

# Проверка Ed25519-подписи SHA256SUMS (OpenSSL 3+ / libressl с Ed25519).
verify_sums_sig() {
  sums_file="$1"
  sig_file="$2"
  command -v openssl >/dev/null 2>&1 || die "нужен openssl для проверки подписи SHA256SUMS"
  # Первая непустая не-комментарий строка = base64 подписи.
  sig_b64=$(awk 'NF && $1 !~ /^#/ { print $1; exit }' "$sig_file")
  [ -n "$sig_b64" ] || die "пустой SHA256SUMS.sig"
  if command -v base64 >/dev/null 2>&1; then
    printf '%s' "$sig_b64" | base64 -d >"$sig_bin" 2>/dev/null || \
      printf '%s' "$sig_b64" | base64 -D >"$sig_bin" 2>/dev/null || die "не удалось декодировать подпись"
  elif command -v openssl >/dev/null 2>&1; then
    printf '%s' "$sig_b64" | openssl base64 -d -A >"$sig_bin" || die "не удалось декодировать подпись"
  else
    die "нужен base64 или openssl для декодирования подписи"
  fi
  write_release_pubkey "$pub_tmp"
  openssl pkeyutl -verify -pubin -inkey "$pub_tmp" -rawin -in "$sums_file" -sigfile "$sig_bin" >/dev/null 2>&1 \
    || die "недействительная подпись SHA256SUMS (скомпрометированный или чужой релиз?)"
}

url=$(asset_url "$asset")
sums_url=$(asset_url "SHA256SUMS")
sig_url=$(asset_url "SHA256SUMS.sig")

echo "zator-launcher: arch=${arch}"
echo "zator-launcher: download ${sums_url}"
download "$sums_url" "$sums_tmp" || die "не удалось скачать SHA256SUMS"
echo "zator-launcher: download ${sig_url}"
download "$sig_url" "$sig_tmp" || die "не удалось скачать SHA256SUMS.sig"
verify_sums_sig "$sums_tmp" "$sig_tmp"
want_sum=$(sum_for_asset "$sums_tmp" "$asset")
[ -n "$want_sum" ] || die "в SHA256SUMS нет суммы для ${asset}"

echo "zator-launcher: download ${url}"
download "$url" "$tmp" || die "не удалось скачать ${url}"
got_sum=$(file_sha256 "$tmp")
[ "$got_sum" = "$want_sum" ] || die "несовпадение sha256 для ${asset}: ожидалось ${want_sum}, получено ${got_sum}"

chmod 755 "$tmp"
mv -f "$tmp" "$install_dir/zator"
rm -f "$sums_tmp" "$sig_tmp" "$sig_bin" "$pub_tmp"
trap - EXIT INT HUP

echo "zator-launcher: установка / обновление (zapret2 latest + автозапуск)..."
"$install_dir/zator" install "$@" || die "install завершился с ошибкой"

# Службы zapret2 и Web UI поднимает сам install. Не держим webui
# в foreground, чтобы не конфликтовать с S92/systemd.
echo "zator-launcher: готово"
if [ -f "$ZATOR_ROOT/config/zator.json" ]; then
  port=$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$ZATOR_ROOT/config/zator.json" 2>/dev/null | head -n1)
fi
port="${port:-17682}"
echo "zator-launcher: Web UI → http://$(hostname -I 2>/dev/null | awk '{print $1}'):${port}/"
echo "zator-launcher: (или http://<IP роутера>:${port}/)"
exit 0
