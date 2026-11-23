import { ref } from 'vue'
import { resetDomains } from './domains'
import { settingsLoaded } from './settings'
import { statusLoaded } from './status'

/** Экран входа: mid-session 401 и явный logout. */
export const needLogin = ref(false)

/** Сброс загруженного состояния и показ формы входа. */
export function forceLogin() {
  needLogin.value = true
  statusLoaded.value = false
  settingsLoaded.value = false
  resetDomains()
}

export function clearLoginGate() {
  needLogin.value = false
}
