<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { domains } from '../api/endpoints'
import { busyActive, busyButton, withBusy } from '../stores/busy'
import {
  DOMAIN_LISTS, DOMAIN_META, MAIN_DOMAIN_LISTS, domainChecks, domainLists, domainsLoaded,
  refreshDomainList, refreshDomains, type DomainListName,
} from '../stores/domains'
import { modeSettings } from '../stores/settings'
import { refreshAll, scope, scopes } from '../stores/status'
import { showToast } from '../stores/toast'
import { confirmDialog } from '../stores/confirm'

const route = useRoute()
const router = useRouter()
const input = ref('')
const bulk = ref('')
const explainInput = ref('')
const explainResult = ref('')
const strategyDraft = ref<Record<string, string>>({})

const labels: Record<DomainListName, string> = {
  netrogat: 'Исключения',
  custom_rkn: 'TCP Custom',
  substring: 'Подстроки RKN',
  netrogat_substring: 'Подстроки исключений',
  googlevideo: 'автосбор Googlevideo',
}

const active = computed<DomainListName>(() => {
  const p = String(route.params.list || 'netrogat')
  return (DOMAIN_LISTS as readonly string[]).includes(p) ? p as DomainListName : 'netrogat'
})
const isGooglevideo = computed(() => active.value === 'googlevideo')
const list = computed(() => domainLists[active.value])
const meta = computed(() => DOMAIN_META[active.value])

onMounted(() => {
  if (!domainsLoaded.value) refreshDomains()
})
const hostlistAuto = computed(() => !!modeSettings.hostlist?.auto)
const autoLocked = computed(() => hostlistAuto.value && (active.value === 'custom_rkn' || active.value === 'substring'))
const scopeOptions = computed(() => scopes.value.scopes || ['default'])

watch(active, (name) => {
  if (!domainLists[name]) refreshDomainList(name)
})
watch(scope, () => {
  void refreshDomainList('custom_rkn')
  if (isGooglevideo.value) void refreshDomainList('googlevideo')
})

