# Стратегии Flowseal/zapret-discord-youtube

Источник: [Flowseal/zapret-discord-youtube](https://github.com/Flowseal/zapret-discord-youtube) (ветка `main`).

Сборка под **Windows**: `winws.exe` + WinDivert. Параметры — классический `--dpi-desync=*` (не `--lua-desync` zapret2 / ZatorGO).

Каждый `general*.bat` — полный профиль из нескольких сегментов `--new` (QUIC, Discord voice, Discord TCP, Google/YT, general, ipset, game filter).

Пути `%BIN%` / `%LISTS%` сокращены до `bin/` и `lists/`. `%GameFilterTCP%` / `%GameFilterUDP%` задаёт `service.bat`.

> Не путать с профилями ZatorGO (`--lua-desync=…:strategy=N`). Прямой перенос 1:1 невозможен; полезны идеи desync / fooling / blobs.

## Оглавление

- [general.bat](#general-bat)
- [general (ALT).bat](#general-alt-bat)
- [general (ALT10).bat](#general-alt10-bat)
- [general (ALT11).bat](#general-alt11-bat)
- [general (ALT12).bat](#general-alt12-bat)
- [general (ALT13).bat](#general-alt13-bat)
- [general (ALT2).bat](#general-alt2-bat)
- [general (ALT3).bat](#general-alt3-bat)
- [general (ALT4).bat](#general-alt4-bat)
- [general (ALT5).bat](#general-alt5-bat)
- [general (ALT6).bat](#general-alt6-bat)
- [general (ALT7).bat](#general-alt7-bat)
- [general (ALT8).bat](#general-alt8-bat)
- [general (ALT9).bat](#general-alt9-bat)
- [general (EXP).bat](#general-exp-bat)
- [general (FAKE TLS AUTO).bat](#general-fake-tls-auto-bat)
- [general (FAKE TLS AUTO ALT).bat](#general-fake-tls-auto-alt-bat)
- [general (FAKE TLS AUTO ALT2).bat](#general-fake-tls-auto-alt2-bat)
- [general (FAKE TLS AUTO ALT3).bat](#general-fake-tls-auto-alt3-bat)
- [general (SIMPLE FAKE).bat](#general-simple-fake-bat)
- [general (SIMPLE FAKE ALT).bat](#general-simple-fake-alt-bat)
- [general (SIMPLE FAKE ALT2).bat](#general-simple-fake-alt2-bat)

## Сводка методов

| Файл | Основные `--dpi-desync` | Заметки |
| --- | --- | --- |
| `general.bat` | fake, multisplit |  |
| `general (ALT).bat` | fake, fakedsplit |  |
| `general (ALT10).bat` | fake |  |
| `general (ALT11).bat` | fake, multisplit |  |
| `general (ALT12).bat` | fake, hostfakesplit, multisplit |  |
| `general (ALT13).bat` | fake, hostfakesplit, multisplit |  |
| `general (ALT2).bat` | fake, multisplit |  |
| `general (ALT3).bat` | fake, hostfakesplit |  |
| `general (ALT4).bat` | fake, multisplit |  |
| `general (ALT5).bat` | fake, multidisorder, syndata | NOT RECOMMENDED |
| `general (ALT6).bat` | fake, multisplit |  |
| `general (ALT7).bat` | fake, multisplit, syndata |  |
| `general (ALT8).bat` | fake |  |
| `general (ALT9).bat` | fake, hostfakesplit |  |
| `general (EXP).bat` | fake, hostfakesplit, multisplit |  |
| `general (FAKE TLS AUTO).bat` | fake, multidisorder |  |
| `general (FAKE TLS AUTO ALT).bat` | fake, fakedsplit |  |
| `general (FAKE TLS AUTO ALT2).bat` | fake, multisplit |  |
| `general (FAKE TLS AUTO ALT3).bat` | fake, multisplit |  |
| `general (SIMPLE FAKE).bat` | fake, hostfakesplit |  |
| `general (SIMPLE FAKE ALT).bat` | fake |  |
| `general (SIMPLE FAKE ALT2).bat` | fake |  |

## Стратегии подробно

### general.bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=568 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_4pda_to.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=568 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_4pda_to.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-split-seqovl=568 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_4pda_to.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT).bat

Методы: `fake, fakedsplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,fakedsplit --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fakedsplit-pattern=0x00 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,fakedsplit --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fakedsplit-pattern=0x00 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fakedsplit-pattern=0x00 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fakedsplit-pattern=0x00 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-fooling=ts --dpi-desync-fakedsplit-pattern=0x00 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n3
```

### general (ALT10).bat

Методы: `fake`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=none
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_4pda_to.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_4pda_to.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_4pda_to.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun.bin --dpi-desync-fake-unknown=bin/tls_clienthello_4pda_to.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT11).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun2.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n4
```

### general (ALT12).bat

Методы: `fake, hostfakesplit, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/stun.bin --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=3
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n4
```

### general (ALT13).bat

Методы: `fake, hostfakesplit, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=5
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=7 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --dpi-desync=hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=mail.ru,altorder=1 --dpi-desync-repeats=5 --dpi-desync-fake-tls=bin/tls_clienthello_sochi_park.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-http=bin/tls_clienthello_sochi_park.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=mail.ru,altorder=1 --dpi-desync-repeats=5 --dpi-desync-fake-tls=bin/tls_clienthello_sochi_park.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-http=bin/tls_clienthello_sochi_park.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=mail.ru,altorder=1 --dpi-desync-repeats=5 --dpi-desync-fake-tls=bin/tls_clienthello_sochi_park.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-http=bin/tls_clienthello_sochi_park.bin --dpi-desync-fake-unknown=bin/tls_clienthello_sochi_park.bin --dpi-desync-fake-unknown=bin/stun2.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n4
```

### general (ALT2).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=multisplit --dpi-desync-split-seqovl=652 --dpi-desync-split-pos=2 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=multisplit --dpi-desync-split-seqovl=652 --dpi-desync-split-pos=2 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=652 --dpi-desync-split-pos=2 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=652 --dpi-desync-split-pos=2 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-split-seqovl=652 --dpi-desync-split-pos=2 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT3).bat

Методы: `fake, hostfakesplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,hostfakesplit --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-hostfakesplit-mod=host=www.google.com,altorder=1 --dpi-desync-fooling=ts
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,hostfakesplit --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-hostfakesplit-mod=host=www.google.com,altorder=1 --dpi-desync-fooling=ts
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-fake-tls-mod=rnd,dupsid,sni=ya.ru --dpi-desync-hostfakesplit-mod=host=ya.ru,altorder=1 --dpi-desync-fooling=ts --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-fake-tls-mod=rnd,dupsid,sni=ya.ru --dpi-desync-hostfakesplit-mod=host=ya.ru,altorder=1 --dpi-desync-fooling=ts --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,hostfakesplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=ya.ru --dpi-desync-hostfakesplit-mod=host=ya.ru,altorder=1 --dpi-desync-fooling=ts --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n4
```

### general (ALT4).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=1000 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,multisplit --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=1000 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=1000 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=1000 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=1000 --dpi-desync-fake-tls=bin/stun.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT5).bat

> В bat помечено: **NOT RECOMMENDED**.

Методы: `fake, multidisorder, syndata`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### IPv4 TCP broad

```
--filter-l3=ipv4 --filter-tcp=80,443,2053,2083,2087,2096,8443 --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=syndata,multidisorder
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=syndata,multidisorder --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=14 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n3
```

### general (ALT6).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT7).bat

Методы: `fake, multisplit, syndata`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=multisplit --dpi-desync-split-pos=2,sniext+1 --dpi-desync-split-seqovl=679 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=multisplit --dpi-desync-split-pos=2,sniext+1 --dpi-desync-split-seqovl=679 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=multisplit --dpi-desync-split-pos=2,sniext+1 --dpi-desync-split-seqovl=679 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=syndata
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=syndata --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT8).bat

Методы: `fake`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake --dpi-desync-fake-tls-mod=none --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake --dpi-desync-fake-tls-mod=none --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-fake-tls-mod=none --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-fake-tls-mod=none --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-fake-tls-mod=none --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (ALT9).bat

Методы: `fake, hostfakesplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=hostfakesplit --dpi-desync-repeats=4 --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=hostfakesplit --dpi-desync-repeats=4 --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=hostfakesplit --dpi-desync-repeats=4 --dpi-desync-fooling=ts,md5sig --dpi-desync-hostfakesplit-mod=host=ozon.ru
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=hostfakesplit --dpi-desync-repeats=4 --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=ozon.ru
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=hostfakesplit --dpi-desync-repeats=4 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=ozon.ru
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (EXP).bat

Методы: `fake, hostfakesplit, multisplit`

#### Game filter TCP

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-l7=quic --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun,unknown --dpi-desync=fake --dpi-desync-any-protocol=1 --dpi-desync-fake-discord=bin/quic_initial_www_google_com.bin --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-unknown-udp=bin/quic_initial_www_google_com.bin --dpi-desync-fake-unknown-udp=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=4 --dpi-desync-cutoff=n4
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=480 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=4 --dpi-desync-split-seqovl-pattern=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --hostlist-exclude-domains=fonts.googleapis.com --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=480 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=4 --dpi-desync-split-seqovl-pattern=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-split-seqovl=664 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=5 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/quic_initial_4pda_to.bin --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n4
```

### general (FAKE TLS AUTO).bat

Методы: `fake, multidisorder`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multidisorder --dpi-desync-split-pos=1,midsld --dpi-desync-repeats=11 --dpi-desync-fooling=badseq --dpi-desync-fake-tls=0x00000000 --dpi-desync-fake-tls=^! --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,multidisorder --dpi-desync-split-pos=1,midsld --dpi-desync-repeats=11 --dpi-desync-fooling=badseq --dpi-desync-fake-tls=0x00000000 --dpi-desync-fake-tls=^! --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multidisorder --dpi-desync-split-pos=1,midsld --dpi-desync-repeats=11 --dpi-desync-fooling=badseq --dpi-desync-fake-tls=0x00000000 --dpi-desync-fake-tls=^! --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multidisorder --dpi-desync-split-pos=1,midsld --dpi-desync-repeats=11 --dpi-desync-fooling=badseq --dpi-desync-fake-tls=0x00000000 --dpi-desync-fake-tls=^! --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multidisorder --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-split-pos=1,midsld --dpi-desync-repeats=11 --dpi-desync-fooling=badseq --dpi-desync-fake-tls=0x00000000 --dpi-desync-fake-tls=^! --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=0x00000000 --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (FAKE TLS AUTO ALT).bat

Методы: `fake, fakedsplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,fakedsplit --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-repeats=8 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,fakedsplit --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-repeats=8 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-repeats=8 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-repeats=8 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,fakedsplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-repeats=8 --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (FAKE TLS AUTO ALT2).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=10000000 --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=10000000 --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=10000000 --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=10000000 --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=10000000 --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (FAKE TLS AUTO ALT3).bat

Методы: `fake, multisplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=11 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake,multisplit --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-split-seqovl=681 --dpi-desync-split-pos=1 --dpi-desync-fooling=ts --dpi-desync-repeats=8 --dpi-desync-split-seqovl-pattern=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-tls-mod=rnd,dupsid,sni=www.google.com --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n3
```

### general (SIMPLE FAKE).bat

Методы: `fake, hostfakesplit`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=hostfakesplit --dpi-desync-fooling=ts --dpi-desync-hostfakesplit-mod=host=www.google.com
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n4 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun2.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n3
```

### general (SIMPLE FAKE ALT).bat

Методы: `fake`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n3 --dpi-desync-fooling=badseq --dpi-desync-badseq-increment=2 --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun2.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_google_com.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=10 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n2
```

### general (SIMPLE FAKE ALT2).bat

Методы: `fake`

#### QUIC UDP (hostlist general)

```
--wf-tcp=80,443,2053,2083,2087,2096,8443,%GameFilterTCP% --wf-udp=443,19294-19344,50000-50100,%GameFilterUDP% --filter-udp=443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### Discord voice UDP (STUN)

```
--filter-udp=19294-19344,50000-50100 --filter-l7=discord,stun --dpi-desync=fake --dpi-desync-fake-discord=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-fake-stun=bin/ACTIVE_DISCORD_UDP.bin --dpi-desync-repeats=6
```

#### Discord TCP (discord.media)

```
--filter-tcp=2053,2083,2087,2096,8443 --hostlist-domains=discord.media --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### Google / YouTube TCP

```
--filter-tcp=443 --hostlist=lists/list-google.txt --ip-id=zero --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/tls_clienthello_www_google_com.bin
```

#### General TCP (hostlist)

```
--filter-tcp=80,443 --hostlist=lists/list-general.txt --hostlist=lists/list-general-user.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### QUIC UDP (ipset-all)

```
--filter-udp=443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fake-quic=bin/quic_initial_www_google_com.bin
```

#### TCP ipset-all

```
--filter-tcp=80,443,8443 --ipset=lists/ipset-all.txt --hostlist-exclude=lists/list-exclude.txt --hostlist-exclude=lists/list-exclude-user.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter TCP

```
--filter-tcp=%GameFilterTCP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=6 --dpi-desync-any-protocol=1 --dpi-desync-cutoff=n5 --dpi-desync-fooling=ts --dpi-desync-fake-tls=bin/stun2.bin --dpi-desync-fake-tls=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-http=bin/tls_clienthello_www_sferum_ru.bin --dpi-desync-fake-unknown=bin/stun2.bin --dpi-desync-fake-unknown=bin/tls_clienthello_www_sferum_ru.bin
```

#### Game filter UDP

```
--filter-udp=%GameFilterUDP% --ipset=lists/ipset-all.txt --ipset-exclude=lists/ipset-exclude.txt --ipset-exclude=lists/ipset-exclude-user.txt --dpi-desync=fake --dpi-desync-repeats=12 --dpi-desync-any-protocol=1 --dpi-desync-fake-unknown-udp=bin/ACTIVE_GAME_UDP.bin --dpi-desync-cutoff=n3
```

## Примечание для ZatorGO

- Flowseal = **winws** + `--dpi-desync=*`; ZatorGO = **nfqws2** + `--lua-desync=*`.
- Ближайшие аналоги: `multisplit` / `fakedsplit` / `hostfakesplit` / `fake` / `multidisorder` / `syndata` ↔ одноимённые lua-функции; `fooling=ts` ↔ `tcp_ts` / `tcp_ts_up`; blobs `bin/*.bin` ↔ `assets/files/fake/`.
- Сегменты Discord UDP / game filter на Linux-роутере обычно другие (нет WinDivert GameFilter).

---
Снято с upstream `main` на момент генерации файла.
