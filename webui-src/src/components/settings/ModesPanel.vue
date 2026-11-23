<script setup lang="ts">
import { applySetting } from '../../api/endpoints'
import { busyActive, withBusy } from '../../stores/busy'
import {
  fallbackSettings, modeSettings, udpGamesSettings,
  refreshFallbackSettings, refreshModeSetting, refreshUdpGamesSettings,
} from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const modes = [
  { key: 'auto_mode', post: 'auto_mode_state', title: 'Авторотация TCP/HTTP', field: 'enabled' as const },
  { key: 'hostlist', post: 'hostlist_state', title: 'Autohostlist', field: 'auto' as const },
  { key: 'fallback', post: 'fallback_state', title: 'Безразборный режим', field: 'enabled' as const, fromFallback: true },
  { key: 'udp-games', post: 'udp_games_state', title: 'UDP Games 1026-65531', field: 'enabled' as const, fromUdp: true },
  { key: 'dns_desync', post: 'dns_desync_state', title: 'Антиспуф DNS (весь UDP/53)', field: 'enabled' as const },
  { key: 'rst_guard', post: 'rst_guard_state', title: 'RST guard', field: 'enabled' as const },
  { key: 'reasm', post: 'reasm_state', title: 'reasm-disable', field: 'enabled' as const },
  { key: 'quic443', post: 'quic443_state', title: 'QUIC fakes :443', field: 'enabled' as const },
]

function modeOn(m: typeof modes[number]) {
  if (m.fromFallback) return fallbackSettings.value?.enabled
  if (m.fromUdp) return udpGamesSettings.value?.enabled
  const d = modeSettings[m.key]
  return m.field === 'auto' ? d?.auto : d?.enabled
}

async function toggleMode(m: typeof modes[number], on: boolean, ev?: Event) {
  try {
    await withBusy(`mode-${m.key}`, async () => {
      announceRestart()
      const res = await applySetting.mode_state(m.post, on)
      showToast(`${m.title}: ${on ? 'вкл' : 'выкл'}.${restartSuffix(res)}`)
      if (m.fromFallback) await refreshFallbackSettings()
      else if (m.fromUdp) await refreshUdpGamesSettings()
      else await refreshModeSetting(m.key)
      await refreshAll()
    })
  } catch (e) {
    const el = ev?.target as HTMLInputElement | undefined
    if (el) el.checked = !on
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-modes" class="panel">
    <div class="panel-h"><h2>Режимы обхода</h2></div>
    <p class="panel-d">Авторотация, hostlist, fallback, DNS, UDP games, RST, reasm, QUIC.</p>
    <div class="grid-2">
      <label v-for="m in modes" :key="m.key" class="toggle">
        <input type="checkbox" :checked="!!modeOn(m)" :disabled="busyActive"
          @change="toggleMode(m, ($event.target as HTMLInputElement).checked, $event)">
        <span>{{ m.title }}</span>
        <span class="chip" :class="{ on: modeOn(m) }">{{ modeOn(m) ? 'вкл' : 'выкл' }}</span>
      </label>
    </div>
    <p v-if="udpGamesSettings?.ports" class="panel-d" style="margin-top:0.75rem">
      UDP Games порты: <code>{{ udpGamesSettings.ports }}</code>
      · стратегия профиля 7 — во вкладке «Стратегии».
    </p>
  </section>
</template>
