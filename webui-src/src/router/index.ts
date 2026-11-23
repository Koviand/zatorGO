import { createRouter, createWebHashHistory } from 'vue-router'
import OverviewView from '../views/OverviewView.vue'
import StrategiesView from '../views/StrategiesView.vue'
import ManualAddView from '../views/ManualAddView.vue'
import DomainsView from '../views/DomainsView.vue'
import SettingsView from '../views/SettingsView.vue'
import { busyActive } from '../stores/busy'
import { statusLoaded } from '../stores/status'
import { showToast } from '../stores/toast'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'status', component: OverviewView },
    { path: '/strategies', name: 'strategies', component: StrategiesView },
    { path: '/manual', name: 'manual', component: ManualAddView },
    { path: '/domains/:list?', name: 'domains', component: DomainsView },
    { path: '/settings/:panel?', name: 'settings', component: SettingsView },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior(to) {
    // /settings/:panel скроллит SettingsView сама — иначе top:0 перебивает якорь.
    if (to.name === 'settings' && to.params.panel) return false
    if (to.hash) return { el: to.hash, behavior: 'smooth' }
    return { top: 0 }
  },
})

let first = true
router.beforeEach(() => {
  if (first) {
    first = false
    return true
  }
  // Пока state не загружен — не блокируем жёстко: App показывает экран «Повторить».
  // Тост только если оболочка уже видна (statusLoaded успел стать true и снова сбросился).
  if (!statusLoaded.value) {
    showToast('Дождитесь загрузки состояния', 'error')
    return false
  }
  // Busy не блокирует навигацию: вкладки уже помечаются is-locked в App.vue.
  // Жёсткий return false оставлял UI «навсегда», если CGI завис без таймаута.
  if (busyActive.value) {
    showToast('Идёт операция — подождите завершения', 'info')
  }
  return true
})
