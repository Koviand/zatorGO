<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { fetchSupersweep, supersweepCancel, supersweepStart } from '../../api/endpoints'
import type { SupersweepPayload } from '../../api/types'
import { busyActive, busyButton, withBusy } from '../../stores/busy'
import { confirmDialog } from '../../stores/confirm'
import { resetDomains } from '../../stores/domains'
import { refreshAll } from '../../stores/status'
import { showToast } from '../../stores/toast'

const props = withDefaults(defineProps<{
  scope?: string
  scopes?: string[]
}>(), {
  scope: 'default',
  scopes: () => ['default'],
})

const emit = defineEmits<{ started: []; cancelled: [] }>()

const data = ref<SupersweepPayload | null>(null)
const loading = ref(false)
const error = ref('')
const runScope = ref(props.scope || 'default')
const domainsText = ref('')
const tlsPref = ref('any')
const states = {
  idle: 'Нет прогона', starting: 'Подготовка', running: 'Выполняется', applying: 'Применение результатов',
  done: 'Завершён', cancelled: 'Отменён', interrupted: 'Прерван',
}
const verdicts = { ok: 'Работает', warn: 'С предупреждением', fail: 'Не работает' }
const stateLabel = computed(() => data.value ? states[data.value.state] : 'Загрузка…')
const active = computed(() => !!data.value?.active)
const scopeOptions = computed(() => {
  const list = props.scopes?.length ? [...props.scopes] : ['default']
  if (!list.includes(runScope.value)) list.unshift(runScope.value)
  return list
})
let mounted = false
let timer: number | undefined
let controller: AbortController | null = null
let wasActive = false

watch(() => props.scope, (v) => {
  if (v && !active.value) runScope.value = v
}, { immediate: true })

const count = (value: unknown) => Number.isSafeInteger(value) && (value as number) >= 0
const strategy = (value: unknown) => value === null || count(value)
const text = (value: unknown) => typeof value === 'string'
function valid(value: SupersweepPayload) {
  return value && typeof value.available === 'boolean' && typeof value.active === 'boolean' &&
    Object.prototype.hasOwnProperty.call(states, value.state) && ['none', 'live', 'saved'].includes(value.source) &&
    [value.started, value.updated, value.completed, value.total].every(count) &&
    text(value.scope) && text(value.phase) &&
    Array.isArray(value.workers) && value.workers.every((worker) => worker &&
      [worker.name, worker.label, worker.current_target, worker.greens, worker.warns, worker.applied].every(text) &&
      [worker.profile, worker.completed, worker.total, worker.ok, worker.warn, worker.fail].every(count) &&
      strategy(worker.current_strategy)) &&
    Array.isArray(value.applied) && value.applied.every((row) => row &&
      ['profile', 'domain'].includes(row.kind) && text(row.target) && count(row.strategy)) &&
    Array.isArray(value.domains) && value.domains.every((row) => row && text(row.domain) &&
      Object.prototype.hasOwnProperty.call(verdicts, row.verdict) && strategy(row.best_strategy) && strategy(row.applied))
}
function date(value: number) {
  return value > 0 ? new Date(value * 1000).toLocaleString('ru-RU') : '—'
}
function stopTimer() {
  window.clearTimeout(timer)
  timer = undefined
}
async function refresh() {
  if (!mounted || document.hidden || loading.value) return
  stopTimer()
  loading.value = true
  const request = new AbortController()
  controller = request
  const timeout = window.setTimeout(() => request.abort(), 12000)
  try {
    const result = await fetchSupersweep(request.signal)
    if (!valid(result)) throw new Error('Некорректный ответ сервера')
    if (mounted) {
      if (wasActive && !result.active) {
        resetDomains()
        refreshAll().catch(() => undefined)
      }
      wasActive = result.active
      data.value = result
      error.value = ''
    }
  } catch (cause) {
    if (mounted) error.value = request.signal.aborted ? 'Превышено время ожидания ответа.'
      : cause instanceof Error ? cause.message : 'Не удалось получить данные.'
  } finally {
    window.clearTimeout(timeout)
    controller = null
    loading.value = false
    if (mounted && !document.hidden) timer = window.setTimeout(refresh, data.value?.active ? 5000 : 15000)
  }
}
function visibilityChanged() {
  stopTimer()
  if (!document.hidden) void refresh()
}

