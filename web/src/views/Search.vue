<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'
import { 
  ChevronLeft, Search, LayoutGrid, Timer, Calendar, 
  AlertTriangle, X, Antenna, Sparkles, Info, 
  Check, Plus, User, Copy, ExternalLink, Trash2, 
  HardDrive, History, Filter, Clock
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface TorrentItem {
  title: string
  url: string
  magnet: string
  size: number
  pub_date: string
  source: string
  bangumi_id: string
  info_hash: string
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

  // 检查 AI 是否已配置
  try {
    const { data } = await request.get('/settings')
    const settings = data as Record<string, string>
    aiConfigured.value = !!(settings.AI_ENDPOINT && settings.AI_API_KEY && settings.AI_MODEL)
  } catch (e) {
    aiConfigured.value = false
  }
  
  const q = route.query.q as string
  if (q) {
    query.value = q
    handleSearch()
  }
})

// 字幕组选择弹窗
const showGroupModal = ref(false)
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
  if (!q) return
  saveSearchHistory(q)
  loading.value = true
  error.value = ''
  selectedSource.value = 'all'
  inResultFilter.value = ''
  const start = Date.now()
  try {
    if (smartSearch.value) {
      const { data } = await request.post('/search/smart', { query: q, limit: 60 }, { timeout: 45000 })
      results.value = data.items || []
      expandedQueries.value = data.expanded_queries || [q]
      usedAI.value = !!data.used_ai
      aiError.value = data.ai_error || ''
    } else {
      const { data } = await request.get('/search', { params: { q }, timeout: 30000 })
      results.value = data || []
      expandedQueries.value = [q]
      usedAI.value = false
      aiError.value = ''
    }
    lastSearchTime.value = new Date().toLocaleTimeString()
    searchDuration.value = Date.now() - start
  } catch (e: any) {
    if (e.code === 'ECONNABORTED') {
      error.value = t('search.error.timeout') || '检索超时'
    } else {
      error.value = e.response?.data?.error || t('search.error.failed') || '检索失败'
    }
  } finally {
    loading.value = false
  }
}

function quickSearch(keyword: string) {
  query.value = keyword
  handleSearch()
}

