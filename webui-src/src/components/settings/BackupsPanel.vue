<script setup lang="ts">
import { computed } from 'vue'
import { backups as backupsApi } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { confirmDialog } from '../../stores/confirm'
import { backups, formatSize } from '../../stores/backups'
import { resetDomains, refreshDomains } from '../../stores/domains'
import { fetchAndApplyState } from '../../stores/state'
import { status } from '../../stores/status'
import { announceRestart, restartSuffix, showToast } from '../../stores/toast'

const blockFullRestore = computed(() => !!status.value?.block_full_restore)

async function downloadBackup(name: string) {
  try {
    await withBusy('bak-dl', async () => {
      await backupsApi.download(name)
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function uploadBackup(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    await withBusy('bak-upload', async () => {
      const res = await backupsApi.upload(file)
      showToast(`Загружен: ${res.name}`)
      await fetchAndApplyState()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function createBackup() {
  try {
    await withBusy('bak-create', async () => {
      const res = await backupsApi.create()
      showToast(`Бэкап: ${res.name}`)
      await fetchAndApplyState()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function deleteBackup(name: string) {
  const ok = await confirmDialog({ title: 'Удалить бэкап?', message: name, danger: true })
  if (!ok) return
  try {
    await withBusy('bak-del', async () => {
      await backupsApi.delete(name)
      await fetchAndApplyState()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function restoreBackup(name: string) {
  const mode = 'smart' as const
  const lines = [
    `Архив: ${name}`,
    'Режим: smart — списки и локи; живой config zapret2 не затирается целиком.',
    'zapret2 будет перезапущен.',
  ]
  if (blockFullRestore.value) {
    lines.splice(1, 0, 'Полное восстановление (full) сейчас заблокировано после обновления.')
  }
  const ok = await confirmDialog({
    title: 'Восстановить бэкап?',
    message: lines,
    danger: true,
    confirmText: 'Восстановить',
  })
  if (!ok) return
  try {
    await withBusy('bak-restore', async () => {
      announceRestart()
      const res = await backupsApi.restore(name, mode)
      showToast(`Восстановлено файлов: ${res.files ?? 0}.${restartSuffix(res)}`)
      await fetchAndApplyState()
      resetDomains()
      await refreshDomains()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-backups" class="panel">
    <div class="panel-h"><h2>Бэкапы</h2></div>
    <p class="panel-d">Снимок конфига и локов. Можно скачать или восстановить из файла.</p>
    <div class="actions">
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'bak-create' }" @click="createBackup">Создать</button>
      <label class="ghost" style="padding:0.65rem 1rem;border:1px solid var(--line);border-radius:10px;cursor:pointer">
        Загрузить
        <input type="file" accept=".tar,application/x-tar" style="display:none" :disabled="busyActive" @change="uploadBackup">
      </label>
    </div>
    <p v-if="blockFullRestore" class="panel-d" style="margin-top:0.6rem">После обновления полное восстановление (full) заблокировано — доступен режим smart.</p>
    <div class="domain-list" style="margin-top:0.8rem">
      <div v-for="b in (backups?.items || [])" :key="b.name" class="domain-item">
        <div>
          <code>{{ b.name }}</code>
          <div style="color:var(--muted);font-size:0.8rem">{{ b.date }} · {{ formatSize(b.size) }}</div>
        </div>
        <div class="row">
          <button type="button" class="ghost" :disabled="busyActive"
            :class="{ 'is-busy': busyButton === 'bak-dl' }"
            @click="downloadBackup(b.name)">Скачать</button>
          <button type="button" class="primary" :disabled="busyActive"
            :class="{ 'is-busy': busyButton === 'bak-restore' }"
            @click="restoreBackup(b.name)">Восстановить</button>
          <button type="button" class="ghost danger" :disabled="busyActive" @click="deleteBackup(b.name)">Удалить</button>
        </div>
      </div>
    </div>
  </section>
</template>
