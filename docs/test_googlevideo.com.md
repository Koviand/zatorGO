# Диагностика обрыва googlevideo (YouTube CDN)

Цель: понять **на каком этапе** рвётся соединение с узлами вида  
`rr4---sn-p5qlsn6z.googlevideo.com`, и от этого выбрать действие.

| Этап сбоя | Что делать |
|-----------|------------|
| DNS | DNS / маршрутизация, не zapret |
| TCP :443 | блок порта / маршрут |
| TLS handshake | лок стратегии **профиля 2** (desync) |
| ~16–64 КБ mid-stream | **WSSIZE lite** или MSS clamp |
| только QUIC (h3) | UDP-профиль / форс TCP |
| curl OK, плеер рвётся | клиент / кэш / расширение |

> **Важно:** обычная проверка Googlevideo в Web UI — TLS к корню CDN (404 нормален).  
> Mid-stream / tiny: `check?profile=2&media_url=<URL videoplayback из DevTools>` — Range 16K/64K, поле `media.class` = `tls|tiny|midstream|play|dns`.  
> Для WSSIZE: `domains?list=googlevideo&action=refresh_ips` обновляет `googlevideo-ipv4.txt`.

---

## 0. Подготовка

1. Зафиксируй условия: zapret **вкл** / **выкл**, лок профиля 2 (номер или auto), WSSIZE on/off.
2. Один тест — одно изменение. Не крути лок и WSSIZE одновременно.
3. Запиши узел `rr…---sn-….googlevideo.com` из теста (см. шаг 1 или DevTools).

Команды ниже — с **ПК** (PowerShell / Git Bash / WSL) или с роутера (Entware), если есть `curl`/`nslookup`.

---

## 1. Узнать свой googlevideo-узел

### Вариант A — из браузера (лучший для mid-stream)

1. Открой YouTube, запусти ролик.
2. F12 → **Network** → фильтр `googlevideo`.
3. Кликни запрос к `*.googlevideo.com` → Copy → **Copy URL**.
4. Сохрани host и полный URL сегмента (для шагов 4–5).

### Вариант B — редиректор кластера (как в netcheck Zator)

```bash
curl -sL --max-time 15 "https://redirector.googlevideo.com/report_mapping"
```

Или возьми host из Web UI / лога проверки профиля 2.

Подставь свой host в переменную (пример):

```bash
# Git Bash / WSL / Entware
HOST="rr4---sn-p5qlsn6z.googlevideo.com"

# PowerShell
$HOST = "rr4---sn-p5qlsn6z.googlevideo.com"
```

---

## 2. DNS

```bash
nslookup "$HOST"
# или
dig +short "$HOST"
```

PowerShell:

```powershell
Resolve-DnsName $HOST
```

- **Нет ответа / NXDOMAIN** → проблема DNS, не DPI/WSSIZE.
- **Есть A/AAAA** → дальше.

---

## 3. TCP :443

```bash
nc -zv "$HOST" 443
# или
curl -v --connect-timeout 5 --max-time 8 "https://$HOST/" -o /dev/null
```

PowerShell:

```powershell
Test-NetConnection $HOST -Port 443
```

- **TcpTestSucceeded = False / connection refused / timeout на connect** → блок порта/маршрута.
- **Порт открыт** → дальше TLS.

---

## 4. TLS handshake (ClientHello)

```bash
curl -vI --connect-timeout 5 --max-time 15 "https://$HOST/"
```

или:

```bash
openssl s_client -connect "$HOST:443" -servername "$HOST" </dev/null 2>&1 | head -40
```

Интерпретация:

| Результат | Вывод |
|-----------|--------|
| timeout / reset до сертификата | обрыв на **handshake** → профиль 2 |
| `CONNECTED`, сертификат, HTTP (даже **404**) | handshake жив → смотри объём данных |

Повтори с zapret **выкл** и с **разными локами** профиля 2 (один номер за раз).

---

## 5. Mid-stream: 16 КБ vs 64 КБ vs полный ответ

Нужен **реальный URL сегмента** из DevTools (шаг 1A), не корень CDN.

```bash
URL='вставь_скопированный_googlevideo_url'

# полный кусок (или сколько отдаст сервер)
curl -L --max-time 30 -o /dev/null -w "full http=%{http_code} size=%{size_download} time=%{time_total}\n" "$URL"

# первые 16 КиБ
curl -L --range 0-16383 --max-time 20 -o /dev/null -w "16k  http=%{http_code} size=%{size_download}\n" "$URL"

# первые 64 КиБ
curl -L --range 0-65535 --max-time 30 -o /dev/null -w "64k  http=%{http_code} size=%{size_download}\n" "$URL"
```

