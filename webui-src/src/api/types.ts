export interface ClientScopeDiagnostics {
  mode?: string
  mask?: number | string
  shift?: number
  max_scope?: number
  scoped_lock_count?: number
  conflicts?: number
  last_seen_scope?: string
  fallback_reason?: string
}

export interface ProfileInfo {
  profile: string | number
  label: string
  description?: string
  current_lock?: string
  max_strategy?: number
  /** orchestra: scoped | default | auto | conflict */
  lock_source?: string
  /** apply-источник: recommendations | auto_relock | manual | fix_discord | supersweep */
  lock_origin?: string
  is_fallback?: boolean
  fallback_enabled?: boolean
  is_udp_games?: boolean
  udp_games_enabled?: boolean
  is_dns_desync?: boolean
  dns_desync_enabled?: boolean
}

export interface HealthPayload {
  overall?: string
  zapret2_running?: boolean
  want_running?: boolean
  watchdog?: boolean
  webui_watchdog?: boolean
  fw_hook_present?: boolean
  fw_ensure?: string
  args_present?: boolean
  post_update_verify?: string
  service_warnings?: number
  auto_relock?: boolean
  auto_relock_fails?: number
  degraded_profiles?: number[]
  busy_profile?: number
  warning?: string
  validator?: string
}

export interface StatusPayload {
  zapret2_running?: boolean
  nfqws_want_running?: boolean
  nfqws_watchdog?: boolean
  strategy_locks_status?: string
  auto_mode?: string
  hostlist_mode?: string
  fwtype?: string
  flowoffload?: string
  tls_blob_mode?: string
  wireguard?: string
  rst_guard?: string
  reasm?: string
  quic443?: string
  provider?: string
  client_scope?: ClientScopeDiagnostics
  profiles?: ProfileInfo[]
  health?: HealthPayload
  service_warnings?: string[]
  args_present?: boolean
  fw_hook_present?: boolean
  pre_update_backup?: string
  post_update_verify?: string
  post_update_verify_detail?: string
  block_full_restore?: boolean
}

export interface ScopesPayload {
  enabled?: boolean
  warning?: string
  scopes?: string[]
  mask?: string | number
  shift?: number
  max_scope?: number
  mode?: string
}

export interface CheckDetail {
  text?: string
  state?: string
}

export interface CheckItem {
  label?: string
  target?: string
  verdict?: string
  text?: string
  tls12?: boolean
  tls13?: boolean
  tls12_detail?: CheckDetail
  tls13_detail?: CheckDetail
  download?: CheckDetail
}

export interface CheckPayload {
  results?: CheckItem[]
  message?: string
  profile?: number
  scope?: string
  lock?: string
  zapret2_running?: boolean
  waited_apply?: boolean
  applied?: boolean
  retried?: boolean
}

/** Batch /api/check?profiles=1,2 (RM0c). */
export interface BatchCheckPayload {
  ok?: boolean
  checks?: CheckPayload[]
}

export interface CapabilitiesPayload {
  ok?: boolean
  version?: string
  goos?: string
  goarch?: string
  auth_required?: boolean
  features?: Record<string, boolean | number | string>
  endpoints?: string[]
}

export interface ApiEvent {
  type: string
  state?: string
  profile?: number
  message?: string
  ts?: number
}

export interface EventsPayload {
  ok?: boolean
  mode?: 'long-poll' | string
  after?: number
  seq?: number
  events?: ApiEvent[]
}

export interface RepairResult {
  ok?: boolean
  started?: boolean
  mode?: string
  previous_lock?: string
  scope?: string
  profile?: number
  async?: boolean
  worker?: string
  [key: string]: unknown
}

export interface LockResult {
  ok?: boolean
  previous?: string
  lock?: string
  changed?: boolean
  scope?: string
  profile?: number
  warning?: string
}

export interface TlsBlobSettings {
  current_mode?: string
  current_blob?: string
  available_blobs?: string[]
  // «профиль -> значение»: "" = как глобальный, fake_default_tls | файл слота
  profile_blobs?: Record<string, string>
  // «профиль -> режим фейков»: classic | clone (нет строки = classic)
  profile_modes?: Record<string, string>
  // «профиль -> SNI клона»: "" = невинный дефолт (www.google.com)
  profile_snis?: Record<string, string>
  // «профиль -> лимит клонов в байтах»: "" = без ограничения (граница ТСПУ 1200)
  profile_sizes?: Record<string, string>
}

export interface WgBlobSettings {
  current_blob?: string
  current_repeats?: string
  available_blobs?: string[]
}

export interface WgStateSettings {
  enabled?: boolean
}

export interface FallbackSettings {
  state?: string
  enabled?: boolean
}

export interface UdpGamesSettings {
  enabled?: boolean
  ports?: string
}

export interface WssizeRampSettings {
  enabled: boolean
  mode: 'full' | 'lite'
  scale: number
  googlevideo_ips: number
  googlevideo_local_collection: 'enabled' | 'disabled' | 'unsupported'
}

export interface ModeSettingData {
  enabled?: boolean
  auto?: boolean
  lua_available?: boolean
}

export interface PortInfo {
  full?: string
  user?: string[]
  base?: string
}

export interface PortsSettings {
  tcp?: PortInfo
  udp?: PortInfo
}

