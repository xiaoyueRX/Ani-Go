<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { listEntry, listView, saveListView, type ListFilter, type ListSort } from '../utils/subscriptionListHistory'
import request from '../utils/request'
import {
  Search, Plus, AlertTriangle,
  X, LayoutGrid, List, RefreshCw,
  RotateCcw, Trash2, Play, Pause, Check, Sparkles, Layers, History, Image,
  SlidersHorizontal
} from 'lucide-vue-next'
import SubscriptionCard from '../components/SubscriptionCard.vue'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number
  season: number
  bangumi_id: string
  subgroup_name: string
  cover_url: string
  anime_type: string
  total_episodes: number
  current_episodes: number
  stalled_episodes: number
  enabled: boolean
  completed: boolean
  created_at: string
  updated_at: string
  deleted_at?: string
}

const router = useRouter()
const subs = ref<Subscription[]>([])
const loading = ref(true)
const error = ref('')
const deletingId = ref<number | null>(null)
const session = localStorage.getItem('token')
const previousEntry = window.history?.state?.subscriptionListEntry
const entry = listEntry(window.history)
const saved = listView(entry, session)
const filterText = ref(saved?.search ?? '')
const filterType = ref<ListFilter>(saved?.filter ?? 'all')
const sortBy = ref<ListSort>(saved?.sort ?? 'created_at')

// 网格 / 列表视图模式
const viewMode = ref<'grid' | 'list'>(
  localStorage.getItem('anigo_subs_view_mode') === 'list' ? 'list' : 'grid'
)
function setViewMode(mode: 'grid' | 'list') {
  viewMode.value = mode
  localStorage.setItem('anigo_subs_view_mode', mode)
}

// 可折叠 AI 一句话追番抽屉
const aiDrawerOpen = ref(false)
const aiInput = ref('')
const aiParsing = ref(false)
const aiResult = ref<any | null>(null)
const aiSubscribing = ref(false)

function openDetail(id: number) {
  router.push({ name: 'subscription-detail', params: { id }, state: { subscriptionListOrigin: entry ?? null } })
}

onBeforeRouteLeave(to => {
  if (!entry || !session || localStorage.getItem('token') !== session) return
  const id = to.name === 'subscription-detail' ? Number(to.params.id) : 0
  const card = id ? document.querySelector(`[data-subscription-id="${id}"]`) : null
  saveListView(entry, session, {
    search: filterText.value, filter: filterType.value, sort: sortBy.value, y: window.scrollY,
    card: card ? { id, top: card.getBoundingClientRect().top } : undefined,
  })
})

async function restoreScroll(ready: Promise<unknown>) {
  await ready
  await nextTick()
  if (window.history.state?.subscriptionListEntry !== entry || localStorage.getItem('token') !== session) return
  if (saved && (error.value || filterText.value !== saved.search || filterType.value !== saved.filter || sortBy.value !== saved.sort)) return
  window.scrollTo({ top: saved?.y ?? 0, behavior: 'instant' })
  if (!saved?.card) return
  const bounds = document.querySelector(`[data-subscription-id="${saved.card.id}"]`)?.getBoundingClientRect()
  if (bounds && (bounds.bottom <= 0 || bounds.top >= window.innerHeight)) {
    window.scrollBy({ top: bounds.top - saved.card.top, behavior: 'instant' })
  }
}

const deletedSubs = ref<Subscription[]>([])
const deletedLoading = ref(false)
const deletedSubsCount = computed(() => deletedSubs.value.length)

const activeSubsCount = computed(() => subs.value.filter(s => s.enabled && !s.completed).length)
const completedSubsCount = computed(() => subs.value.filter(s => s.completed).length)
const stalledSubsCount = computed(() => subs.value.filter(s => s.stalled_episodes > 0).length)

const statusTabs = computed(() => [
  { key: 'all' as ListFilter, label: locale.value === 'zh' ? '全部' : t('subs.stats.all'), count: subs.value.length },
  { key: 'active' as ListFilter, label: locale.value === 'zh' ? '连载中' : t('subs.stats.active'), count: activeSubsCount.value },
  { key: 'completed' as ListFilter, label: locale.value === 'zh' ? '已完结' : t('subs.stats.completed'), count: completedSubsCount.value },
  { key: 'stalled' as ListFilter, label: locale.value === 'zh' ? '异常' : t('subs.stats.stalled'), count: stalledSubsCount.value },
  { key: 'recycle' as ListFilter, label: locale.value === 'zh' ? '回收站' : t('subs.filter.recycle'), count: deletedSubsCount.value },
])

const batchDeleteMode = ref(false)
const batchDeleteSelected = ref<Set<number>>(new Set())

const undoBarVisible = ref(false)
const remainingSeconds = ref(15)
const UNDO_TIMEOUT_SECONDS = 15
let undoInterval: ReturnType<typeof setInterval> | null = null

const deleteModalOpen = ref(false)
const deletingSub = ref<Subscription | null>(null)
const deleteFilesChecked = ref(true)

const batchDeleteModalOpen = ref(false)
const batchDeleteFilesChecked = ref(true)

const pendingDeletes = ref(new Map<number, boolean>())
const undoDeleteFiles = computed(() => [...pendingDeletes.value.values()].some(Boolean))
const undoCount = computed(() => pendingDeletes.value.size)
let deleteTimer: ReturnType<typeof setTimeout> | undefined