function clearQuery() {
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

// 多维度过滤与排序
const filteredResults = computed(() => {
  let list = results.value

  if (selectedSource.value !== 'all') {
    list = list.filter(item => item.source === selectedSource.value)
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

async function openSubscribe(item: TorrentItem) {
  if (!item.bangumi_id) {
    await subscribe(item, '')
    return
  }
  selectedItem.value = item
  subgroups.value = []
  subgroupSearch.value = ''
  groupError.value = ''
  showGroupModal.value = true
  groupLoading.value = true
  try {
    const { data } = await request.get('/mikan/groups', {
      params: { bangumi_id: item.bangumi_id },
      timeout: 15000,
    })
    subgroups.value = data || []
  } catch (e: any) {
    groupError.value = e.code === 'ECONNABORTED' ? (t('schedule.error.timeout') || '超时') : (t('schedule.error.failed') || '获取字幕组失败')
  } finally {
    groupLoading.value = false
  }
}

declare global {
  interface Window {
    showToast: (message: string, type?: 'success' | 'error' | 'info') => void
  }
}

async function subscribe(item: TorrentItem, rssUrl: string, subgroupName?: string) {
  try {
    await request.post('/subscriptions', {
      title_cn: item.title,
      bangumi_id: item.bangumi_id,
      rss_url: rssUrl || undefined,
      subgroup_name: subgroupName || undefined,
      filter_json: JSON.stringify({ source_url: item.url }),
      cover_url: item.cover_url || '',
    })
    subscribed.value.add(item.title)
    window.showToast?.(t('search.subscribed', { title: item.title }) || `成功订阅: ${item.title}`, 'success')
    showGroupModal.value = false
  } catch (e: any) {
    window.showToast?.(t('search.subscribeFailed', { error: e.response?.data?.error || e.message }) || '订阅失败', 'error')
  }
}

async function copyMagnet(item: TorrentItem) {
  if (!item.magnet) {
    window.showToast?.('无可用磁力链接', 'error')
    return
  }
  try {
    await navigator.clipboard.writeText(item.magnet)
    window.showToast?.(t('search.action.magnetCopied') || '磁力链接已复制到剪贴板', 'success')
  } catch {
    const input = document.createElement('textarea')
    input.value = item.magnet
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    document.body.removeChild(input)
    window.showToast?.(t('search.action.magnetCopied') || '磁力链接已复制到剪贴板', 'success')
  }
}

function formatSize(size: number): string {
  if (!size || size <= 0) return '0 B'
  const mb = size / 1024 / 1024
  return mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(1) + ' MB'
}

function formatPubDate(d?: string): string {
  if (!d) return ''
  try {
    const date = new Date(d)
    if (isNaN(date.getTime())) return d
    const now = Date.now()
    const diffHours = (now - date.getTime()) / (1000 * 60 * 60)
    if (diffHours < 1) return '刚刚'
    if (diffHours < 24) return `${Math.floor(diffHours)}小时前`
    if (diffHours < 48) return '昨天'
    const days = Math.floor(diffHours / 24)
    if (days <= 30) return `${days}天前`
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
  } catch {
    return d
  }
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
          <span v-if="results.length > 0" class="badge badge-neutral text-xs font-mono font-bold">{{ results.length }} 条结果</span>
        </div>
        <p class="text-xs text-base-content/60 font-medium">{{ $t('search.subtitle') }}</p>
      </div>
      
      <button 
        class="btn btn-ghost btn-sm border border-base-200/80 rounded-xl gap-2 px-4 hover:bg-base-200 transition-all self-start sm:self-auto" 
        @click="router.push('/schedule')"
      >
        <ChevronLeft :size="16" />
        <span class="text-xs font-bold">{{ $t('search.back') }}</span>
      </button>
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
    <div v-if="results.length > 0" class="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-base-100 p-4 rounded-2xl border border-base-200/80 shadow-sm max-w-5xl mx-auto w-full">
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

      <!-- Quick Search & Sort -->
      <div class="flex items-center gap-2 flex-wrap sm:flex-nowrap">
        <div class="relative flex-1 sm:w-56">
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
      </div>
    </div>

    <!-- Results List -->
    <div v-if="filteredResults.length > 0" class="grid gap-3 sm:gap-4 max-w-5xl mx-auto w-full">
      <div 
        v-for="(item, idx) in filteredResults" 
        :key="item.info_hash || idx"
        class="group bg-base-100 rounded-2xl sm:rounded-3xl border border-base-200/80 shadow-sm hover:shadow-xl hover:border-primary/30 transition-all duration-300 overflow-hidden p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4"
      >
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
              <span class="text-[10px] font-black uppercase tracking-wider py-0.5 px-2 rounded-md border" :class="sourceBadge(item.source)">
                {{ item.source }}
              </span>
              <span v-if="item.size > 0" class="text-[10px] font-bold font-mono bg-base-200 text-base-content/60 py-0.5 px-2 rounded-md flex items-center gap-1">
                <HardDrive :size="10" />
                {{ formatSize(item.size) }}
              </span>
              <span v-if="item.pub_date" class="text-[10px] font-bold font-mono bg-base-200 text-base-content/60 py-0.5 px-2 rounded-md flex items-center gap-1">
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
        <div class="flex items-center gap-2 shrink-0 self-end sm:self-center w-full sm:w-auto justify-end pt-2 sm:pt-0 border-t sm:border-t-0 border-base-200/50">
          <button 
            v-if="item.magnet"
            @click="copyMagnet(item)"
            class="btn btn-ghost btn-sm rounded-xl gap-1.5 text-xs font-bold hover:bg-base-200 border border-base-200"
            :title="$t('search.action.copyMagnet')"
          >
            <Copy :size="14" />
            <span class="text-[11px]">{{ $t('search.action.copyMagnet') }}</span>
          </button>

          <a 
            v-if="item.url" 
            :href="item.url" 
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
              <button class="btn btn-ghost btn-circle btn-sm" @click="showGroupModal = false">
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
                placeholder="搜索字幕组..." 
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
                <button class="btn btn-primary rounded-xl font-bold text-xs" @click="selectedItem && subscribe(selectedItem, '')">
                  {{ $t('search.modal.proceed') }}
                </button>
                <button class="btn btn-ghost btn-sm rounded-xl text-xs" @click="showGroupModal = false">{{ $t('search.modal.cancel') }}</button>
              </div>
            </div>

            <div v-else class="space-y-2 max-h-[320px] overflow-y-auto pr-1 custom-scrollbar">
              <button 
                v-for="g in filteredSubgroups" :key="g.rss_url"
                class="w-full bg-base-200/40 hover:bg-primary/10 hover:border-primary/30 border border-base-200 rounded-2xl p-3.5 flex items-center justify-between transition-all group/item text-left"
                @click="selectedItem && subscribe(selectedItem, g.rss_url, g.name)"
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
              <button class="btn btn-ghost btn-xs text-base-content/50" @click="selectedItem && subscribe(selectedItem, '')">
                跳过字幕组，直接通用追番
              </button>
              <button class="btn btn-ghost btn-xs" @click="showGroupModal = false">{{ $t('search.modal.cancel') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
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
