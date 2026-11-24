<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { applySetting, clearLock, setLock, profileCheck } from '../api/endpoints'
import type { ApplyResult } from '../api/types'
import type { ProfileInfo } from '../api/types'
import { gatedReason, isProfileGated } from '../gating'
import { BUSY_RESTART, busyActive, busyButton, withBusy, withBusyRestart } from '../stores/busy'
import { fetchAndApplyState } from '../stores/state'
import { locks, refreshAll, scope, scopes, statusLoaded, strategyChecks } from '../stores/status'
import { refreshTlsBlobSettings, tlsBlobSettings } from '../stores/settings'
import { restartSuffix, showToast } from '../stores/toast'
import CheckResults from '../components/ui/CheckResults.vue'

const OVERRIDE_PROFILES = new Set(['1', '2', '3', '4', '8'])

const router = useRouter()
const route = useRoute()
/** Vue 3.4+ для input[type=number] кладёт в v-model number — всегда нормализуем в строку. */
type DraftMap = Record<string, string | number>
const draft = ref<DraftMap>({})
const fakeMode = ref<Record<string, string>>({})
const tlsProfile = ref<Record<string, string>>({})
const cloneSize = ref<Record<string, string>>({})
const overrideDirty = ref<Record<string, boolean>>({})

function draftText(profile: string): string {
  const v = draft.value[profile]
  return v === undefined || v === null ? '' : String(v).trim()
}

function lockText(p: ProfileInfo): string {
  const lock = String(p.current_lock || 'auto')
  return lock === 'auto' ? '' : lock
}

function draftDirty(p: ProfileInfo): boolean {
  const id = String(p.profile)
  const d = draftText(id)
  if (!d) return false
  return d !== lockText(p)
}

function focusProfilesFromQuery() {
  const raw = String(route.query.focus || '').trim()
  if (!raw) return
  const ids = raw.split(/[,\s]+/).map((x) => x.trim()).filter(Boolean)
  const first = ids[0]
  if (!first) return
  nextTick(() => {
    document.getElementById(`strategy-${first}`)?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  })
}

function syncDraftFromLocks(list: ProfileInfo[] | undefined | null, force = false) {
  const next: DraftMap = force ? {} : { ...draft.value }
  for (const p of list || []) {
    const id = String(p.profile)
    if (force || next[id] === undefined) {
      next[id] = p.current_lock && p.current_lock !== 'auto' ? String(p.current_lock) : ''
    }
  }
  draft.value = next
}

function syncOverridesFromSettings(force = false) {
  const t = tlsBlobSettings.value
  if (!t) return
  for (const p of locks.value || []) {
    const id = String(p.profile)
    if (!OVERRIDE_PROFILES.has(id)) continue
    if (!force && overrideDirty.value[id]) continue
    fakeMode.value[id] = t.profile_modes?.[id] || 'classic'
    tlsProfile.value[id] = t.profile_blobs?.[id] ?? ''
    cloneSize.value[id] = t.profile_sizes?.[id] || ''
  }
}

watch(locks, (list) => syncDraftFromLocks(list, false), { immediate: true, deep: true })
watch(tlsBlobSettings, () => syncOverridesFromSettings(false), { immediate: true, deep: true })

const scopeOptions = computed(() => scopes.value.scopes || ['default'])

