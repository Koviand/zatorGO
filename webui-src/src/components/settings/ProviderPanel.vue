<script setup lang="ts">
import { ref, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { provider, refreshProvider } from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { showToast } from '../../stores/toast'

const providerName = ref('')
const providerCity = ref('')
watch(provider, (p) => {
  const raw = String(p?.provider || '')
  if (raw === 'Не определён') {
    providerName.value = ''
    providerCity.value = ''
    return
  }
  const parts = raw.split(' - ')
  providerName.value = parts[0] || ''
  providerCity.value = parts.slice(1).join(' - ')
}, { immediate: true })

async function saveProvider() {
  try {
    await withBusy('provider', async () => {
      await applySetting.provider_set(providerName.value.trim(), providerCity.value.trim())
      showToast('Провайдер сохранён')
      await refreshProvider()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function redetectProvider() {
  try {
    await withBusy('provider-redetect', async () => {
      await applySetting.provider_redetect()
      showToast('Провайдер переопределён')
      await refreshProvider()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-provider" class="panel">
    <div class="panel-h"><h2>Провайдер</h2></div>
    <p class="panel-d">Нужен для рекомендаций стратегий. Можно задать вручную или определить автоматически.</p>
    <div class="grid-2">
      <label class="field"><span>Имя</span><input v-model="providerName" :disabled="busyActive"></label>
      <label class="field"><span>Город</span><input v-model="providerCity" :disabled="busyActive"></label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive || !providerName.trim()" @click="saveProvider">Сохранить</button>
      <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'provider-redetect' }" @click="redetectProvider">Определить заново</button>
    </div>
  </section>
</template>
