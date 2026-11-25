<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'
import SearchPluginDialog, { type SearchPluginTarget } from '../components/SearchPluginDialog.vue'
import SubscriptionReviewDialog, { type ReviewRelease } from '../components/SubscriptionReviewDialog.vue'
import {
  ChevronLeft, Search, Timer, Calendar,
  AlertTriangle, X, Sparkles, Info,
  Check, Plus, User, Copy, ExternalLink, Trash2,
  HardDrive, History, Filter, Clock, Download,
  CheckSquare, Square, ListChecks, CloudUpload, Globe
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { evaluateSubgroupSubmission, resolveMikanSelectionPayload } from '../utils/subscriptionRules'
import { useDefaultSubgroups } from '../composables/useDefaultSubgroups'

const { t, locale } = useI18n()

interface TorrentItem {
  title: string
  url: string
  magnet: string
  alternate_magnet?: string
  size: number
  pub_date: string
  source: string
  bangumi_id: string
  info_hash: string
  episode_url?: string
  cover_url?: string
  aired_time?: string
  aired_date?: string
}

interface SubgroupInfo {
  name: string
  rss_url: string
}

const router = useRouter()
const route = useRoute()
const searchInputRef = ref<HTMLInputElement | null>(null)

const query = ref('')
const results = ref<TorrentItem[]>([])
const loading = ref(false)
const error = ref('')
const rawError = ref('')
const hasSearched = ref(false)
const subscribed = ref<Set<string>>(new Set())
const subscribedIdMap = ref<Map<string, number>>(new Map())
const { defaultSubgroups, loadDefaultSubgroups, isDefaultSubgroup } = useDefaultSubgroups()

const normalizeSubKey = (value?: string) => (value || '').trim().toLowerCase()

function registerSubscribedId(id: unknown, keys: Array<string | undefined>) {
  if (typeof id !== 'number' || id <= 0) return
  const next = new Map(subscribedIdMap.value)
  for (const key of keys) {
    const clean = normalizeSubKey(key)
    if (clean) next.set(clean, id)
  }
  subscribedIdMap.value = next
}

function findSubscribedId(item: TorrentItem): number | undefined {
  return subscribedIdMap.value.get(normalizeSubKey(item.bangumi_id))
    ?? subscribedIdMap.value.get(normalizeSubKey(item.title))
}
const lastSearchTime = ref('')
const searchDuration = ref(0)
const smartSearch = ref(false)
const expandedQueries = ref<string[]>([])
const usedAI = ref(false)
const aiError = ref('')
const aiConfigured = ref(false)

// 多源搜索开关条（从 /api/settings 真实读取，严格 val === 'true'，缺省为 false）
interface SourceToggleDef {
  key: 'ACGRIP_ENABLED' | 'NYAA_ENABLED' | 'ANIMETOSHO_ENABLED'
  label: string
  tag: string
}
const extraSources: SourceToggleDef[] = [
  { key: 'ACGRIP_ENABLED', label: 'ACG.RIP', tag: '国内友站' },
  { key: 'NYAA_ENABLED', label: 'Nyaa', tag: '海外主站' },
  { key: 'ANIMETOSHO_ENABLED', label: 'AnimeTosho', tag: '镜像索引' },
]
const sourceEnabled = ref<Record<string, boolean>>({
  ACGRIP_ENABLED: false,
  NYAA_ENABLED: false,
  ANIMETOSHO_ENABLED: false,
})
const sourceToggling = ref<Record<string, boolean>>({})
const settingsLoaded = ref(false)

async function loadSourceSettings() {
  try {
    const { data } = await request.get<Record<string, string>>('/settings')
    sourceEnabled.value = {
      ACGRIP_ENABLED: data?.ACGRIP_ENABLED === 'true',
      NYAA_ENABLED: data?.NYAA_ENABLED === 'true',
      ANIMETOSHO_ENABLED: data?.ANIMETOSHO_ENABLED === 'true',
    }
    settingsLoaded.value = true
  } catch {
    settingsLoaded.value = false
  }
}

async function toggleSource(src: SourceToggleDef) {
  if (sourceToggling.value[src.key]) return
  const next = !sourceEnabled.value[src.key]
  sourceToggling.value[src.key] = true
  sourceEnabled.value[src.key] = next
  try {
    await request.put('/settings', { settings: { [src.key]: next ? 'true' : 'false' } })
    window.showToast?.(
      next ? `已开启 ${src.label} 搜索源` : `已关闭 ${src.label} 搜索源`,
      'success'
    )
  } catch (e: any) {
    sourceEnabled.value[src.key] = !next
    window.showToast?.(e?.response?.data?.error || `切换 ${src.label} 失败`, 'error')
  } finally {
    sourceToggling.value[src.key] = false
  }
}

function humanizeError(raw: string): string {
  const lower = raw.toLowerCase()
  if (lower.includes('network is unreachable') || lower.includes('no such host') || lower.includes('lookup ')) {
    return '无法解析或连接资源站域名，请检查本地 DNS、宿主网络连通性或系统代理设置。'
  }
  if (lower.includes('timeout') || lower.includes('context deadline exceeded') || lower.includes('econnaborted')) {
    return '连接资源站响应超时，可在多源开关中切换直连站点（如 ACG.RIP）或检查代理后重试。'
  }
  if (lower.includes('connection refused')) {
    return '目标站点或代理端口拒绝连接，请检查镜像域名或网络端口配置。'
  }
  return raw
}

const selectedSource = ref('all')
const inResultFilter = ref('')
const sortBy = ref<'default' | 'date' | 'size_desc' | 'size_asc'>('default')

const FILTER_PREF_KEY = 'anigo_search_filter_pref'
const selectedSubgroup = ref(localStorage.getItem(FILTER_PREF_KEY) || 'all')

const directDownloading = ref<Record<string, boolean>>({})
const batchMode = ref(false)
const selectedItems = ref<TorrentItem[]>([])
const batchProcessing = ref(false)

interface SearchPlugin {
  id: string
  name: string
  enabled: boolean
  status: string
  origin: string
  is_builtin: boolean
  url?: string
  panel?: { actions: { id: string; label: string; placement?: string; input?: { format?: string }; result: string }[] }
}
const pluginTargets = ref<SearchPluginTarget[]>([])
const pluginSelection = ref<{ item: { title: string; magnet: string }; targets: SearchPluginTarget[] } | null>(null)
let catalogRequest = 0
let searchRequest: AbortController | undefined
let activeView = true

async function loadPluginTargets() {
  const generation = ++catalogRequest
  try {
    const { data } = await request.get<SearchPlugin[]>('/plugins')
    if (generation !== catalogRequest) return
    pluginTargets.value = data.filter(p => p.enabled && p.status === 'active').flatMap(p =>
      (p.panel?.actions ?? []).filter(a => a.placement === 'search_result' && a.input?.format === 'magnet' && a.result === 'message')
        .map(a => ({
          id: p.id, name: p.name, official: p.is_builtin === true && p.origin === 'official',
          action: a.id, label: a.label, url: p.url ?? ''
        })))
  } catch {
    if (generation === catalogRequest) pluginTargets.value = []
  }
}

function sourceLink(item: TorrentItem) {
  for (const candidate of [item.episode_url, item.url]) {
    if (!candidate) continue
    try {
      const url = new URL(candidate)
      if (url.protocol === 'https:' || url.protocol === 'http:') return url.href
    } catch {}
  }
  return ''
}

function resourceMagnet(item: TorrentItem) {
  return item.magnet || item.alternate_magnet || ''
}

function choosePlugin(item: TorrentItem) {
  const magnet = resourceMagnet(item)
  if (magnet && pluginTargets.value.length) pluginSelection.value = { item: { title: item.title, magnet }, targets: pluginTargets.value }
}

const SEARCH_HISTORY_KEY = 'anigo_search_history'
const searchHistory = ref<string[]>([])
const popularTags = [
  '葬送的芙莉莲', '我推的孩子', '迷宫饭', '孤独摇滚',
  '鬼灭之刃', '间谍过家家', '无职转生', '1080p', '4K', '简日双语'
]

function loadSearchHistory() {
  try {
    const raw = localStorage.getItem(SEARCH_HISTORY_KEY)
    if (raw) searchHistory.value = JSON.parse(raw)
  } catch {}
}

function saveSearchHistory(q: string) {
  const clean = q.trim()
  if (!clean) return
  const set = new Set([clean, ...searchHistory.value])
  searchHistory.value = Array.from(set).slice(0, 10)
  try { localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(searchHistory.value)) } catch {}
}

