import { api, downloadBlob, formBody } from './client'
import type {
  ApplyResult, BackupsPayload, CheckPayload, DomainsImportResult, DomainsListPayload,
  FallbackSettings, IfacesSettings, LockResult, ModeSettingData, OkResult, PortInfo, PortsSettings, ProfileInfo, ProviderSettings,
  ScopesPayload, StatePayload, StatusPayload, TlsBlobSettings, UdpGamesSettings, UpdateCheckResult,
  WgBlobSettings, WgStateSettings, Recommendations, SupersweepPayload, WssizeRampSettings,
} from './types'

export const fetchStatus = (scope: string) =>
  api<StatusPayload>(`/api/status?scope=${encodeURIComponent(scope)}`)

export const fetchState = (scope: string) =>
  api<StatePayload>(`/api/state?scope=${encodeURIComponent(scope)}`)

export const fetchRecommendations = (signal?: AbortSignal) =>
  api<Recommendations>('/api/settings?setting=recommendations', { signal })

export const fetchSupersweep = (signal?: AbortSignal) =>
  api<SupersweepPayload>('/api/settings?setting=supersweep', { signal })

export const supersweepStart = (opts: {
  scope?: string
  domain?: string
  tls_pref?: string
} = {}) => {
  const body: Record<string, string> = { setting: 'supersweep', value: 'start' }
  if (opts.scope) body.scope = opts.scope
  if (opts.domain?.trim()) body.domain = opts.domain.trim()
  if (opts.tls_pref) body.tls_pref = opts.tls_pref
  return api<{ ok: boolean; started?: boolean }>('/api/settings', formBody(body))
}

export const supersweepCancel = () =>
  api<{ ok: boolean; cancelled?: boolean }>('/api/settings',
    formBody({ setting: 'supersweep', value: 'cancel' }))

export const fetchScopes = () => api<ScopesPayload>('/api/scopes')

export const setLock = (profile: string, strategy: number, scope: string) =>
  api<LockResult>('/api/set-lock',
    formBody({ profile, strategy, scope }))

export const clearLock = (profile: string, scope: string) =>
  api<LockResult>('/api/clear-lock',
    formBody({ profile, scope }))

export const serviceAction = (action: 'start' | 'stop' | 'restart') =>
  api<OkResult>('/api/service', formBody({ action }))

export const runCheck = () =>
  api<CheckPayload>('/api/check', { method: 'POST' })

export const profileCheck = (
  profile: string,
  scope: string,
  opts: { waitApply?: boolean; expectLock?: string } = {},
) =>
  api<CheckPayload>('/api/check',
    formBody({
      profile,
      scope,
      wait_apply: opts.waitApply ? '1' : undefined,
      expect_lock: opts.expectLock || undefined,
    }))

export const fetchTlsBlobSettings = () =>
  api<TlsBlobSettings>('/api/settings')

export const fetchSetting = {
  wg_blob: () => api<WgBlobSettings>('/api/settings?setting=wg_blob'),
  wg_state: () => api<WgStateSettings>('/api/settings?setting=wg_state'),
  fallback: () => api<FallbackSettings>('/api/settings?setting=fallback'),
  'udp-games': () => api<UdpGamesSettings>('/api/settings?setting=udp-games'),
  auto_mode: () => api<ModeSettingData>('/api/settings?setting=auto_mode'),
  hostlist: () => api<ModeSettingData>('/api/settings?setting=hostlist'),
  rst_guard: () => api<ModeSettingData>('/api/settings?setting=rst_guard'),
  reasm: () => api<ModeSettingData>('/api/settings?setting=reasm'),
  quic443: () => api<ModeSettingData>('/api/settings?setting=quic443'),
  dns_desync: () => api<ModeSettingData>('/api/settings?setting=dns_desync'),
  ports: () => api<PortsSettings>('/api/settings?setting=ports'),
  provider: () => api<ProviderSettings>('/api/settings?setting=provider'),
  update_check: () => api<UpdateCheckResult>('/api/settings?setting=update_check'),
  wssize_ramp: () => api<WssizeRampSettings>('/api/settings?setting=wssize_ramp'),
  ifaces: () => api<IfacesSettings>('/api/settings?setting=ifaces'),
  config_raw: () => api<{ content?: string; path?: string; error?: string }>('/api/settings?setting=config_raw'),
  nfqws2_errors: () => api<{ available?: boolean; content?: string; path?: string; lines?: number }>('/api/settings?setting=nfqws2_errors'),
}

export const registerStrategy = (opts: {
  profile: string
  desync: string
  lock?: boolean
  scope?: string
  restart?: boolean
}) =>
  api<ApplyResult & { strategy?: number; max_strategy?: number; locked?: boolean; profile?: number }>(
    '/api/settings',
    formBody({
      setting: 'strategy_register',
      profile: opts.profile,
      desync: opts.desync,
      lock: opts.lock ? '1' : '0',
      scope: opts.scope || 'default',
      restart: opts.restart === false ? '0' : '1',
    }))

