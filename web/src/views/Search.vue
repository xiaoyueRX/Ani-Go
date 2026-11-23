<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'
import SearchPluginDialog, { type SearchPluginTarget } from '../components/SearchPluginDialog.vue'
import SubscriptionReviewDialog, { type ReviewRelease } from '../components/SubscriptionReviewDialog.vue'
import { 
  ChevronLeft, Search, Timer, Calendar,
  AlertTriangle, X, Antenna, Sparkles, Info, 
  Check, Plus, User, Copy, ExternalLink, Trash2, 
  HardDrive, History, Filter, Clock, Download,
  CheckSquare, Square, ListChecks, CloudUpload
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

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

// 核心搜索状态
const query = ref('')
const results = ref<TorrentItem[]>([])
const loading = ref(false)
const error = ref('')
const subscribed = ref<Set<string>>(new Set())
const lastSearchTime = ref('')
const searchDuration = ref(0)
const smartSearch = ref(false)
const expandedQueries = ref<string[]>([])
const usedAI = ref(false)
const aiError = ref('')
const aiConfigured = ref(false)

// 结果内即时筛选与排序
const selectedSource = ref('all')
const inResultFilter = ref('')
const sortBy = ref<'default' | 'date' | 'size_desc' | 'size_asc'>('default')

// 字幕组记忆与快速过滤
const FILTER_PREF_KEY = 'anigo_search_filter_pref'
const selectedSubgroup = ref(localStorage.getItem(FILTER_PREF_KEY) || 'all')

// 直接下载与批量模式
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
        .map(a => ({ id: p.id, name: p.name, official: p.is_builtin === true && p.origin === 'official',
          action: a.id, label: a.label, url: p.url ?? '' })))
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
    } catch { /* Try the other URL supplied by this source. */ }
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

// 搜索历史与热门标签
const SEARCH_HISTORY_KEY = 'anigo_search_history'
const searchHistory = ref<string[]>([])
const popularTags = [
  '葬送的芙莉莲', '我推的孩子', '迷宫饭', '孤独摇滚', 
  '鬼灭之刃', '间谍过家家', '无职转生', '1080p', '4K', '简日双语'
]

function loadSearchHistory() {
  try {
    const raw = localStorage.getItem(SEARCH_HISTORY_KEY)
    if (raw) {
      searchHistory.value = JSON.parse(raw)
    }
  } catch {}
}

function saveSearchHistory(q: string) {
  const clean = q.trim()
  if (!clean) return
  const set = new Set([clean, ...searchHistory.value])
  searchHistory.value = Array.from(set).slice(0, 10)
  try {
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(searchHistory.value))
  } catch {}
}

function removeHistoryItem(item: string) {
  searchHistory.value = searchHistory.value.filter(s => s !== item)
  try {
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(searchHistory.value))
  } catch {}
}

function clearAllHistory() {
  searchHistory.value = []
  try {
    localStorage.removeItem(SEARCH_HISTORY_KEY)
  } catch {}
}

// 加载已有追番列表
async function loadSubscribedList() {
  try {
    const { data } = await request.get('/subscriptions')
    if (Array.isArray(data)) {
      subscribed.value = new Set(data.map((s: any) => s.title_cn))
    }
  } catch {}
}

