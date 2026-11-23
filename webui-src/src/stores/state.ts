import { fetchState } from '../api/endpoints'
import { backups } from './backups'
import {
  fallbackSettings, ifacesSettings, modeSettings, ports, provider, settingsLoaded, wssizeRampSettings,
  tlsBlobSettings, udpGamesSettings, wgBlobSettings, wgStateSettings,
} from './settings'
import { authRequired, locks, scope, scopes, status, statusLoaded, versionInfo } from './status'
import { showToast } from './toast'

function sectionError(value: unknown): string | null {
  if (!value || typeof value !== 'object') return null
  const err = (value as { _error?: unknown })._error
  return typeof err === 'string' && err ? err : null
}

function takeSection<T>(value: T | undefined, label: string, errors: string[]): T | null {
  const err = sectionError(value)
  if (err) {
    errors.push(`${label}: ${err}`)
    return null
  }
  return (value ?? null) as T | null
}

// Одна агрегирующая загрузка вместо ~15 отдельных CGI: state.cgi читает
// всё состояние за один процесс на роутере.
export async function fetchAndApplyState() {
  const payload = await fetchState(scope.value)
  const partial: string[] = []
  authRequired.value = payload.auth_required !== false
  status.value = payload.status
  versionInfo.value = payload.version
  locks.value = payload.status?.profiles || []
  scopes.value = payload.scopes
  tlsBlobSettings.value = takeSection(payload.tls_blob, 'TLS', partial) as typeof tlsBlobSettings.value
  wgBlobSettings.value = takeSection(payload.wg_blob, 'WG blob', partial) as typeof wgBlobSettings.value
  wgStateSettings.value = takeSection(payload.wg_state, 'WG', partial) as typeof wgStateSettings.value
  fallbackSettings.value = takeSection(payload.fallback, 'fallback', partial) as typeof fallbackSettings.value
  udpGamesSettings.value = takeSection(payload.udp_games, 'UDP games', partial) as typeof udpGamesSettings.value
  wssizeRampSettings.value = takeSection(payload.wssize_ramp, 'wssize', partial) as typeof wssizeRampSettings.value
  ifacesSettings.value = takeSection(payload.ifaces, 'ifaces', partial)
  const autoMode = takeSection(payload.auto_mode, 'auto_mode', partial)
  if (autoMode) modeSettings.auto_mode = autoMode
  const hostlist = takeSection(payload.hostlist, 'hostlist', partial)
  if (hostlist) modeSettings.hostlist = hostlist
  const rst = takeSection(payload.rst_guard, 'rst_guard', partial)
  if (rst) modeSettings.rst_guard = rst
  const reasm = takeSection(payload.reasm, 'reasm', partial)
  if (reasm) modeSettings.reasm = reasm
  const quic = takeSection(payload.quic443, 'quic443', partial)
  if (quic) modeSettings.quic443 = quic
  const dns = takeSection(payload.dns_desync, 'dns_desync', partial)
  if (dns) modeSettings.dns_desync = dns
  const portsSection = takeSection(payload.ports, 'ports', partial)
  if (portsSection) ports.value = portsSection
  const providerSection = takeSection(payload.provider, 'provider', partial)
  if (providerSection) provider.value = providerSection
  const bak = takeSection(payload.backups, 'backups', partial)
  if (bak) backups.value = bak
  statusLoaded.value = true
  settingsLoaded.value = true
  if (partial.length) {
    showToast(`Часть настроек не загрузилась: ${partial.slice(0, 3).join('; ')}`, 'warning')
  }
}