async function changeScope(v: string) {
  scope.value = v || 'default'
  router.replace({ query: { ...route.query, scope: scope.value } })
  // Синхронизировать status/locks с выбранным scope (иначе вкладка «Стратегии» видит старые локи).
  try {
    await withBusy('domain-scope', () => refreshAll())
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

watch(list, (payload) => {
  if (!payload?.items) return
  const next: Record<string, string> = { ...strategyDraft.value }
  const alive = new Set(payload.items.map((i) => i.value))
  for (const key of Object.keys(next)) {
    if (!alive.has(key)) delete next[key]
  }
  for (const item of payload.items) {
    const strat = item.strategy != null && item.strategy >= 1 ? String(item.strategy) : ''
    if (next[item.value] === undefined) {
      next[item.value] = strat
    } else if (!String(next[item.value] ?? '').trim() && strat) {
      next[item.value] = strat
    }
  }
  strategyDraft.value = next
}, { deep: true, immediate: true })

function go(name: DomainListName) {
  router.push(`/domains/${name}`)
}

async function add() {
  const d = input.value.trim()
  if (!d) return
  try {
    await withBusy('domain-add', async () => {
      const res = await domains.add(active.value, d, scope.value)
      input.value = ''
      await refreshDomainList(active.value)
      if (res.check) domainChecks[d] = res.check
      showToast('Добавлено')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function remove(domain: string) {
  if (autoLocked.value) return
  const ok = await confirmDialog({
    title: 'Удалить запись?',
    message: `Удалить «${domain}» из списка?`,
    danger: true,
  })
  if (!ok) return
  try {
    await withBusy('domain-rm', async () => {
      const res = await domains.remove(active.value, domain, scope.value)
      delete strategyDraft.value[domain]
      await refreshDomainList(active.value)
      const warn = String((res as { warning?: string }).warning || '')
      if (warn) showToast(warn, 'warning')
      else showToast('Удалено')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function importList() {
  if (!bulk.value.trim()) return
  try {
    await withBusy('domain-import', async () => {
      const res = await domains.import(active.value, bulk.value, scope.value)
      bulk.value = ''
      await refreshDomainList(active.value)
      showToast(`Импорт: +${res.added || 0}`)
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function refreshCheckGv() {
  try {
    await withBusy('gv-refresh-check', async () => {
      const res = await domains.refresh_check() as { check_ok?: boolean; hosts_added?: number; ipv4_count?: number }
      await refreshDomainList('googlevideo')
      showToast(`GV: hosts +${res.hosts_added || 0}, ipv4 ${res.ipv4_count ?? '—'}, check ${res.check_ok ? 'ok' : 'fail'}`,
        res.check_ok ? undefined : 'warning')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function explainDomain() {
  const d = explainInput.value.trim()
  if (!d) return
  try {
    await withBusy('domain-explain', async () => {
      const res = await domains.explain(d)
      if (!res.matched) {
        explainResult.value = `${res.domain}: не в обходе` +
          (res.hostlist_mode ? ` (режим: ${res.hostlist_mode})` : '')
      } else {
        explainResult.value = `${res.domain}: ${(res.matches || []).map((m) => {
          const role = m.role ? `:${m.role}` : ''
          const note = m.note ? ` (${m.note})` : ''
          return `${m.list}/${m.match}${role}${note}`
        }).join(', ')}`
      }
      if (res.custom_rkn_hint) showToast(res.custom_rkn_hint, 'warning')
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function refreshLists() {
  try {
    await withBusy('domains-refresh', async () => {
      if (isGooglevideo.value) {
        await domains.refresh('googlevideo', scope.value)
        await refreshDomainList('googlevideo')
      } else {
        await refreshDomains()
      }
    })
    if (isGooglevideo.value) showToast('Список googlevideo обновлён')
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function setStrategy(domain: string) {
  const raw = String(strategyDraft.value[domain] ?? '').trim()
  if (raw && (!/^\d+$/.test(raw) || Number(raw) < 0)) {
    showToast('Некорректный номер стратегии', 'error')
    return
  }
  try {
    await withBusy(`domain-st-${domain}`, async () => {
      if (!raw || raw === '0') {
        await domains.clear_strategy(domain, scope.value)
        strategyDraft.value[domain] = ''
      } else {
        const res = await domains.set_strategy(domain, raw, scope.value)
        strategyDraft.value[domain] = raw
        if (res.check) domainChecks[domain] = res.check
      }
      await refreshDomainList(active.value)
      showToast((!raw || raw === '0') ? `Стратегия снята с ${domain}` : `Стратегия ${raw} для ${domain}`)
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function clearAll() {
  if (autoLocked.value) return
  const ok = await confirmDialog({
    title: 'Очистить список?',
    message: isGooglevideo.value
      ? 'Все собранные googlevideo-хосты будут удалены.'
      : 'Все записи будут удалены.',
    danger: true,
  })
  if (!ok) return
  try {
    await withBusy('domain-clear', async () => {
      const res = await domains.clear(active.value, scope.value)
      strategyDraft.value = {}
      await refreshDomainList(active.value)
      const warn = String((res as { warning?: string }).warning || '')
      showToast(warn || 'Список очищен', warn ? 'warning' : undefined)
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}

async function checkDomain(domain: string) {
  try {
    await withBusy(`domain-check-${domain}`, async () => {
      const res = await domains.check(domain)
      if (res.check) domainChecks[domain] = res.check
    })
  } catch (e) {
    showToast((e as Error).message, 'error')
  }
}
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>Домены</h1>
        <p>Hostlist и автосбор media CDN YouTube.</p>
      </div>
      <div class="actions">
        <label v-if="scopes.enabled" class="field" style="margin:0">
          <span>Scope</span>
          <select :value="scope" :disabled="busyActive" @change="changeScope(($event.target as HTMLSelectElement).value)">
            <option v-for="s in scopeOptions" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <button type="button" :disabled="busyActive" :class="{ 'is-busy': busyButton === 'domains-refresh' }"
          @click="refreshLists">Обновить</button>
      </div>
    </div>

    <div class="subtabs">
      <button v-for="name in MAIN_DOMAIN_LISTS" :key="name" type="button"
        :class="{ 'is-active': active === name }" :disabled="busyActive" @click="go(name)">
        {{ labels[name] }}
      </button>
      <button type="button" class="subtab-end"
        :class="{ 'is-active': active === 'googlevideo' }" :disabled="busyActive"
        @click="go('googlevideo')">
        {{ labels.googlevideo }}
      </button>
    </div>

    <div class="panel">
      <div class="panel-h">
        <h2>{{ list?.title || labels[active] }}</h2>
        <span class="chip">{{ list?.items?.length ?? 0 }}</span>
      </div>
      <div v-if="isGooglevideo && list?.quality" class="panel-d" style="margin-bottom:0.65rem">
        <strong>YouTube media CDN</strong> ·
        seed {{ list.quality.seed_state || '—' }} ·
        auto_locks {{ list.quality.auto_locks_count ?? 0 }} ·
        ipv4 {{ list.quality.ipv4_count ?? 0 }}{{ list.quality.ipv4_fresh ? ' (fresh)' : '' }} ·
        wssize {{ list.quality.wssize ? 'on' : 'off' }} ·
        hosts {{ list.quality.hosts_count ?? 0 }}
        <div class="actions" style="margin-top:0.45rem">
          <button type="button" :disabled="busyActive" @click="refreshCheckGv">Обновить + проверить</button>
        </div>
      </div>
      <p v-if="isGooglevideo" class="panel-d">
        Автопробой TLS/CDN + обучение на трафике:
        <code>^rr\d+---sn-[a-z0-9-]+\.googlevideo\.com$</code>
        (пример <code>rr18---sn-pmcg-bg0s.googlevideo.com</code>).
        Сбор: {{ list?.collection || '—' }}.
        Seed:
        <template v-if="list?.seed?.state === 'probing'">
          probing {{ list.seed.host_key }} → #{{ list.seed.strategy }}
        </template>
        <template v-else-if="list?.seed?.last_result">
          {{ list.seed.state || 'idle' }}
          · last {{ list.seed.last_host }} #{{ list.seed.last_strategy }} = {{ list.seed.last_result }}
          <span v-if="list.seed.pending"> · pending {{ list.seed.pending }}</span>
        </template>
        <template v-else>{{ list?.seed?.state || '—' }}</template>.
      </p>
      <p v-else class="panel-d">{{ list?.description || meta.addLabel }}</p>
      <p v-if="autoLocked" class="panel-d" style="color:var(--warn)">
        Autohostlist включён: ручное изменение этого списка заблокировано. Выключите автосбор в Настройках → Режимы.
      </p>

      <form class="row" @submit.prevent="add">
        <input v-model="input" :placeholder="meta.placeholder" :disabled="busyActive || autoLocked" style="flex:1;min-width:12rem">
        <button type="submit" class="primary" :disabled="busyActive || autoLocked" :class="{ 'is-busy': busyButton === 'domain-add' }">Добавить</button>
        <button type="button" class="ghost danger"
          :disabled="busyActive || autoLocked || !(list?.items?.length)"
          @click="clearAll">Очистить</button>
      </form>

      <div class="field" style="margin-top:0.9rem">
        <span>Импорт списком</span>
        <textarea v-model="bulk"
          :disabled="busyActive || autoLocked"
          rows="4" placeholder="по одному на строку"></textarea>
        <div class="actions" style="margin-top:0.5rem">
          <button type="button"
            :disabled="busyActive || autoLocked"
            :class="{ 'is-busy': busyButton === 'domain-import' }"
            @click="importList">Импортировать</button>
        </div>
      </div>

      <div class="field" style="margin-top:0.9rem">
        <span>Проверить домен (explain)</span>
        <div class="row">
          <input v-model="explainInput" placeholder="example.com" :disabled="busyActive" style="flex:1;min-width:12rem">
          <button type="button" :disabled="busyActive" @click="explainDomain">Почему в обходе?</button>
        </div>
        <p v-if="explainResult" class="panel-d">{{ explainResult }}</p>
      </div>
    </div>

    <div v-if="isGooglevideo && (list?.auto_locks || []).length" class="domain-list" style="margin-top:0.85rem">
      <p class="panel-d" style="margin-bottom:0.5rem">POP (sn → strat)</p>
      <div v-for="lock in (list?.auto_locks || [])" :key="lock.host + ':' + lock.strategy" class="domain-item">
        <div>
          <code>{{ lock.host }}</code>
          <div class="panel-d" style="margin:0.35rem 0 0">
            стратегия {{ lock.strategy }}
            · {{ lock.source === 'seed' ? 'seed' : 'learned' }}
            <span v-if="lock.scope && lock.scope !== 'default'"> · {{ lock.scope }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="domain-list">
      <div v-for="item in (list?.items || [])" :key="item.value" class="domain-item">
        <div>
          <code>{{ item.value }}</code>
          <div v-if="isGooglevideo && (item.ip || item.source)" class="panel-d" style="margin:0.35rem 0 0">
            <span v-if="item.ip">{{ item.ip }}</span>
            <span v-if="item.source"> · {{ item.source }}</span>
          </div>
          <div v-if="active === 'custom_rkn'" class="row" style="margin-top:0.45rem">
            <input
              :value="strategyDraft[item.value] ?? (item.strategy != null && item.strategy >= 1 ? String(item.strategy) : '')"
              type="number" min="1" placeholder="стратегия"
              :disabled="busyActive" style="max-width:7rem"
              @input="strategyDraft[item.value] = ($event.target as HTMLInputElement).value"
            >
            <button type="button" :disabled="busyActive"
              :class="{ 'is-busy': busyButton === `domain-st-${item.value}` }"
              @click="setStrategy(item.value)">Стратегия</button>
          </div>
        </div>
        <div class="row">
          <button v-if="active === 'custom_rkn'" type="button" class="ghost" :disabled="busyActive"
            :class="{ 'is-busy': busyButton === `domain-check-${item.value}` }"
            @click="checkDomain(item.value)">Проверить</button>
          <button type="button" class="ghost danger" :disabled="busyActive || autoLocked"
            @click="remove(item.value)">Удалить</button>
        </div>
        <p v-if="domainChecks[item.value]" class="panel-d" style="grid-column:1/-1;margin:0.35rem 0 0">
          {{ (domainChecks[item.value].results || []).map(r => r.verdict || r.text).join(' · ') || domainChecks[item.value].message || 'Готово' }}
        </p>
      </div>
      <p v-if="domainsLoaded && !(list?.items || []).length" class="panel-d">
        {{ isGooglevideo ? 'Список пуст — узлы появятся после просмотра YouTube.' : 'Список пуст.' }}
      </p>
    </div>
  </section>
</template>