function enterBatchDeleteMode() {
  batchDeleteMode.value = true
  batchDeleteSelected.value = new Set()
}

function exitBatchDeleteMode() {
  batchDeleteMode.value = false
  batchDeleteSelected.value = new Set()
}

function toggleBatchSelect(id: number) {
  const newSet = new Set(batchDeleteSelected.value)
  if (newSet.has(id)) newSet.delete(id)
  else newSet.add(id)
  batchDeleteSelected.value = newSet
}

function openBatchDeleteModal() {
  if (batchDeleteSelected.value.size === 0) return
  batchDeleteFilesChecked.value = true
  batchDeleteModalOpen.value = true
}

function confirmBatchDeleteWithFiles() {
  batchDeleteModalOpen.value = false
  const ids = Array.from(batchDeleteSelected.value)
  scheduleDelete(ids, batchDeleteFilesChecked.value)
  exitBatchDeleteMode()
}

function scheduleDelete(ids: number[], deleteFiles: boolean) {
  for (const id of ids) pendingDeletes.value.set(id, deleteFiles)
  clearTimeout(deleteTimer)
  clearUndoTimer()
  undoBarVisible.value = true
  startUndoCountdown()
  deleteTimer = setTimeout(executePendingDeletes, UNDO_TIMEOUT_SECONDS * 1000)
}

async function executePendingDeletes() {
  const pending = pendingDeletes.value
  undoDelete()
  for (const deleteFiles of [false, true]) {
    const ids = [...pending].filter(([, files]) => files === deleteFiles).map(([id]) => id)
    for (let start = 0; start < ids.length; start += 100) {
      const batch = ids.slice(start, start + 100)
      try {
        await request.post('/subscriptions/batch-delete', { ids: batch, delete_files: deleteFiles })
        subs.value = subs.value.filter(s => !batch.includes(s.id))
      } catch (e: any) {
        error.value = e.response?.data?.error || '批量删除失败，请检查订阅列表后重试'
        window.showToast?.(error.value, 'error')
      }
    }
  }
}

function undoDelete() {
  clearTimeout(deleteTimer)
  clearUndoTimer()
  pendingDeletes.value = new Map()
  undoBarVisible.value = false
}

function isPending(id: number): boolean {
  return pendingDeletes.value.has(id)
}

function startUndoCountdown() {
  remainingSeconds.value = UNDO_TIMEOUT_SECONDS
  undoInterval = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      clearUndoTimer()
      undoBarVisible.value = false
    }
  }, 1000)
}

function clearUndoTimer() {
  if (undoInterval) {
    clearInterval(undoInterval)
    undoInterval = null
  }
}

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.startsWith('/api/') || url.startsWith('data:') || url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const filteredSubs = computed(() => {
  let list = filterType.value === 'recycle' ? [...deletedSubs.value] : [...subs.value]
  if (filterType.value === 'active') list = list.filter(s => s.enabled && !s.completed)
  else if (filterType.value === 'completed') list = list.filter(s => s.completed)
  else if (filterType.value === 'stalled') list = list.filter(s => s.stalled_episodes > 0)

  const q = filterText.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      s.title_cn.toLowerCase().includes(q) ||
      (s.title_en && s.title_en.toLowerCase().includes(q)) ||
      (s.subgroup_name && s.subgroup_name.toLowerCase().includes(q)) ||
      (s.year && String(s.year).includes(q))
    )
  }

  list.sort((a, b) => {
    if (sortBy.value === 'title') return a.title_cn.localeCompare(b.title_cn)
    if (sortBy.value === 'progress') {
      const pa = a.total_episodes ? a.current_episodes / a.total_episodes : 0
      const pb = b.total_episodes ? b.current_episodes / b.total_episodes : 0
      if (pa !== pb) return pb - pa
      return b.current_episodes - a.current_episodes
    }
    if (sortBy.value === 'year') return b.year - a.year
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  })

  return list
})

const selectableVisibleSubs = computed(() => filterType.value === 'recycle'
  ? []
  : filteredSubs.value.filter(sub => !isPending(sub.id)))
const allVisibleSelected = computed(() => selectableVisibleSubs.value.length > 0
  && selectableVisibleSubs.value.every(sub => batchDeleteSelected.value.has(sub.id)))

function toggleSelectAllBatch() {
  const deselect = allVisibleSelected.value
  for (const { id } of selectableVisibleSubs.value) {
    if (deselect) batchDeleteSelected.value.delete(id)
    else batchDeleteSelected.value.add(id)
  }
}

let listClosed = false

function coalesceRefresh(load: () => Promise<void>) {
  let pending: Promise<void> | undefined
  let requested = false
  return () => {
    if (pending) {
      requested = true
      return pending
    }
    pending = (async () => {
      try {
        do {
          requested = false
          await load()
        } while (requested && !listClosed && !document.hidden)
      } finally {
        pending = undefined
      }
    })()
    return pending
  }
}

const fetchSubscriptions = coalesceRefresh(async () => {
  error.value = ''
  try {
    const { data } = await request.get('/subscriptions')
    subs.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.load')
  } finally {
    loading.value = false
  }
})

