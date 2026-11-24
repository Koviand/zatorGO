<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { ProfileInfo } from '../api/types'
import { fetchEvents, fetchStrategyJournal, profileCheck, repairProfile, runCheck, serviceAction } from '../api/endpoints'
import CheckResults from '../components/ui/CheckResults.vue'
import { busyActive, busyButton, withBusy } from '../stores/busy'
import { confirmDialog } from '../stores/confirm'
import {
  refreshAll,
  scope,
  status,
  statusCheck,
  statusCheckRan,
  statusLoaded,
  versionInfo,
} from '../stores/status'
import { formatApiError, showApiError, showToast } from '../stores/toast'

type JournalEntry = { ts?: string; profile?: number; strategy?: number; result?: string; source?: string }
const journalEnabled = ref(false)
const journalEntries = ref<JournalEntry[]>([])
const repairNote = ref('')
const mediaURL = ref('')
const mediaHint = ref('')
let mounted = false
let eventsAbort: AbortController | null = null
let eventsSeq = 0

const running = computed(() => Boolean(status.value?.zapret2_running))
const health = computed(() => status.value?.health)
const overall = computed(() => health.value?.overall || (running.value ? 'ok' : 'down'))
const overallLabel = computed(() => {
  switch (overall.value) {
    case 'ok': return 'Норма'
    case 'degraded': return 'Деградация'
    case 'down': return 'Остановлен'
    default: return String(overall.value)
  }
})
const warnings = computed(() => status.value?.service_warnings || [])
const want = computed(() => {
  if (typeof health.value?.want_running === 'boolean') return health.value.want_running
  return Boolean(status.value?.nfqws_want_running)
})
const watchdog = computed(() => {
  if (typeof health.value?.watchdog === 'boolean') return health.value.watchdog
  return Boolean(status.value?.nfqws_watchdog)
})
const fwEnsure = computed(() => health.value?.fw_ensure || '—')
const ver = computed(() => versionInfo.value?.zator_version || '—')

/** Профили для блока «Что сломалось»: degraded + validator degraded. */
const brokenProfiles = computed(() => {
  const profiles = status.value?.profiles || []
  const deg = new Set(health.value?.degraded_profiles || [])
  const byId = new Map<number, ProfileInfo>()
  for (const p of profiles) {
    const id = Number(p.profile)
    if (!Number.isFinite(id)) continue
    byId.set(id, p)
  }
  const ids = deg.size
    ? [...deg]
    : (health.value?.validator === 'degraded' || overall.value === 'degraded')
      ? profiles.map((p) => Number(p.profile)).filter((n) => Number.isFinite(n))
      : []
  return ids.map((id) => {
    const p = byId.get(id)
    return {
      id,
      label: p?.label || `Профиль ${id}`,
      lock: p?.current_lock || '—',
      origin: p?.lock_origin || '—',
      source: p?.lock_source || '—',
    }
  })
})
const showBroken = computed(() =>
  brokenProfiles.value.length > 0
  || overall.value === 'degraded'
  || health.value?.validator === 'degraded'
  || Boolean(health.value?.warning),
)

function yn(v: boolean | undefined) {
  if (v === true) return 'да'
  if (v === false) return 'нет'
  return '—'
}

async function loadJournal() {
  try {
    const j = await fetchStrategyJournal()
    journalEnabled.value = !!j.enabled
    journalEntries.value = j.entries || []
  } catch {
    journalEnabled.value = false
    journalEntries.value = []
  }
}

async function refresh() {
  try {
    await withBusy('refresh-status', async () => {
      await refreshAll()
      await loadJournal()
    })
  } catch (e) {
    showApiError(e)
  }
}

function fmtTs(ts?: string) {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ts
  }
}

async function check() {
  try {
    await withBusy('status-check', async () => {
      statusCheck.value = await runCheck()
      statusCheckRan.value = true
    })
    showToast('Проверка TLS завершена')
    mediaHint.value = 'TLS ok ≠ видео ok: для mid-stream вставьте videoplayback URL профиля 2.'
  } catch (e) {
    showApiError(e)
  }
}

async function checkMedia() {
  const url = mediaURL.value.trim()
  if (!url) {
    showToast('Вставьте URL videoplayback из DevTools (Network → googlevideo)', 'warning')
    return
  }
  try {
    await withBusy('media-check', async () => {
      statusCheck.value = await profileCheck('2', scope.value || 'default', { mediaUrl: url })
      statusCheckRan.value = true
      const cls = (statusCheck.value?.results || []).map((r) => r.media?.class).find(Boolean)
      mediaHint.value = cls
        ? `media.class=${cls} (tls|tiny|midstream|play|dns). TLS ok ≠ видео ok.`
        : 'Проверка Range 16K/64K выполнена. Смотрите download/verdict в результатах.'
    })
    showToast('Mid-stream проверка профиля 2 завершена')
  } catch (e) {
    showApiError(e)
  }
}