function removeHistoryItem(item: string) {
  searchHistory.value = searchHistory.value.filter(s => s !== item)
  try { localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(searchHistory.value)) } catch {}
}

function clearAllHistory() {
  searchHistory.value = []
  try { localStorage.removeItem(SEARCH_HISTORY_KEY) } catch {}
}

async function loadSubscribedList() {
  try {
    const { data } = await request.get('/subscriptions')
    if (Array.isArray(data)) {
      subscribed.value = new Set(data.map((s: any) => s.title_cn))
      for (const s of data) {
        registerSubscribedId(s?.id, [s?.mikan_id, s?.bangumi_id, s?.metadata_id, s?.title_cn])
      }
    }
  } catch {}
}

onMounted(async () => {
  loadSearchHistory()
  loadSubscribedList()
  void loadDefaultSubgroups()
  void loadSourceSettings()
  void loadPluginTargets()
  window.addEventListener?.('focus', loadPluginTargets)

  const q = route.query.q
  if (typeof q === 'string' && q.trim()) {
    query.value = q
    void handleSearch()
  }

  try {
    const { data } = await request.get('/search/capabilities')
    aiConfigured.value = data.smart_search === true
  } catch {
    aiConfigured.value = false
  }
})

onUnmounted(() => {
  ++catalogRequest
  activeView = false
  searchRequest?.abort()
  groupRequest?.abort()
  window.removeEventListener('focus', loadPluginTargets)
})

const reviewItems = ref<TorrentItem[]>([])
const showGroupModal = ref(false)
const groupSubmitting = ref(false)
let groupRequest: AbortController | undefined
const selectedItem = ref<TorrentItem | null>(null)
const subgroups = ref<SubgroupInfo[]>([])
const subgroupSearch = ref('')
const groupLoading = ref(false)
const groupError = ref('')

const filteredSubgroups = computed(() => {
  if (!subgroupSearch.value.trim()) return subgroups.value
  const kw = subgroupSearch.value.toLowerCase()
  return subgroups.value.filter(g => g.name.toLowerCase().includes(kw))
})

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.startsWith('http') || url.startsWith('//')) {
    const target = url.startsWith('//') ? 'https:' + url : url
    return `/api/proxy/image?url=${encodeURIComponent(target)}`
  }
  return url
}

async function handleSearch() {
  const q = query.value.trim()
  if (!q || batchProcessing.value) return
  searchRequest?.abort()
  const current = new AbortController()
  searchRequest = current
  exitBatchMode()
  results.value = []
  expandedQueries.value = []
  usedAI.value = false
  aiError.value = ''
  lastSearchTime.value = ''
  saveSearchHistory(q)
  hasSearched.value = true
  loading.value = true
  error.value = ''
  rawError.value = ''
  selectedSource.value = 'all'
  inResultFilter.value = ''
  const start = Date.now()
  try {
    if (smartSearch.value) {
      const { data } = await request.post('/search/smart', { query: q, limit: 60 }, { timeout: 45000, signal: current.signal })
      if (current.signal.aborted) return
      results.value = data.items || []
      expandedQueries.value = data.expanded_queries || [q]
      usedAI.value = !!data.used_ai
      aiError.value = data.ai_error || ''
    } else {
      const { data } = await request.get('/search', { params: { q }, timeout: 30000, signal: current.signal })
      if (current.signal.aborted) return
      results.value = data || []
      expandedQueries.value = [q]
      usedAI.value = false
      aiError.value = ''
    }
    lastSearchTime.value = new Date().toLocaleTimeString()
    searchDuration.value = Date.now() - start
  } catch (e: any) {
    if (current.signal.aborted) return
    const raw = e.response?.data?.error || e.message || t('search.error.failed') || '检索失败'
    rawError.value = raw
    error.value = e.code === 'ECONNABORTED' ? (t('search.error.timeout') || '检索超时') : humanizeError(raw)
  } finally {
    if (current === searchRequest) loading.value = false
  }
}

function quickSearch(keyword: string) {
  query.value = keyword
  handleSearch()
}

function clearQuery() {
  if (batchProcessing.value) return
  searchRequest?.abort()
  searchRequest = undefined
  loading.value = false
  hasSearched.value = false
  results.value = []
  error.value = ''
  rawError.value = ''
  expandedQueries.value = []
  aiError.value = ''
  usedAI.value = false
  lastSearchTime.value = ''
  exitBatchMode()
  query.value = ''
  searchInputRef.value?.focus()
}

const availableSources = computed(() => {
  const map = new Map<string, number>()
  for (const item of results.value) {
    const s = item.source || 'Other'
    map.set(s, (map.get(s) || 0) + 1)
  }
  return map
})