const fetchDeletedSubscriptions = coalesceRefresh(async () => {
  deletedLoading.value = true
  try {
    const { data } = await request.get('/subscriptions?deleted=true')
    deletedSubs.value = data || []
  } catch (e: any) {
    console.error('Failed to load recycle bin:', e)
  } finally {
    deletedLoading.value = false
  }
})

async function restoreSubscription(ids: number[]) {
  if (ids.length === 0) return
  try {
    await request.post('/subscriptions/batch-restore', { ids })
    window.showToast(t('subs.recycle.restoreSuccess', { count: ids.length }), 'success')
    await Promise.all([fetchSubscriptions(), fetchDeletedSubscriptions()])
  } catch (e: any) {
    window.showToast(e.response?.data?.error || t('subs.error.op'), 'error')
  }
}

watch(filterType, (newVal) => {
  if (newVal === 'recycle') {
    fetchDeletedSubscriptions()
  }
})

async function parseAiTask() {
  if (!aiInput.value.trim()) return
  aiParsing.value = true
  aiResult.value = null
  try {
    const { data } = await request.post('/parse', { input: aiInput.value.trim() })
    aiResult.value = data
  } catch (e: any) {
    window.showToast(e.response?.data?.error || '解析失败，请检查输入', 'error')
  } finally {
    aiParsing.value = false
  }
}

async function applyAiSubscribe() {
  if (!aiResult.value || !aiResult.value.title) return
  aiSubscribing.value = true
  try {
    const payload: any = {
      title_cn: aiResult.value.title,
      season: aiResult.value.season || 1,
      source_name: 'Mikan',
    }
    if (aiResult.value.subgroup_pref) {
      payload.subgroup_name = aiResult.value.subgroup_pref
      payload.allowed_subgroups = [aiResult.value.subgroup_pref]
    }
    await request.post('/subscriptions', payload)
    window.showToast(t('subs.quickSubscribeSuccess'), 'success')
    aiResult.value = null
    aiInput.value = ''
    await fetchSubscriptions()
  } catch (e: any) {
    window.showToast(e.response?.data?.error || t('subs.error.op'), 'error')
  } finally {
    aiSubscribing.value = false
  }
}

function adjustAiRules() {
  if (!aiResult.value) return
  router.push({
    path: '/subscriptions/new',
    query: {
      title: aiResult.value.title || '',
      season: String(aiResult.value.season || 1),
      subgroup: aiResult.value.subgroup_pref || undefined,
    },
  })
}

async function toggleEnabled(sub: Subscription) {
  try {
    await request.put(`/subscriptions/${sub.id}`, { enabled: !sub.enabled })
    sub.enabled = !sub.enabled
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.operation')
  }
}

function handleDelete(sub: Subscription) {
  deletingSub.value = sub
  deleteFilesChecked.value = true
  deleteModalOpen.value = true
}

function confirmSingleDelete() {
  const sub = deletingSub.value
  if (!sub) return
  deleteModalOpen.value = false
  scheduleDelete([sub.id], deleteFilesChecked.value)
}

declare global {
  interface Window {
    showToast: (message: string, type?: 'success' | 'error' | 'info') => void
  }
}

async function triggerSupplement(sub: any) {
  try {
    await request.post(`/subscriptions/${sub.id}/trigger-supplement`)
    window.showToast(t('subs.supplementTriggered'))
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subs.error.supplement')
    window.showToast(error.value, 'error')
  }
}

async function triggerSupplementAll() {
  try {
    const { data } = await request.post('/subscriptions/supplement-all')
    window.showToast(data.message || t('subs.supplementAllTriggered'), 'success')
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subs.error.supplementAll')
    window.showToast(error.value, 'error')
  }
}

const mergingCollections = ref(false)

async function triggerMergeCollections() {
  mergingCollections.value = true
  try {
    const { data } = await request.post('/subscriptions/merge-collections', { apply: true })
    const count = data?.data?.total_applied ?? 0
    if (count > 0) {
      window.showToast(t('subs.mergeCollectionsSuccess', { count }), 'success')
      await fetchSubscriptions()
    } else {
      window.showToast(data?.message || t('subs.mergeCollectionsNoChanges'), 'info')
    }
  } catch (e: any) {
    const msg = e.response?.data?.error || t('subs.error.op')
    window.showToast(msg, 'error')
  } finally {
    mergingCollections.value = false
  }
}

let refreshTimer: ReturnType<typeof setInterval>
const taskCenter = ref({
  active: false,
  total: 0,
  completed: 0,
  logs: [] as { id: number; title: string; message: string }[],
  minimized: false
})

const addLog = (title: string, message: string) => {
  taskCenter.value.logs.unshift({ id: Date.now() + Math.random(), title, message })
  if (taskCenter.value.logs.length > 50) taskCenter.value.logs.pop()
}
let eventSource: EventSource | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | undefined

function closeEventStream() {
  clearTimeout(reconnectTimer)
  if (eventSource) {
    eventSource.onerror = null
    eventSource.close()
    eventSource = null
  }
}

