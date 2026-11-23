<script setup lang="ts">
import { reactive, watch } from 'vue'
import { applySetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { fetchAndApplyState } from '../../stores/state'
import { scopes } from '../../stores/status'
import { announceRestart, showToast } from '../../stores/toast'

const scopeForm = reactive({ enable: false, mask: '0xff00', shift: '8', max: '255' })
watch(scopes, (s) => {
  if (!s) return
  scopeForm.enable = !!s.enabled
  if (s.mask != null && String(s.mask) !== '') scopeForm.mask = String(s.mask)
  if (s.shift != null) scopeForm.shift = String(s.shift)
  if (s.max_scope != null) scopeForm.max = String(s.max_scope)
}, { immediate: true })

async function saveScope() {
  try {
    await withBusy('scope', async () => {
      announceRestart()
      const res = await applySetting.client_scope({
        enable: scopeForm.enable,
        mask: String(scopeForm.mask ?? '').trim(),
        shift: String(scopeForm.shift ?? '').trim(),
        max: String(scopeForm.max ?? '').trim(),
      })
      await fetchAndApplyState()
      const enabled = res.enabled ?? !!scopes.value?.enabled
      scopeForm.enable = !!enabled
      if (res.warning) {
        showToast(res.warning, 'warning')
      } else {
        showToast(enabled ? 'Client scopes включены' : 'Client scopes выключены')
      }
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-scopes" class="panel">
    <div class="panel-h"><h2>Client scopes</h2><span class="chip" :class="{ on: scopeForm.enable }">{{ scopeForm.enable ? 'вкл' : 'выкл' }}</span></div>
    <p class="panel-d">Разные стратегии для разных клиентов по mark. Локи — во вкладке «Стратегии».</p>
    <div class="grid-2">
      <label class="toggle"><input v-model="scopeForm.enable" type="checkbox" :disabled="busyActive"><span>Включить режим mark</span></label>
      <label class="field"><span>mask</span><input v-model="scopeForm.mask" :disabled="busyActive"></label>
      <label class="field"><span>shift</span><input v-model="scopeForm.shift" type="number" min="0" max="31" :disabled="busyActive"></label>
      <label class="field"><span>max</span><input v-model="scopeForm.max" type="number" min="1" max="255" :disabled="busyActive"></label>
    </div>
    <div class="actions" style="margin-top:0.8rem">
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'scope' }" @click="saveScope">Сохранить scopes</button>
    </div>
  </section>
</template>
