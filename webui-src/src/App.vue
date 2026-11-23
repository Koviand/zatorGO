<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, formBody } from './api/client'
import { logout as apiLogout, serviceAction } from './api/endpoints'
import { clearLoginGate, forceLogin, needLogin } from './stores/auth'
import { busyActive, busyButton, withBusy } from './stores/busy'
import { fetchAndApplyState } from './stores/state'
import { authRequired, refreshAll, status, statusLoaded } from './stores/status'
import { showToast } from './stores/toast'
import { theme, type ThemeMode } from './stores/theme'
import ToastHost from './components/ui/ToastHost.vue'
import ConfirmDialog from './components/ui/ConfirmDialog.vue'

const STATUS_POLL_MS = 15_000

const route = useRoute()
const tabs = [
  { to: '/', name: 'status', label: 'Обзор', short: 'Обзор', hint: 'Пульт состояния' },
  { to: '/strategies', name: 'strategies', label: 'Стратегии', short: 'Локи', hint: 'Локи профилей' },
  { to: '/manual', name: 'manual', label: 'Вручную', short: 'Тест', hint: 'Desync / blob / supersweep' },
  { to: '/domains', name: 'domains', label: 'Домены', short: 'Списки', hint: 'Hostlist / googlevideo' },
  { to: '/settings', name: 'settings', label: 'Настройки', short: 'Опции', hint: 'Режимы и сеть' },
]
const themes: { id: ThemeMode; label: string }[] = [
  { id: 'light', label: 'Светлая' },
  { id: 'dark', label: 'Тёмная' },
  { id: 'auto', label: 'Авто' },
]
const active = computed(() => String(route.name || 'status'))

const loginPass = ref('')
const loginError = ref('')
const loginBusy = ref(false)
const logoutBusy = ref(false)
const bootstrapError = ref('')
const bootstrapBusy = ref(false)
let pollTimer = 0
let pollInFlight = false

const running = computed(() => Boolean(status.value?.zapret2_running))
const healthOverall = computed(() => status.value?.health?.overall || (running.value ? 'ok' : 'down'))
const warnCount = computed(() => {
  const n = status.value?.health?.service_warnings
  if (typeof n === 'number') return n
  return (status.value?.service_warnings || []).length
})
const healthLabel = computed(() => {
  switch (healthOverall.value) {
    case 'ok': return 'Норма'
    case 'degraded': return 'Деградация'
    case 'down': return 'Стоп'
    default: return healthOverall.value
  }
})

function setTheme(mode: ThemeMode) {
  theme.value = mode
}

function stopStatusPoll() {
  window.clearTimeout(pollTimer)
  pollTimer = 0
}

function scheduleStatusPoll() {
  stopStatusPoll()
  if (needLogin.value || !!bootstrapError.value || document.hidden) return
  pollTimer = window.setTimeout(() => { void pollStatus() }, STATUS_POLL_MS)
}

async function pollStatus() {
  if (pollInFlight || needLogin.value || !!bootstrapError.value || document.hidden || busyActive.value) {
    scheduleStatusPoll()
    return
  }
  pollInFlight = true
  try {
    await refreshAll()
  } catch {
    // фоновый опрос: тосты не спамим
  } finally {
    pollInFlight = false
    scheduleStatusPoll()
  }
}

function onVisibilityChange() {
  if (document.hidden) {
    stopStatusPoll()
    return
  }
  if (statusLoaded.value && !needLogin.value && !bootstrapError.value) {
    void pollStatus()
  }
}

async function bootstrap() {
  bootstrapBusy.value = true
  bootstrapError.value = ''
  try {
    await fetchAndApplyState()
    clearLoginGate()
    scheduleStatusPoll()
  } catch (e) {
    const err = e as Error & { status?: number }
    if (err.status === 401) {
      forceLogin()
      return
    }
    statusLoaded.value = false
    bootstrapError.value = err.message || 'Не удалось загрузить состояние'
    stopStatusPoll()
  } finally {
    bootstrapBusy.value = false
  }
}

async function submitLogin() {
  loginError.value = ''
  loginBusy.value = true
  try {
    await api('/api/login', formBody({ password: loginPass.value }))
    loginPass.value = ''
    clearLoginGate()
    statusLoaded.value = false
    // При ошибке bootstrap покажется экран «Повторить», а не пустая оболочка.
    await bootstrap()
  } catch (e) {
    loginError.value = (e as Error).message
  } finally {
    loginBusy.value = false
  }
}

async function logout() {
  if (!authRequired.value) return
  logoutBusy.value = true
  stopStatusPoll()
  try {
    try {
      await apiLogout()
    } catch {
      // ignore
    }
    forceLogin()
    loginPass.value = ''
    loginError.value = ''
    bootstrapError.value = ''
  } finally {
    logoutBusy.value = false
  }
}

