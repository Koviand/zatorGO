<script setup lang="ts">
import { ref, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, withBusy } from '../../stores/busy'
import { fetchAndApplyState } from '../../stores/state'
import { status } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const fw = ref(status.value?.fwtype || 'iptables')
const offload = ref(status.value?.flowoffload || 'donttouch')
watch(status, (s) => {
  if (s?.fwtype) fw.value = s.fwtype
  if (s?.flowoffload) offload.value = s.flowoffload
}, { immediate: true })

async function saveSelect(setting: string, value: string, label: string) {
  try {
    await withBusy(`sel-${setting}`, async () => {
      announceRestart()
      const res = await applySetting.value(setting, value)
      showToast(`${label}: ${value}.${restartSuffix(res)}`)
      await fetchAndApplyState()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-firewall" class="panel">
    <div class="panel-h"><h2>Firewall / Offload</h2></div>
    <p class="panel-d">Тип межсетевого экрана и flow offload.</p>
    <div class="grid-2">
      <label class="field">
        <span>FWTYPE</span>
        <select v-model="fw" :disabled="busyActive">
          <option value="iptables">iptables</option>
          <option value="nftables">nftables</option>
        </select>
      </label>
      <label class="field">
        <span>FLOWOFFLOAD</span>
        <select v-model="offload" :disabled="busyActive">
          <option value="donttouch">donttouch</option>
          <option value="none">none</option>
          <option value="software">software</option>
          <option value="hardware">hardware</option>
        </select>
      </label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive" @click="saveSelect('fwtype', fw, 'FWTYPE')">Сохранить FW</button>
      <button type="button" class="primary" :disabled="busyActive" @click="saveSelect('flowoffload', offload, 'Offload')">Сохранить Offload</button>
    </div>
  </section>
</template>