async function repair(profile: number) {
  const ok = await confirmDialog({
    title: `Починить профиль ${profile}?`,
    message: 'Узкий ремонт: auto-relock или рекомендации для этого профиля. Полный суперсвип не запускается.',
    confirmText: 'Починить',
  })
  if (!ok) return
  try {
    await withBusy(`repair-${profile}`, async () => {
      const res = await repairProfile({ profile, mode: 'auto', scope: scope.value || 'default' })
      repairNote.value = `p${profile}: mode=${res.mode || 'auto'}${res.async ? ' (async)' : ''}`
      showToast(`Ремонт профиля ${profile} запущен (${res.mode || 'auto'})`)
      await refreshAll()
      await loadJournal()
    })
  } catch (e) {
    showApiError(e)
    repairNote.value = `p${profile}: fail — ${formatApiError(e)}`
  }
}

async function listenRepairEvents() {
  while (mounted) {
    if (document.hidden) {
      await new Promise((r) => setTimeout(r, 1000))
      continue
    }
    const ac = new AbortController()
    eventsAbort = ac
    try {
      const res = await fetchEvents({
        after: eventsSeq,
        timeout: 25,
        type: 'repair',
        signal: ac.signal,
      })
      if (typeof res.seq === 'number') eventsSeq = res.seq
      for (const ev of res.events || []) {
        const msg = ev.message || ev.state || 'repair'
        repairNote.value = `p${ev.profile ?? '?'}: ${msg}`
        if (ev.state === 'done' || ev.state === 'fail') {
          showToast(
            ev.state === 'done'
              ? `Ремонт профиля ${ev.profile ?? ''} завершён`
              : `Ремонт не удался: ${msg}`,
            ev.state === 'done' ? undefined : 'error',
          )
          void refreshAll().then(loadJournal)
        }
      }
    } catch {
      if (!mounted || ac.signal.aborted) return
      await new Promise((r) => setTimeout(r, 2000))
    }
  }
}

async function svc(action: 'start' | 'stop' | 'restart') {
  if (action === 'restart') {
    const ok = await confirmDialog({
      title: 'Перезапуск zapret2',
      message: 'Служба будет остановлена и запущена заново.',
      confirmText: 'Перезапустить',
    })
    if (!ok) return
  }
  try {
    await withBusy(`svc-${action}`, async () => {
      await serviceAction(action)
      await refreshAll()
    })
    const labels = { start: 'запущен', stop: 'остановлен', restart: 'перезапущен' } as const
    showToast(`zapret2 ${labels[action]}`)
  } catch (e) {
    showApiError(e)
  }
}

onMounted(() => {
  mounted = true
  if (!statusLoaded.value) void refresh()
  else void loadJournal()
  void listenRepairEvents()
})
onUnmounted(() => {
  mounted = false
  eventsAbort?.abort()
})
</script>

