<script setup lang="ts">
import { ref } from 'vue'
import { applySetting, fetchSetting } from '../../api/endpoints'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { confirmDialog } from '../../stores/confirm'
import { fetchAndApplyState } from '../../stores/state'
import { showToast } from '../../stores/toast'

const updateInfo = ref<{ update_available?: boolean; release?: string; checked_at?: string; error?: string } | null>(null)

/** Ждём подъёма Web UI после update_apply (процесс умирает ~1.5 с). */
async function waitForWebUI(timeoutMs = 90_000): Promise<boolean> {
  const started = Date.now()
  await new Promise((r) => setTimeout(r, 2000))
  while (Date.now() - started < timeoutMs) {
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => ctrl.abort(), 4000)
    try {
      const resp = await fetch('/api/status', {
        headers: { 'X-Zator-Client': 'webui' },
        signal: ctrl.signal,
        cache: 'no-store',
      })
      if (resp.ok || resp.status === 401) return true
    } catch {
      // ещё не поднялся
    } finally {
      window.clearTimeout(timer)
    }
    await new Promise((r) => setTimeout(r, 1500))
  }
  return false
}

async function checkUpdates() {
  try {
    await withBusy('upd-check', async () => {
      updateInfo.value = await fetchSetting.update_check()
      if (updateInfo.value?.error) showToast(updateInfo.value.error, 'error')
      else showToast(updateInfo.value?.update_available ? 'Есть обновление' : 'Актуально', updateInfo.value?.update_available ? 'info' : 'success')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function applyUpdates() {
  const ok = await confirmDialog({
    title: 'Установить обновление ZatorGO?',
    message: ['Будут скачаны файлы релиза. Может занять несколько минут.'],
  })
  if (!ok) return
  try {
    await withBusy('upd-apply', async () => {
      const res = await applySetting.update_apply()
      if (res.up_to_date) {
        showToast('Уже актуально')
        await fetchAndApplyState()
      } else if (res.restart_required) {
        showToast(`Обновлено (файлов: ${res.installed ?? 0}). Перезапуск WebUI…`)
        const up = await waitForWebUI()
        if (up) {
          window.location.reload()
        } else {
          showToast('Web UI не ответил вовремя — обновите страницу вручную', 'warning')
        }
      } else {
        showToast(`Обновлено (файлов: ${res.installed ?? 0})`)
        await fetchAndApplyState()
      }
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section id="sec-updates" class="panel">
    <div class="panel-h"><h2>Обновления</h2></div>
    <p class="panel-d">Проверка и установка релизов ZatorGO.</p>
    <p v-if="updateInfo" class="panel-d">
      {{ updateInfo.update_available ? 'Есть обновление' : 'Актуально' }}
      <template v-if="updateInfo.release"> · {{ updateInfo.release }}</template>
      <template v-if="updateInfo.checked_at"> · {{ updateInfo.checked_at }}</template>
    </p>
    <div class="actions">
      <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'upd-check' }" @click="checkUpdates">Проверить</button>
      <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'upd-apply' }" @click="applyUpdates">Установить</button>
    </div>
  </section>
</template>
