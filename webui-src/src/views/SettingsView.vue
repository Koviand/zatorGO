<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BackupsPanel from '../components/settings/BackupsPanel.vue'
import ConfigPanel from '../components/settings/ConfigPanel.vue'
import ErrorsPanel from '../components/settings/ErrorsPanel.vue'
import FirewallPanel from '../components/settings/FirewallPanel.vue'
import IfacesPanel from '../components/settings/IfacesPanel.vue'
import ModesPanel from '../components/settings/ModesPanel.vue'
import PortsPanel from '../components/settings/PortsPanel.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'
import ScopesPanel from '../components/settings/ScopesPanel.vue'
import TlsPanel from '../components/settings/TlsPanel.vue'
import UpdatesPanel from '../components/settings/UpdatesPanel.vue'
import WgPanel from '../components/settings/WgPanel.vue'
import WssizePanel from '../components/settings/WssizePanel.vue'
import { busyActive, busyButton, withBusy } from '../stores/busy'
import { settingsLoaded } from '../stores/settings'
import { fetchAndApplyState } from '../stores/state'
import { showToast } from '../stores/toast'

const route = useRoute()
const router = useRouter()
const configPanel = ref<{ loadConfig: () => Promise<void> } | null>(null)
const errorsPanel = ref<{ loadErrors: () => Promise<void> } | null>(null)

const sections = [
  { id: 'sec-updates', label: 'Обновления', group: 'Система' },
  { id: 'sec-provider', label: 'Провайдер', group: 'Система' },
  { id: 'sec-backups', label: 'Бэкапы', group: 'Система' },
  { id: 'sec-modes', label: 'Режимы', group: 'Обход' },
  { id: 'sec-tls', label: 'TLS', group: 'Обход' },
  { id: 'sec-wg', label: 'WireGuard', group: 'Обход' },
  { id: 'sec-wssize', label: 'WSSIZE', group: 'Обход' },
  { id: 'sec-firewall', label: 'Firewall', group: 'Сеть' },
  { id: 'sec-ifaces', label: 'Интерфейсы', group: 'Сеть' },
  { id: 'sec-ports', label: 'Порты', group: 'Сеть' },
  { id: 'sec-scopes', label: 'Scopes', group: 'Сеть' },
  { id: 'sec-config', label: 'Конфиг', group: 'Сервис' },
  { id: 'sec-errors', label: 'Ошибки', group: 'Сервис' },
]

const settingGroups = [
  { id: 'grp-system', title: 'Система', desc: 'Обновления, провайдер и резервные копии.' },
  { id: 'grp-bypass', title: 'Обход DPI', desc: 'Режимы стратегий, TLS/WG blob и wssize.' },
  { id: 'grp-net', title: 'Сеть', desc: 'Firewall, интерфейсы WAN/LAN, порты NFQUEUE и client scopes.' },
  { id: 'grp-svc', title: 'Сервис', desc: 'Сырой конфиг zapret2 и журнал ошибок.' },
]

const panelAliases: Record<string, string> = {
  updates: 'sec-updates', provider: 'sec-provider', backups: 'sec-backups',
  modes: 'sec-modes', 'auto-mode': 'sec-modes', fallback: 'sec-modes', hostlist: 'sec-modes',
  'udp-games': 'sec-modes', 'dns-desync': 'sec-modes', 'rst-guard': 'sec-modes',
  reasm: 'sec-modes', quic443: 'sec-modes',
  tls: 'sec-tls', 'tls-blob': 'sec-tls',
  wg: 'sec-wg', wireguard: 'sec-wg',
  wssize: 'sec-wssize', 'wssize-ramp': 'sec-wssize',
  firewall: 'sec-firewall', fwtype: 'sec-firewall', flowoffload: 'sec-firewall',
  ifaces: 'sec-ifaces', iface: 'sec-ifaces', interfaces: 'sec-ifaces',
  ports: 'sec-ports', scopes: 'sec-scopes', 'client-scope': 'sec-scopes',
  config: 'sec-config', errors: 'sec-errors',
}

