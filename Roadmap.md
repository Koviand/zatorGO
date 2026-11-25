# Roadmap ZatorGO — открытое

Только незакрытое. Закрытое — в git history / [`docs/PARITY.md`](docs/PARITY.md) / [`docs/API.md`](docs/API.md).

**Легенда:** ☐ todo · ◐ partial  
**Ограничения:** без новых deps без «да»; не ослаблять auth / Ed25519 / shell-safe; `assets/webui` только через `npm run build`.

---

## RM2 — без другого железа · P2 · ◐

На Keenetic Entware arm64 (v0.1.47) закрыто: install/S90/ndm/iptables, Web UI, locks, update path, BP5, BP10, **BP1 полный цикл** (2026-11-25: fail→probe→`lock_origin=auto_relock`, YT 2→3).

Ещё **невозможно на этом роутере** (нужно другое устройство / WAN / ключ подписи):

- [ ] Ubuntu 22/24: launcher → Web UI → start zapret2
- [ ] Debian cloud VPS: install + check YouTube/Discord
- [ ] OpenWRT: procd, nftables, wssize-ramp
- [ ] Merlin: firewall-start hook
- [ ] Offline bundle на устройстве без GitHub (нужен `pack-offline` + `ZATOR_RELEASE_SIGNING_KEY`)
- [ ] BP4 успешный TLS к googlevideo sn-* с WAN — supersweep `workers=gv` = **44/44 fail**, curl к sn-* timeout; API/`wssize`/ipv4 ok

**Правило:** ☑ в PARITY только после прогона на железе. Баги → `fix:` + ссылка на RM2.

---

## Вне скоупа (не делать)

- Переписывание Lua desync / nfqws2 / bol-van API
- VPN/прокси как замена DPI-обхода
- Панели 3x-ui / marzban (RM19), Redis stats (RM20), ANSI-меню (RM21)
