<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { applySetting, fetchBypassFlags } from '../../api/endpoints'
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

const bypassBusy = ref(false)
const autoRelock = ref(false)
const journalEnabled = ref(false)
const sweepOnProvider = ref(false)
const blobFallback = ref(false)
const scheduleHours = ref(0)
const scheduleDraft = ref('0')

async function toggleFlag(
  on: boolean,
  apply: (v: boolean) => Promise<unknown>,
  setLocal: (v: boolean) => void,
  label: string,
  ev?: Event,
) {
  try {
    bypassBusy.value = true
    await apply(on)
    setLocal(on)
    showToast(`${label}: ${on ? 'вкл' : 'выкл'}`)
  } catch (e) {
    const el = ev?.target as HTMLInputElement | undefined
    if (el) el.checked = !on
    showToast((e as Error).message, 'error')
  } finally {
    bypassBusy.value = false
  }
}

async function saveSchedule() {
  const n = Number.parseInt(scheduleDraft.value, 10)
  if (!Number.isFinite(n) || n < 0 || n > 168) {
    showToast('Интервал: 0…168 часов', 'error')
    return
  }
  try {
    bypassBusy.value = true
    const res = await applySetting.sweep_schedule_hours(n)
    scheduleHours.value = n
    scheduleDraft.value = String(n)
    showToast(n === 0 ? 'Schedule supersweep выкл' : `Schedule supersweep: каждые ${n} ч`)
    if (res.warning) showToast(res.warning, 'warning')
  } catch (e) {
    showToast((e as Error).message, 'error')
  } finally {
    bypassBusy.value = false
  }
}

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
      let msg = `${m.title}: ${on ? 'вкл' : 'выкл'}.${restartSuffix(res)}`
      if (m.key === 'quic443' && !on) {
        msg += ' Внимание: Discord voice/UDP может пострадать, если клиент уходит в QUIC.'
      }
      showToast(msg, m.key === 'quic443' && !on ? 'warning' : undefined)
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

onMounted(async () => {
  try {
    const f = await fetchBypassFlags()
    autoRelock.value = !!f.auto_relock
    journalEnabled.value = !!f.journal_enabled
    sweepOnProvider.value = !!f.sweep_on_provider_change
    blobFallback.value = !!f.blob_fallback
    scheduleHours.value = Number(f.sweep_schedule_hours) || 0
    scheduleDraft.value = String(scheduleHours.value)
  } catch { /* ignore */ }
})
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

    <h3 class="section-title" style="margin-top:1.1rem">Адаптация DPI</h3>
    <div class="grid-2">
      <label class="toggle">
        <input type="checkbox" :checked="autoRelock" :disabled="busyActive || bypassBusy"
          @change="toggleFlag(($event.target as HTMLInputElement).checked, applySetting.auto_relock, v => autoRelock = v, 'Auto-relock', $event)">
        <span>Auto-relock при деградации TLS</span>
        <span class="chip" :class="{ on: autoRelock }">{{ autoRelock ? 'вкл' : 'выкл' }}</span>
      </label>
      <label class="toggle">
        <input type="checkbox" :checked="sweepOnProvider" :disabled="busyActive || bypassBusy"
          @change="toggleFlag(($event.target as HTMLInputElement).checked, applySetting.sweep_on_provider_change, v => sweepOnProvider = v, 'Sweep при смене провайдера', $event)">
        <span>Soft-sweep при смене провайдера</span>
        <span class="chip" :class="{ on: sweepOnProvider }">{{ sweepOnProvider ? 'вкл' : 'выкл' }}</span>
      </label>
      <label class="toggle">
        <input type="checkbox" :checked="blobFallback" :disabled="busyActive || bypassBusy"
          @change="toggleFlag(($event.target as HTMLInputElement).checked, applySetting.blob_fallback, v => blobFallback = v, 'Blob fallback', $event)">
        <span>Blob/clone fallback при fail</span>
        <span class="chip" :class="{ on: blobFallback }">{{ blobFallback ? 'вкл' : 'выкл' }}</span>
      </label>
      <label class="toggle">
        <input type="checkbox" :checked="journalEnabled" :disabled="busyActive || bypassBusy"
          @change="toggleFlag(($event.target as HTMLInputElement).checked, applySetting.journal_enabled, v => journalEnabled = v, 'Журнал стратегий', $event)">
        <span>Локальный журнал стратегий</span>
        <span class="chip" :class="{ on: journalEnabled }">{{ journalEnabled ? 'вкл' : 'выкл' }}</span>
      </label>
    </div>
    <div class="field" style="margin-top:0.75rem;max-width:16rem">
      <span>Schedule supersweep (часы, 0=выкл)</span>
      <div style="display:flex;gap:0.5rem;align-items:center">
        <input type="number" min="0" max="168" v-model="scheduleDraft" :disabled="busyActive || bypassBusy" style="width:6rem">
        <button type="button" class="ghost" :disabled="busyActive || bypassBusy" @click="saveSchedule">Сохранить</button>
      </div>
    </div>
    <p class="panel-d">
      По умолчанию всё выкл. Schedule нагружает роутер — включайте осознанно.
      Журнал пишется на диск только при включении; лента — на Обзоре.
    </p>
  </section>
</template>
