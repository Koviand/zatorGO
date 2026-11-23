<script setup lang="ts">
import { ref } from 'vue'
import { fetchSetting } from '../../api/endpoints'
import { busyActive, withBusy } from '../../stores/busy'
import { showToast } from '../../stores/toast'

const errors = ref({ available: false, content: '', path: '', lines: 0 })

async function loadErrors() {
  try {
    await withBusy('err-load', async () => {
      const data = await fetchSetting.nfqws2_errors()
      errors.value = {
        available: !!data.available,
        content: data.content || '',
        path: data.path || '',
        lines: data.lines || 0,
      }
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

defineExpose({ loadErrors })
</script>

<template>
  <section id="sec-errors" class="panel">
    <div class="panel-h">
      <h2>Ошибки nfqws2</h2>
      <button type="button" class="ghost" :disabled="busyActive" @click="loadErrors">Обновить</button>
    </div>
    <p class="panel-d">Последние сообщения демона nfqws2.</p>
    <p v-if="!errors.available" class="panel-d">Журнал пуст или не найден.</p>
    <template v-else>
      <p class="panel-d"><code>{{ errors.path }}</code> · строк: {{ errors.lines }}</p>
      <pre class="error-log">{{ errors.content || '(пусто)' }}</pre>
    </template>
  </section>
</template>
