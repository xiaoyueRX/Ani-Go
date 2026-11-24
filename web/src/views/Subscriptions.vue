<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { listEntry, listView, saveListView, type ListFilter, type ListSort } from '../utils/subscriptionListHistory'
import request from '../utils/request'
import { 
  Search, Plus, AlertTriangle, 
  X, LayoutGrid, RefreshCw,
  RotateCcw, Trash2, Play, Check, Sparkles, Layers
} from 'lucide-vue-next'
import SubscriptionCard from '../components/SubscriptionCard.vue'
import { PageHeader, StatCard, EmptyState } from '../components/ui'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number; season: number
  bangumi_id: string; subgroup_name: string
  cover_url: string; anime_type: string
  total_episodes: number; current_episodes: number
  stalled_episodes: number
  enabled: boolean; completed: boolean
  created_at: string; updated_at: string
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

// 回收站状态
const deletedSubs = ref<Subscription[]>([])
const deletedLoading = ref(false)
const deletedSubsCount = computed(() => deletedSubs.value.length)

// AI 自然语言一句话追番状态
const aiInput = ref('')
const aiParsing = ref(false)
const aiResult = ref<any | null>(null)
const aiSubscribing = ref(false)

const activeSubsCount = computed(() => subs.value.filter(s => s.enabled && !s.completed).length)
const completedSubsCount = computed(() => subs.value.filter(s => s.completed).length)
const stalledSubsCount = computed(() => subs.value.filter(s => s.stalled_episodes > 0).length)

const batchDeleteMode = ref(false)
const batchDeleteSelected = ref<Set<number>>(new Set())

const undoBarVisible = ref(false)
const remainingSeconds = ref(15)
const UNDO_TIMEOUT_SECONDS = 15
let undoInterval: ReturnType<typeof setInterval> | null = null

// 删除确认弹窗
const deleteModalOpen = ref(false)
const deletingSub = ref<Subscription | null>(null)
const deleteFilesChecked = ref(true)

const batchDeleteModalOpen = ref(false)
const batchDeleteFilesChecked = ref(true)

// 延迟删除队列
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

// 替换 confirmBatchDelete 为打开 modal
function openBatchDeleteModal() {
  if (batchDeleteSelected.value.size === 0) return
  batchDeleteFilesChecked.value = true
  batchDeleteModalOpen.value = true
}

// 确认批量删除
function confirmBatchDeleteWithFiles() {
  batchDeleteModalOpen.value = false
  const ids = Array.from(batchDeleteSelected.value)
  scheduleDelete(ids, batchDeleteFilesChecked.value)
  exitBatchDeleteMode()
}

// 调度延迟删除
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
  // 状态筛选
  if (filterType.value === 'active') list = list.filter(s => s.enabled && !s.completed)
  else if (filterType.value === 'completed') list = list.filter(s => s.completed)
  else if (filterType.value === 'stalled') list = list.filter(s => s.stalled_episodes > 0)
  // 文字搜索
  const q = filterText.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      s.title_cn.toLowerCase().includes(q) ||
      (s.title_en && s.title_en.toLowerCase().includes(q)) ||
      (s.subgroup_name && s.subgroup_name.toLowerCase().includes(q))
    )
  }

  // 排序
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

// 回收站只支持还原，等待删除的订阅也不能再次选中。
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

// 多个轮询或补全事件同时到达时，等当前请求结束后再获取一次最新状态。
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

async function toggleEnabled(sub: Subscription) {
  try {
    await request.put(`/subscriptions/${sub.id}`, { enabled: !sub.enabled })
    sub.enabled = !sub.enabled
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.operation')
  }
}

// 替换 handleDelete：打开 modal 替代 confirm
function handleDelete(sub: Subscription) {
  deletingSub.value = sub
  deleteFilesChecked.value = true
  deleteModalOpen.value = true
}