function panelIdFromRoute(): string {
  const raw = String(route.params.panel || (route.hash || '').replace(/^#/, '') || '')
  if (!raw) return ''
  return panelAliases[raw] || (raw.startsWith('sec-') ? raw : `sec-${raw}`)
}

function scrollToPanel(forceId?: string) {
  const id = forceId || panelIdFromRoute()
  if (!id || !settingsLoaded.value) return
  const el = document.getElementById(id)
  if (!el) return
  const nav = document.querySelector('.settings-nav') as HTMLElement | null
  const strip = document.querySelector('.status-strip') as HTMLElement | null
  const offset = (nav?.offsetHeight || 0) + (strip?.offsetHeight || 0) + 12
  const top = el.getBoundingClientRect().top + window.scrollY - offset
  window.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
}

function goSection(id: string, ev?: Event) {
  ev?.preventDefault()
  const slug = id.replace(/^sec-/, '')
  const target = `/settings/${slug}`
  if (route.path === target || route.fullPath.endsWith(`/settings/${slug}`)) {
    void nextTick(() => scrollToPanel(id))
    return
  }
  void router.push(target).then(() => {
    void nextTick(() => {
      requestAnimationFrame(() => scrollToPanel(id))
    })
  })
}

async function refresh() {
  try {
    await withBusy('refresh-settings', fetchAndApplyState)
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

watch(() => [route.params.panel, route.hash, settingsLoaded.value] as const, async () => {
  if (!settingsLoaded.value) return
  await nextTick()
  requestAnimationFrame(() => scrollToPanel())
}, { immediate: true })

// Последовательно: параллельный withBusy раньше сбрасывал busy чужим finally.
watch(settingsLoaded, async (ready) => {
  if (!ready) return
  await nextTick()
  await configPanel.value?.loadConfig()
  await errorsPanel.value?.loadErrors()
  await nextTick()
  scrollToPanel()
}, { immediate: true })
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>Настройки</h1>
        <p>Система, обход DPI, сеть и сервис — якоря сверху.</p>
      </div>
      <div class="actions">
        <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'refresh-settings' }" @click="refresh">Обновить</button>
      </div>
    </div>

    <nav class="settings-nav" aria-label="Разделы настроек">
      <a
        v-for="s in sections"
        :key="s.id"
        href="#"
        :class="{ 'is-active': panelIdFromRoute() === s.id }"
        @click="goSection(s.id, $event)"
      >{{ s.label }}</a>
    </nav>

    <div v-if="!settingsLoaded" class="loading">Загрузка…</div>
    <template v-else>
      <div id="grp-system" class="settings-group">
        <h2>{{ settingGroups[0].title }}</h2>
        <p>{{ settingGroups[0].desc }}</p>
      </div>
      <UpdatesPanel />
      <ProviderPanel />
      <BackupsPanel />

      <div id="grp-bypass" class="settings-group">
        <h2>{{ settingGroups[1].title }}</h2>
        <p>{{ settingGroups[1].desc }}</p>
      </div>
      <ModesPanel />
      <TlsPanel />
      <WgPanel />
      <WssizePanel />

      <div id="grp-net" class="settings-group">
        <h2>{{ settingGroups[2].title }}</h2>
        <p>{{ settingGroups[2].desc }}</p>
      </div>
      <FirewallPanel />
      <IfacesPanel />
      <PortsPanel />
      <ScopesPanel />

      <div id="grp-svc" class="settings-group">
        <h2>{{ settingGroups[3].title }}</h2>
        <p>{{ settingGroups[3].desc }}</p>
      </div>
      <ConfigPanel ref="configPanel" />
      <ErrorsPanel ref="errorsPanel" />
    </template>
  </section>
</template>
