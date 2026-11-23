<script setup lang="ts">
import { ref, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { refreshTlsBlobSettings, tlsBlobSettings } from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const tlsMode = ref('fake_default_tls')
const tlsBlob = ref('')
watch(tlsBlobSettings, (t) => {
  if (!t) return
  tlsMode.value = t.current_mode === 'fake_default_tls' ? 'fake_default_tls' : 'custom'
  tlsBlob.value = t.current_blob || ''
}, { immediate: true })

async function saveTls() {
  if (tlsMode.value !== 'fake_default_tls' && !tlsBlob.value) {
    showToast('Выберите файл TLS blob', 'error')
    return
  }
  try {
    await withBusy('tls', async () => {
      announceRestart()
      const res = tlsMode.value === 'fake_default_tls'
        ? await applySetting.tls_blob('fake_default_tls')
        : await applySetting.tls_blob(tlsBlob.value)
      showToast(`TLS blob сохранён.${restartSuffix(res)}`)
      await refreshTlsBlobSettings()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-tls" class="panel">
    <div class="panel-h"><h2>TLS blob</h2><span class="chip">{{ tlsBlobSettings?.current_mode || '—' }}</span></div>
    <p class="panel-d">Глобальный blob ClientHello для TLS-фейков.</p>
    <div class="grid-2">
      <label class="field">
        <span>Режим</span>
        <select v-model="tlsMode" :disabled="busyActive">
          <option value="fake_default_tls">fake_default_tls (встроенный)</option>
          <option value="custom">файл maxru</option>
        </select>
      </label>
      <label class="field">
        <span>Файл</span>
        <select v-model="tlsBlob" :disabled="busyActive || tlsMode === 'fake_default_tls'">
          <option v-for="b in (tlsBlobSettings?.available_blobs || [])" :key="b" :value="b">{{ b }}</option>
        </select>
      </label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'tls' }" @click="saveTls">Сохранить TLS</button>
    </div>
  </section>
</template>
