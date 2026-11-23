<script setup lang="ts">
import { ref, watch } from 'vue'
import type { WssizeRampSettings } from '../../api/types'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { refreshWssizeRampSettings, wssizeRampSettings } from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const wssizeOn = ref(false)
const wssizeMode = ref<WssizeRampSettings['mode']>('full')
const wssizeScale = ref(1)
watch(wssizeRampSettings, (w) => {
  if (!w) return
  wssizeOn.value = w.enabled
  wssizeMode.value = w.mode
  wssizeScale.value = w.scale
}, { immediate: true })

async function saveWssize() {
  if (!wssizeRampSettings.value) return
  if (!Number.isInteger(wssizeScale.value) || wssizeScale.value < 0 || wssizeScale.value > 14) {
    showToast('Scale должен быть 0–14', 'error')
    return
  }
  try {
    await withBusy('wssize', async () => {
      announceRestart()
      const res = await applySetting.wssize_ramp({
        ...wssizeRampSettings.value!,
        enabled: wssizeOn.value,
        mode: wssizeMode.value,
        scale: wssizeScale.value,
      })
      showToast(`wssize_ramp сохранён.${restartSuffix(res)}`)
      await refreshWssizeRampSettings()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-wssize" class="panel">
    <div class="panel-h">
      <h2>WSSIZE ramp</h2>
      <span v-if="wssizeRampSettings" class="chip" :class="{ on: wssizeOn }">{{ wssizeOn ? 'вкл' : 'выкл' }}</span>
    </div>
    <p v-if="!wssizeRampSettings" class="panel-d">Настройка недоступна в этой сборке / ещё не загружена.</p>
    <template v-else>
      <p class="panel-d">
        Googlevideo IP: {{ wssizeRampSettings.googlevideo_ips }} ·
        локальный DNS-сбор: {{ wssizeRampSettings.googlevideo_local_collection }}
      </p>
      <div class="grid-2">
        <label class="toggle"><input v-model="wssizeOn" type="checkbox" :disabled="busyActive"><span>Включить wssize_ramp</span></label>
        <label class="field">
          <span>Режим</span>
          <select v-model="wssizeMode" :disabled="busyActive">
            <option value="full">full</option>
            <option value="lite">lite</option>
          </select>
        </label>
        <label class="field">
          <span>Scale (0–14)</span>
          <input v-model.number="wssizeScale" type="number" min="0" max="14" :disabled="busyActive">
        </label>
      </div>
      <div class="actions" style="margin-top:0.8rem">
        <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'wssize' }" @click="saveWssize">Сохранить</button>
      </div>
    </template>
  </section>
</template>