onMounted(async () => {
  const q = String(route.query.scope || '')
  if (q && q !== scope.value) {
    scope.value = q
  }
  // Всегда подтягиваем локи для текущего scope ( Domains мог сменить scope без refreshAll ).
  try {
    await refreshAll()
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
  if (statusLoaded.value) focusProfilesFromQuery()
})

watch(statusLoaded, (ok) => { if (ok) focusProfilesFromQuery() })
watch(() => route.query.focus, () => { if (statusLoaded.value) focusProfilesFromQuery() })

async function changeScope(v: string) {
  scope.value = v || 'default'
  router.replace({ query: { scope: scope.value } })
  for (const key of Object.keys(strategyChecks)) delete strategyChecks[key]
  draft.value = {}
  overrideDirty.value = {}
  try {
    await withBusy('scope-change', async () => {
      await refreshAll()
      syncDraftFromLocks(locks.value, true)
      syncOverridesFromSettings(true)
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

function markOverrideDirty(profile: string) {
  overrideDirty.value[profile] = true
}

function canOverride(p: ProfileInfo) {
  return OVERRIDE_PROFILES.has(String(p.profile))
}

const ORIGIN_LABELS: Record<string, string> = {
  recommendations: 'рекомендации',
  auto_relock: 'auto-relock',
  manual: 'вручную',
  fix_discord: 'Discord',
  supersweep: 'суперсвип',
}

function originLabel(origin?: string): string {
  if (!origin) return ''
  return ORIGIN_LABELS[origin] || origin
}

function gated(p: ProfileInfo) {
  return isProfileGated(p)
}

async function refresh() {
  try {
    await withBusy('refresh-locks', fetchAndApplyState)
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function save(profile: string, p: ProfileInfo) {
  if (gated(p)) {
    showToast(gatedReason(p), 'error')
    return
  }
  try {
    const raw = draftText(profile)
    if (!raw) {
      showToast('Укажите номер стратегии', 'error')
      return
    }
    const n = Number(raw)
    if (!Number.isInteger(n) || n < 0) {
      showToast('Некорректный номер стратегии', 'error')
      return
    }
    const max = Number(p.max_strategy || 0)
    if (n !== 0 && max > 0 && n > max) {
      showToast(`Стратегия вне диапазона (1–${max}, или 0)`, 'error')
      return
    }
    const { lockRes, checkPayload } = await withBusy(`lock-${profile}`, async () => {
      const lockRes = await setLock(profile, n, scope.value)
      draft.value[profile] = String(n)
      delete strategyChecks[profile]
      await refreshAll()
      syncDraftFromLocks(locks.value, true)
      let checkPayload: Awaited<ReturnType<typeof profileCheck>> | null = null
      if (n !== 0) {
        try {
          checkPayload = await profileCheck(profile, scope.value, {
            waitApply: true,
            expectLock: String(n),
          })
          strategyChecks[profile] = checkPayload
        } catch (e) {
          showToast(`Лок записан, проверка не выполнена: ${(e as Error).message}`, 'warning')
        }
      }
      return { lockRes, checkPayload }
    })
    const changed = lockRes.changed ? `${lockRes.previous || '?'}→${lockRes.lock}` : `лок ${n}`
    if (n === 0) {
      showToast(`Профиль ${profile} выключен (${changed})`)
      return
    }
    if (checkPayload?.applied === false) {
      showToast(`Профиль ${profile}: store не подтвердил лок ${n} (сейчас ${checkPayload.lock})`, 'error')
      return
    }
    const v = checkPayload?.results?.[0]?.verdict
    if (v === 'ok') showToast(`Профиль ${profile}: ${changed} — цель доступна`)
    else if (v === 'warn') showToast(`Профиль ${profile}: ${changed} — с предупреждением`, 'warning')
    else if (v === 'fail') showToast(`Профиль ${profile}: ${changed} записан, цель недоступна`, 'warning')
    else if (checkPayload?.message) showToast(`Профиль ${profile}: ${changed}. ${checkPayload.message}`, 'info')
    else showToast(`Профиль ${profile}: ${changed}`)
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function reset(profile: string, p: ProfileInfo) {
  if (gated(p)) {
    showToast(gatedReason(p), 'error')
    return
  }
  try {
    let prev = p.current_lock || 'auto'
    await withBusy(`clear-${profile}`, async () => {
      const res = await clearLock(profile, scope.value)
      prev = res.previous || prev
      draft.value[profile] = ''
      delete strategyChecks[profile]
      await refreshAll()
    })
    showToast(`Профиль ${profile}: ${prev}→auto`)
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function check(profile: string, p: ProfileInfo) {
  if (gated(p)) {
    showToast(gatedReason(p), 'error')
    return
  }
  try {
    await withBusy(`check-${profile}`, async () => {
      const expect = String(p.current_lock || 'auto')
      const payload = await profileCheck(profile, scope.value, {
        waitApply: true,
        expectLock: expect === 'auto' ? undefined : expect,
      })
      strategyChecks[profile] = payload
      if (payload.applied === false) {
        showToast(`Лок в store ${payload.lock} ≠ UI ${expect}`, 'error')
        return
      }
      const v = payload.results?.[0]?.verdict
      if (v === 'ok') showToast(`Профиль ${profile}: цель доступна при локе ${payload.lock || expect}`)
      else if (v === 'warn') showToast(`Профиль ${profile}: предупреждение при проверке`, 'warning')
      else if (v === 'fail') showToast(`Профиль ${profile}: цель недоступна`, 'warning')
      else if (payload.message) showToast(payload.message, 'info')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function fixDiscord() {
  try {
    await withBusy('fix-discord', async () => {
      const res = await applySetting.fix_discord()
      await refreshAll()
      const label = res.result === 'ok' ? 'ok' : res.result === 'partial' ? 'частично' : 'fail'
      const udpLabel =
        res.udp === 'configured' ? 'настроен (без verify)' :
        res.udp === 'skipped' ? 'пропущен' :
        res.udp
      showToast(
        `Discord: ${label} (TCP ${res.tcp}, UDP ${udpLabel})`,
        res.result === 'ok' ? undefined : 'warning',
      )
      if (res.warning) showToast(res.warning, 'warning')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function sweepBroken() {
  try {
    await withBusy('sweep-broken', async () => {
      const res = await applySetting.sweep_broken()
      if (res.started) showToast(`Суперсвип сломанных: ${(res.workers || []).join(', ') || 'старт'}`)
      else showToast((res as ApplyResult & { message?: string }).message || 'Нет сломанных профилей')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function updateStrategiesFeed() {
  try {
    await withBusyRestart(async () => {
      const res = await applySetting.strategies_feed()
      await refreshAll()
      const reg = res.registered ?? 0
      const skip = res.skipped ?? 0
      const tag = res.tag ? ` ${res.tag}` : ''
      showToast(
        `Каталог стратегий${tag}: новых ${reg}, без изменений ${skip}.${restartSuffix(res)}`,
        reg > 0 ? undefined : 'info',
      )
      if (res.warning) showToast(res.warning, 'warning')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function applyRecs(profile: string) {
  try {
    await withBusy(`recs-${profile}`, async () => {
      const res = await applySetting.recommendations_apply(Number(profile), 3)
      await refreshAll()
      if (res.ok && res.strategy) {
        draft.value[profile] = String(res.strategy)
        showToast(`Профиль ${profile}: рекомендация ${res.strategy}`)
      } else {
        showToast((res as { error?: string }).error || 'Рекомендация не прошла verify', 'warning')
      }
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function saveFake(profile: string) {
  try {
    await withBusy(`fake-${profile}`, async () => {
      const res = await applySetting.fake_mode(profile, fakeMode.value[profile] || 'classic')
      showToast(`Режим фейков профиля ${profile}.${restartSuffix(res)}`)
      overrideDirty.value[profile] = false
      await refreshTlsBlobSettings()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function saveTlsProfile(profile: string) {
  try {
    await withBusy(`tlsprof-${profile}`, async () => {
      const res = await applySetting.tls_blob_profile(profile, tlsProfile.value[profile] ?? '')
      showToast(`TLS профиля ${profile}.${restartSuffix(res)}`)
      overrideDirty.value[profile] = false
      await refreshTlsBlobSettings()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function saveCloneSize(profile: string) {
  try {
    await withBusy(`clone-${profile}`, async () => {
      const res = await applySetting.clone_size(profile, cloneSize.value[profile] || '')
      showToast(`Лимит клона профиля ${profile}.${restartSuffix(res)}`)
      overrideDirty.value[profile] = false
      await refreshTlsBlobSettings()
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section>
    <div class="page-head sticky-head">
      <div>
        <h1>Стратегии</h1>
        <p>Локи профилей. Тестовые desync — во «Вручную».</p>
      </div>
      <div class="actions">
        <label class="field" style="min-width:140px">
          <span>Scope</span>
          <select :value="scope" :disabled="busyActive" @change="changeScope(($event.target as HTMLSelectElement).value)">
            <option v-for="s in scopeOptions" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'refresh-locks' }" @click="refresh">Обновить</button>
        <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === BUSY_RESTART }" @click="updateStrategiesFeed">Каталог стратегий</button>
        <button type="button" :disabled="busyActive" @click="fixDiscord">Починить Discord</button>
        <button type="button" :disabled="busyActive" @click="sweepBroken">Сломанные</button>
        <router-link class="ghost btn-link" to="/manual#supersweep-panel">Суперсвип</router-link>
      </div>
    </div>

    <p v-if="scopes.warning" class="panel-d" style="color:var(--warn)">{{ scopes.warning }}</p>

    <h2 class="section-title">Локи профилей</h2>
    <div v-if="!statusLoaded" class="loading">Загрузка…</div>
    <div v-else class="strategy-grid">
      <article :id="`strategy-${p.profile}`" v-for="p in locks" :key="p.profile" class="strategy" :class="{ 'is-gated': gated(p) }">
        <div class="strategy-head">
          <h3>{{ p.profile }}. {{ p.label }}</h3>
          <div class="meta">
            lock {{ p.current_lock || 'auto' }}
            <template v-if="p.lock_origin"> · origin {{ originLabel(p.lock_origin) }}</template>
            <template v-if="p.lock_source && p.lock_source !== 'auto'"> · src {{ p.lock_source }}</template>
            · max {{ p.max_strategy ?? '—' }}
            <template v-if="p.is_fallback"> · fb {{ p.fallback_enabled ? 'on' : 'off' }}</template>
            <template v-if="p.is_udp_games"> · games {{ p.udp_games_enabled ? 'on' : 'off' }}</template>
            <template v-if="p.is_dns_desync"> · dns {{ p.dns_desync_enabled ? 'on' : 'off' }}</template>
          </div>
        </div>
        <p v-if="p.description" class="meta" style="opacity:0.85">{{ p.description }}</p>
        <p v-if="gated(p)" class="panel-d" style="color:var(--warn);margin:0">{{ gatedReason(p) }}</p>
        <div class="strategy-actions">
          <input
            type="number" min="0" :max="p.max_strategy || 999"
            :value="draft[String(p.profile)] ?? ''"
            :disabled="busyActive || gated(p)" placeholder="№"
            @input="draft[String(p.profile)] = ($event.target as HTMLInputElement).value">
          <button type="button" class="primary" :disabled="busyActive || gated(p)"
            :class="{ 'is-busy': busyButton === `lock-${p.profile}` }"
            @click="save(String(p.profile), p)">Лок</button>
          <button type="button" class="ghost" :disabled="busyActive || gated(p)"
            :class="{ 'is-busy': busyButton === `clear-${p.profile}` }"
            @click="reset(String(p.profile), p)">Авто</button>
          <button type="button" class="ghost" :disabled="busyActive || gated(p)"
            :class="{ 'is-busy': busyButton === `check-${p.profile}` }"
            @click="check(String(p.profile), p)">Check</button>
          <button
            v-if="Number(p.profile) >= 1 && Number(p.profile) <= 4"
            type="button" class="ghost" :disabled="busyActive || gated(p)"
            :class="{ 'is-busy': busyButton === `recs-${p.profile}` }"
            @click="applyRecs(String(p.profile))"
          >Топ рек.</button>
        </div>
        <p v-if="draftDirty(p)" class="panel-d" style="color:var(--warn);margin:0">
          Черновик {{ draftText(String(p.profile)) }} ≠ {{ p.current_lock || 'auto' }}
        </p>
        <CheckResults
          v-if="!gated(p) && strategyChecks[String(p.profile)]"
          class="inline-check"
          :payload="strategyChecks[String(p.profile)]"
          empty-message="Нет результатов проверки."
          :empty-hidden="false"
        />
        <div v-if="canOverride(p)" class="grid-2" style="margin-top:0.35rem">
          <label class="field">
            <span>Режим фейков</span>
            <select v-model="fakeMode[String(p.profile)]" :disabled="busyActive"
              @change="markOverrideDirty(String(p.profile))">
              <option value="classic">classic</option>
              <option value="clone">clone</option>
            </select>
          </label>
          <div class="actions" style="align-items:end">
            <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === `fake-${p.profile}` }"
              @click="saveFake(String(p.profile))">Сохранить</button>
          </div>
          <label class="field">
            <span>TLS blob</span>
            <select v-model="tlsProfile[String(p.profile)]" :disabled="busyActive"
              @change="markOverrideDirty(String(p.profile))">
              <option value="">как глобальный</option>
              <option value="fake_default_tls">fake_default_tls</option>
              <option v-for="b in (tlsBlobSettings?.available_blobs || [])" :key="b" :value="b">{{ b }}</option>
            </select>
          </label>
          <div class="actions" style="align-items:end">
            <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === `tlsprof-${p.profile}` }"
              @click="saveTlsProfile(String(p.profile))">TLS</button>
          </div>
          <label v-if="fakeMode[String(p.profile)] === 'clone'" class="field">
            <span>Лимит клона</span>
            <input v-model="cloneSize[String(p.profile)]" :disabled="busyActive" placeholder="1200"
              @input="markOverrideDirty(String(p.profile))">
          </label>
          <div v-if="fakeMode[String(p.profile)] === 'clone'" class="actions" style="align-items:end">
            <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === `clone-${p.profile}` }"
              @click="saveCloneSize(String(p.profile))">Лимит</button>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>