// 确认单条删除
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
// 任务中心状态
const taskCenter = ref({
  active: false,
  total: 0,
  completed: 0,
  logs: [] as {id: number, title: string, message: string}[],
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

  eventSource.onopen = () => {
    console.log('✅ SSE 已连接')
  }

  eventSource.onerror = () => {
    console.error('❌ SSE 错误')
    closeEventStream()
    reconnectTimer = setTimeout(setupEventStream, 5000)
  }
  eventSource.onmessage = (event) => {
    const ev = JSON.parse(event.data)
    const payload = ev.Payload || {}
    
    if (ev.Type === "supplement.triggered") {
      taskCenter.value.active = true
      taskCenter.value.total++
      addLog(payload.title, "开始扫描补全...")
    } else if (ev.Type === "supplement.completed") {
      taskCenter.value.completed++
      if (payload.error) {
        addLog(payload.title, "补全扫描出错：" + payload.error)
      } else if (!payload.new_count) {
        addLog(payload.title, "补全扫描完成，没有新增下载")
      } else {
        addLog(payload.title, "补全扫描完成，新增 " + payload.new_count + " 个下载")
      }
      if (payload.new_count > 0 && !document.hidden) void fetchSubscriptions()
      // 如果全部完成，10秒后自动关闭，但如果有新任务则继续显示
      setTimeout(() => {
        if (taskCenter.value.completed >= taskCenter.value.total) {
          taskCenter.value.active = false
          taskCenter.value.total = 0
          taskCenter.value.completed = 0
        }
      }, 10000)
    } else if (ev.Type === "supplement.progress") {
      // 过滤掉重复的或者太频繁的日志，只显示关键进度
      if (payload.message && (payload.message.includes("已添加下载") || payload.message.includes("获取到")) ) {
        addLog(payload.title, payload.message)
      }
      if (payload.message && payload.message.includes("已添加下载") && !document.hidden) void fetchSubscriptions()
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
  <div class="space-y-6 pb-20">
    <!-- 顶部页头与常用动作 -->
    <PageHeader :title="$t('subs.title')" :subtitle="$t('subs.subtitle')" :badge="subs.length">
      <template #actions>
        <template v-if="batchDeleteMode && filterType !== 'recycle'">
          <div class="flex items-center gap-2 flex-wrap">
            <button type="button" class="btn btn-ghost btn-sm rounded-lg font-medium" :disabled="selectableVisibleSubs.length === 0" @click="toggleSelectAllBatch">
              {{ allVisibleSelected ? $t('subs.unselectAll') : $t('subs.selectAll') }}
            </button>
            <span class="text-xs font-medium text-base-content/65 whitespace-nowrap">{{ $t('subs.selectedCount', { count: batchDeleteSelected.size }) }}</span>
            <button type="button" class="btn btn-ghost btn-sm rounded-lg" @click="exitBatchDeleteMode">
              {{ $t('common.cancel') }}
            </button>
            <button type="button" class="btn btn-error btn-sm rounded-lg gap-1.5 font-medium" :disabled="batchDeleteSelected.size === 0" @click="openBatchDeleteModal">
              <Trash2 :size="14" />
              <span>{{ $t('common.delete') }} ({{ batchDeleteSelected.size }})</span>
            </button>
          </div>
        </template>

        <template v-else>
          <button
            v-if="filterType !== 'recycle'"
            type="button"
            class="btn btn-ghost btn-sm border border-base-content/12 rounded-lg gap-1.5 text-xs font-medium whitespace-nowrap"
            @click="enterBatchDeleteMode"
          >
            <Trash2 :size="14" class="text-base-content/60" />
            <span>{{ $t('subs.batchDelete') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm border border-base-content/12 rounded-lg gap-1.5 text-xs font-medium whitespace-nowrap"
            @click="triggerSupplementAll"
            :disabled="loading"
          >
            <RotateCcw :size="14" class="text-base-content/60" />
            <span>{{ $t('subs.supplementAll') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm border border-base-content/12 rounded-lg gap-1.5 text-xs font-medium whitespace-nowrap"
            @click="triggerMergeCollections"
            :disabled="loading || mergingCollections"
            :title="$t('subs.mergeCollectionsDesc')"
          >
            <Layers :size="14" class="text-base-content/60" :class="{ 'animate-spin': mergingCollections }" />
            <span>{{ $t('subs.mergeCollections') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm border border-base-content/12 rounded-lg gap-1.5 text-xs font-medium whitespace-nowrap"
            @click="router.push('/search')"
          >
            <Search :size="14" class="text-base-content/60" />
            <span>{{ $t('subs.find') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm rounded-lg gap-1.5 px-3.5 text-xs font-semibold whitespace-nowrap"
            @click="router.push('/search')"
          >
            <Plus :size="15" />
            <span>{{ $t('subs.new') }}</span>
          </button>
        </template>
      </template>
    </PageHeader>

    <!-- AI 一句话快速追番栏 -->
    <div class="app-surface border-primary/20 p-4 rounded-xl space-y-3">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 text-primary font-semibold text-xs">
          <Sparkles :size="15" />
          <span>{{ $t('subs.quickSubscribe') }}</span>
        </div>
        <span class="text-xs text-base-content/50 hidden sm:inline">{{ $t('subs.quickSubscribeDesc') }}</span>
      </div>

      <div class="flex flex-col sm:flex-row gap-2">
        <div class="relative flex-1">
          <input
            v-model="aiInput"
            type="text"
            :placeholder="$t('subs.quickSubscribePlaceholder')"
            class="input w-full h-9 bg-base-200/70 border border-base-content/12 rounded-lg text-xs sm:text-sm pr-8"
            @keyup.enter="parseAiTask"
          />
          <button
            v-if="aiInput"
            type="button"
            class="btn btn-ghost btn-circle btn-xs absolute right-2 top-1/2 -translate-y-1/2 text-base-content/45 hover:text-base-content"
            @click="aiInput = ''; aiResult = null"
          >
            <X :size="13" />
          </button>
        </div>
        <button
          type="button"
          class="btn btn-primary h-9 min-h-0 rounded-lg gap-1.5 px-4 font-semibold text-xs shrink-0"
          :disabled="aiParsing || !aiInput.trim()"
          @click="parseAiTask"
        >
          <RefreshCw v-if="aiParsing" :size="14" class="animate-spin" />
          <Sparkles v-else :size="14" />
          <span>{{ aiParsing ? $t('subs.quickSubscribeParsing') : $t('subs.quickSubscribeBtn') }}</span>
        </button>
      </div>

      <Transition name="fade">
        <div v-if="aiResult" class="app-surface-muted p-3 rounded-lg space-y-2.5">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap items-center gap-2">
              <span class="badge badge-primary badge-sm font-semibold">
                {{ aiResult.action === 'subscribe' ? '追番' : aiResult.action }}
              </span>
              <span class="font-semibold text-sm text-base-content">{{ aiResult.title }}</span>
              <span class="badge badge-outline badge-sm font-mono">S{{ aiResult.season || 1 }}</span>
              <span v-if="aiResult.resolution" class="badge badge-ghost badge-sm font-mono">{{ aiResult.resolution }}</span>
              <span v-if="aiResult.subgroup_pref" class="badge badge-secondary badge-outline badge-sm">{{ aiResult.subgroup_pref }}</span>
              <span v-if="aiResult.confidence" class="text-xs text-base-content/55">置信度 {{ Math.round(aiResult.confidence * 100) }}%</span>
            </div>
            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="btn btn-primary btn-xs rounded-md gap-1 h-7 px-2.5 font-semibold"
                :disabled="aiSubscribing"
                @click="applyAiSubscribe"
              >
                <Check :size="12" />
                <span>{{ $t('subs.oneClickSubscribe') }}</span>
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-xs rounded-md h-7 px-2.5"
                @click="router.push(`/search?q=${encodeURIComponent(aiResult.title)}`)"
              >
                <Search :size="12" />
                <span>{{ $t('subs.goToSearch') }}</span>
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-circle btn-xs"
                @click="aiResult = null"
              >
                <X :size="13" />
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </div>

    <!-- 统计概览筛选卡片 -->
    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
      <StatCard
        clickable
        :active="filterType === 'all'"
        :label="$t('subs.stats.all')"
        :value="subs.length"
        @click="filterType = 'all'"
      >
        <template #icon><LayoutGrid :size="18" /></template>
      </StatCard>

      <StatCard
        clickable
        tone="primary"
        :active="filterType === 'active'"
        :label="$t('subs.stats.active')"
        :value="activeSubsCount"
        @click="filterType = 'active'"
      >
        <template #icon><Play :size="18" /></template>
      </StatCard>

      <StatCard
        clickable
        tone="success"
        :active="filterType === 'completed'"
        :label="$t('subs.stats.completed')"
        :value="completedSubsCount"
        @click="filterType = 'completed'"
      >
        <template #icon><Check :size="18" /></template>
      </StatCard>

      <StatCard
        clickable
        tone="warning"
        :active="filterType === 'stalled'"
        :label="$t('subs.stats.stalled')"
        :value="stalledSubsCount"
        @click="filterType = 'stalled'"
      >
        <template #icon><AlertTriangle :size="18" /></template>
      </StatCard>

      <StatCard
        clickable
        :tone="deletedSubsCount > 0 ? 'error' : 'default'"
        :active="filterType === 'recycle'"
        :label="$t('subs.filter.recycle')"
        :value="deletedSubsCount"
        class="col-span-2 sm:col-span-1"
        @click="filterType = 'recycle'"
      >
        <template #icon><Trash2 :size="18" /></template>
      </StatCard>
    </div>

    <!-- 搜索、筛选与排序工具栏 -->
    <div class="app-toolbar">
      <div class="relative w-full sm:w-72">
        <Search :size="15" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40" />
        <input
          v-model="filterText"
          type="text"
          :placeholder="$t('subs.searchPlaceholder')"
          class="input input-sm w-full h-9 bg-base-200/70 border border-base-content/12 rounded-lg pl-9 text-xs sm:text-sm"
        />
      </div>

      <div class="flex items-center gap-2 flex-wrap">
        <div class="app-segmented overflow-x-auto no-scrollbar max-w-full">
          <button
            v-for="tab in [
              { key: 'all', label: $t('subs.filter.all') || '全部' },
              { key: 'active', label: $t('subs.filter.active') || '连载中' },
              { key: 'completed', label: $t('subs.filter.completed') || '已完结' },
              { key: 'stalled', label: $t('subs.filter.stalled') || '超时异常' },
              { key: 'recycle', label: $t('subs.filter.recycle') || '回收站' }
            ]"
            :key="tab.key"
            type="button"
            class="app-segmented-btn"
            :data-active="filterType === tab.key ? 'true' : 'false'"
            @click="filterType = tab.key as any"
          >
            <span>{{ tab.label }}</span>
            <span v-if="tab.key === 'recycle' && deletedSubsCount > 0" class="badge badge-error badge-xs font-mono tabular-nums">
              {{ deletedSubsCount }}
            </span>
          </button>
        </div>

        <select v-model="sortBy" class="select select-sm h-9 min-h-0 bg-base-200/70 border border-base-content/12 rounded-lg text-xs font-medium">
          <option value="created_at">{{ $t('subs.sort.createdAt') }}</option>
          <option value="title">{{ $t('subs.sort.title') }}</option>
          <option value="progress">{{ $t('subs.sort.progress') }}</option>
          <option value="year">{{ $t('subs.sort.year') }}</option>
        </select>

        <span class="hidden xl:inline text-xs text-base-content/45 font-mono tabular-nums px-1">
          {{ $t('subs.itemsTracked', { count: subs.length }) }}
        </span>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="error" class="alert bg-error/10 border border-error/25 text-error rounded-xl p-4 flex items-start gap-3">
      <AlertTriangle :size="20" class="shrink-0 mt-0.5" />
      <div class="flex-1 min-w-0">
        <h3 class="font-semibold text-sm">{{ $t('subs.error.op') }}</h3>
        <p class="text-xs opacity-85 mt-0.5">{{ error }}</p>
      </div>
      <button type="button" class="btn btn-ghost btn-circle btn-xs" @click="error = ''">
        <X :size="15" />
      </button>
    </div>

    <!-- 加载骨架 -->
    <div v-if="loading" class="grid gap-4 sm:gap-5 grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 animate-pulse">
      <div v-for="i in 12" :key="i" class="aspect-[2/3] bg-base-200 rounded-xl"></div>
    </div>

    <!-- 空状态 -->
    <EmptyState
      v-else-if="filteredSubs.length === 0"
      :title="filterType === 'recycle' ? $t('subs.recycle.emptyTitle') : (subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs'))"
      :description="filterType === 'recycle' ? $t('subs.recycle.emptyDesc') : (subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc'))"
    >
      <template #icon>
        <Trash2 v-if="filterType === 'recycle'" :size="22" />
        <LayoutGrid v-else :size="22" />
      </template>
      <template #actions>
        <button
          v-if="filterType === 'recycle'"
          type="button"
          class="btn btn-outline btn-sm rounded-lg gap-1.5 border-base-content/15"
          @click="filterType = 'all'"
        >
          <RotateCcw :size="14" />
          <span>返回活跃订阅</span>
        </button>
        <button
          v-else-if="subs.length === 0"
          type="button"
          class="btn btn-primary btn-sm rounded-lg gap-1.5 px-4"
          @click="router.push('/search')"
        >
          <Search :size="14" />
          <span>{{ $t('subs.empty.discover') }}</span>
        </button>
        <button
          v-else
          type="button"
          class="btn btn-outline btn-sm rounded-lg gap-1.5 border-base-content/15"
          @click="filterText = ''; filterType = 'all'"
        >
          <RefreshCw :size="14" />
          <span>{{ $t('subs.empty.clear') }}</span>
        </button>
      </template>
    </EmptyState>

    <!-- 回收站专用列表 -->
    <div v-else-if="filterType === 'recycle'" class="space-y-4">
      <div class="flex items-center justify-between bg-base-100 p-4 sm:p-5 rounded-3xl border border-base-200/80 shadow-sm">
        <div class="space-y-0.5">
          <h3 class="font-black text-sm text-base-content">{{ $t('subs.recycle.title') }}</h3>
          <p class="text-xs text-base-content/50">{{ $t('subs.recycle.desc') }}</p>
        </div>
        <button 
          v-if="filteredSubs.length > 0"
          class="btn btn-primary btn-sm rounded-xl gap-2 font-bold px-4"
          @click="restoreSubscription(filteredSubs.map(s => s.id))"
        >
          <RotateCcw :size="14" />
          {{ $t('subs.recycle.batchRestore') }} ({{ filteredSubs.length }})
        </button>
      </div>

      <div class="grid gap-3 sm:gap-4 grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
        <div 
          v-for="sub in filteredSubs" 
          :key="sub.id"
          class="bg-base-100 p-4 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between gap-4 hover:border-primary/40 transition-colors"
        >
          <div class="flex items-center gap-3.5 min-w-0">
            <img 
              v-if="sub.cover_url" 
              :src="proxyImage(sub.cover_url)" 
              class="w-12 h-16 object-cover rounded-xl shrink-0 bg-base-200" 
            />
            <div v-else class="w-12 h-16 bg-base-200 rounded-xl shrink-0 flex items-center justify-center font-bold text-xs opacity-40">
              S{{ sub.season }}
            </div>
            <div class="min-w-0 space-y-1">
              <h4 class="font-black text-sm truncate" :title="sub.title_cn">{{ sub.title_cn }}</h4>
              <p class="text-xs opacity-60 truncate">
                {{ sub.subgroup_name || '通用' }} · S{{ sub.season }} · {{ sub.total_episodes ? sub.total_episodes + '集' : '连载' }}
              </p>
              <span v-if="sub.deleted_at" class="text-[10px] text-error font-medium block">
                {{ $t('subs.recycle.deletedAt') }}: {{ new Date(sub.deleted_at).toLocaleString() }}
              </span>
            </div>
          </div>
          <button 
            class="btn btn-outline btn-primary btn-sm rounded-xl gap-1.5 shrink-0 font-bold hover:scale-105 active:scale-95 transition-all"
            @click="restoreSubscription([sub.id])"
          >
            <RotateCcw :size="14" />
            {{ $t('subs.recycle.restore') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Subscription Grid -->
    <div v-else>
      <div class="grid gap-3 sm:gap-6 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7 3xl:grid-cols-8">
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
    </div>

    <!-- 撤回浮条 -->
    <Transition name="slide-up">
      <div v-if="undoBarVisible" class="fixed bottom-16 lg:bottom-4 left-0 right-0 z-50 p-3 sm:p-4 pointer-events-none">
        <div class="max-w-lg mx-auto bg-base-300 rounded-2xl shadow-2xl px-6 py-4 flex items-center gap-4 pointer-events-auto">
          <span class="text-sm font-bold flex-1">
            {{ $t('subs.undo.msg', { seconds: remainingSeconds, count: undoCount, files: undoDeleteFiles ? $t('subs.undo.withFiles') : '' }) }}
          </span>
          <button class="btn btn-primary btn-sm rounded-xl" @click="undoDelete">{{ $t('subs.undo.btn') }}</button>
        </div>
      </div>
    </Transition>

    <dialog v-if="deleteModalOpen" class="modal modal-open" @click.self="deleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">{{ $t('subs.modal.deleteTitle', { title: deletingSub?.title_cn }) }}</h3>
        <p class="text-sm text-base-content/60 mb-6">{{ $t('subs.modal.deleteDesc') }}</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="deleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">{{ $t('subs.modal.deleteFiles') }}</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="deleteModalOpen = false">{{ $t('common.cancel') }}</button>
          <button class="btn btn-error rounded-xl" @click="confirmSingleDelete">{{ $t('subs.modal.confirmDelete') }}</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>

    <dialog v-if="batchDeleteModalOpen" class="modal modal-open" @click.self="batchDeleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">{{ $t('subs.modal.batchDeleteTitle', { count: batchDeleteSelected.size }) }}</h3>
        <p class="text-sm text-base-content/60 mb-6">{{ $t('subs.modal.deleteDesc') }}</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="batchDeleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">{{ $t('subs.modal.deleteFiles') }}</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="batchDeleteModalOpen = false">{{ $t('common.cancel') }}</button>
          <button class="btn btn-error rounded-xl" @click="confirmBatchDeleteWithFiles">{{ $t('subs.modal.confirmDelete') }}</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>

    <!-- 聚合任务中心 -->
    <div v-if="taskCenter.active" class="fixed bottom-20 right-4 z-[100] pointer-events-none w-80 max-w-[calc(100%-2rem)] lg:bottom-6 lg:right-6">
      <div class="bg-base-300/95 backdrop-blur-2xl border border-white/10 rounded-3xl shadow-2xl pointer-events-auto overflow-hidden transition-all duration-500"
           :class="{ 'h-14': taskCenter.minimized }">
        <!-- Header -->
        <div class="flex items-center justify-between p-4 bg-primary/10 cursor-pointer" @click="taskCenter.minimized = !taskCenter.minimized">
          <div class="flex items-center gap-3">
            <div class="p-1.5 bg-primary/20 rounded-lg text-primary">
              <RefreshCw :size="14" :class="{ 'animate-spin': taskCenter.completed < taskCenter.total }" />
            </div>
            <span class="text-[10px] font-black uppercase tracking-widest text-primary">
              补全中 ({{ taskCenter.completed }}/{{ taskCenter.total }})
            </span>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-ghost btn-xs btn-circle" @click.stop="taskCenter.active = false"><X :size="12" /></button>
          </div>
        </div>
        
        <!-- Progress Bar -->
        <div class="h-1 w-full bg-base-content/5 overflow-hidden">
          <div class="h-full bg-primary transition-all duration-500" 
               :style="{ width: (taskCenter.completed / Math.max(1, taskCenter.total)) * 100 + '%' }"></div>
        </div>

        <!-- Logs -->
        <div v-if="!taskCenter.minimized" class="p-4 space-y-3 max-h-64 overflow-y-auto no-scrollbar">
          <div v-for="log in taskCenter.logs.slice(0, 10)" :key="log.id" class="flex flex-col gap-0.5 animate-in fade-in slide-in-from-left duration-300">
            <p class="text-[9px] font-black uppercase tracking-widest text-base-content/30">{{ log.title }}</p>
            <p class="text-[11px] font-bold opacity-80 leading-relaxed">{{ log.message }}</p>
          </div>
          <div v-if="taskCenter.logs.length === 0" class="py-8 text-center opacity-20">
             <p class="text-xs font-bold">暂无任务日志</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.no-scrollbar::-webkit-scrollbar {
  display: none;
}

.progress-list-enter-active, .progress-list-leave-active { transition: all 0.5s ease; }
.progress-list-enter-from { opacity: 0; transform: translateX(30px); }
.progress-list-leave-to { opacity: 0; transform: scale(0.9); }
</style>
