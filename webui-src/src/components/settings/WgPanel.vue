<script setup lang="ts">
import { ref, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import {
  refreshWgBlobSettings, refreshWgStateSettings, wgBlobSettings, wgStateSettings,
} from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { announceRestart, showToast } from '../../stores/toast'

const wgOn = ref(false)
const wgBlob = ref('')
const wgRepeats = ref(2)
watch(wgStateSettings, (w) => { if (w) wgOn.value = !!w.enabled }, { immediate: true })
watch(wgBlobSettings, (w) => {
  if (!w) return
  wgBlob.value = w.current_blob || ''
  wgRepeats.value = Number(w.current_repeats || 2)
}, { immediate: true })

async function saveWg(restart = true) {
  try {
    const repeats = Number(wgRepeats.value)
    if (!Number.isInteger(repeats) || repeats < 2 || repeats > 99) {
      showToast('Repeats WireGuard: целое число 2–99', 'error')
      return
    }
    if (wgOn.value && !String(wgBlob.value || '').trim()) {
      showToast('Выберите WG blob перед включением', 'error')
      return
    }
    await withBusy('wg', async () => {
      if (restart) announceRestart()
      await applySetting.wg_state(wgOn.value, false)
      if (wgBlob.value) await applySetting.wg_blob(wgBlob.value, false)
      await applySetting.wg_repeats(repeats, restart)
      showToast('WireGuard сохранён')
      await refreshWgBlobSettings()
      await refreshWgStateSettings()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-wg" class="panel">
    <div class="panel-h"><h2>WireGuard fakes</h2><span class="chip" :class="{ on: wgOn }">{{ wgOn ? 'вкл' : 'выкл' }}</span></div>
    <div class="grid-2">
      <label class="toggle"><input v-model="wgOn" type="checkbox" :disabled="busyActive"><span>Включить WG-стратегию</span></label>
      <label class="field">
        <span>Repeats</span>
        <input v-model.number="wgRepeats" type="number" min="2" max="99" :disabled="busyActive">
      </label>
      <label class="field" style="grid-column:1/-1">
        <span>Blob</span>
        <select v-model="wgBlob" :disabled="busyActive">
          <option v-for="b in (wgBlobSettings?.available_blobs || [])" :key="b" :value="b">{{ b }}</option>
        </select>
      </label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'wg' }" @click="saveWg(true)">Сохранить WG</button>
    </div>
  </section>
</template>