function setupEventStream() {
  const token = localStorage.getItem('token')
  const apiPath = window.location.origin + '/api/events/stream' + (token ? '?token=' + token : '')
  eventSource = new EventSource(apiPath)

  eventSource.onerror = () => {
    closeEventStream()
    reconnectTimer = setTimeout(setupEventStream, 5000)
  }
  eventSource.onmessage = (event) => {
    const ev = JSON.parse(event.data)
    const payload = ev.Payload || {}

    if (ev.Type === 'supplement.triggered') {
      taskCenter.value.active = true
      taskCenter.value.total++
      addLog(payload.title, '开始扫描补全...')
    } else if (ev.Type === 'supplement.completed') {
      taskCenter.value.completed++
      if (payload.error) {
        addLog(payload.title, '补全扫描出错：' + payload.error)
      } else if (!payload.new_count) {
        addLog(payload.title, '补全扫描完成，没有新增下载')
      } else {
        addLog(payload.title, '补全扫描完成，新增 ' + payload.new_count + ' 个下载')
      }
      if (payload.new_count > 0 && !document.hidden) void fetchSubscriptions()
      setTimeout(() => {
        if (taskCenter.value.completed >= taskCenter.value.total) {
          taskCenter.value.active = false
          taskCenter.value.total = 0
          taskCenter.value.completed = 0
        }
      }, 10000)
    } else if (ev.Type === 'supplement.progress') {
      if (payload.message && (payload.message.includes('已添加下载') || payload.message.includes('获取到'))) {
        addLog(payload.title, payload.message)
      }
      if (payload.message && payload.message.includes('已添加下载') && !document.hidden) void fetchSubscriptions()
    }
  }
}

function refreshVisible(includeDeleted = false) {
  if (document.hidden) return
  void fetchSubscriptions()
  if (includeDeleted || filterType.value === 'recycle') void fetchDeletedSubscriptions()
}

const onVisibilityChange = () => refreshVisible(true)

onMounted(() => {
  const active = fetchSubscriptions()
  const deleted = fetchDeletedSubscriptions()
  if (previousEntry) {
    void restoreScroll(filterType.value === 'recycle' ? Promise.all([active, deleted]) : active)
  } else {
    window.scrollTo?.({ top: 0, behavior: 'instant' })
  }
  setupEventStream()
  document.addEventListener('visibilitychange', onVisibilityChange)
  refreshTimer = setInterval(refreshVisible, 30000)
})
onUnmounted(() => {
  listClosed = true
  document.removeEventListener('visibilitychange', onVisibilityChange)
  clearInterval(refreshTimer)
  closeEventStream()
  clearUndoTimer()
  if (pendingDeletes.value.size) void executePendingDeletes()
})
</script>