function selectSubgroup(group: string) {
  selectedSubgroup.value = group
  try { localStorage.setItem(FILTER_PREF_KEY, group) } catch {}
}

function extractItemSubgroup(title: string): string {
  const m = title.match(/^[\[【]([^\]】]+)[\]】]/)
  if (m && m[1]) {
    const g = m[1].trim()
    if (!/^(1080p|720p|4k|2160p|hevc|x264|x265|bilibili|web-dl)$/i.test(g) && g.length <= 28) {
      return g
    }
  }
  return ''
}

function extractQualityTags(title: string): string[] {
  const hits: string[] = []
  const patterns: [RegExp, string][] = [
    [/2160p|4K/i, '4K'],
    [/1080p/i, '1080P'],
    [/720p/i, '720P'],
    [/HEVC|x265|H\.265/i, 'HEVC'],
    [/简日|简繁|CHS|CHT/i, '中文'],
  ]
  for (const [re, label] of patterns) {
    if (re.test(title) && !hits.includes(label)) hits.push(label)
  }
  return hits.slice(0, 3)
}

const extractedSubgroups = computed(() => {
  const map = new Map<string, number>()
  for (const item of results.value) {
    const g = extractItemSubgroup(item.title)
    if (g) map.set(g, (map.get(g) || 0) + 1)
  }
  return Array.from(map.entries()).sort((a, b) => b[1] - a[1]).slice(0, 10)
})

const filteredResults = computed(() => {
  let list = results.value
  if (selectedSource.value !== 'all') {
    list = list.filter(item => item.source === selectedSource.value)
  }
  if (selectedSubgroup.value !== 'all') {
    list = list.filter(item => item.title.includes(selectedSubgroup.value))
  }
  const kw = inResultFilter.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(item => item.title.toLowerCase().includes(kw))
  }
  if (sortBy.value === 'date') {
    list = [...list].sort((a, b) => new Date(b.pub_date || 0).getTime() - new Date(a.pub_date || 0).getTime())
  } else if (sortBy.value === 'size_desc') {
    list = [...list].sort((a, b) => (b.size || 0) - (a.size || 0))
  } else if (sortBy.value === 'size_asc') {
    list = [...list].sort((a, b) => (a.size || 0) - (b.size || 0))
  }
  return list
})

function getItemKey(item: TorrentItem): string {
  return item.info_hash || item.magnet || item.url || item.title
}

function canDownload(item: TorrentItem) {
  return Boolean(item.magnet || (item.url && (!item.url.includes('/Home/Bangumi/') || item.info_hash)))
}

function queueDownload(item: TorrentItem) {
  return request.post('/downloads', { title: item.title, url: item.url, magnet: item.magnet, info_hash: item.info_hash })
}

async function directDownload(item: TorrentItem) {
  const key = getItemKey(item)
  if (!canDownload(item) || directDownloading.value[key]) return
  directDownloading.value[key] = true
  try {
    await queueDownload(item)
    window.showToast?.(t('search.action.downloadDirectSuccess') || '已添加到下载队列', 'success')
  } catch (e: any) {
    window.showToast?.(t('search.action.downloadDirectFailed', { error: e.response?.data?.error || e.message }) || '添加下载失败', 'error')
  } finally {
    directDownloading.value[key] = false
  }
}

function isItemSelected(item: TorrentItem): boolean {
  const key = getItemKey(item)
  return selectedItems.value.some(i => getItemKey(i) === key)
}

function toggleItemSelection(item: TorrentItem) {
  const key = getItemKey(item)
  if (batchProcessing.value || !canDownload(item)) return
  const idx = selectedItems.value.findIndex(i => getItemKey(i) === key)
  if (idx >= 0) selectedItems.value.splice(idx, 1)
  else selectedItems.value.push(item)
}

const visibleDownloads = computed(() => filteredResults.value.filter(canDownload))
const allVisibleSelected = computed(() => visibleDownloads.value.length > 0 && visibleDownloads.value.every(isItemSelected))

function toggleSelectAll() {
  if (batchProcessing.value) return
  const selected = new Map(selectedItems.value.map(item => [getItemKey(item), item]))
  const remove = allVisibleSelected.value
  for (const item of visibleDownloads.value) {
    if (remove) selected.delete(getItemKey(item))
    else selected.set(getItemKey(item), item)
  }
  selectedItems.value = [...selected.values()]
}

function exitBatchMode() {
  if (batchProcessing.value) return
  batchMode.value = false
  selectedItems.value = []
}

async function submitBatch(operation: 'download' | 'subscribe') {
  if (batchProcessing.value || !selectedItems.value.length) return
  const items = [...selectedItems.value]
  if (operation === 'subscribe') {
    reviewItems.value = items
    return
  }
  const failed: TorrentItem[] = []
  batchProcessing.value = true
  for (const item of items) {
    if (!activeView) break
    try { await queueDownload(item) } catch { failed.push(item) }
  }
  batchProcessing.value = false
  if (!activeView) return
  if (failed.length) {
    selectedItems.value = failed
    window.showToast?.(t('search.action.batchIncomplete', { succeeded: items.length - failed.length, failed: failed.length }), 'error')
  } else {
    window.showToast?.(t('search.action.batchDownloadSuccess', { count: items.length }), 'success')
    exitBatchMode()
  }
}

function markReviewedItems(items: ReviewRelease[], createdId?: number) {
  const accepted = new Set(items.map(item => item.title))
  for (const title of accepted) subscribed.value.add(title)
  registerSubscribedId(createdId, items.flatMap(item => [item.bangumi_id, item.title]))
  selectedItems.value = selectedItems.value.filter(item => !accepted.has(item.title))
  if (batchMode.value && !selectedItems.value.length) exitBatchMode()
}

function closeGroups() {
  groupRequest?.abort()
  showGroupModal.value = false
  selectedModalSubgroups.value = new Set()
  searchSubgroupPromptMode.value = null
  highlightSearchGroups.value = false
}

const selectedModalSubgroups = ref<Set<string>>(new Set())
const searchSubgroupPromptMode = ref<'missing_default' | 'no_selection' | null>(null)
const highlightSearchGroups = ref(false)

function toggleModalSubgroup(name: string) {
  const next = new Set(selectedModalSubgroups.value)
  if (next.has(name)) next.delete(name)
  else next.add(name)
  selectedModalSubgroups.value = next
}

function focusSearchSubgroupList() {
  searchSubgroupPromptMode.value = null
  highlightSearchGroups.value = true
}