PowerShell (если curl есть):

```powershell
$URL = 'вставь_скопированный_googlevideo_url'
curl.exe -L --max-time 30 -o NUL -w "full http=%{http_code} size=%{size_download}`n" $URL
curl.exe -L --range 0-16383 --max-time 20 -o NUL -w "16k size=%{size_download}`n" $URL
curl.exe -L --range 0-65535 --max-time 30 -o NUL -w "64k size=%{size_download}`n" $URL
```

Интерпретация:

| 16K | 64K / full | Вывод |
|-----|------------|--------|
| fail | fail | скорее handshake/ранний блок → профиль 2 |
| OK (~16384) | fail / reset / size << ожидаемого | **mid-stream cut** → WSSIZE **lite** / MSS |
| OK | OK | CDN с curl жив; смотри плеер / QUIC (шаг 6) |

Ожидаемые размеры при успешном Range: `size_download` ≈ 16384 и ≈ 65536 (если сервер поддерживает Range).

---

## 6. TCP vs QUIC (h2 vs h3)

В Chrome DevTools → Network → колонка **Protocol**:

- `h2` / `http/1.1` — TCP :443 → уместны профиль 2 + WSSIZE.
- `h3` — QUIC (UDP) → WSSIZE не поможет; смотри UDP-профиль или форс TCP.

Быстрая проверка «только TCP»:

```bash
# форс HTTP/1.1 (без h3)
curl --http1.1 -L --max-time 30 -o /dev/null -w "h1 size=%{size_download}\n" "$URL"
```

Если h1/h2 стабилен, а в браузере только h3 рвётся — проблема в UDP-пути.

---

## 7. Контроль: YouTube HTML vs CDN

```bash
curl -L --max-time 20 -o /dev/null -w "youtube http=%{http_code} size=%{size_download}\n" "https://www.youtube.com/"
```

| YouTube | Googlevideo media | Вывод |
|---------|-------------------|--------|
| OK | fail | проблема **профиля 2 / CDN**, не HTML |
| fail | fail | шире, чем только googlevideo |
| OK | OK (curl), плеер рвётся | клиент / расширения / другой `sn-*` |

В Web UI Zator: сравнение проверки YouTube и Googlevideo полезно для handshake, но при зелёном GV и рвущемся плеере всегда делай шаг 5.

---

## 8. Матрица «вкл/выкл» (минимум 4 прогона)

Для **одного и того же** media URL и одного `HOST`:

| # | zapret | лок профиля 2 | WSSIZE | TLS | 16K | 64K |
|---|--------|---------------|--------|-----|-----|-----|
| 1 | выкл | — | — | | | |
| 2 | вкл | auto / N | выкл | | | |
| 3 | вкл | N (лучший) | выкл | | | |
| 4 | вкл | N | lite | | | |

Заполни клетки `ok` / `fail` / `timeout` / размер.

Выводы:

- 1=ok, 2=fail на TLS → нужен другой лок профиля 2.
- TLS всегда ok, 16K ok, 64K fail только при zapret → странно; чаще 16K fail без WSSIZE, ok с lite → mid-stream.
- без разницы вкл/выкл → не стратегия (DNS, маршрут, сам CDN-узел).

---

## 9. Разные `sn-*` узлы

Иногда один `rr…sn-…` живёт, другой режется.

1. В Network собери 2–3 разных host.
2. Повтори шаги 4–5 на каждом.
3. Если разброс большой — это CDN/гео, не «сломан весь zapret»; держи актуальный IP-лист googlevideo и узкий mark для WSSIZE.

---

## 10. Что писать в отчёт

```
HOST:
URL сегмента: (обрежь signature при публикации)
zapret: on/off
lock profile2:
wssize: off / lite / full
DNS: ok/fail
TCP443: ok/fail
TLS: ok/fail/timeout
16k size / http:
64k size / http:
full size / http:
Protocol в браузере: h2 / h3
Плеер: ok / обрыв через ~Ns
```

---

## Краткая шпаргалка решений

1. **TLS timeout/reset** → SuperSweep / зафиксировать номер профиля 2; WSSIZE вторичен.  
2. **Handshake OK, ~16 КБ cut** → `Z2R_WSSIZE_RAMP_MODE=lite` + mark + IP-лист; не `full`.  
3. **Только h3** → UDP-стратегии или отключить QUIC на клиенте для проверки.  
4. **curl OK, плеер нет** → другой URL/узел, кэш, расширения, приложение vs браузер.

Нагрузка на роутер: предпочтительно **лок профиля 2 + WSSIZE lite только на googlevideo mark**, без `full` ramp.