export const uploadBlob = async (opts: {
  kind: 'tls' | 'wg'
  name: string
  file: Blob
  select?: boolean
  restart?: boolean
}) => {
  const q = new URLSearchParams({
    kind: opts.kind,
    name: opts.name,
  })
  if (opts.select) q.set('select', '1')
  if (opts.restart === false) q.set('restart', '0')
  return api<ApplyResult & { name?: string; kind?: string; available_blobs?: string[]; selected?: boolean }>(
    `/api/blob-upload?${q}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: opts.file,
    })
}

export const applySetting = {
  tls_blob: (value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting: 'tls_blob', value })),
  tls_blob_profile: (profile: string, value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting: 'tls_blob_profile', profile, value })),
  fake_mode: (profile: string, value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting: 'fake_mode', profile, value })),
  clone_size: (profile: string, value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting: 'clone_size', profile, value })),
  wg_blob: (value: string, restart: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody(restart ? { setting: 'wg_blob', value } : { setting: 'wg_blob', value, restart: '0' })),
  wg_repeats: (value: number, restart: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody(restart ? { setting: 'wg_repeats', value } : { setting: 'wg_repeats', value, restart: '0' })),
  wg_state: (enabled: boolean, restart: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody(restart
        ? { setting: 'wg_state', value: enabled ? '1' : '0' }
        : { setting: 'wg_state', value: enabled ? '1' : '0', restart: '0' })),
  fallback_state: (enabled: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'fallback_state', value: enabled ? '1' : '0' })),
  udp_games_state: (enabled: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'udp_games_state', value: enabled ? '1' : '0' })),
  mode_state: (postKey: string, enabled: boolean) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: postKey, value: enabled ? '1' : '0' })),
  ports_add: (proto: string, value: string) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'ports_add', proto, value })),
  ports_remove: (proto: string, value: string) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'ports_remove', proto, value })),
  provider_set: (name: string, city: string) =>
    api<OkResult>('/api/settings',
      formBody({ setting: 'provider_set', name, city })),
  provider_redetect: () =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'provider_redetect' })),
  wssize_ramp: (settings: WssizeRampSettings) =>
    api<ApplyResult>('/api/settings',
      formBody({ setting: 'wssize_ramp', value: settings.enabled ? '1' : '0',
        mode: settings.mode, scale: String(settings.scale) })),
  ifaces: (opts: { lan: string; wan: string } | { action: 'redetect' }) =>
    api<ApplyResult & IfacesSettings>('/api/settings',
      formBody('action' in opts
        ? { setting: 'ifaces', action: opts.action }
        : { setting: 'ifaces', action: 'set', lan: opts.lan, wan: opts.wan })),
  /** Простая переменная конфига с белым списком на сервере (fwtype, flowoffload, …). */
  value: (setting: string, value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting, value })),
  client_scope: (opts: { enable: boolean; mask: string; shift: string; max: string }) =>
    api<ApplyResult & { enabled?: boolean; warning?: string }>('/api/settings', formBody({
      setting: 'client_scope',
      client_scope_enable: opts.enable ? '1' : '0',
      client_scope_mark_mask: opts.mask,
      client_scope_mark_shift: opts.shift,
      client_scope_mark_max: opts.max,
    })),
  config_raw: (value: string) =>
    api<ApplyResult>('/api/settings', formBody({ setting: 'config_raw', value })),
  update_apply: () =>
    api<ApplyResult & { up_to_date?: boolean; installed?: number; release?: string; restart_required?: boolean }>(
      '/api/settings',
      { ...formBody({ setting: 'update_apply' }), timeoutMs: 200_000 },
    ),
}

export const fetchDomainsList = (list: string, scope = 'default') =>
  api<DomainsListPayload>(`/api/domains?${new URLSearchParams({ list, scope })}`)

const domainsPost = (body: Record<string, string>) =>
  api<ApplyResult & DomainsImportResult>('/api/domains', formBody(body))

export const domains = {
  add: (list: string, domain: string, scope = 'default') =>
    domainsPost({ list, action: 'add', domain, scope }),
  remove: (list: string, domain: string, scope = 'default') =>
    domainsPost({ list, action: 'remove', domain, scope }),
  rename: (list: string, domain: string, new_domain: string, scope = 'default') =>
    domainsPost({ list, action: 'rename', domain, new_domain, scope }),
  import: (list: string, domain: string, scope = 'default') =>
    domainsPost({ list, action: 'import', domain, scope }),
  clear: (list: string, scope = 'default') => domainsPost({ list, action: 'clear', scope }),
  refresh: (list: string, scope = 'default') =>
    domainsPost({ list, action: 'refresh', scope }),
  check: (domain: string) =>
    api<ApplyResult>('/api/domains',
      formBody({ list: 'custom_rkn', action: 'check', domain })),
  set_strategy: (domain: string, strategy: string, scope = 'default') =>
    domainsPost({ list: 'custom_rkn', action: 'set_strategy', domain, strategy, scope }),
  clear_strategy: (domain: string, scope = 'default') =>
    domainsPost({ list: 'custom_rkn', action: 'clear_strategy', domain, scope }),
}

export const logout = () => api<{ ok: boolean }>('/api/logout', formBody({}))

export const fetchBackups = () => api<BackupsPayload>('/api/backups')

export const backups = {
  create: () => api<{ name: string }>('/api/backups', formBody({ action: 'create' })),
  delete: (name: string) => api<OkResult>('/api/backups', formBody({ action: 'delete', name })),
  restore: (name: string, mode: 'full' | 'smart' | 'lists' = 'smart') =>
    api<{ ok: boolean; name: string; files?: number; restarted?: boolean }>(
      '/api/backups', formBody({ action: 'restore', name, mode })),
  downloadUrl: (name: string) => `/api/backups?action=download&name=${encodeURIComponent(name)}`,
  download: (name: string) => downloadBlob(
    `/api/backups?action=download&name=${encodeURIComponent(name)}`,
    name.endsWith('.tar') || name.endsWith('.tgz') || name.endsWith('.gz') ? name : `${name}.tar`,
  ),
  upload: (file: File) => api<{ name: string }>(
    `/api/backups?action=upload&name=${encodeURIComponent(file.name)}`,
    { method: 'POST', headers: { 'Content-Type': 'application/x-tar' }, body: file }),
}

export type { ProfileInfo, PortInfo }