async function startRun() {
  const ok = await confirmDialog({
    title: 'Запустить суперсвип?',
    message: 'Прогон подберёт стратегии для профилей и доменов РКН, затем применит победителей. Это может занять несколько минут.',
  })
  if (!ok) return
  try {
    await withBusy('ss-start', async () => {
      await supersweepStart({
        scope: runScope.value || 'default',
        domain: domainsText.value,
        tls_pref: tlsPref.value,
      })
      showToast('Суперсвип запущен')
      emit('started')
      wasActive = true
      await refresh()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function cancelRun() {
  const ok = await confirmDialog({
    title: 'Остановить суперсвип?',
    message: 'Текущий прогон будет отменён. Уже применённые локи останутся.',
    danger: true,
  })
  if (!ok) return
  try {
    await withBusy('ss-cancel', async () => {
      await supersweepCancel()
      showToast('Суперсвип отменён')
      emit('cancelled')
      await refresh()
      await refreshAll()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

onMounted(() => {
  mounted = true
  document.addEventListener('visibilitychange', visibilityChanged)
  void refresh()
})
onUnmounted(() => {
  mounted = false
  stopTimer()
  controller?.abort()
  document.removeEventListener('visibilitychange', visibilityChanged)
})

defineExpose({ refresh })
</script>

<template>
  <section id="supersweep-panel" class="supersweep-panel" aria-labelledby="supersweep-title">
    <div class="supersweep-header">
      <h2 id="supersweep-title">Суперсвип</h2>
      <div class="actions">
        <button id="supersweep-refresh-btn" type="button" :disabled="loading || busyActive" :aria-busy="loading" @click="refresh">
          {{ loading ? 'Обновляем…' : 'Обновить' }}
        </button>
        <button type="button" class="primary" :disabled="busyActive || active"
          :class="{ 'is-busy': busyButton === 'ss-start' }" @click="startRun">
          Запустить
        </button>
        <button type="button" class="ghost danger" :disabled="busyActive || !active"
          :class="{ 'is-busy': busyButton === 'ss-cancel' }" @click="cancelRun">
          Остановить
        </button>
      </div>
    </div>

    <p class="panel-d">Автоподбор стратегий для профилей и доменов РКН. Пустой список доменов — эталонный набор (xhamster.com, anidub.com, amnezia.org).</p>

    <div class="grid-2 supersweep-controls">
      <label class="field">
        <span>Scope</span>
        <select v-model="runScope" :disabled="busyActive || active">
          <option v-for="s in scopeOptions" :key="s" :value="s">{{ s }}</option>
        </select>
      </label>
      <label class="field">
        <span>TLS preference</span>
        <select v-model="tlsPref" :disabled="busyActive || active">
          <option value="any">any</option>
          <option value="12">TLS 1.2</option>
          <option value="13">TLS 1.3</option>
          <option value="both">both</option>
        </select>
      </label>
      <label class="field" style="grid-column:1/-1">
        <span>Домены РКН (по одному в строке, необязательно)</span>
        <textarea v-model="domainsText" rows="3" :disabled="busyActive || active"
          placeholder="xhamster.com&#10;anidub.com&#10;amnezia.org"></textarea>
      </label>
    </div>

    <p v-if="error" class="supersweep-error" role="alert">Не удалось обновить суперсвип: {{ error }}
      <span v-if="data"> Показаны последние полученные данные — они могут быть устаревшими.</span>
    </p>
    <p v-if="!data && !error" role="status">Загружаем данные суперсвипа…</p>
    <template v-if="data">
      <p role="status"><strong>{{ stateLabel }}</strong>
        · {{ data.source === 'saved' ? 'Сохранённый отчёт' : data.source === 'live' ? 'Текущий отчёт' : 'Нет отчёта' }}
        <span v-if="error"> · Устаревшие данные</span>
      </p>
      <p v-if="!data.available && !active">Прогонов пока нет. Заполните параметры выше и нажмите «Запустить».</p>
      <template v-else>
        <p>Фаза: {{ data.phase || '—' }} · Выполнено: {{ data.completed }} / {{ data.total }}</p>
        <progress v-if="data.total > 0" :value="data.completed" :max="data.total" aria-label="Общий прогресс суперсвипа" />
        <p class="supersweep-meta">Scope прогона: {{ data.scope || '—' }} · Начало: {{ date(data.started) }} · Обновлено: {{ date(data.updated) }}</p>
        <div class="supersweep-workers">
          <article v-for="worker in data.workers" :key="worker.name" class="supersweep-worker">
            <h3>{{ worker.label || worker.name }} · Профиль {{ worker.profile }}</h3>
            <p>Выполнено: {{ worker.completed }} / {{ worker.total }}</p>
            <progress v-if="worker.total > 0" :value="worker.completed" :max="worker.total" :aria-label="'Прогресс: ' + (worker.label || worker.name)" />
            <p>Текущая стратегия: {{ worker.current_strategy ?? '—' }} · Цель: {{ worker.current_target || '—' }}</p>
            <p><span class="supersweep-ok">Успех: {{ worker.ok }}</span> · <span class="supersweep-warn">Предупреждения: {{ worker.warn }}</span> · <span class="supersweep-error">Ошибки: {{ worker.fail }}</span></p>
            <p>Зелёные стратегии: {{ worker.greens || '—' }}</p>
            <p>Жёлтые стратегии: {{ worker.warns || '—' }}</p>
            <p>Применено по отчёту: {{ worker.applied || '—' }}</p>
          </article>
        </div>
        <h3>Применённые победители</h3>
        <p class="supersweep-meta">Применение зафиксировано в этом прогоне; текущие локи — во вкладке «Стратегии».</p>
        <ul v-if="data.applied.length">
          <li v-for="(row, index) in data.applied" :key="index">{{ row.kind === 'profile' ? 'Профиль' : 'Домен' }} {{ row.target }} → стратегия {{ row.strategy }}</li>
        </ul>
        <p v-else>В этом прогоне стратегии ещё не применялись.</p>
        <details v-if="data.domains.length">
          <summary>Результаты доменов РКН ({{ data.domains.length }})</summary>
          <ul class="supersweep-domains">
            <li v-for="row in data.domains" :key="row.domain">
              <strong>{{ row.domain }}</strong> · <span :class="'supersweep-' + row.verdict">{{ verdicts[row.verdict] }}</span>
              · Лучшая найденная: {{ row.best_strategy ?? '—' }} · Применена в прогоне: {{ row.applied ?? '—' }}
            </li>
          </ul>
        </details>
      </template>
    </template>
  </section>
</template>

<style scoped>
.supersweep-panel {
  margin-bottom: 0.75rem;
  padding: 0.85rem 0.9rem;
  border: 1px dashed color-mix(in srgb, var(--line) 80%, var(--muted));
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--surface) 65%, transparent);
  box-shadow: none;
  font-size: 0.82rem;
  overflow-wrap: anywhere;
  opacity: 0.95;
}
.supersweep-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.65rem; }
.supersweep-panel h2, .supersweep-panel h3 { margin: 0; font-size: 0.95rem; font-family: var(--font-display); }
.supersweep-panel h3 { font-size: 0.88rem; }
.supersweep-panel p { margin: 0.45rem 0; color: var(--muted); }
.supersweep-panel progress { width: 100%; accent-color: var(--ok); }
.supersweep-meta { color: var(--muted); font-family: var(--font-mono); font-size: 0.78rem; }
.supersweep-controls { margin: 0.65rem 0 0.85rem; }
.supersweep-controls textarea { width: 100%; resize: vertical; min-height: 4.5rem; }
.supersweep-workers { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr)); gap: 0.65rem; margin: 0.65rem 0; }
.supersweep-worker { min-width: 0; padding: 0.65rem; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface-2); }
.supersweep-ok { color: var(--ok); }
.supersweep-warn { color: var(--warn); }
.supersweep-error, .supersweep-fail { color: var(--danger); }
.supersweep-panel summary { cursor: pointer; margin-top: 0.65rem; }
.supersweep-domains li { margin: 0.45rem 0; }
</style>