async function openSubscribe(item: TorrentItem) {
  if (groupSubmitting.value) return
  closeGroups()
  if (!item.bangumi_id || item.source !== 'Mikan') {
    reviewItems.value = [item]
    return
  }
  const current = new AbortController()
  groupRequest = current
  selectedItem.value = item
  subgroups.value = []
  selectedModalSubgroups.value = new Set()
  subgroupSearch.value = ''
  groupError.value = ''
  showGroupModal.value = true
  groupLoading.value = true
  try {
    const { data } = await request.get('/mikan/groups', {
      params: { bangumi_id: item.bangumi_id },
      timeout: 15000, signal: current.signal,
    })
    if (!current.signal.aborted) subgroups.value = data || []
  } catch (e: any) {
    if (!current.signal.aborted) groupError.value = t(e.code === 'ECONNABORTED' ? 'schedule.error.timeout' : 'schedule.error.failed')
  } finally {
    if (groupRequest === current) groupLoading.value = false
  }
}

declare global {
  interface Window {
    showToast: (message: string, type?: 'success' | 'error' | 'info') => void
  }
}

async function saveSubscription(
  item: TorrentItem,
  rssUrl: string,
  subgroupName?: string,
  allowedSubgroups?: string[],
) {
  const { data } = await request.post<{ id?: number }>('/subscriptions', {
    title_cn: item.title,
    bangumi_id: item.source === 'Mikan' ? undefined : item.bangumi_id,
    mikan_id: item.source === 'Mikan' ? item.bangumi_id : undefined,
    rss_url: rssUrl || undefined,
    subgroup_name: subgroupName || undefined,
    subgroup_names: allowedSubgroups?.length ? allowedSubgroups : undefined,
    allowed_subgroups: allowedSubgroups?.length ? allowedSubgroups : undefined,
    filter_json: JSON.stringify({ source_url: item.url }),
    cover_url: item.cover_url || '',
  })
  subscribed.value.add(item.title)
  registerSubscribedId(data?.id, [item.bangumi_id, item.title])
}

async function subscribe(
  item: TorrentItem,
  rssUrl: string,
  subgroupName?: string,
  allowedSubgroups?: string[],
  usedDefaultGroups?: string[],
) {
  if (groupSubmitting.value) return
  groupSubmitting.value = true
  try {
    await saveSubscription(item, rssUrl, subgroupName, allowedSubgroups)
    if (usedDefaultGroups?.length) {
      window.showToast?.(t('schedule.modal.subscribeSuccessDefault', { title: item.title, groups: usedDefaultGroups.join('、') }), 'success')
    } else {
      window.showToast?.(t('search.subscribed', { title: item.title }) || `成功订阅: ${item.title}`, 'success')
    }
    closeGroups()
  } catch (e: any) {
    window.showToast?.(t('search.subscribeFailed', { error: e.response?.data?.error || e.message }) || '订阅失败', 'error')
  } finally {
    groupSubmitting.value = false
  }
}

async function confirmGroupSubscribe(options?: { allowUnrestricted?: boolean }) {
  if (!selectedItem.value) return
  const decision = evaluateSubgroupSubmission({
    selectedNames: Array.from(selectedModalSubgroups.value),
    defaultSubgroups: defaultSubgroups.value,
    availableGroups: subgroups.value,
    hasLiveGroupList: subgroups.value.length > 0,
    allowUnrestricted: options?.allowUnrestricted,
  })
  if (decision.action !== 'submit') {
    searchSubgroupPromptMode.value = decision.action === 'prompt_missing_default' ? 'missing_default' : 'no_selection'
    return
  }
  searchSubgroupPromptMode.value = null
  const payload = resolveMikanSelectionPayload(
    subgroups.value,
    decision.groups,
  )
  await subscribe(
    selectedItem.value,
    payload.rss_url || '',
    payload.subgroup_name,
    payload.allowed_subgroups,
    decision.usedDefaults ? decision.groups : undefined,
  )
}

async function copyMagnet(item: TorrentItem) {
  const magnet = resourceMagnet(item)
  if (!magnet) return
  let copied = false
  try {
    await navigator.clipboard.writeText(magnet)
    copied = true
  } catch {
    const input = document.createElement('textarea')
    input.value = magnet
    try {
      document.body.appendChild(input)
      input.select()
      copied = document.execCommand('copy')
    } catch { copied = false } finally { input.remove() }
  }
  window.showToast?.(t(copied ? 'search.action.magnetCopied' : 'search.action.copyFailed'), copied ? 'success' : 'error')
}

function formatSize(size: number): string {
  if (!size || size <= 0) return '0 B'
  const mb = size / 1024 / 1024
  return mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(1) + ' MB'
}

function formatPubDate(d?: string): string {
  if (!d || d.startsWith('0001-01-01')) return ''
  const date = new Date(d)
  if (Number.isNaN(date.getTime())) return ''
  const hours = (Date.now() - date.getTime()) / 3600000
  if (hours >= 0 && hours < 1) return t('search.time.now')
  if (hours >= 1 && hours < 24) return t('search.time.hours', { count: Math.floor(hours) })
  if (hours >= 24 && hours < 48) return t('search.time.yesterday')
  if (hours >= 48 && hours < 744) return t('search.time.days', { count: Math.floor(hours / 24) })
  return date.toLocaleDateString(locale.value)
}

function sourceBadgeClass(source: string): string {
  const map: Record<string, string> = {
    Mikan: 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] border-[var(--v2-accent)]/30',
    Nyaa: 'bg-sky-500/12 text-sky-400 border-sky-500/30',
    'ACG.RIP': 'bg-emerald-500/12 text-emerald-400 border-emerald-500/30',
    AnimeTosho: 'bg-violet-500/12 text-violet-400 border-violet-500/30',
  }
  return map[source] || 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)]'
}
</script>