<template>
  <div class="space-y-5 pb-16">
    <!-- Compact 2-Tier Studio Header -->
    <section class="v2-panel p-4 sm:p-5 space-y-4">
      <!-- Row 1: 标题 + 状态筛选胶囊带计数 + 视图切换/AI追番/批量删除/+新增 -->
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-3.5">
        <div class="flex flex-wrap items-center gap-3 min-w-0">
          <div class="mr-1">
            <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">
              {{ $t('subs.title') }}
            </h1>
          </div>

          <div class="flex items-center gap-1.5 flex-wrap">
            <button
              v-for="tab in statusTabs"
              :key="tab.key"
              type="button"
              @click="filterType = tab.key"
              class="h-7 px-3 rounded-full text-xs font-semibold flex items-center gap-1.5 border transition-all cursor-pointer"
              :class="filterType === tab.key
                ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)] shadow-xs'
                : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)] hover:text-[var(--v2-ink)]'"
            >
              <span>{{ tab.label }}</span>
              <span
                class="font-mono text-[10px] px-1.5 py-0.2 rounded-full tabular-nums"
                :class="filterType === tab.key ? 'bg-white/20 text-white' : 'bg-[var(--v2-surface)] text-[var(--v2-ink-subtle)]'"
              >
                {{ tab.count }}
              </span>
            </button>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <template v-if="batchDeleteMode && filterType !== 'recycle'">
            <button type="button" class="v2-btn v2-btn-sm" :disabled="selectableVisibleSubs.length === 0" @click="toggleSelectAllBatch">
              {{ allVisibleSelected ? $t('subs.unselectAll') : $t('subs.selectAll') }}
            </button>
            <span class="text-xs font-medium text-[var(--v2-ink-muted)]">{{ $t('subs.selectedCount', { count: batchDeleteSelected.size }) }}</span>
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="exitBatchDeleteMode">
              {{ $t('common.cancel') }}
            </button>
            <button type="button" class="v2-btn v2-btn-sm bg-red-500 border-red-500 text-white hover:bg-red-600" :disabled="batchDeleteSelected.size === 0" @click="openBatchDeleteModal">
              <Trash2 :size="13" />
              <span>{{ $t('common.delete') }} ({{ batchDeleteSelected.size }})</span>
            </button>
          </template>

          <template v-else>
            <!-- 网格 / 列表切换 -->
            <div class="inline-flex items-center p-0.5 rounded-lg bg-[var(--v2-surface-2)] border border-[var(--v2-border)]">
              <button
                type="button"
                class="h-7 px-2.5 rounded-md text-xs font-semibold flex items-center gap-1 transition-colors cursor-pointer"
                :class="viewMode === 'grid' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)] shadow-xs' : 'text-[var(--v2-ink-muted)]'"
                @click="setViewMode('grid')"
                :title="locale === 'zh' ? '海报网格' : 'Poster Grid'"
              >
                <LayoutGrid :size="13" />
                <span class="hidden sm:inline">{{ locale === 'zh' ? '网格' : 'Grid' }}</span>
              </button>
              <button
                type="button"
                class="h-7 px-2.5 rounded-md text-xs font-semibold flex items-center gap-1 transition-colors cursor-pointer"
                :class="viewMode === 'list' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)] shadow-xs' : 'text-[var(--v2-ink-muted)]'"
                @click="setViewMode('list')"
                :title="locale === 'zh' ? '工作室列表' : 'Studio List'"
              >
                <List :size="13" />
                <span class="hidden sm:inline">{{ locale === 'zh' ? '列表' : 'List' }}</span>
              </button>
            </div>

            <button
              type="button"
              class="v2-btn v2-btn-sm"
              :class="aiDrawerOpen ? 'border-[var(--v2-accent)] text-[var(--v2-accent)]' : ''"
              @click="aiDrawerOpen = !aiDrawerOpen"
            >
              <Sparkles :size="13" class="text-[var(--v2-accent)]" />
              <span>{{ locale === 'zh' ? 'AI 一句话追番' : 'AI Track' }}</span>
            </button>

            <button
              type="button"
              class="v2-btn v2-btn-sm hidden sm:inline-flex"
              @click="triggerSupplementAll"
              :disabled="loading"
              :title="$t('subs.supplementAll')"
            >
              <RotateCcw :size="13" />
              <span>{{ $t('subs.supplementAll') }}</span>
            </button>

            <button
              type="button"
              class="v2-btn v2-btn-sm hidden md:inline-flex"
              @click="triggerMergeCollections"
              :disabled="loading || mergingCollections"
              :title="$t('subs.mergeCollectionsDesc')"
            >
              <Layers :size="13" :class="{ 'animate-spin': mergingCollections }" />
              <span>{{ $t('subs.mergeCollections') }}</span>
            </button>

            <button
              v-if="filterType !== 'recycle'"
              type="button"
              class="v2-btn v2-btn-sm"
              @click="enterBatchDeleteMode"
            >
              <Trash2 :size="13" />
              <span>{{ $t('subs.batchDelete') }}</span>
            </button>

            <button
              type="button"
              class="v2-btn v2-btn-primary v2-btn-sm"
              @click="router.push('/subscriptions/new')"
            >
              <Plus :size="14" />
              <span>{{ locale === 'zh' ? '新增订阅' : $t('subs.new') }}</span>
            </button>
          </template>
        </div>
      </div>

      <!-- Row 2: 搜索输入框 + 排序选择器 -->
      <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 pt-3 border-t border-[var(--v2-border)]">
        <div class="relative flex-1">
          <Search :size="15" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
          <input
            v-model="filterText"
            type="text"
            :placeholder="locale === 'zh' ? '搜索追番标题、字幕组或年份...' : $t('subs.searchPlaceholder')"
            class="v2-input pl-9 pr-8"
          />
          <button
            v-if="filterText"
            type="button"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)] hover:text-[var(--v2-ink)]"
            @click="filterText = ''"
          >
            <X :size="14" />
          </button>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <select v-model="sortBy" class="v2-input w-full sm:w-44 text-xs font-medium cursor-pointer">
            <option value="created_at">{{ $t('subs.sort.createdAt') }}</option>
            <option value="title">{{ $t('subs.sort.title') }}</option>
            <option value="progress">{{ $t('subs.sort.progress') }}</option>
            <option value="year">{{ $t('subs.sort.year') }}</option>
          </select>
          <span class="hidden lg:inline text-xs text-[var(--v2-ink-subtle)] font-mono tabular-nums px-1">
            {{ filteredSubs.length }} / {{ subs.length }}
          </span>
        </div>
      </div>

      <!-- 折叠的 AI 一句话追番抽屉 -->
      <Transition name="fade">
        <div v-if="aiDrawerOpen" class="v2-panel-muted p-3.5 space-y-3 border border-[var(--v2-accent)]/25">
          <div class="flex flex-col sm:flex-row gap-2">
            <input
              v-model="aiInput"
              type="text"
              :placeholder="$t('subs.quickSubscribePlaceholder')"
              class="v2-input flex-1"
              @keyup.enter="parseAiTask"
            />
            <button
              type="button"
              class="v2-btn v2-btn-primary shrink-0"
              :disabled="aiParsing || !aiInput.trim()"
              @click="parseAiTask"
            >
              <RefreshCw v-if="aiParsing" :size="14" class="animate-spin" />
              <Sparkles v-else :size="14" />
              <span>{{ aiParsing ? $t('subs.quickSubscribeParsing') : $t('subs.quickSubscribeBtn') }}</span>
            </button>
          </div>
          <div v-if="aiResult" class="v2-panel p-3 flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap items-center gap-2 text-xs">
              <span class="px-2 py-0.5 rounded-full bg-[var(--v2-accent)] text-white font-semibold">
                {{ aiResult.action === 'subscribe' ? '追番' : aiResult.action }}
              </span>
              <span class="font-semibold text-[var(--v2-ink)]">{{ aiResult.title }}</span>
              <span class="v2-pill font-mono">S{{ aiResult.season || 1 }}</span>
              <span v-if="aiResult.subgroup_pref" class="v2-pill">{{ aiResult.subgroup_pref }}</span>
            </div>
            <div class="flex flex-wrap items-center gap-1.5">
              <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" :disabled="aiSubscribing" @click="applyAiSubscribe">
                <Check :size="12" />
                <span>{{ $t('subs.oneClickSubscribe') }}</span>
              </button>
              <button type="button" class="v2-btn v2-btn-sm" @click="adjustAiRules">
                <SlidersHorizontal :size="12" />
                <span>{{ $t('subs.adjustRules') }}</span>
              </button>
              <button type="button" class="v2-btn v2-btn-sm" @click="router.push(`/search?q=${encodeURIComponent(aiResult.title)}`)">
                <Search :size="12" />
                <span>{{ $t('subs.goToSearch') }}</span>
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </section>

    <!-- 错误提示 -->
    <div v-if="error" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 p-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2.5 text-xs font-medium">
        <AlertTriangle :size="18" class="shrink-0" />
        <span>{{ error }}</span>
      </div>
      <button type="button" class="p-1 rounded hover:bg-red-500/20" @click="error = ''">
        <X :size="14" />
      </button>
    </div>

    <!-- 加载骨架 -->
    <div v-if="loading" class="grid gap-4 sm:gap-5 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 animate-pulse">
      <div v-for="i in 12" :key="i" class="aspect-[2/3] bg-[var(--v2-surface-2)] rounded-xl"></div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="filteredSubs.length === 0" class="v2-panel py-16 px-6 text-center space-y-3">
      <div class="w-12 h-12 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] flex items-center justify-center mx-auto">
        <Trash2 v-if="filterType === 'recycle'" :size="22" />
        <LayoutGrid v-else :size="22" />
      </div>
      <h3 class="text-base font-bold text-[var(--v2-ink)]">
        {{ filterType === 'recycle' ? $t('subs.recycle.emptyTitle') : (subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs')) }}
      </h3>
      <p class="text-xs text-[var(--v2-ink-muted)] max-w-md mx-auto">
        {{ filterType === 'recycle' ? $t('subs.recycle.emptyDesc') : (subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc')) }}
      </p>
      <div class="pt-2">
        <button v-if="filterType === 'recycle'" type="button" class="v2-btn v2-btn-sm" @click="filterType = 'all'">
          <RotateCcw :size="13" />
          <span>返回活跃订阅</span>
        </button>
        <button v-else-if="subs.length === 0" type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="router.push('/search')">
          <Search :size="13" />
          <span>{{ $t('subs.empty.discover') }}</span>
        </button>
        <button v-else type="button" class="v2-btn v2-btn-sm" @click="filterText = ''; filterType = 'all'">
          <RefreshCw :size="13" />
          <span>{{ $t('subs.empty.clear') }}</span>
        </button>
      </div>
    </div>

    <!-- 回收站视图 -->
    <div v-else-if="filterType === 'recycle'" class="space-y-4">
      <div class="v2-panel p-4 flex items-center justify-between gap-4">
        <div>
          <h3 class="font-bold text-sm text-[var(--v2-ink)]">{{ $t('subs.recycle.title') }}</h3>
          <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('subs.recycle.desc') }}</p>
        </div>
        <button
          v-if="filteredSubs.length > 0"
          type="button"
          class="v2-btn v2-btn-primary v2-btn-sm"
          @click="restoreSubscription(filteredSubs.map(s => s.id))"
        >
          <RotateCcw :size="13" />
          <span>{{ $t('subs.recycle.batchRestore') }} ({{ filteredSubs.length }})</span>
        </button>
      </div>

      <div class="grid gap-3 grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
        <div
          v-for="sub in filteredSubs"
          :key="sub.id"
          class="v2-panel p-4 flex items-center justify-between gap-4 hover:border-[var(--v2-accent)]/40 transition-colors"
        >
          <div class="flex items-center gap-3 min-w-0">
            <img
              v-if="sub.cover_url"
              :src="proxyImage(sub.cover_url)"
              class="w-12 h-16 object-cover rounded-lg shrink-0 bg-[var(--v2-surface-2)]"
            />
            <div v-else class="w-12 h-16 bg-[var(--v2-surface-2)] rounded-lg shrink-0 flex items-center justify-center font-mono font-bold text-xs text-[var(--v2-ink-subtle)]">
              S{{ sub.season }}
            </div>
            <div class="min-w-0 space-y-1">
              <h4 class="font-semibold text-sm truncate text-[var(--v2-ink)]" :title="sub.title_cn">{{ sub.title_cn }}</h4>
              <p class="text-xs text-[var(--v2-ink-muted)] truncate">
                {{ sub.subgroup_name || '通用' }} · S{{ sub.season }} · {{ sub.total_episodes ? sub.total_episodes + '集' : '连载' }}
              </p>
            </div>
          </div>
          <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="restoreSubscription([sub.id])">
            <RotateCcw :size="13" />
            <span>{{ $t('subs.recycle.restore') }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 工作室列表视图 -->
    <div v-else-if="viewMode === 'list'" class="v2-panel overflow-hidden divide-y divide-[var(--v2-border)]">
      <div
        v-for="sub in filteredSubs"
        :key="sub.id"
        :data-subscription-id="sub.id"
        class="p-3.5 sm:px-5 flex items-center justify-between gap-4 hover:bg-[var(--v2-surface-2)]/65 transition-colors cursor-pointer"
        :class="{ 'opacity-65': !sub.enabled, 'opacity-40 pointer-events-none': isPending(sub.id) }"
        @click="batchDeleteMode ? (!isPending(sub.id) && toggleBatchSelect(sub.id)) : openDetail(sub.id)"
      >
        <div class="flex items-center gap-3.5 min-w-0 flex-1">
          <input
            v-if="batchDeleteMode && !isPending(sub.id)"
            type="checkbox"
            :checked="batchDeleteSelected.has(sub.id)"
            class="checkbox checkbox-primary checkbox-sm shrink-0"
            @click.stop
            @change="toggleBatchSelect(sub.id)"
          />
          <div class="w-11 h-16 rounded-lg overflow-hidden bg-[var(--v2-surface-2)] border border-[var(--v2-border)] shrink-0 flex items-center justify-center">
            <img
              v-if="sub.cover_url"
              :src="proxyImage(sub.cover_url)"
              :alt="sub.title_cn"
              class="w-full h-full object-cover"
              loading="lazy"
            />
            <Image v-else :size="18" class="text-[var(--v2-ink-subtle)]" />
          </div>
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex items-center gap-2 flex-wrap">
              <a
                :href="`/subscriptions/${sub.id}`"
                class="font-semibold text-sm text-[var(--v2-ink)] hover:text-[var(--v2-accent)] truncate"
                @click.stop.prevent="openDetail(sub.id)"
              >
                {{ sub.title_cn }}
              </a>
              <span v-if="sub.completed" class="text-[10px] font-bold px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400">已完结</span>
              <span v-else-if="!sub.enabled" class="text-[10px] font-bold px-2 py-0.5 rounded-full bg-zinc-500/20 text-[var(--v2-ink-muted)]">已暂停</span>
              <span v-if="sub.stalled_episodes > 0" class="text-[10px] font-bold px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-400">超时 {{ sub.stalled_episodes }}</span>
            </div>
            <div class="flex items-center gap-2 text-xs text-[var(--v2-ink-muted)] flex-wrap">
              <span class="font-medium text-[var(--v2-accent)]">{{ sub.subgroup_name || $t('card.generic') }}</span>
              <span>•</span>
              <span class="font-mono">S{{ sub.season || 1 }}</span>
              <span v-if="sub.year">•</span>
              <span v-if="sub.year" class="font-mono">{{ sub.year }}</span>
              <span v-if="sub.anime_type" class="v2-pill text-[10px] h-5">{{ sub.anime_type }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-4 shrink-0" @click.stop>
          <div class="hidden sm:block w-32 space-y-1">
            <div class="flex items-center justify-between text-[11px] font-mono tabular-nums text-[var(--v2-ink-muted)]">
              <span>{{ sub.current_episodes }} / {{ sub.total_episodes || '?' }}</span>
              <span v-if="sub.total_episodes > 0">{{ Math.round(Math.min(100, (sub.current_episodes / sub.total_episodes) * 100)) }}%</span>
            </div>
            <div class="h-1.5 w-full bg-[var(--v2-surface-3)] rounded-full overflow-hidden">
              <div
                class="h-full rounded-full transition-all"
                :class="sub.completed ? 'bg-emerald-500' : 'bg-[var(--v2-accent)]'"
                :style="{ width: `${sub.total_episodes > 0 ? Math.min(100, (sub.current_episodes / sub.total_episodes) * 100) : 0}%` }"
              ></div>
            </div>
          </div>

          <div class="flex items-center gap-1.5">
            <button type="button" class="v2-btn v2-btn-sm w-8 px-0" :title="$t('card.supplement')" @click="triggerSupplement(sub)">
              <History :size="13" />
            </button>
            <button type="button" class="v2-btn v2-btn-sm w-8 px-0" :title="$t(sub.enabled ? 'detail.actions.suspend' : 'detail.actions.resume')" @click="toggleEnabled(sub)">
              <component :is="sub.enabled ? Pause : Play" :size="13" />
            </button>
            <button type="button" class="v2-btn v2-btn-sm w-8 px-0 hover:text-red-400 hover:border-red-500/40" :title="$t('common.delete')" @click="handleDelete(sub)">
              <Trash2 :size="13" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 海报网格视图 -->
    <div v-else class="grid gap-4 sm:gap-5 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
      <SubscriptionCard
        v-for="sub in filteredSubs"
        :key="sub.id"
        :sub="sub"
        :data-subscription-id="sub.id"
        :deleting="deletingId === sub.id"
        :pending="isPending(sub.id)"
        :batch-delete-mode="batchDeleteMode && !isPending(sub.id)"
        :batch-selected="batchDeleteSelected.has(sub.id)"
        @click="batchDeleteMode ? (!isPending(sub.id) && toggleBatchSelect(sub.id)) : openDetail(sub.id)"
        @toggle="toggleEnabled(sub)"
        @delete="isPending(sub.id) ? null : handleDelete(sub)"
        @supplement="triggerSupplement(sub)"
      />
    </div>

    <!-- 撤回浮条 -->
    <Transition name="fade">
      <div v-if="undoBarVisible" class="fixed bottom-16 lg:bottom-5 left-0 right-0 z-50 px-4 pointer-events-none">
        <div class="max-w-lg mx-auto v2-panel border border-[var(--v2-border-strong)] shadow-2xl px-5 py-3.5 flex items-center gap-4 pointer-events-auto">
          <span class="text-xs sm:text-sm font-semibold flex-1 text-[var(--v2-ink)]">
            {{ $t('subs.undo.msg', { seconds: remainingSeconds, count: undoCount, files: undoDeleteFiles ? $t('subs.undo.withFiles') : '' }) }}
          </span>
          <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="undoDelete">{{ $t('subs.undo.btn') }}</button>
        </div>
      </div>
    </Transition>

    <!-- 单条删除确认 Modal -->
    <dialog v-if="deleteModalOpen" class="modal modal-open" @click.self="deleteModalOpen = false">
      <div class="modal-box">
        <h3 class="text-base font-bold mb-2">{{ $t('subs.modal.deleteTitle', { title: deletingSub?.title_cn }) }}</h3>
        <p class="text-xs text-[var(--v2-ink-muted)] mb-4">{{ $t('subs.modal.deleteDesc') }}</p>
        <label class="flex items-center gap-2.5 p-3 v2-panel-muted cursor-pointer mb-5">
          <input type="checkbox" v-model="deleteFilesChecked" class="checkbox checkbox-primary checkbox-sm" />
          <span class="text-xs font-semibold">{{ $t('subs.modal.deleteFiles') }}</span>
        </label>
        <div class="flex gap-2 justify-end">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="deleteModalOpen = false">{{ $t('common.cancel') }}</button>
          <button type="button" class="v2-btn v2-btn-sm bg-red-500 border-red-500 text-white" @click="confirmSingleDelete">{{ $t('subs.modal.confirmDelete') }}</button>
        </div>
      </div>
      <div class="modal-backdrop" @click="deleteModalOpen = false"></div>
    </dialog>

    <!-- 批量删除确认 Modal -->
    <dialog v-if="batchDeleteModalOpen" class="modal modal-open" @click.self="batchDeleteModalOpen = false">
      <div class="modal-box">
        <h3 class="text-base font-bold mb-2">{{ $t('subs.modal.batchDeleteTitle', { count: batchDeleteSelected.size }) }}</h3>
        <p class="text-xs text-[var(--v2-ink-muted)] mb-4">{{ $t('subs.modal.deleteDesc') }}</p>
        <label class="flex items-center gap-2.5 p-3 v2-panel-muted cursor-pointer mb-5">
          <input type="checkbox" v-model="batchDeleteFilesChecked" class="checkbox checkbox-primary checkbox-sm" />
          <span class="text-xs font-semibold">{{ $t('subs.modal.deleteFiles') }}</span>
        </label>
        <div class="flex gap-2 justify-end">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="batchDeleteModalOpen = false">{{ $t('common.cancel') }}</button>
          <button type="button" class="v2-btn v2-btn-sm bg-red-500 border-red-500 text-white" @click="confirmBatchDeleteWithFiles">{{ $t('subs.modal.confirmDelete') }}</button>
        </div>
      </div>
      <div class="modal-backdrop" @click="batchDeleteModalOpen = false"></div>
    </dialog>

    <!-- 聚合任务中心 -->
    <div v-if="taskCenter.active" class="fixed bottom-20 right-4 z-[100] w-80 max-w-[calc(100%-2rem)] lg:bottom-6 lg:right-6">
      <div class="v2-panel border border-[var(--v2-border-strong)] shadow-2xl overflow-hidden" :class="{ 'h-12': taskCenter.minimized }">
        <div class="flex items-center justify-between px-4 py-3 bg-[var(--v2-accent)]/12 cursor-pointer" @click="taskCenter.minimized = !taskCenter.minimized">
          <div class="flex items-center gap-2.5">
            <RefreshCw :size="14" class="text-[var(--v2-accent)]" :class="{ 'animate-spin': taskCenter.completed < taskCenter.total }" />
            <span class="text-xs font-bold text-[var(--v2-accent)]">补全中 ({{ taskCenter.completed }}/{{ taskCenter.total }})</span>
          </div>
          <button type="button" class="p-1 rounded hover:bg-black/10" @click.stop="taskCenter.active = false"><X :size="13" /></button>
        </div>
        <div class="h-1 w-full bg-[var(--v2-surface-3)]">
          <div class="h-full bg-[var(--v2-accent)] transition-all" :style="{ width: (taskCenter.completed / Math.max(1, taskCenter.total)) * 100 + '%' }"></div>
        </div>
        <div v-if="!taskCenter.minimized" class="p-3 space-y-2 max-h-56 overflow-y-auto">
          <div v-for="log in taskCenter.logs.slice(0, 10)" :key="log.id" class="text-xs space-y-0.5">
            <div class="text-[10px] font-mono text-[var(--v2-ink-subtle)]">{{ log.title }}</div>
            <div class="text-[var(--v2-ink)]">{{ log.message }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
