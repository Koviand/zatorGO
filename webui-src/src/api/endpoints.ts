import { api, downloadBlob, formBody } from './client'
import type {
  ApplyResult, BackupsPayload, BatchCheckPayload, CapabilitiesPayload, CheckPayload,
  DomainsImportResult, DomainsListPayload, EventsPayload, FallbackSettings, IfacesSettings,
  LockResult, ModeSettingData, OkResult, PortInfo, PortsSettings, ProfileInfo, ProviderSettings,
  RepairResult, ScopesPayload, StatePayload, StatusPayload, TlsBlobSettings, UdpGamesSettings,
  UpdateCheckResult, WgBlobSettings, WgStateSettings, Recommendations, SupersweepPayload, WssizeRampSettings,
} from './types'

export const login = (password: string) =>
  api<{ ok?: boolean }>('/api/login', formBody({ password }))

export const fetchStatus = (scope: string) =>
  api<StatusPayload>(`/api/status?scope=${encodeURIComponent(scope)}`)

export const fetchState = (scope: string, include?: string[]) => {
  const q = new URLSearchParams({ scope })
  if (include?.length) q.set('include', include.join(','))
  return api<StatePayload>(`/api/state?${q}`)
}

export const fetchCapabilities = () =>
  api<CapabilitiesPayload>('/api/capabilities')

export const fetchEvents = (opts: {
  after?: number
  timeout?: number
  type?: string
  signal?: AbortSignal
} = {}) => {
  const q = new URLSearchParams()
  if (opts.after != null) q.set('after', String(opts.after))
  if (opts.timeout != null) q.set('timeout', String(opts.timeout))
  if (opts.type) q.set('type', opts.type)
  const qs = q.toString()
  // long-poll: таймаут клиента чуть больше серверного wait
  const serverWait = (opts.timeout ?? 25) * 1000
  return api<EventsPayload>(`/api/events${qs ? `?${qs}` : ''}`, {
    signal: opts.signal,
    timeoutMs: Math.max(serverWait + 5000, 30_000),
  })
}

export const repairProfile = (opts: {
  profile: number | string
  mode?: 'auto' | 'relock' | 'recommendations' | 'sweep'
  scope?: string
  top_k?: number
}) =>
  api<RepairResult>('/api/repair', formBody({
    profile: opts.profile,
    mode: opts.mode || 'auto',
    scope: opts.scope || 'default',
    top_k: opts.top_k != null ? opts.top_k : undefined,
  }))

export const fetchRecommendations = (signal?: AbortSignal) =>
  api<Recommendations>('/api/settings?setting=recommendations', { signal })

export const fetchBypassFlags = (signal?: AbortSignal) =>
  api<{
    auto_relock?: boolean
    auto_relock_fails?: number
    sweep_on_provider_change?: boolean
    sweep_schedule_hours?: number
    blob_fallback?: boolean
    journal_enabled?: boolean
  }>('/api/settings?setting=bypass_flags', { signal })

export const fetchStrategyJournal = (signal?: AbortSignal) =>
  api<{
    enabled?: boolean
    entries?: { ts?: string; profile?: number; strategy?: number; result?: string; source?: string }[]
  }>('/api/settings?setting=strategy_journal', { signal })

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

/** Batch check: profiles=1,2,4 → {ok, checks:[...]} (RM0c). */
export const batchCheck = (profiles: Array<number | string>, scope = 'default') =>
  api<BatchCheckPayload>('/api/check',
    formBody({ profiles: profiles.join(','), scope }))

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
  auto_relock: (enabled: boolean, fails?: number) =>
    api<ApplyResult & { auto_relock?: boolean; auto_relock_fails?: number }>('/api/settings',
      formBody({
        setting: 'auto_relock', value: enabled ? '1' : '0',
        ...(fails != null ? { fails: String(fails) } : {}),
      })),
  journal_enabled: (enabled: boolean) =>
    api<ApplyResult & { journal_enabled?: boolean; enabled?: boolean }>('/api/settings',
      formBody({ setting: 'journal_enabled', value: enabled ? '1' : '0' })),
  sweep_on_provider_change: (enabled: boolean) =>
    api<ApplyResult & { sweep_on_provider_change?: boolean; enabled?: boolean }>('/api/settings',
      formBody({ setting: 'sweep_on_provider_change', value: enabled ? '1' : '0' })),
  sweep_schedule_hours: (hours: number) =>
    api<ApplyResult & { sweep_schedule_hours?: number; warning?: string }>('/api/settings',
      formBody({ setting: 'sweep_schedule_hours', value: String(hours) })),
  blob_fallback: (enabled: boolean) =>
    api<ApplyResult & { blob_fallback?: boolean; enabled?: boolean }>('/api/settings',
      formBody({ setting: 'blob_fallback', value: enabled ? '1' : '0' })),
  recommendations_apply: (profile: number, topK = 1) =>
    api<ApplyResult & { strategy?: number; rolled_back?: boolean; origin?: string }>('/api/settings',
      formBody({ setting: 'recommendations_apply', profile: String(profile), top_k: String(topK) })),
  fix_discord: () =>
    api<{
      ok: boolean
      result: 'ok' | 'partial' | 'fail' | string
      tcp: 'ok' | 'fail' | string
      /** configured = стратегия+порты; не TLS-verify голоса */
      udp: 'configured' | 'fail' | 'skipped' | string
      udp_verified?: boolean
      tcp_strategy?: number
      voice_strategy?: number
      hostlist_ok?: boolean
      warning?: string
    }>('/api/settings', formBody({ setting: 'fix_discord' })),
  sweep_broken: () =>
    api<ApplyResult & { started?: boolean; workers?: string[] }>('/api/settings',
      formBody({ setting: 'sweep_broken' })),
  strategies_feed: () =>
    api<ApplyResult & {
      applied?: number
      registered?: number
      skipped?: number
      tag?: string
      path?: string
      warning?: string
    }>('/api/settings', formBody({ setting: 'strategies_feed', value: 'update' })),
  scope_preset: (preset: string, copyLocks = false) =>
    api<ApplyResult & { scope?: string; hint?: string }>('/api/settings',
      formBody({
        setting: 'scope_preset', preset,
        ...(copyLocks ? { copy_locks: '1' } : {}),
      })),
  scope_copy_locks: (scope: string, undo = false) =>
    api<ApplyResult & { copied?: number; cleared?: number; undone?: boolean; scope?: string }>(
      '/api/settings',
      formBody({
        setting: 'scope_copy_locks',
        scope,
        ...(undo ? { undo: '1' } : {}),
      })),
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
  refresh_ips: (list = 'googlevideo', scope = 'default') =>
    domainsPost({ list, action: 'refresh_ips', scope }),
  check: (domain: string) =>
    api<ApplyResult>('/api/domains',
      formBody({ list: 'custom_rkn', action: 'check', domain })),
  set_strategy: (domain: string, strategy: string, scope = 'default') =>
    domainsPost({ list: 'custom_rkn', action: 'set_strategy', domain, strategy, scope }),
  clear_strategy: (domain: string, scope = 'default') =>
    domainsPost({ list: 'custom_rkn', action: 'clear_strategy', domain, scope }),
  refresh_check: () =>
    domainsPost({ list: 'googlevideo', action: 'refresh_check' }),
  explain: (domain: string) =>
    api<{
      ok: boolean
      matched: boolean
      matches: { list: string; kind: string; match: string; role?: string; entry?: string; note?: string }[]
      domain: string
      hostlist_mode?: string
      reason?: string
      custom_rkn_hint?: string
    }>(`/api/domains?${new URLSearchParams({ action: 'explain', domain })}`),
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