async function stripSvc(action: 'start' | 'stop') {
  try {
    await withBusy(`strip-${action}`, async () => {
      await serviceAction(action)
      await refreshAll()
      // start асинхронный — короткий догон статуса для шапки.
      if (action === 'start') {
        await new Promise((r) => setTimeout(r, 1500))
        try { await refreshAll() } catch { /* ignore */ }
      }
    })
    showToast(action === 'stop' ? 'zapret2 остановлен' : 'zapret2 запущен')
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibilityChange)
  void bootstrap()
})
onUnmounted(() => {
  stopStatusPoll()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div v-if="needLogin" class="login-screen">
    <form class="login-card" @submit.prevent="submitLogin">
      <img class="login-logo" :src="'./icon.png'" width="72" height="72" alt="ZatorGO">
      <div class="login-brand">Zator<span>GO</span></div>
      <p>Операторская панель zapret2</p>
      <p v-if="loginError" class="login-err">{{ loginError }}</p>
      <label>
        <span>Пароль</span>
        <input v-model="loginPass" type="password" autocomplete="current-password" required autofocus :disabled="loginBusy">
      </label>
      <button type="submit" class="primary" :disabled="loginBusy || !loginPass">Войти</button>
    </form>
  </div>
  <div v-else-if="bootstrapError" class="login-screen">
    <div class="login-card">
      <img class="login-logo" :src="'./icon.png'" width="72" height="72" alt="ZatorGO">
      <div class="login-brand">Zator<span>GO</span></div>
      <p>Не удалось загрузить состояние</p>
      <p class="login-err">{{ bootstrapError }}</p>
      <button type="button" class="primary" :disabled="bootstrapBusy" @click="bootstrap">
        {{ bootstrapBusy ? 'Повтор…' : 'Повторить' }}
      </button>
    </div>
  </div>
  <div v-else class="shell">
    <aside class="rail" aria-label="Навигация">
      <div class="brand">
        <div class="brand-row">
          <img class="brand-logo" :src="'./icon.png'" width="38" height="38" alt="">
          <div class="brand-text">
            <div class="brand-mark">Zator<span>GO</span></div>
            <div class="brand-sub">Консоль обхода DPI</div>
          </div>
        </div>
      </div>
      <nav class="nav" aria-label="Разделы">
        <router-link
          v-for="t in tabs"
          :key="t.name"
          :to="t.to"
          :class="{ 'is-active': active === t.name, 'is-locked': busyActive || !statusLoaded }"
        >
          <strong>{{ t.label }}</strong>
          <small>{{ t.hint }}</small>
        </router-link>
      </nav>
      <div class="rail-foot">
        <div class="theme-field">
          <span>Тема</span>
          <div class="theme-seg" role="group" aria-label="Тема">
            <button
              v-for="t in themes"
              :key="t.id"
              type="button"
              :class="{ 'is-active': theme === t.id }"
              :aria-pressed="theme === t.id"
              @click="setTheme(t.id)"
            >{{ t.label }}</button>
          </div>
        </div>
        <button
          v-if="authRequired"
          type="button"
          class="ghost logout-btn"
          :disabled="logoutBusy || busyActive"
          @click="logout"
        >Выйти</button>
      </div>
    </aside>

    <div class="workspace">
      <header class="status-strip" aria-label="Состояние службы">
        <div class="strip-left">
          <span class="strip-dot" :class="healthOverall" :title="healthLabel"></span>
          <div class="strip-meta">
            <strong class="mono">{{ running ? 'zapret2 UP' : 'zapret2 DOWN' }}</strong>
            <span>{{ healthLabel }}<template v-if="warnCount"> · {{ warnCount }} предупр.</template></span>
          </div>
        </div>
        <div class="strip-actions">
          <button
            type="button"
            class="primary strip-btn"
            :disabled="busyActive || !statusLoaded"
            :class="{ 'is-busy': busyButton === (running ? 'strip-stop' : 'strip-start') }"
            @click="stripSvc(running ? 'stop' : 'start')"
          >{{ running ? 'Стоп' : 'Старт' }}</button>
        </div>
      </header>

      <main class="main">
        <router-view />
      </main>
    </div>

    <nav class="tab-bar" aria-label="Мобильная навигация">
      <router-link
        v-for="t in tabs"
        :key="t.name"
        :to="t.to"
        :class="{ 'is-active': active === t.name, 'is-locked': busyActive || !statusLoaded }"
      >{{ t.short }}</router-link>
    </nav>

    <ToastHost />
    <ConfirmDialog />
  </div>
</template>