<template>
  <section class="console">
    <div v-if="!statusLoaded" class="loading">Загрузка пульта…</div>
    <template v-else>
      <div class="console-hero">
        <div class="console-hero-top">
          <div class="console-state">
            <div class="eyebrow">Состояние системы</div>
            <h1 :class="overall">{{ overallLabel }}</h1>
            <div class="sub mono">
              {{ running ? 'zapret2 работает' : 'zapret2 не запущен' }}
              · want {{ want ? 'on' : 'off' }}
              · v{{ ver }}
            </div>
          </div>
          <span class="chip" :class="{ on: running }">{{ running ? 'running' : 'stopped' }}</span>
        </div>

        <div class="health-grid">
          <div class="health-cell">
            <span class="k">Health</span>
            <span class="v" :class="overall === 'ok' ? 'ok' : overall === 'down' ? 'bad' : 'warn'">{{ overall }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Want</span>
            <span class="v">{{ yn(want) }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Watchdog</span>
            <span class="v" :class="watchdog ? 'ok' : 'warn'">{{ yn(watchdog) }}</span>
          </div>
          <div class="health-cell">
            <span class="k">FW ensure</span>
            <span class="v mono">{{ fwEnsure }}</span>
          </div>
          <div class="health-cell">
            <span class="k">WebUI WD</span>
            <span class="v">{{ yn(health?.webui_watchdog) }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Args / hook</span>
            <span class="v">{{ yn(health?.args_present ?? status?.args_present) }} / {{ yn(health?.fw_hook_present ?? status?.fw_hook_present) }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Warnings</span>
            <span class="v" :class="warnings.length ? 'warn' : 'ok'">{{ warnings.length }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Post-update</span>
            <span class="v mono">{{ health?.post_update_verify || status?.post_update_verify || '—' }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Auto-relock</span>
            <span class="v" :class="health?.auto_relock ? 'ok' : ''">{{ yn(health?.auto_relock) }}</span>
          </div>
          <div class="health-cell">
            <span class="k">Validator</span>
            <span
              class="v mono"
              :class="{
                ok: health?.validator === 'ok',
                bad: health?.validator === 'degraded',
              }"
            >{{ health?.validator || '—' }}</span>
          </div>
        </div>

        <div v-if="showBroken" class="warn-list broken-panel">
          <strong>Что сломалось</strong>
          <div class="panel-d" style="margin:0.25rem 0 0.5rem">
            Validator: <span class="mono">{{ health?.validator || '—' }}</span>
            · health: <span class="mono">{{ overall }}</span>
            <template v-if="health?.warning"> · {{ health.warning }}</template>
          </div>
          <div v-if="!brokenProfiles.length" class="panel-d">Деградировавших профилей в health нет — проверьте TLS или журнал.</div>
          <div v-for="row in brokenProfiles" :key="row.id" class="broken-row">
            <div class="mono">
              p{{ row.id }} · {{ row.label }}
              · lock {{ row.lock }}
              · origin {{ row.origin }}
              · source {{ row.source }}
            </div>
            <button
              type="button"
              class="primary"
              :disabled="busyActive"
              :class="{ 'is-busy': busyButton === `repair-${row.id}` }"
              @click="repair(row.id)"
            >Починить</button>
          </div>
          <div v-if="repairNote" class="mono" style="opacity:0.85;margin-top:0.35rem">{{ repairNote }}</div>
        </div>

        <div v-if="warnings.length" class="warn-list">
          <strong>Предупреждения службы</strong>
          <div v-for="(w, i) in warnings" :key="i">{{ w }}</div>
        </div>

        <div v-if="journalEnabled" class="warn-list" style="margin-top:0.75rem">
          <strong>Журнал стратегий</strong>
          <div v-if="!journalEntries.length" class="panel-d">Пока пусто</div>
          <div v-for="(e, i) in journalEntries.slice().reverse().slice(0, 12)" :key="i" class="mono" style="opacity:0.9">
            {{ fmtTs(e.ts) }} · p{{ e.profile }} s{{ e.strategy }} · {{ e.result }} · {{ e.source || '—' }}
          </div>
        </div>

        <div class="security-hint warn-list" style="margin-top:0.75rem">
          <strong>Безопасность Web UI</strong>
          <div>
            По умолчанию с паролем UI слушает все интерфейсы на порту <span class="mono">17682</span>.
            Без auth запуск только с <span class="mono">-host</span> или явным <span class="mono">-listen-all</span>.
            Ограничьте доступ firewall’ом / reverse proxy.
            Подробнее: <a href="https://github.com/Koviand/zatorGO/blob/main/.github/SECURITY.md" rel="noopener" target="_blank">SECURITY.md</a>.
          </div>
        </div>

        <div class="console-cta">
          <button type="button" class="primary" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'status-check' }" @click="check">Проверить</button>
          <button
            type="button"
            :disabled="busyActive"
            :class="{ 'is-busy': busyButton === (running ? 'svc-stop' : 'svc-start') }"
            @click="svc(running ? 'stop' : 'start')"
          >{{ running ? 'Стоп' : 'Старт' }}</button>
          <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'svc-restart' }" @click="svc('restart')">Рестарт</button>
          <button type="button" class="ghost" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'refresh-status' }" @click="refresh">Обновить</button>
        </div>
      </div>

      <div class="panel">
        <div class="panel-h">
          <h2>Результаты TLS</h2>
          <span class="chip">scope {{ scope }}</span>
        </div>
        <p class="panel-d">Проверка доступности эталонных целей. <strong>TLS ok ≠ видео ok</strong> — для YouTube mid-stream нужна Range-проба профиля 2.</p>
        <div class="media-check" style="display:grid;gap:0.45rem;margin:0.5rem 0 0.75rem">
          <label class="panel-d" for="media-url">Googlevideo videoplayback URL (профиль 2)</label>
          <input id="media-url" v-model="mediaURL" type="url" placeholder="https://rr…googlevideo.com/videoplayback?…" autocomplete="off">
          <div class="actions">
            <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'media-check' }" @click="checkMedia">
              Mid-stream 16K/64K
            </button>
          </div>
          <p v-if="mediaHint" class="panel-d mono">{{ mediaHint }}</p>
        </div>
        <CheckResults
          :payload="statusCheck"
          :empty-hidden="false"
          empty-message="Нажмите «Проверить», чтобы получить TLS-результаты."
        />
      </div>

      <div class="section-label">Контекст</div>
      <div class="console-links">
        <router-link class="console-link" to="/settings/provider">
          <strong>Провайдер</strong>
          <span>{{ status?.provider || '—' }}</span>
        </router-link>
        <router-link class="console-link" to="/settings/modes">
          <strong>Hostlist</strong>
          <span>{{ status?.hostlist_mode || '—' }}</span>
        </router-link>
        <router-link class="console-link" to="/manual">
          <strong>TLS blob</strong>
          <span>{{ status?.tls_blob_mode || '—' }}</span>
        </router-link>
        <router-link class="console-link" to="/strategies">
          <strong>Локи стратегий</strong>
          <span>{{ status?.strategy_locks_status || '—' }}</span>
        </router-link>
        <router-link class="console-link" to="/settings/firewall">
          <strong>Firewall</strong>
          <span>{{ status?.fwtype || '—' }} · FO {{ status?.flowoffload || '—' }}</span>
        </router-link>
        <router-link class="console-link" to="/settings/modes">
          <strong>Авторежим</strong>
          <span>{{ status?.auto_mode || '—' }}</span>
        </router-link>
      </div>
    </template>
  </section>
</template>