export interface RecommendationProfile {
  samples: number
  top: { strategy: number; success_pct: number; samples: number; mode: 'classic' | 'clone' | 'mixed' }[]
  clone_recommended: boolean
  classic_pct: number | null
  clone_pct: number | null
}

export interface Recommendations {
  provider: string
  samples: number
  minimum: 10
  generated_at: number
  status: 'ready' | 'insufficient' | 'unavailable' | 'unknown_provider' | 'stale'
  profiles: Record<string, RecommendationProfile>
}

export interface SupersweepPayload {
  available: boolean
  state: 'idle' | 'starting' | 'running' | 'applying' | 'done' | 'cancelled' | 'interrupted'
  active: boolean
  source: 'none' | 'live' | 'saved'
  started: number
  updated: number
  scope: string
  phase: string
  completed: number
  total: number
  workers: {
    name: string
    label: string
    profile: number
    completed: number
    total: number
    ok: number
    warn: number
    fail: number
    current_strategy: number | null
    current_target: string
    greens: string
    warns: string
    applied: string
  }[]
  applied: { kind: 'profile' | 'domain'; target: string; strategy: number }[]
  domains: {
    domain: string
    best_strategy: number | null
    verdict: 'ok' | 'warn' | 'fail'
    applied: number | null
  }[]
}

export interface ProviderSettings {
  provider?: string
}

export interface BackupItem {
  name: string
  date?: string
  size?: number
}

export interface BackupsPayload {
  items?: BackupItem[]
}

export interface DomainItem {
  value: string
  strategy?: number
  ip?: string
  observed_at?: number
  source?: string
}

export interface GooglevideoAutoLock {
  host: string
  strategy: number
  scope?: string
  askey?: string
  /** seed = фоновый TLS-пробой; learned = живой circular_quality */
  source?: 'seed' | 'learned' | string
}

export interface GooglevideoSeedStatus {
  state?: string
  host_key?: string
  strategy?: number
  last_host?: string
  last_strategy?: number
  last_result?: string
  last_error?: string
  updated_at?: number
  pending?: number
}

export interface DomainsListPayload {
  title?: string
  description?: string
  items?: DomainItem[]
  is_custom_rkn?: boolean
  max_strategy?: number
  collection?: string
  /** Per-SNI (sn-*) auto-locks for profile 2 */
  auto_locks?: GooglevideoAutoLock[]
  seed?: GooglevideoSeedStatus
  quality?: {
    seed_state?: string
    auto_locks_count?: number
    ipv4_count?: number
    ipv4_fresh?: boolean
    wssize?: boolean
    hosts_count?: number
  }
}

export interface ApplyResult {
  ok?: boolean
  restarted?: boolean
  restart_required?: boolean
  /** ports_add отдаёт строку «8080,9000»; domains — число. */
  added?: number | string
  skipped?: string
  name?: string
  provider?: string
  duplicate?: boolean
  check?: CheckPayload
}

/** Успешный ответ без полезной нагрузки (service / delete backup). */
export interface OkResult {
  ok?: boolean
  restarted?: boolean
  restart_required?: boolean
}

export interface UpdateCheckResult {
  update_available: boolean
  release?: string
  latest_zator_date?: string
  latest_webui_date?: string
  checked_at?: string
  error?: string
}

export interface DomainsImportResult {
  added?: number
  duplicates?: number
  skipped?: number
}

export interface VersionInfo {
  zapret2_version: string
  zator_version: string
  zator_date: string
  webui_version: string
  webui_date: string
  tracking: string
  update_available: boolean
  latest_release: string
  latest_zator_date: string
  latest_webui_date: string
  config_date: string
  config_default_date: string
  config_update_pending: boolean
}

export interface ValueSetting {
  state?: string
  value?: string
}

export interface IfaceInfo {
  name: string
  up?: boolean
  addrs?: string[]
  kind?: string
}

export interface IfacesSettings {
  wan?: string
  lan?: string
  wan6?: string
  suggested?: { wan?: string; lan?: string; source?: string }
  available?: IfaceInfo[]
  needs_autodetect?: boolean
}

export interface StatePayload {
  auth_required?: boolean
  status: StatusPayload
  version: VersionInfo
  scopes: ScopesPayload
  tls_blob: TlsBlobSettings
  wg_blob: WgBlobSettings
  wg_state: WgStateSettings
  fallback: FallbackSettings
  udp_games: UdpGamesSettings
  wssize_ramp: WssizeRampSettings
  auto_mode: ModeSettingData
  hostlist: ModeSettingData
  rst_guard: ModeSettingData
  reasm: ModeSettingData
  quic443: ModeSettingData
  dns_desync: ModeSettingData
  ports: PortsSettings
  provider: ProviderSettings
  fwtype?: ValueSetting
  flowoffload?: ValueSetting
  ifaces?: IfacesSettings
  client_scope?: ScopesPayload
  nfqws2_errors?: { available?: boolean; content?: string; path?: string; lines?: number }
  backups: BackupsPayload
  /** RM0c: при ?include=capabilities|journal|health */
  capabilities?: CapabilitiesPayload
  journal?: {
    enabled?: boolean
    entries?: { ts?: string; profile?: number; strategy?: number; result?: string; source?: string }[]
  }
}
