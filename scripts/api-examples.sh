#!/usr/bin/env sh
# Примеры HTTP API ZatorGO (RM0d). Без зависимостей кроме curl.
# Usage:
#   BASE=http://127.0.0.1:17682 PASS=secret ./scripts/api-examples.sh
#   ./scripts/api-examples.sh              # без auth, если auth_enabled=false
set -eu

BASE="${BASE:-http://127.0.0.1:17682}"
PASS="${PASS:-}"
COOKIE_JAR="${COOKIE_JAR:-$(mktemp)}"
HDR=(-H "X-Zator-Client: api-examples" -H "Accept: application/json")
trap 'rm -f "$COOKIE_JAR"' EXIT

jget() {
  path="$1"
  shift
  curl -sS -b "$COOKIE_JAR" -c "$COOKIE_JAR" "${HDR[@]}" "$@" "${BASE}${path}"
}

jpost() {
  path="$1"
  shift
  curl -sS -b "$COOKIE_JAR" -c "$COOKIE_JAR" "${HDR[@]}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -X POST "$@" "${BASE}${path}"
}

echo "== capabilities =="
jget /api/capabilities
echo

if [ -n "$PASS" ]; then
  echo "== login =="
  jpost /api/login --data-urlencode "password=${PASS}"
  echo
fi

echo "== state (include=capabilities,journal) =="
jget "/api/state?scope=default&include=capabilities,journal"
echo

echo "== set-lock profile=1 strategy=1 =="
jpost /api/set-lock -d "profile=1&strategy=1&scope=default"
echo

echo "== check profile=1 =="
jget "/api/check?profile=1&scope=default"
echo

echo "== events (timeout=0) =="
jget "/api/events?timeout=0"
echo

echo "== supersweep status =="
jget "/api/settings?setting=supersweep"
echo

# Не стартуем полный supersweep по умолчанию — только если START_SWEEP=1.
if [ "${START_SWEEP:-0}" = "1" ]; then
  echo "== supersweep start =="
  jpost /api/settings -d "setting=supersweep&value=start&scope=default"
  echo
  echo "== events long-poll (supersweep, 10s) =="
  jget "/api/events?timeout=10&type=supersweep"
  echo
fi

echo "OK — см. docs/API.md и docs/openapi.yaml"
