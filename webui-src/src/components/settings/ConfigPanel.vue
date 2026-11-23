<script setup lang="ts">
import { ref } from 'vue'
import { applySetting, fetchSetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { confirmDialog } from '../../stores/confirm'
import { announceRestart, showToast } from '../../stores/toast'

const configText = ref('')
const configPath = ref('')
const configLoaded = ref(false)

async function loadConfig() {
  try {
    await withBusy('cfg-load', async () => {
      const data = await fetchSetting.config_raw()
      if (data.error) throw new Error(data.error)
      configText.value = data.content || ''
      configPath.value = data.path || ''
      configLoaded.value = true
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function saveConfig() {
  const ok = await confirmDialog({
    title: 'Записать config?',
    message: ['Файл zapret2/config будет перезаписан.', 'zapret2 перезапустится.'],
    danger: true,
  })
  if (!ok) return
  try {
    await withBusy('cfg-save', async () => {
      announceRestart()
      await applySetting.config_raw(configText.value)
      showToast('Конфиг сохранён')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

defineExpose({ loadConfig })
</script>

<template>
  <section id="sec-config" class="panel">
    <div class="panel-h">
      <h2>Конфиг zapret2</h2>
      <button type="button" class="ghost" :disabled="busyActive" @click="loadConfig">Перечитать</button>
    </div>
    <p class="panel-d">Сырой файл · <code>{{ configPath || '/opt/zapret2/config' }}</code></p>
    <textarea v-if="configLoaded" v-model="configText" class="config-editor" :disabled="busyActive" spellcheck="false"></textarea>
    <p v-else class="loading">Загрузка конфига…</p>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive || !configLoaded" :class="{ 'is-busy': busyButton === 'cfg-save' }" @click="saveConfig">Сохранить и перезапустить</button>
    </div>
  </section>
</template>
