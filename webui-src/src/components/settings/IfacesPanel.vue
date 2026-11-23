<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { ifacesSettings, refreshIfacesSettings } from '../../stores/settings'
import { refreshAll } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const ifaceWan = ref('')
const ifaceLan = ref('')
watch(ifacesSettings, (i) => {
  if (!i) return
  ifaceWan.value = i.wan || i.suggested?.wan || ''
  ifaceLan.value = i.lan || i.suggested?.lan || ''
}, { immediate: true })

const ifaceOptions = computed(() => {
  const list = ifacesSettings.value?.available || []
  const names = list.map((x) => x.name)
  for (const extra of [ifaceWan.value, ifaceLan.value, ifacesSettings.value?.suggested?.wan, ifacesSettings.value?.suggested?.lan]) {
    if (extra && !names.includes(extra)) names.push(extra)
  }
  return names
})

async function saveIfaces() {
  if (!ifaceWan.value.trim() || !ifaceLan.value.trim()) {
    showToast('Укажите WAN и LAN', 'error')
    return
  }
  try {
    await withBusy('ifaces', async () => {
      announceRestart()
      const res = await applySetting.ifaces({ lan: ifaceLan.value.trim(), wan: ifaceWan.value.trim() })
      showToast(`Интерфейсы: LAN=${ifaceLan.value}, WAN=${ifaceWan.value}.${restartSuffix(res)}`)
      await refreshIfacesSettings()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function redetectIfaces() {
  try {
    await withBusy('ifaces-auto', async () => {
      announceRestart()
      const res = await applySetting.ifaces({ action: 'redetect' })
      showToast(`Автоопределение: LAN=${res.lan || '—'}, WAN=${res.wan || '—'}.${restartSuffix(res)}`)
      await refreshIfacesSettings()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-ifaces" class="panel">
    <div class="panel-h">
      <h2>Интерфейсы WAN / LAN</h2>
      <span v-if="ifacesSettings?.needs_autodetect" class="chip">нужно задать</span>
    </div>
    <p class="panel-d">
      На Keenetic/Entware без <code>IFACE_WAN</code>/<code>IFACE_LAN</code> правила NFQUEUE не цепляют трафик.
      WAN — uplink (default route), LAN — локальный bridge (обычно <code>br0</code>).
    </p>
    <p v-if="ifacesSettings?.suggested" class="panel-d">
      Автоопределение:
      WAN=<code>{{ ifacesSettings.suggested.wan || '—' }}</code>,
      LAN=<code>{{ ifacesSettings.suggested.lan || '—' }}</code>
      <template v-if="ifacesSettings.suggested.source"> · {{ ifacesSettings.suggested.source }}</template>
    </p>
    <div class="grid-2">
      <label class="field">
        <span>IFACE_WAN (внешний)</span>
        <select v-model="ifaceWan" :disabled="busyActive">
          <option v-for="n in ifaceOptions" :key="'w-'+n" :value="n">{{ n }}</option>
        </select>
      </label>
      <label class="field">
        <span>IFACE_LAN (локальный)</span>
        <select v-model="ifaceLan" :disabled="busyActive">
          <option v-for="n in ifaceOptions" :key="'l-'+n" :value="n">{{ n }}</option>
        </select>
      </label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'ifaces' }" @click="saveIfaces">Сохранить</button>
      <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'ifaces-auto' }" @click="redetectIfaces">Определить заново</button>
    </div>
  </section>
</template>