<template>
  <div class="space-y-5 pb-16">
    <!-- 顶部页头：第一个子元素必须为原生 <header>，仅使用 ChevronLeft/Plus/Search 图标（契约测试要求） -->
    <header class="app-page-header">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h1 class="app-page-title">{{ $t('search.title') }}</h1>
          <span v-if="results.length > 0" class="v2-pill font-mono text-xs">
            {{ $t('search.resultsCount', { count: results.length }) }}
          </span>
        </div>
        <p class="app-page-subtitle">{{ $t('search.subtitle') }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <router-link to="/subscriptions/new" class="v2-btn v2-btn-sm">
          <Plus :size="15" aria-hidden="true" />
          <span>{{ $t('search.addManually') }}</span>
        </router-link>
        <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="router.push('/schedule')">
          <ChevronLeft :size="15" />
          <span>{{ $t('search.back') }}</span>
        </button>
      </div>
    </header>

    <!-- 宽幅指令检索 Hero + 实时多源开关条 -->
    <section class="v2-panel p-4 sm:p-6 space-y-4">
      <div class="flex items-center p-1.5 rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border-strong)] focus-within:border-[var(--v2-accent)] focus-within:ring-2 focus-within:ring-[var(--v2-accent)]/20 transition-all">
        <div class="pl-3 text-[var(--v2-ink-subtle)] shrink-0">
          <Search :size="19" />
        </div>
        <input
          ref="searchInputRef"
          v-model="query"
          type="text"
          class="flex-1 min-w-0 bg-transparent border-none outline-none px-3 py-2 text-sm sm:text-base font-medium text-[var(--v2-ink)] placeholder:text-[var(--v2-ink-subtle)]"
          :placeholder="$t('search.placeholder')"
          @keyup.enter="handleSearch"
        />
        <button
          v-if="query"
          type="button"
          @click="clearQuery"
          class="p-1.5 rounded-lg text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] mr-1"
        >
          <X :size="15" />
        </button>
        <button
          type="button"
          class="v2-btn v2-btn-primary px-5 shrink-0"
          :disabled="loading || !query.trim()"
          @click="handleSearch"
        >
          <span v-if="loading" class="loading loading-spinner loading-xs"></span>
          <template v-else>
            <Search :size="15" />
            <span>{{ $t('search.execute') }}</span>
          </template>
        </button>
      </div>

      <!-- 实时多源开关条 + AI 扩展开关 -->
      <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3 pt-1">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-[11px] font-semibold text-[var(--v2-ink-subtle)] flex items-center gap-1 mr-1">
            <Globe :size="13" />
            <span>{{ locale === 'zh' ? '检索数据源:' : 'Sources:' }}</span>
          </span>

          <!-- Mikan 内置源 -->
          <div class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-[var(--v2-accent)]/12 border border-[var(--v2-accent)]/30 text-xs font-semibold text-[var(--v2-accent)]">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--v2-accent)]"></span>
            <span>Mikan</span>
            <span class="text-[10px] opacity-75">[{{ locale === 'zh' ? '内置' : 'Built-in' }}]</span>
          </div>

          <!-- 可即时切换的扩展源（直接读取与写入 PUT /api/settings） -->
          <button
            v-for="src in extraSources"
            :key="src.key"
            type="button"
            role="switch"
            :aria-checked="sourceEnabled[src.key]"
            :disabled="!!sourceToggling[src.key]"
            @click="toggleSource(src)"
            class="inline-flex items-center gap-2 px-2.5 py-1 rounded-full border text-xs font-medium transition-all cursor-pointer"
            :class="sourceEnabled[src.key]
              ? 'bg-emerald-500/12 border-emerald-500/35 text-emerald-400 font-semibold'
              : 'bg-[var(--v2-surface-2)] border-[var(--v2-border)] text-[var(--v2-ink-muted)] hover:border-[var(--v2-border-strong)]'"
          >
            <span
              class="w-1.5 h-1.5 rounded-full"
              :class="sourceEnabled[src.key] ? 'bg-emerald-400' : 'bg-[var(--v2-ink-subtle)]'"
            ></span>
            <span>{{ src.label }}</span>
            <span class="text-[10px] opacity-70">{{ src.tag }}</span>
            <span class="font-mono text-[10px] px-1.5 py-0.2 rounded bg-black/20">
              {{ sourceEnabled[src.key] ? 'ON' : 'OFF' }}
            </span>
          </button>
        </div>

        <div class="flex items-center justify-between sm:justify-end gap-4">
          <label class="flex items-center gap-2 cursor-pointer text-xs font-semibold text-[var(--v2-ink-muted)] select-none">
            <Sparkles :size="14" class="text-[var(--v2-accent)]" />
            <span>{{ $t('search.smart') }}</span>
            <button
              type="button"
              role="switch"
              class="v2-switch"
              :aria-checked="smartSearch"
              :disabled="!aiConfigured"
              @click="aiConfigured && (smartSearch = !smartSearch)"
            >
              <span class="v2-switch-thumb"></span>
            </button>
            <span v-if="!aiConfigured" class="text-[11px] text-amber-400">({{ $t('settings.ai.notConfiguredTip') }})</span>
          </label>

          <div v-if="lastSearchTime && results.length > 0" class="text-[11px] font-mono text-[var(--v2-ink-subtle)] flex items-center gap-2 tabular-nums">
            <span class="flex items-center gap-1"><Timer :size="12" /> {{ (searchDuration / 1000).toFixed(2) }}s</span>
            <span>•</span>
            <span class="flex items-center gap-1"><Clock :size="12" /> {{ lastSearchTime }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- AI 降级提示 -->
    <div v-if="smartSearch && aiError" class="v2-panel border-amber-500/30 bg-amber-500/10 text-amber-400 px-4 py-3 flex items-center gap-2.5 text-xs font-medium">
      <Info :size="16" class="shrink-0" />
      <span>{{ $t('search.degraded') }}</span>
    </div>

    <!-- 扩展检索词 -->
    <div v-if="results.length > 0 && expandedQueries.length > 1" class="v2-panel p-3 flex flex-wrap items-center gap-2">
      <span class="text-[11px] font-semibold text-[var(--v2-ink-subtle)] flex items-center gap-1 mr-1">
        <Sparkles :size="12" class="text-[var(--v2-accent)]" /> {{ $t('search.expanded') }}:
      </span>
      <span v-for="(item, idx) in expandedQueries" :key="`${item}-${idx}`" class="v2-pill text-xs">
        {{ item }}
      </span>
    </div>

    <!-- 人性化网络错误诊断 -->
    <div v-if="error" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 p-4 space-y-2">
      <div class="flex items-start justify-between gap-3">
        <div class="flex items-start gap-3">
          <AlertTriangle :size="18" class="shrink-0 mt-0.5" />
          <div class="space-y-1">
            <h4 class="font-semibold text-xs">{{ $t('search.error.title') }}</h4>
            <p class="text-xs text-[var(--v2-ink)]">{{ error }}</p>
          </div>
        </div>
        <button type="button" class="p-1 rounded hover:bg-red-500/20" @click="error = ''; rawError = ''">
          <X :size="14" />
        </button>
      </div>
      <details v-if="rawError && rawError !== error" class="pl-7 text-[11px] font-mono text-[var(--v2-ink-muted)]">
        <summary class="cursor-pointer hover:text-[var(--v2-ink)]">查看原始网络错误</summary>
        <pre class="mt-1.5 p-2 rounded-lg bg-black/25 whitespace-pre-wrap break-all">{{ rawError }}</pre>
      </details>
    </div>

    <!-- 结果筛选工具栏 -->
    <div v-if="results.length > 0" class="v2-panel p-3.5 space-y-3">
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-3">
        <div class="flex items-center gap-1.5 overflow-x-auto">
          <button
            type="button"
            @click="selectedSource = 'all'"
            class="h-7 px-3 rounded-full text-xs font-semibold transition-colors whitespace-nowrap cursor-pointer"
            :class="selectedSource === 'all' ? 'bg-[var(--v2-accent)] text-white' : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)]'"
          >
            {{ $t('search.filterAll') }} ({{ results.length }})
          </button>
          <button
            v-for="[src, count] in availableSources"
            :key="src"
            type="button"
            @click="selectedSource = src"
            class="h-7 px-3 rounded-full text-xs font-semibold transition-colors whitespace-nowrap cursor-pointer"
            :class="selectedSource === src ? 'bg-[var(--v2-accent)] text-white' : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)]'"
          >
            {{ src }} ({{ count }})
          </button>
        </div>

        <div class="flex items-center gap-2 flex-wrap sm:flex-nowrap">
          <div class="relative flex-1 sm:w-52">
            <Filter :size="12" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
            <input v-model="inResultFilter" type="text" :placeholder="$t('search.filterKeyword')" class="v2-input pl-7 h-8 text-xs" />
          </div>
          <select v-model="sortBy" class="v2-input w-36 h-8 text-xs font-medium cursor-pointer">
            <option value="default">{{ $t('search.sortDefault') }}</option>
            <option value="date">{{ $t('search.sortDate') }}</option>
            <option value="size_desc">{{ $t('search.sortSizeDesc') }}</option>
            <option value="size_asc">{{ $t('search.sortSizeAsc') }}</option>
          </select>
          <button
            type="button"
            @click="batchMode ? exitBatchMode() : (batchMode = true)"
            :disabled="batchProcessing"
            class="v2-btn v2-btn-sm"
            :class="batchMode ? 'v2-btn-primary' : ''"
          >
            <ListChecks :size="14" />
            <span>{{ batchMode ? $t('search.action.batchCancel') : $t('search.action.batchSelect') }}</span>
          </button>
        </div>
      </div>

      <div v-if="extractedSubgroups.length > 0" class="flex items-center gap-1.5 overflow-x-auto pt-2 border-t border-[var(--v2-border)] text-xs">
        <span class="text-[11px] font-semibold text-[var(--v2-ink-subtle)] shrink-0">{{ t('search.filterGroups') }}</span>
        <button
          type="button"
          @click="selectSubgroup('all')"
          class="h-6 px-2.5 rounded-md text-[11px] font-medium whitespace-nowrap cursor-pointer"
          :class="selectedSubgroup === 'all' ? 'bg-[var(--v2-ink)] text-[var(--v2-bg)]' : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)]'"
        >
          {{ t('search.allGroups') }}
        </button>
        <button
          v-for="[group, count] in extractedSubgroups"
          :key="group"
          type="button"
          @click="selectSubgroup(group)"
          class="h-6 px-2.5 rounded-md text-[11px] font-medium whitespace-nowrap cursor-pointer"
          :class="selectedSubgroup === group ? 'bg-[var(--v2-ink)] text-[var(--v2-bg)]' : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)]'"
        >
          {{ group }} ({{ count }})
        </button>
      </div>
    </div>

    <!-- 批量浮动操作栏 -->
    <div v-if="batchMode && filteredResults.length > 0" class="sticky top-18 z-30 v2-panel border-[var(--v2-accent)]/40 p-3 flex flex-wrap items-center justify-between gap-3 shadow-xl">
      <div class="flex items-center gap-3">
        <button type="button" @click="toggleSelectAll" :disabled="batchProcessing || !visibleDownloads.length" class="v2-btn v2-btn-sm">
          <component :is="allVisibleSelected ? CheckSquare : Square" :size="14" class="text-[var(--v2-accent)]" />
          <span>{{ allVisibleSelected ? $t('search.action.deselectAll') : $t('search.action.selectAll') }}</span>
        </button>
        <span class="v2-pill font-mono">{{ $t('search.action.selectedCount', { count: selectedItems.length }) }}</span>
      </div>

      <div class="flex items-center gap-2">
        <button type="button" @click="submitBatch('download')" :disabled="selectedItems.length === 0 || batchProcessing" class="v2-btn v2-btn-primary v2-btn-sm">
          <Download :size="13" />
          <span>{{ $t('search.action.batchDownload') }}</span>
        </button>
        <button type="button" @click="submitBatch('subscribe')" :disabled="selectedItems.length === 0 || batchProcessing" class="v2-btn v2-btn-sm">
          <Plus :size="13" />
          <span>{{ $t('search.action.batchSubscribe') }}</span>
        </button>
        <button type="button" @click="exitBatchMode" :disabled="batchProcessing" class="v2-btn v2-btn-ghost v2-btn-sm">
          {{ $t('search.action.batchCancel') }}
        </button>
      </div>
    </div>

    <!-- 高密度结构化搜索结果行 -->
    <div v-if="filteredResults.length > 0" class="v2-panel overflow-hidden divide-y divide-[var(--v2-border)]">
      <div
        v-for="(item, idx) in filteredResults"
        :key="item.info_hash || idx"
        class="p-3.5 sm:px-5 hover:bg-[var(--v2-surface-2)]/55 transition-colors flex flex-col lg:flex-row lg:items-center justify-between gap-3"
        :class="{ 'bg-[var(--v2-accent)]/6': isItemSelected(item) }"
      >
        <div class="flex items-start gap-3 min-w-0 flex-1">
          <input
            v-if="batchMode && canDownload(item)"
            type="checkbox"
            :checked="isItemSelected(item)"
            :disabled="batchProcessing"
            :aria-label="t('search.action.selectResult', { title: item.title })"
            @change="toggleItemSelection(item)"
            class="checkbox checkbox-primary checkbox-sm mt-1 shrink-0"
          />

          <div v-if="item.cover_url" class="w-11 h-15 rounded-lg bg-[var(--v2-surface-2)] shrink-0 overflow-hidden border border-[var(--v2-border)] hidden sm:block">
            <img :src="proxyImage(item.cover_url)" :alt="item.title" class="w-full h-full object-cover" loading="lazy" decoding="async" />
          </div>

          <div class="min-w-0 flex-1 space-y-1.5">
            <div class="flex flex-wrap items-center gap-1.5">
              <span
                v-if="item.url && item.url.includes('/Home/Bangumi/') && !item.magnet"
                class="text-[10px] font-bold px-2 py-0.5 rounded-md bg-[var(--v2-accent)] text-white"
              >
                {{ t('search.seriesEntry') }}
              </span>
              <span v-else class="text-[10px] font-bold px-2 py-0.5 rounded-md border" :class="sourceBadgeClass(item.source)">
                {{ item.source }}
              </span>

              <span v-if="extractItemSubgroup(item.title)" class="text-[11px] font-semibold px-2 py-0.5 rounded-md bg-[var(--v2-surface-2)] text-[var(--v2-ink)] border border-[var(--v2-border)]">
                {{ extractItemSubgroup(item.title) }}
              </span>

              <span v-for="tag in extractQualityTags(item.title)" :key="tag" class="text-[10px] font-mono font-semibold px-1.5 py-0.5 rounded bg-[var(--v2-accent)]/10 text-[var(--v2-accent)]">
                {{ tag }}
              </span>
            </div>

            <h3 class="text-sm font-semibold leading-snug text-[var(--v2-ink)] break-all select-all">
              {{ item.title }}
            </h3>

            <div class="flex flex-wrap items-center gap-3 text-[11px] font-mono text-[var(--v2-ink-subtle)] tabular-nums">
              <span v-if="item.size > 0" class="flex items-center gap-1">
                <HardDrive :size="11" /> {{ formatSize(item.size) }}
              </span>
              <span v-if="formatPubDate(item.pub_date)" class="flex items-center gap-1">
                <Calendar :size="11" /> {{ formatPubDate(item.pub_date) }}
              </span>
              <span v-if="item.bangumi_id">BGM: {{ item.bangumi_id }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-1.5 shrink-0 flex-wrap justify-end">
          <button
            v-if="canDownload(item)"
            type="button"
            @click="directDownload(item)"
            :disabled="directDownloading[getItemKey(item)]"
            class="v2-btn v2-btn-sm text-[var(--v2-accent)]"
            :title="$t('search.action.downloadDirect')"
          >
            <Download :size="13" />
            <span>{{ $t('search.action.downloadDirect') }}</span>
          </button>

          <button
            v-if="resourceMagnet(item) && pluginTargets.length"
            type="button"
            @click="choosePlugin(item)"
            class="v2-btn v2-btn-sm"
            :title="t('search.plugin.open')"
          >
            <CloudUpload :size="13" />
          </button>

          <button
            v-if="resourceMagnet(item)"
            type="button"
            @click="copyMagnet(item)"
            class="v2-btn v2-btn-sm"
            :title="$t('search.action.copyMagnet')"
          >
            <Copy :size="13" />
            <span class="hidden sm:inline">{{ $t('search.action.copyMagnet') }}</span>
          </button>

          <a
            v-if="sourceLink(item)"
            :href="sourceLink(item)"
            target="_blank"
            rel="noopener noreferrer"
            class="v2-btn v2-btn-sm w-8 px-0"
            :title="$t('search.action.openSource')"
          >
            <ExternalLink :size="13" />
          </a>

          <button
            v-if="subscribed.has(item.title) && findSubscribedId(item)"
            type="button"
            class="v2-btn v2-btn-sm bg-emerald-500/15 border-emerald-500/40 text-emerald-400 hover:bg-emerald-500/25"
            @click="router.push(`/subscriptions/${findSubscribedId(item)}`)"
          >
            <Check :size="13" />
            <span>{{ $t('search.action.viewDetail') }}</span>
          </button>
          <button
            v-else
            type="button"
            class="v2-btn v2-btn-sm"
            :class="subscribed.has(item.title) ? 'bg-emerald-500/20 border-emerald-500/40 text-emerald-400' : 'v2-btn-primary'"
            :disabled="subscribed.has(item.title)"
            @click="openSubscribe(item)"
          >
            <component :is="subscribed.has(item.title) ? Check : Plus" :size="13" />
            <span>{{ subscribed.has(item.title) ? $t('search.action.subscribed') : $t('search.action.subscribe') }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 空闲发现 2 栏 Bento（最近搜索 | 热门标签） -->
    <div v-else-if="!loading && results.length === 0" class="space-y-4">
      <div v-if="hasSearched && !error" class="v2-panel py-12 px-6 text-center space-y-2">
        <div class="w-11 h-11 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] flex items-center justify-center mx-auto">
          <Search :size="20" />
        </div>
        <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('search.empty.noResults') }}</h3>
        <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('search.empty.noResultsDesc') }}</p>
      </div>

      <div class="grid md:grid-cols-2 gap-4">
        <!-- 最近搜索 -->
        <div class="v2-panel p-5 space-y-3.5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2 text-xs font-semibold text-[var(--v2-ink)]">
              <History :size="14" class="text-[var(--v2-accent)]" />
              <span>{{ $t('search.historyTitle') }}</span>
            </div>
            <button
              v-if="searchHistory.length > 0"
              type="button"
              @click="clearAllHistory"
              class="text-xs text-[var(--v2-ink-subtle)] hover:text-red-400 flex items-center gap-1 cursor-pointer"
            >
              <Trash2 :size="12" />
              <span>{{ $t('search.historyClear') }}</span>
            </button>
          </div>
          <div v-if="searchHistory.length > 0" class="flex flex-wrap gap-2">
            <div
              v-for="hist in searchHistory"
              :key="hist"
              class="flex items-center gap-1.5 bg-[var(--v2-surface-2)] hover:border-[var(--v2-accent)]/40 hover:text-[var(--v2-accent)] px-2.5 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer border border-[var(--v2-border)]"
              @click="quickSearch(hist)"
            >
              <span>{{ hist }}</span>
              <button type="button" @click.stop="removeHistoryItem(hist)" class="opacity-50 hover:opacity-100 hover:text-red-400 p-0.5">
                <X :size="12" />
              </button>
            </div>
          </div>
          <p v-else class="text-xs text-[var(--v2-ink-subtle)] py-2">
            {{ locale === 'zh' ? '暂无最近搜索记录，试试从右侧热门词条发起检索。' : 'No recent search history yet.' }}
          </p>
        </div>

        <!-- 热门发现 -->
        <div class="v2-panel p-5 space-y-3.5">
          <div class="flex items-center gap-2 text-xs font-semibold text-[var(--v2-ink)]">
            <Sparkles :size="14" class="text-emerald-400" />
            <span>{{ $t('search.recommendTitle') }}</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="tag in popularTags"
              :key="tag"
              type="button"
              @click="quickSearch(tag)"
              class="bg-[var(--v2-surface-2)] hover:bg-[var(--v2-accent)]/12 hover:text-[var(--v2-accent)] hover:border-[var(--v2-accent)]/35 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors border border-[var(--v2-border)] cursor-pointer"
            >
              {{ tag }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 字幕组选择 Modal -->
    <div v-if="showGroupModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="w-full max-w-lg v2-panel p-6 space-y-5 shadow-2xl border border-[var(--v2-border-strong)]">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-base font-bold">{{ $t('search.modal.title') }}</h3>
            <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('search.modal.subtitle') }}</p>
          </div>
          <button type="button" class="p-1.5 rounded-lg hover:bg-[var(--v2-surface-2)]" :disabled="groupSubmitting" @click="closeGroups">
            <X :size="16" />
          </button>
        </div>

        <div class="v2-panel-muted p-3 text-xs font-medium text-[var(--v2-ink)] line-clamp-2">
          {{ selectedItem?.title }}
        </div>

        <div v-if="subgroups.length > 5" class="relative">
          <Search :size="14" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
          <input v-model="subgroupSearch" type="text" :placeholder="t('search.filterGroups')" class="v2-input pl-8 h-8 text-xs" />
        </div>

        <div v-if="groupLoading" class="flex flex-col items-center justify-center py-10 gap-2">
          <span class="loading loading-spinner loading-md text-[var(--v2-accent)]"></span>
          <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('search.modal.fetching') }}</p>
        </div>

        <div v-else-if="groupError" class="v2-panel-muted border border-red-500/30 text-red-400 p-4 text-center text-xs">
          {{ groupError }}
        </div>

        <div v-else-if="subgroups.length === 0" class="text-center py-6 space-y-4">
          <p class="text-sm font-semibold">{{ $t('search.modal.empty') }}</p>
          <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('search.modal.emptyDesc') }}</p>
          <div class="flex justify-center gap-2">
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" :disabled="groupSubmitting" @click="closeGroups">{{ $t('search.modal.cancel') }}</button>
            <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" :disabled="groupSubmitting" @click="selectedItem && subscribe(selectedItem, '')">
              {{ $t('search.modal.proceed') }}
            </button>
          </div>
        </div>

        <div v-else class="space-y-2 max-h-72 overflow-y-auto pr-0.5" :class="{ 'ring-2 ring-[var(--v2-accent)]/60 rounded-xl p-1 -m-1 transition-all': highlightSearchGroups }">
          <label
            v-for="g in filteredSubgroups"
            :key="g.rss_url"
            class="w-full rounded-xl border p-3 flex items-center gap-3 text-left transition-colors cursor-pointer select-none"
            :class="selectedModalSubgroups.has(g.name)
              ? 'bg-[var(--v2-accent)]/12 border-[var(--v2-accent)]'
              : 'v2-panel-muted hover:border-[var(--v2-accent)]/40'"
          >
            <input
              type="checkbox"
              class="checkbox checkbox-sm checkbox-primary shrink-0"
              :checked="selectedModalSubgroups.has(g.name)"
              :disabled="groupSubmitting"
              @change="toggleModalSubgroup(g.name)"
            />
            <div class="flex items-center gap-2 min-w-0 flex-1">
              <User :size="14" class="text-[var(--v2-accent)] shrink-0" />
              <span class="font-semibold text-xs truncate text-[var(--v2-ink)]">{{ g.name }}</span>
            </div>
            <span v-if="isDefaultSubgroup(g.name)" class="px-1.5 py-0.5 rounded bg-[var(--v2-accent)]/15 text-[var(--v2-accent)] text-[10px] font-semibold shrink-0">
              {{ $t('schedule.modal.defaultTag') }}
            </span>
          </label>
        </div>

        <div v-if="subgroups.length > 0" class="pt-2 flex justify-between items-center gap-2 text-xs border-t border-[var(--v2-border)]">
          <span class="text-[var(--v2-ink-muted)] truncate">
            <template v-if="selectedModalSubgroups.size > 0">
              {{ $t('schedule.modal.selectedCount', { count: selectedModalSubgroups.size }) }}
            </template>
            <template v-else-if="defaultSubgroups.length > 0">
              {{ $t('search.modal.willUseDefault', { groups: defaultSubgroups.join('、') }) }}
            </template>
            <template v-else>
              {{ $t('search.modal.proceed') }}
            </template>
          </span>
          <div class="flex items-center gap-2 shrink-0">
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" :disabled="groupSubmitting" @click="closeGroups">{{ $t('search.modal.cancel') }}</button>
            <button type="button" class="v2-btn v2-btn-primary v2-btn-sm px-4" :disabled="groupSubmitting" @click="confirmGroupSubscribe()">
              <span>{{ $t('search.modal.confirm') }}</span>
              <span v-if="selectedModalSubgroups.size > 0" class="font-mono text-[11px] opacity-85">({{ selectedModalSubgroups.size }})</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 搜索页缺失默认字幕组 / 未选择字幕组提示弹窗 -->
    <div v-if="searchSubgroupPromptMode" class="fixed inset-0 z-[70] flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm">
      <div class="w-full max-w-sm v2-panel p-5 space-y-4 shadow-2xl border border-[var(--v2-border-strong)]">
        <div class="flex items-start gap-3">
          <div class="w-9 h-9 rounded-xl bg-amber-500/15 text-amber-400 flex items-center justify-center shrink-0">
            <AlertTriangle :size="18" />
          </div>
          <div class="space-y-1 min-w-0">
            <h3 class="text-base font-bold text-[var(--v2-ink)]">
              {{ searchSubgroupPromptMode === 'missing_default' ? $t('schedule.modal.missingDefaultTitle') : $t('schedule.modal.noSelectionTitle') }}
            </h3>
            <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">
              {{ searchSubgroupPromptMode === 'missing_default'
                ? $t('schedule.modal.missingDefaultDesc', { groups: defaultSubgroups.join('、') })
                : $t('schedule.modal.noSelectionDesc') }}
            </p>
          </div>
        </div>
        <div class="space-y-2 pt-1">
          <button type="button" class="v2-btn v2-btn-primary w-full" @click="focusSearchSubgroupList">
            {{ $t('schedule.modal.goChooseSubgroup') }}
          </button>
          <button
            type="button"
            class="v2-btn v2-btn-ghost w-full"
            :disabled="groupSubmitting"
            @click="confirmGroupSubscribe({ allowUnrestricted: true })"
          >
            {{ $t('schedule.modal.subscribeAnySubgroup') }}
          </button>
        </div>
      </div>
    </div>

    <SubscriptionReviewDialog v-if="reviewItems.length" :items="reviewItems" @saved="markReviewedItems" @close="reviewItems = []" />
    <SearchPluginDialog v-if="pluginSelection" :item="pluginSelection.item" :targets="pluginSelection.targets" @close="pluginSelection = null; loadPluginTargets()" />
  </div>
</template>
