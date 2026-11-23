import { reactive, ref } from 'vue'
import { fetchDomainsList } from '../api/endpoints'
import type { CheckPayload, DomainsListPayload } from '../api/types'
import { scope } from './status'
import { showToast } from './toast'

export const DOMAIN_LISTS = ['netrogat', 'custom_rkn', 'substring', 'netrogat_substring', 'googlevideo'] as const
export type DomainListName = typeof DOMAIN_LISTS[number]
/** Основные hostlist-вкладки (без автосбора googlevideo). */
export const MAIN_DOMAIN_LISTS = ['netrogat', 'custom_rkn', 'substring', 'netrogat_substring'] as const satisfies readonly DomainListName[]

export const DOMAIN_META: Record<DomainListName, { kind: string; addLabel: string; placeholder: string; itemName: string }> = {
  netrogat: { kind: 'domain', addLabel: 'Домен', placeholder: 'example.com', itemName: 'Домен' },
  custom_rkn: { kind: 'domain', addLabel: 'Домен', placeholder: 'example.com', itemName: 'Домен' },
  substring: { kind: 'substring', addLabel: 'Подстрока', placeholder: 'cdn, media, static, …', itemName: 'Подстрока' },
  netrogat_substring: { kind: 'substring', addLabel: 'Подстрока', placeholder: 'bank, shop, gov, …', itemName: 'Подстрока' },
  googlevideo: {
    kind: 'domain',
    addLabel: 'CDN-узел',
    placeholder: 'rr18---sn-pmcg-bg0s.googlevideo.com',
    itemName: 'CDN-узел',
  },
}

export const domainLists = reactive<Record<DomainListName, DomainsListPayload | null>>({
  netrogat: null,
  custom_rkn: null,
  substring: null,
  netrogat_substring: null,
  googlevideo: null,
})
export const domainChecks = reactive<Record<string, CheckPayload>>({})
export const expandedDomain = ref<string | null>(null)
// списки грузятся один раз (первый заход на вкладку) и дальше живут в сторе:
// обновление — кнопкой «Обновить» и точечно после мутаций
export const domainsLoaded = ref(false)

/** Сброс кэша списков (logout / restore / смена сессии). */
export function resetDomains() {
  for (const name of DOMAIN_LISTS) {
    domainLists[name] = null
  }
  for (const key of Object.keys(domainChecks)) delete domainChecks[key]
  expandedDomain.value = null
  domainsLoaded.value = false
}

export async function refreshDomainList(name: DomainListName): Promise<boolean> {
  try {
    domainLists[name] = await fetchDomainsList(name, scope.value || 'default')
    return true
  } catch (error) {
    showToast((error as Error).message, 'error')
    return false
  }
}

export async function refreshDomains() {
  const results = await Promise.all(DOMAIN_LISTS.map((name) => refreshDomainList(name)))
  domainsLoaded.value = results.some(Boolean)
}