onMounted(async () => {
  loadSearchHistory()
  loadSubscribedList()
  void loadPluginTargets()
  window.addEventListener?.('focus', loadPluginTargets)

  const q = route.query.q
  if (typeof q === 'string' && q.trim()) {
    query.value = q
    void handleSearch()
  }

  // 检查 AI 是否已配置
  try {
    const { data } = await request.get('/search/capabilities')
    aiConfigured.value = data.smart_search === true
  } catch (e) {
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

// 字幕组选择弹窗
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
  loading.value = true
  error.value = ''
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
    if (e.code === 'ECONNABORTED') {
      error.value = t('search.error.timeout') || '检索超时'
    } else {
      error.value = e.response?.data?.error || t('search.error.failed') || '检索失败'
    }
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
  results.value = []
  error.value = ''
  expandedQueries.value = []
  aiError.value = ''
  usedAI.value = false
  lastSearchTime.value = ''
  exitBatchMode()
  query.value = ''
  if (searchInputRef.value) {
    searchInputRef.value.focus()
  }
}

// 统计结果数据源
const availableSources = computed(() => {
  const map = new Map<string, number>()
  for (const item of results.value) {
    const s = item.source || 'Other'
    map.set(s, (map.get(s) || 0) + 1)
  }
  return map
})

// 字幕组智能提取与记忆
function selectSubgroup(group: string) {
  selectedSubgroup.value = group
  try {
    localStorage.setItem(FILTER_PREF_KEY, group)
  } catch {}
}

const extractedSubgroups = computed(() => {
  const map = new Map<string, number>()
  const regex = /^[\[【]([^\]】]+)[\]】]/
  for (const item of results.value) {
    const match = item.title.match(regex)
    if (match && match[1]) {
      const g = match[1].trim()
      if (!/^(1080p|720p|4k|2160p|hevc|x264|x265|bilibili|web-dl)$/i.test(g) && g.length <= 25) {
        map.set(g, (map.get(g) || 0) + 1)
      }
    }
  }
  return Array.from(map.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10)
})

// 多维度过滤与排序
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
    list = [...list].sort((a, b) => {
      const ta = new Date(a.pub_date || 0).getTime()
      const tb = new Date(b.pub_date || 0).getTime()
      return tb - ta
    })
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
  if (idx >= 0) {
    selectedItems.value.splice(idx, 1)
  } else {
    selectedItems.value.push(item)
  }
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
    try {
      await queueDownload(item)
    } catch {
      failed.push(item)
    }
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

function markReviewedItems(items: ReviewRelease[]) {
  const accepted = new Set(items.map(item => item.title))
  for (const title of accepted) subscribed.value.add(title)
  selectedItems.value = selectedItems.value.filter(item => !accepted.has(item.title))
  if (batchMode.value && !selectedItems.value.length) exitBatchMode()
}

function closeGroups() {
  groupRequest?.abort()
  showGroupModal.value = false
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

async function saveSubscription(item: TorrentItem, rssUrl: string, subgroupName?: string) {
  await request.post('/subscriptions', {
    title_cn: item.title,
    bangumi_id: item.source === 'Mikan' ? undefined : item.bangumi_id,
    mikan_id: item.source === 'Mikan' ? item.bangumi_id : undefined,
    rss_url: rssUrl || undefined,
    subgroup_name: subgroupName || undefined,
    filter_json: JSON.stringify({ source_url: item.url }),
    cover_url: item.cover_url || '',
  })
  subscribed.value.add(item.title)
}

async function subscribe(item: TorrentItem, rssUrl: string, subgroupName?: string) {
  if (groupSubmitting.value) return
  groupSubmitting.value = true
  try {
    await saveSubscription(item, rssUrl, subgroupName)
    window.showToast?.(t('search.subscribed', { title: item.title }) || `成功订阅: ${item.title}`, 'success')
    showGroupModal.value = false
  } catch (e: any) {
    window.showToast?.(t('search.subscribeFailed', { error: e.response?.data?.error || e.message }) || '订阅失败', 'error')
  } finally {
    groupSubmitting.value = false
  }
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
    } catch {
      copied = false
    } finally {
      input.remove()
    }
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

function sourceBadge(source: string): string {
  const map: Record<string, string> = {
    Mikan: 'bg-primary/10 text-primary border-primary/20',
    Nyaa: 'bg-secondary/10 text-secondary border-secondary/20',
    'ACG.RIP': 'bg-accent/10 text-accent border-accent/20',
    AnimeTosho: 'bg-info/10 text-info border-info/20',
  }
  return map[source] || 'bg-base-200 text-base-content/40 border-base-300'
}
</script>

<template>
  <div class="space-y-8 pb-20 max-w-7xl mx-auto animate-in fade-in duration-300">
    <!-- Header Section -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-base-100 p-6 sm:p-7 rounded-3xl border border-base-200/80 shadow-sm">
      <div class="space-y-1">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shadow-inner">
            <Search :size="20" />
          </div>
          <h1 class="text-2xl sm:text-3xl font-black tracking-tight italic">{{ $t('search.title') }}</h1>
          <span v-if="results.length > 0" class="badge badge-neutral text-xs font-mono font-bold">{{ t('search.resultsCount', { count: results.length }) }}</span>
        </div>
        <p class="text-xs text-base-content/60 font-medium">{{ $t('search.subtitle') }}</p>
      </div>
      
      <div class="flex flex-wrap items-center gap-2">
        <router-link to="/subscriptions/new" class="btn btn-outline btn-sm rounded-xl gap-2">
          <Plus :size="16" aria-hidden="true" />
          {{ $t('search.addManually') }}
        </router-link>
        <button class="btn btn-ghost btn-sm border border-base-200/80 rounded-xl gap-2 px-4 hover:bg-base-200 transition-all"
          @click="router.push('/schedule')">
          <ChevronLeft :size="16" />
          <span class="text-xs font-bold">{{ $t('search.back') }}</span>
        </button>
      </div>
    </div>

    <!-- Modern Search Interface -->
    <div class="space-y-4 max-w-4xl mx-auto w-full">
      <div class="relative group w-full">
        <div class="absolute -inset-1 bg-gradient-to-r from-primary via-secondary to-accent rounded-[2.5rem] blur opacity-15 group-focus-within:opacity-35 transition-opacity duration-500"></div>
        <div class="relative bg-base-100 rounded-2xl sm:rounded-[2.2rem] border border-base-200/80 shadow-2xl flex items-center p-1.5 sm:p-2 overflow-hidden">
          <div class="pl-3 sm:pl-5 text-base-content/30 group-focus-within:text-primary transition-colors shrink-0">
            <Search :size="22" class="sm:w-6 sm:h-6" />
          </div>
          <input 
            ref="searchInputRef"
            v-model="query" 
            type="text"
            class="flex-1 min-w-0 bg-transparent border-none outline-none px-3 sm:px-5 py-2.5 sm:py-3.5 font-bold text-sm sm:text-base placeholder:text-base-content/30 placeholder:font-bold"
            :placeholder="$t('search.placeholder')"
            @keyup.enter="handleSearch"
          />
          <button 
            v-if="query" 
            @click="clearQuery"
            class="btn btn-ghost btn-circle btn-xs mr-2 text-base-content/40 hover:text-base-content"
          >
            <X :size="14" />
          </button>
          <button 
            class="btn btn-primary h-11 sm:h-12 min-h-0 px-5 sm:px-8 rounded-xl sm:rounded-[1.6rem] shadow-md gap-2 shrink-0 group/btn transition-all active:scale-95"
            :disabled="loading || !query.trim()" 
            @click="handleSearch"
          >
            <span v-if="loading" class="loading loading-spinner loading-xs sm:loading-sm"></span>
            <template v-else>
              <span class="hidden sm:inline text-xs font-black uppercase tracking-wider">{{ $t('search.execute') }}</span>
              <Search :size="16" class="group-hover/btn:scale-110 transition-transform" />
            </template>
          </button>
        </div>
      </div>

      <!-- Search Options Bar -->
      <div class="flex items-center justify-between px-2 flex-wrap gap-3">
        <label class="flex items-center gap-2 cursor-pointer text-xs font-bold select-none text-base-content/70 hover:text-base-content transition-colors">
          <Sparkles :size="15" class="text-secondary" />
          <span>{{ $t('search.smart') }}</span>
          <input v-model="smartSearch" type="checkbox" class="toggle toggle-primary toggle-sm rounded-full" :disabled="!aiConfigured" />
          <span v-if="!aiConfigured" class="text-[10px] font-bold text-warning ml-1">
            ({{ $t('settings.ai.error.aiNotConfigured') }})
          </span>
        </label>

        <div v-if="lastSearchTime && results.length > 0" class="text-[11px] font-mono opacity-50 flex items-center gap-3">
          <span class="flex items-center gap-1"><Timer :size="12" /> {{ (searchDuration / 1000).toFixed(2) }}s</span>
          <span>•</span>
          <span class="flex items-center gap-1"><Clock :size="12" /> {{ lastSearchTime }}</span>
        </div>
      </div>
    </div>

    <!-- AI Degradation Notice -->
    <Transition name="fade">
      <div v-if="smartSearch && aiError" class="max-w-4xl mx-auto alert bg-warning/10 border border-warning/20 text-warning rounded-2xl py-3 px-4 flex items-center gap-3">
        <Info :size="18" class="shrink-0" />
        <span class="text-xs font-bold">{{ $t('search.degraded') }}</span>
      </div>
    </Transition>

    <!-- Expanded Query Chips -->
    <div v-if="results.length > 0 && expandedQueries.length > 1" class="max-w-5xl mx-auto flex flex-wrap items-center gap-2 bg-base-100 p-3 rounded-2xl border border-base-200/80">
      <span class="text-[10px] font-black uppercase tracking-wider text-base-content/40 mr-1 flex items-center gap-1">
        <Sparkles :size="12" class="text-primary" /> {{ $t('search.expanded') }}:
      </span>
      <span v-for="(item, idx) in expandedQueries" :key="`${item}-${idx}`"
        class="badge badge-sm border-base-300 bg-base-200/60 font-bold"
        :class="idx === 0 ? 'text-primary border-primary/20' : 'opacity-80'">
        {{ item }}
      </span>
    </div>

    <!-- Error Alert -->
    <div v-if="error" class="max-w-4xl mx-auto">
      <div class="alert bg-error/10 border-error/20 text-error rounded-2xl p-4 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <AlertTriangle :size="20" class="shrink-0" />
          <div>
            <h4 class="font-black text-xs uppercase tracking-wider">{{ $t('search.error.title') }}</h4>
            <p class="text-xs font-bold opacity-80 mt-0.5">{{ error }}</p>
          </div>
        </div>
        <button class="btn btn-ghost btn-circle btn-xs" @click="error = ''">
          <X :size="14" />
        </button>
      </div>
    </div>

    <!-- Results Filter & Sorting Toolbar (when results > 0) -->
    <div v-if="results.length > 0" class="flex flex-col gap-3 bg-base-100 p-4 rounded-2xl border border-base-200/80 shadow-sm max-w-5xl mx-auto w-full">
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <!-- Source Filter Chips -->
        <div class="flex items-center gap-1.5 overflow-x-auto no-scrollbar pb-1 md:pb-0 text-xs">
          <button 
            @click="selectedSource = 'all'" 
            class="btn btn-xs sm:btn-sm rounded-xl font-bold whitespace-nowrap"
            :class="selectedSource === 'all' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
          >
            {{ $t('search.filterAll') }} ({{ results.length }})
          </button>
          <button 
            v-for="[src, count] in availableSources" 
            :key="src"
            @click="selectedSource = src" 
            class="btn btn-xs sm:btn-sm rounded-xl font-bold whitespace-nowrap"
            :class="selectedSource === src ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
          >
            {{ src }} ({{ count }})
          </button>
        </div>

        <!-- Quick Search, Sort & Batch Toggle -->
        <div class="flex items-center gap-2 flex-wrap sm:flex-nowrap">
          <div class="relative flex-1 sm:w-52">
            <input 
              v-model="inResultFilter" 
              type="text" 
              :placeholder="$t('search.filterKeyword')" 
              class="input input-bordered input-xs sm:input-sm w-full rounded-xl pl-7 text-xs font-medium"
            />
            <Filter :size="12" class="absolute left-2.5 top-1/2 -translate-y-1/2 opacity-40" />
          </div>

          <select v-model="sortBy" class="select select-bordered select-xs sm:select-sm rounded-xl text-xs font-bold">
            <option value="default">{{ $t('search.sortDefault') }}</option>
            <option value="date">{{ $t('search.sortDate') }}</option>
            <option value="size_desc">{{ $t('search.sortSizeDesc') }}</option>
            <option value="size_asc">{{ $t('search.sortSizeAsc') }}</option>
          </select>

          <button 
            @click="batchMode ? exitBatchMode() : (batchMode = true)" :disabled="batchProcessing"
            class="btn btn-xs sm:btn-sm rounded-xl font-bold gap-1.5 whitespace-nowrap"
            :class="batchMode ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60'"
          >
            <ListChecks :size="14" />
            <span>{{ batchMode ? $t('search.action.batchCancel') : $t('search.action.batchSelect') }}</span>
          </button>
        </div>
      </div>

      <!-- Subgroup Quick Filter Chips -->
      <div v-if="extractedSubgroups.length > 0" class="flex items-center gap-1.5 overflow-x-auto no-scrollbar pt-2 border-t border-base-200/60 text-xs">
        <span class="text-[10px] font-black uppercase text-base-content/40 shrink-0">{{ t('search.filterGroups') }}</span>
        <button 
          @click="selectSubgroup('all')" 
          class="btn btn-xs rounded-lg font-bold whitespace-nowrap"
          :class="selectedSubgroup === 'all' ? 'btn-neutral' : 'btn-ghost opacity-60 border border-base-200'"
        >
          {{ t('search.allGroups') }}
        </button>
        <button 
          v-for="[group, count] in extractedSubgroups" 
          :key="group"
          @click="selectSubgroup(group)" 
          class="btn btn-xs rounded-lg font-bold whitespace-nowrap"
          :class="selectedSubgroup === group ? 'btn-neutral' : 'btn-ghost opacity-70 border border-base-200'"
        >
          {{ group }} ({{ count }})
        </button>
      </div>
    </div>

    <!-- Batch Action Floating Bar -->
    <Transition name="fade">
      <div v-if="batchMode && filteredResults.length > 0" class="sticky top-4 z-40 max-w-5xl mx-auto w-full bg-base-100/95 backdrop-blur-md p-3 sm:p-4 rounded-2xl border border-primary/30 shadow-xl flex flex-wrap items-center justify-between gap-3 animate-in slide-in-from-top-2 duration-200">
        <div class="flex items-center gap-3">
          <button 
            @click="toggleSelectAll" :disabled="batchProcessing || !visibleDownloads.length"
            class="btn btn-ghost btn-xs sm:btn-sm rounded-xl gap-1.5 text-xs font-bold border border-base-300"
          >
            <component :is="allVisibleSelected ? CheckSquare : Square" :size="14" class="text-primary" />
            <span>{{ allVisibleSelected ? $t('search.action.deselectAll') : $t('search.action.selectAll') }}</span>
          </button>
          <span class="badge badge-primary badge-sm font-bold font-mono">
            {{ $t('search.action.selectedCount', { count: selectedItems.length }) }}
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button 
            @click="submitBatch('download')"
            :disabled="selectedItems.length === 0 || batchProcessing"
            class="btn btn-primary btn-xs sm:btn-sm rounded-xl gap-1.5 text-xs font-bold shadow-sm"
          >
            <span v-if="batchProcessing" class="loading loading-spinner loading-xs"></span>
            <Download v-else :size="14" />
            <span>{{ $t('search.action.batchDownload') }}</span>
          </button>

          <button 
            @click="submitBatch('subscribe')"
            :disabled="selectedItems.length === 0 || batchProcessing"
            class="btn btn-secondary btn-xs sm:btn-sm rounded-xl gap-1.5 text-xs font-bold shadow-sm"
          >
            <span v-if="batchProcessing" class="loading loading-spinner loading-xs"></span>
            <Plus v-else :size="14" />
            <span>{{ $t('search.action.batchSubscribe') }}</span>
          </button>

          <button 
            @click="exitBatchMode" :disabled="batchProcessing"
            class="btn btn-ghost btn-xs sm:btn-sm rounded-xl text-xs font-bold"
          >
            {{ $t('search.action.batchCancel') }}
          </button>
        </div>
      </div>
    </Transition>

    <!-- Results List -->
    <div v-if="filteredResults.length > 0" class="grid gap-3 sm:gap-4 max-w-5xl mx-auto w-full">
      <div 
        v-for="(item, idx) in filteredResults" 
        :key="item.info_hash || idx"
        class="group bg-base-100 rounded-2xl sm:rounded-3xl border border-base-200/80 shadow-sm hover:shadow-xl hover:border-primary/30 transition-all duration-300 overflow-hidden p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4"
        :class="{ 'ring-2 ring-primary/40': isItemSelected(item) }"
      >
        <!-- Batch Checkbox -->
        <div v-if="batchMode && canDownload(item)" class="shrink-0 flex items-center sm:self-center">
          <input 
            type="checkbox" 
            :checked="isItemSelected(item)" :disabled="batchProcessing" :aria-label="t('search.action.selectResult', { title: item.title })"
            @change="toggleItemSelection(item)"
            class="checkbox checkbox-primary checkbox-sm rounded-lg cursor-pointer"
          />
        </div>

        <!-- Poster & Meta -->
        <div class="flex items-start gap-3.5 sm:gap-5 min-w-0 flex-1 w-full">
          <!-- Poster Thumbnail -->
          <div class="w-14 h-20 sm:w-16 sm:h-24 rounded-xl bg-base-200 shrink-0 overflow-hidden relative shadow-sm group-hover:scale-105 transition-transform duration-300">
            <img 
              v-if="item.cover_url" 
              :src="proxyImage(item.cover_url)" 
              :alt="item.title" 
              class="w-full h-full object-cover" 
              loading="lazy" 
              @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" 
            />
            <div class="absolute inset-0 flex items-center justify-center text-base-content/20" v-else>
              <Antenna :size="24" />
            </div>
          </div>

          <!-- Info Details -->
          <div class="flex-1 min-w-0 space-y-2">
            <h3 class="text-sm sm:text-base font-black tracking-tight line-clamp-2 leading-snug group-hover:text-primary transition-colors select-all" :title="item.title">
              {{ item.title }}
            </h3>
            
            <div class="flex flex-wrap items-center gap-1.5 sm:gap-2">
              <span v-if="item.url && item.url.includes('/Home/Bangumi/') && !item.magnet" class="text-[10px] font-black uppercase tracking-wider py-0.5 px-2.5 rounded-md bg-primary text-primary-content shadow-sm">
                {{ t('search.seriesEntry') }}
              </span>
              <span v-else class="text-[10px] font-black uppercase tracking-wider py-0.5 px-2 rounded-md border" :class="sourceBadge(item.source)">
                {{ item.source }}
              </span>
              <span v-if="item.size > 0" class="text-[10px] font-bold font-mono bg-base-200 text-base-content/60 py-0.5 px-2 rounded-md flex items-center gap-1">
                <HardDrive :size="10" />
                {{ formatSize(item.size) }}
              </span>
              <span v-if="formatPubDate(item.pub_date)" class="text-[10px] font-bold font-mono bg-base-200 text-base-content/60 py-0.5 px-2 rounded-md flex items-center gap-1">
                <Calendar :size="10" />
                {{ formatPubDate(item.pub_date) }}
              </span>
              <span v-if="item.bangumi_id" class="text-[10px] font-bold font-mono bg-base-200 text-base-content/40 py-0.5 px-2 rounded-md">
                BGM: {{ item.bangumi_id }}
              </span>
            </div>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex items-center gap-2 shrink-0 self-end sm:self-center w-full sm:w-auto justify-end pt-2 sm:pt-0 border-t sm:border-t-0 border-base-200/50 flex-wrap">
          <!-- 立即下载 (无需订阅) -->
          <button 
            v-if="canDownload(item)"
            @click="directDownload(item)"
            :disabled="directDownloading[getItemKey(item)]"
            class="btn btn-ghost btn-sm rounded-xl gap-1.5 text-xs font-bold hover:bg-base-200 border border-base-200 text-primary"
            :title="$t('search.action.downloadDirect')"
          >
            <span v-if="directDownloading[getItemKey(item)]" class="loading loading-spinner loading-xs"></span>
            <Download v-else :size="14" />
            <span class="text-[11px]">{{ $t('search.action.downloadDirect') }}</span>
          </button>

          <button v-if="resourceMagnet(item) && pluginTargets.length" type="button" @click="choosePlugin(item)"
            class="btn btn-ghost btn-sm rounded-xl gap-1.5 border border-base-200" :aria-label="t('search.plugin.open')" :title="t('search.plugin.open')">
            <CloudUpload :size="15" /><span class="text-[11px] hidden sm:inline">{{ t('search.plugin.open') }}</span>
          </button>

          <button 
            v-if="resourceMagnet(item)"
            @click="copyMagnet(item)"
            class="btn btn-ghost btn-sm rounded-xl gap-1.5 text-xs font-bold hover:bg-base-200 border border-base-200"
            :title="$t('search.action.copyMagnet')"
          >
            <Copy :size="14" />
            <span class="text-[11px] hidden sm:inline">{{ $t('search.action.copyMagnet') }}</span>
          </button>

          <a 
            v-if="sourceLink(item)"
            :href="sourceLink(item)"
            target="_blank" 
            rel="noopener noreferrer"
            class="btn btn-ghost btn-sm btn-square rounded-xl hover:bg-base-200 border border-base-200"
            :title="$t('search.action.openSource')"
          >
            <ExternalLink :size="14" />
          </a>

          <button 
            class="btn btn-sm rounded-xl gap-1.5 px-4 font-bold shadow-sm transition-all"
            :class="subscribed.has(item.title) ? 'btn-success text-success-content' : 'btn-primary'"
            :disabled="subscribed.has(item.title)" 
            @click="openSubscribe(item)"
          >
            <component :is="subscribed.has(item.title) ? Check : Plus" :size="15" />
            <span class="text-xs">{{ subscribed.has(item.title) ? $t('search.action.subscribed') : $t('search.action.subscribe') }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Empty Search / Welcome Dashboard -->
    <div v-else-if="!loading && results.length === 0" class="max-w-4xl mx-auto space-y-8">
      <!-- Welcome Hero Card -->
      <div class="flex flex-col items-center justify-center py-16 text-center bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6">
        <div class="w-20 h-20 bg-primary/10 text-primary rounded-3xl flex items-center justify-center mb-5 shadow-inner">
          <Search :size="36" />
        </div>
        <h3 class="text-xl sm:text-2xl font-black tracking-tight mb-2">
          {{ query ? $t('search.empty.noResults') : $t('search.empty.welcome') }}
        </h3>
        <p class="text-xs sm:text-sm text-base-content/50 max-w-md mx-auto leading-relaxed">
          {{ query ? $t('search.empty.noResultsDesc') : $t('search.empty.welcomeDesc') }}
        </p>
      </div>

      <!-- Recent Searches (搜索历史) -->
      <div v-if="searchHistory.length > 0" class="bg-base-100 p-5 sm:p-6 rounded-3xl border border-base-200/80 shadow-sm space-y-3">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2 text-xs font-black uppercase tracking-wider text-base-content/60">
            <History :size="14" class="text-primary" />
            <span>{{ $t('search.historyTitle') }}</span>
          </div>
          <button @click="clearAllHistory" class="text-[11px] font-bold text-base-content/40 hover:text-error transition-colors flex items-center gap-1">
            <Trash2 :size="12" />
            <span>{{ $t('search.historyClear') }}</span>
          </button>
        </div>
        <div class="flex flex-wrap gap-2">
          <div 
            v-for="hist in searchHistory" 
            :key="hist" 
            class="group/chip flex items-center gap-1.5 bg-base-200/70 hover:bg-primary/10 hover:text-primary px-3 py-1.5 rounded-xl text-xs font-bold transition-all cursor-pointer border border-base-200"
            @click="quickSearch(hist)"
          >
            <span>{{ hist }}</span>
            <button @click.stop="removeHistoryItem(hist)" class="opacity-40 hover:opacity-100 hover:text-error p-0.5 rounded-full">
              <X :size="12" />
            </button>
          </div>
        </div>
      </div>

      <!-- Hot Discoveries (热门推荐) -->
      <div class="bg-base-100 p-5 sm:p-6 rounded-3xl border border-base-200/80 shadow-sm space-y-3">
        <div class="flex items-center gap-2 text-xs font-black uppercase tracking-wider text-base-content/60">
          <Sparkles :size="14" class="text-secondary" />
          <span>{{ $t('search.recommendTitle') }}</span>
        </div>
        <div class="flex flex-wrap gap-2">
          <button 
            v-for="tag in popularTags" 
            :key="tag" 
            @click="quickSearch(tag)"
            class="bg-base-200/50 hover:bg-secondary/10 hover:text-secondary hover:border-secondary/30 px-3.5 py-1.5 rounded-xl text-xs font-bold transition-all border border-base-200"
          >
            {{ tag }}
          </button>
        </div>
      </div>
    </div>

    <!-- Subgroup Modal -->
    <Transition name="scale">
      <div v-if="showGroupModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-md">
        <div class="w-full max-w-lg bg-base-100 rounded-3xl shadow-2xl border border-base-200 overflow-hidden animate-in zoom-in-95 duration-200">
          <div class="p-6 sm:p-8 space-y-6">
            <div class="flex items-center justify-between">
              <div class="space-y-0.5">
                <h3 class="text-xl font-black tracking-tight">{{ $t('search.modal.title') }}</h3>
                <p class="text-[11px] font-bold text-base-content/50">{{ $t('search.modal.subtitle') }}</p>
              </div>
              <button class="btn btn-ghost btn-circle btn-sm" :disabled="groupSubmitting" @click="closeGroups">
                <X :size="18" />
              </button>
            </div>

            <div class="bg-base-200/60 p-3.5 rounded-2xl border border-base-300/40">
              <p class="text-xs font-bold tracking-tight text-base-content/80 line-clamp-2">{{ selectedItem?.title }}</p>
            </div>

            <!-- Subgroup Search Input if many subgroups -->
            <div v-if="subgroups.length > 5" class="relative">
              <input 
                v-model="subgroupSearch" 
                type="text" 
                :placeholder="t('search.filterGroups')"
                class="input input-bordered input-sm w-full rounded-xl pl-8 text-xs font-medium"
              />
              <Search :size="14" class="absolute left-2.5 top-1/2 -translate-y-1/2 opacity-40" />
            </div>

            <div v-if="groupLoading" class="flex flex-col items-center justify-center py-12 gap-3">
              <span class="loading loading-spinner loading-md text-primary"></span>
              <p class="text-xs font-bold text-base-content/40">{{ $t('search.modal.fetching') }}</p>
            </div>

            <div v-else-if="groupError" class="bg-error/10 border border-error/20 text-error rounded-2xl p-5 flex flex-col items-center gap-2">
              <AlertTriangle :size="24" />
              <p class="text-xs font-bold">{{ groupError }}</p>
            </div>

            <div v-else-if="subgroups.length === 0" class="text-center py-8 space-y-5">
              <div class="w-16 h-16 bg-base-200 rounded-2xl flex items-center justify-center mx-auto text-base-content/30">
                <User :size="28" />
              </div>
              <div class="space-y-1">
                <p class="text-base font-black tracking-tight">{{ $t('search.modal.empty') }}</p>
                <p class="text-xs font-medium text-base-content/40 max-w-xs mx-auto">{{ $t('search.modal.emptyDesc') }}</p>
              </div>
              <div class="flex flex-col gap-2 pt-2">
                <button class="btn btn-primary rounded-xl font-bold text-xs" :disabled="groupSubmitting" @click="selectedItem && subscribe(selectedItem, '')">
                  {{ $t('search.modal.proceed') }}
                </button>
                <button class="btn btn-ghost btn-sm rounded-xl text-xs" :disabled="groupSubmitting" @click="closeGroups">{{ $t('search.modal.cancel') }}</button>
              </div>
            </div>

            <div v-else class="space-y-2 max-h-[320px] overflow-y-auto pr-1 custom-scrollbar">
              <button 
                v-for="g in filteredSubgroups" :key="g.rss_url"
                class="w-full bg-base-200/40 hover:bg-primary/10 hover:border-primary/30 border border-base-200 rounded-2xl p-3.5 flex items-center justify-between transition-all group/item text-left"
                :disabled="groupSubmitting" @click="selectedItem && subscribe(selectedItem, g.rss_url, g.name)"
              >
                <div class="flex items-center gap-3 min-w-0">
                  <div class="w-8 h-8 rounded-xl bg-base-200 flex items-center justify-center text-base-content/40 group-hover/item:bg-primary group-hover/item:text-primary-content transition-colors shrink-0">
                    <User :size="14" />
                  </div>
                  <span class="font-bold text-xs tracking-tight group-hover/item:text-primary transition-colors truncate">{{ g.name }}</span>
                </div>
                <Plus :size="16" class="opacity-30 group-hover/item:opacity-100 text-primary transition-opacity shrink-0" />
              </button>
            </div>

            <div v-if="subgroups.length > 0" class="pt-2 flex justify-between items-center text-xs">
              <button class="btn btn-ghost btn-xs text-base-content/50" :disabled="groupSubmitting" @click="selectedItem && subscribe(selectedItem, '')">
                {{ t('search.modal.proceed') }}
              </button>
              <button class="btn btn-ghost btn-xs" :disabled="groupSubmitting" @click="closeGroups">{{ $t('search.modal.cancel') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    <SubscriptionReviewDialog v-if="reviewItems.length" :items="reviewItems"
      @saved="markReviewedItems" @close="reviewItems = []" />
    <SearchPluginDialog v-if="pluginSelection" :item="pluginSelection.item" :targets="pluginSelection.targets"
      @close="pluginSelection = null; loadPluginTargets()" />
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 5px;
}
.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: hsl(var(--bc) / 0.15);
  border-radius: 10px;
}
</style>
