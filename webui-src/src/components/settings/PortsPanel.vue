<script setup lang="ts">
import { computed, ref } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, withBusy } from '../../stores/busy'
import { ports, refreshPorts } from '../../stores/settings'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const portProto = ref<'tcp' | 'udp'>('tcp')
const portValue = ref('')
const portList = computed(() => {
  const info = ports.value?.[portProto.value]
  return info?.user || []
})

async function addPort() {
  try {
    const raw = String(portValue.value ?? '').trim()
    if (!raw) {
      showToast('Укажите порт или список портов', 'error')
      return
    }
    await withBusy('port-add', async () => {
      announceRestart()
      const res = await applySetting.ports_add(portProto.value, raw)
      portValue.value = ''
      showToast(`Порты добавлены.${restartSuffix(res)}`)
      await refreshPorts()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function removePort(v: string) {
  try {
    await withBusy('port-rm', async () => {
      announceRestart()
      await applySetting.ports_remove(portProto.value, v)
      await refreshPorts()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-ports" class="panel">
    <div class="panel-h"><h2>Порты NFQWS2</h2></div>
    <p class="panel-d">Дополнительные порты, которые обрабатывает nfqws2.</p>
    <div class="row">
      <select v-model="portProto" :disabled="busyActive" style="max-width:7rem">
        <option value="tcp">TCP</option>
        <option value="udp">UDP</option>
      </select>
      <input v-model="portValue" :disabled="busyActive" placeholder="443 или 1000-2000" style="flex:1;min-width:10rem">
      <button type="button" class="primary" :disabled="busyActive" @click="addPort">Добавить</button>
    </div>
    <p v-if="ports?.[portProto]?.full" class="panel-d">Текущие: <code>{{ ports[portProto]?.full }}</code></p>
    <div class="domain-list" style="margin-top:0.8rem">
      <div v-for="p in portList" :key="p" class="domain-item">
        <code>{{ p }}</code>
        <button type="button" class="ghost danger" :disabled="busyActive" @click="removePort(p)">Удалить</button>
      </div>
      <p v-if="!portList.length" class="panel-d">Пользовательских портов пока нет.</p>
    </div>
  </section>
</template>
