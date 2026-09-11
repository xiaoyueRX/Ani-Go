<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Search, Plus, AlertTriangle, 
  X, LayoutGrid, RefreshCw,
  RotateCcw, Trash2, Antenna, Check, Sparkles
} from 'lucide-vue-next'
import SubscriptionCard from '../components/SubscriptionCard.vue'

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
const filterText = ref('')
const filterType = ref<'all' | 'active' | 'completed' | 'stalled' | 'recycle'>('all')
const sortBy = ref<'created_at' | 'title' | 'progress' | 'year'>('created_at')

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

function toggleSelectAllBatch() {
  if (batchDeleteSelected.value.size === filteredSubs.value.length) {
    batchDeleteSelected.value = new Set()
  } else {
    batchDeleteSelected.value = new Set(filteredSubs.value.map(s => s.id))
  }
}
const undoBarVisible = ref(false)
const undoDeletedCount = ref(0)
const undoDeletedIds = ref<number[]>([])
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
interface PendingDelete {
  ids: number[]
  deleteFiles: boolean
  timer: ReturnType<typeof setTimeout>
}
const pendingDeletes = ref<PendingDelete[]>([])
const undoDeleteFiles = ref(false)
const undoCount = ref(0)

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
  // 合并到现有倒计时
  const existing = pendingDeletes.value.length > 0 ? pendingDeletes.value[0] : null
  if (existing) {
    for (const id of ids) {
      if (!existing.ids.includes(id)) existing.ids.push(id)
    }
    existing.deleteFiles = existing.deleteFiles || deleteFiles
    clearTimeout(existing.timer)
    existing.timer = setTimeout(() => executePendingDeletes(), UNDO_TIMEOUT_SECONDS * 1000)
    remainingSeconds.value = UNDO_TIMEOUT_SECONDS
    undoCount.value = existing.ids.length
    undoDeleteFiles.value = existing.deleteFiles
    undoBarVisible.value = true
    return
  }

  const timer = setTimeout(() => executePendingDeletes(), UNDO_TIMEOUT_SECONDS * 1000)
  pendingDeletes.value = [{ ids, deleteFiles, timer }]
  undoCount.value = ids.length
  undoDeleteFiles.value = deleteFiles
  undoBarVisible.value = true
  remainingSeconds.value = UNDO_TIMEOUT_SECONDS
  startUndoCountdown()
}

// 执行真正删除
async function executePendingDeletes() {
  const batch = pendingDeletes.value.shift()
  if (!batch) return
  undoBarVisible.value = false
  clearUndoTimer()
  
  try {
    await request.post('/subscriptions/batch-delete', {
      ids: batch.ids,
      delete_files: batch.deleteFiles
    })
    subs.value = subs.value.filter(s => !batch.ids.includes(s.id))
  } catch (e: any) {
    error.value = '批量删除失败，请重试'
  }
}

// 撤回
function undoDelete() {
  const batch = pendingDeletes.value.shift()
  if (!batch) return
  clearTimeout(batch.timer)
  undoBarVisible.value = false
  clearUndoTimer()
}

// 判断是否待删除
function isPending(id: number): boolean {
  return pendingDeletes.value.some(b => b.ids.includes(id))
}

function startUndoCountdown() {
  remainingSeconds.value = UNDO_TIMEOUT_SECONDS
  undoInterval = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      clearUndoTimer()
      hideUndoBar()
    }
  }, 1000)
}

function clearUndoTimer() {
  if (undoInterval) {
    clearInterval(undoInterval)
    undoInterval = null
  }
}

function hideUndoBar() {
  undoBarVisible.value = false
  undoDeletedIds.value = []
  undoDeletedCount.value = 0
  clearUndoTimer()
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

async function fetchSubscriptions() {
  error.value = ''
  try {
    const { data } = await request.get('/subscriptions')
    subs.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.load')
  } finally {
    loading.value = false
  }
}

async function fetchDeletedSubscriptions() {
  deletedLoading.value = true
  try {
    const { data } = await request.get('/subscriptions?deleted=true')
    deletedSubs.value = data || []
  } catch (e: any) {
    console.error('Failed to load recycle bin:', e)
  } finally {
    deletedLoading.value = false
  }
}

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
      payload.allowed_subgroups = JSON.stringify([aiResult.value.subgroup_pref])
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

function setupEventStream() {
  const token = localStorage.getItem('token')
  const apiPath = window.location.origin + '/api/events/stream' + (token ? '?token=' + token : '')
  eventSource = new EventSource(apiPath)

  eventSource.onopen = () => {
    console.log('✅ SSE 已连接')
  }

  eventSource.onerror = (e) => {
    console.error('❌ SSE 错误:', e)
    eventSource?.close()
    // 5秒后尝试重连
    setTimeout(setupEventStream, 5000)
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
      addLog(payload.title, "补全扫描完成 ✅")
      fetchSubscriptions()
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
      if (payload.message && payload.message.includes("已添加下载")) fetchSubscriptions()
    }
  }
}


onMounted(() => {
  fetchSubscriptions()
  fetchDeletedSubscriptions()
  setupEventStream()
  refreshTimer = setInterval(() => {
    fetchSubscriptions()
    if (filterType.value === 'recycle') {
      fetchDeletedSubscriptions()
    }
  }, 30000)
})
onUnmounted(() => {
  clearInterval(refreshTimer)
  if (eventSource) eventSource.close()
  clearUndoTimer()
  while (pendingDeletes.value.length > 0) {
    const batch = pendingDeletes.value.shift()!
    clearTimeout(batch.timer)
    request.post('/subscriptions/batch-delete', {
      ids: batch.ids,
      delete_files: batch.deleteFiles
    }).catch(() => {})
  }
})
</script>

<template>
  <div class="space-y-10">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('subs.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('subs.subtitle') }}</p>
      </div>
      
      <div class="flex items-center gap-2 overflow-x-auto no-scrollbar max-w-full pb-1">
        <template v-if="batchDeleteMode">
          <div class="flex items-center gap-2.5 flex-none">
            <button class="btn btn-ghost btn-xs rounded-xl h-10 min-h-0 px-3 text-xs font-bold" @click="toggleSelectAllBatch">
              {{ batchDeleteSelected.size === filteredSubs.length && filteredSubs.length > 0 ? $t('subs.unselectAll') : $t('subs.selectAll') }}
            </button>
            <span class="text-xs font-bold opacity-60 whitespace-nowrap">{{ $t('subs.selectedCount', { count: batchDeleteSelected.size }) }}</span>
            <button class="btn btn-ghost btn-xs rounded-xl h-10 min-h-0 px-3" @click="exitBatchDeleteMode">
              {{ $t('common.cancel') }}
            </button>
            <button class="btn btn-error btn-xs rounded-xl h-10 min-h-0 px-4 gap-1.5 whitespace-nowrap font-bold" :disabled="batchDeleteSelected.size === 0" @click="openBatchDeleteModal">
              <Trash2 :size="14" />
              {{ $t('common.delete') }} ({{ batchDeleteSelected.size }})
            </button>
          </div>
        </template>
        <button v-if="!batchDeleteMode"
          class="flex-none btn btn-ghost border border-base-300/50 rounded-2xl gap-2 px-4 h-11 min-h-0 hover:bg-base-200 transition-all active:scale-95 whitespace-nowrap"
          @click="enterBatchDeleteMode">
          <Trash2 :size="16" class="opacity-50" />
          <span class="text-[10px] font-black uppercase tracking-widest">{{ $t('subs.batchDelete') }}</span>
        </button>
        <button 
          class="flex-none btn btn-ghost border border-base-300/50 rounded-2xl gap-2 px-4 h-11 min-h-0 hover:bg-base-200 transition-all active:scale-95 whitespace-nowrap" 
          @click="triggerSupplementAll"
          :disabled="loading"
        >
          <RotateCcw :size="16" class="opacity-50" />
          <span class="text-[10px] font-black uppercase tracking-widest">{{ $t('subs.supplementAll') }}</span>
        </button>
        <button 
          class="flex-none btn btn-ghost border border-base-300/50 rounded-2xl gap-2 px-4 h-11 min-h-0 hover:bg-base-200 transition-all active:scale-95 whitespace-nowrap" 
          @click="router.push('/search')"
        >
          <Search :size="16" class="opacity-50" />
          <span class="text-[10px] font-black uppercase tracking-widest">{{ $t('subs.find') }}</span>
        </button>
        <button 
          class="flex-none btn btn-primary rounded-2xl gap-2 px-5 h-11 min-h-0 shadow-lg shadow-primary/20 hover:scale-105 active:scale-95 transition-all whitespace-nowrap" 
          @click="router.push('/search')"
        >
          <Plus :size="16" />
          <span class="text-[10px] font-black uppercase tracking-widest">{{ $t('subs.new') }}</span>
        </button>
      </div>
    </div>

    <!-- AI 一句话智能追番栏 -->
    <div class="bg-gradient-to-r from-primary/10 via-base-100 to-primary/5 p-4 sm:p-5 rounded-3xl border border-primary/20 shadow-sm space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2 text-primary font-black text-xs uppercase tracking-wider">
          <Sparkles :size="16" class="animate-pulse" />
          <span>{{ $t('subs.quickSubscribe') }}</span>
        </div>
        <span class="text-[11px] opacity-60 hidden sm:inline">{{ $t('subs.quickSubscribeDesc') }}</span>
      </div>

      <div class="flex flex-col sm:flex-row gap-2">
        <div class="relative flex-1">
          <input 
            v-model="aiInput" 
            type="text" 
            :placeholder="$t('subs.quickSubscribePlaceholder')"
            class="input w-full bg-base-100 border-primary/30 focus:border-primary focus:ring-0 rounded-2xl text-xs font-semibold h-11"
            @keyup.enter="parseAiTask"
          />
          <button 
            v-if="aiInput" 
            class="btn btn-ghost btn-circle btn-xs absolute right-3 top-1/2 -translate-y-1/2 opacity-40 hover:opacity-100"
            @click="aiInput = ''; aiResult = null"
          >
            <X :size="14" />
          </button>
        </div>
        <button 
          class="btn btn-primary rounded-2xl gap-2 h-11 min-h-0 px-6 font-black text-xs shadow-md shrink-0"
          :disabled="aiParsing || !aiInput.trim()"
          @click="parseAiTask"
        >
          <RefreshCw v-if="aiParsing" :size="14" class="animate-spin" />
          <Sparkles v-else :size="14" />
          <span>{{ aiParsing ? $t('subs.quickSubscribeParsing') : $t('subs.quickSubscribeBtn') }}</span>
        </button>
      </div>

      <!-- AI 解析结果展示卡片 -->
      <Transition name="fade">
        <div v-if="aiResult" class="bg-base-100/90 backdrop-blur p-3.5 rounded-2xl border border-primary/20 space-y-3">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap items-center gap-2">
              <span class="badge badge-primary font-black text-xs">
                {{ aiResult.action === 'subscribe' ? '追番' : aiResult.action }}
              </span>
              <span class="font-bold text-sm text-base-content">{{ aiResult.title }}</span>
              <span class="badge badge-outline text-xs font-mono">S{{ aiResult.season || 1 }}</span>
              <span v-if="aiResult.resolution" class="badge badge-ghost text-xs font-mono">{{ aiResult.resolution }}</span>
              <span v-if="aiResult.subgroup_pref" class="badge badge-secondary badge-outline text-xs">{{ aiResult.subgroup_pref }}</span>
              <span v-if="aiResult.confidence" class="text-[10px] opacity-60">置信度: {{ Math.round(aiResult.confidence * 100) }}%</span>
            </div>
            <div class="flex items-center gap-2">
              <button 
                class="btn btn-primary btn-xs rounded-xl gap-1.5 h-8 px-3 font-bold"
                :disabled="aiSubscribing"
                @click="applyAiSubscribe"
              >
                <Check :size="12" />
                <span>{{ $t('subs.oneClickSubscribe') }}</span>
              </button>
              <button 
                class="btn btn-ghost btn-xs rounded-xl h-8 px-3 font-bold opacity-75 hover:opacity-100"
                @click="router.push(`/search?q=${encodeURIComponent(aiResult.title)}`)"
              >
                <Search :size="12" />
                <span>{{ $t('subs.goToSearch') }}</span>
              </button>
              <button 
                class="btn btn-ghost btn-circle btn-xs opacity-50 hover:opacity-100"
                @click="aiResult = null"
              >
                <X :size="14" />
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </div>

    <!-- 订阅统计概览仪表盘 -->
    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 sm:gap-4">
      <div 
        @click="filterType = 'all'" 
        class="bg-base-100 p-3 sm:p-4 rounded-2xl border transition-all cursor-pointer hover:border-primary/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'all' ? 'border-primary shadow-md shadow-primary/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">{{ $t('subs.stats.all') }}</span>
          <p class="text-xl font-black font-mono text-base-content">{{ subs.length }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-base-200 text-base-content/60 flex items-center justify-center">
          <LayoutGrid :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'active'" 
        class="bg-base-100 p-3 sm:p-4 rounded-2xl border transition-all cursor-pointer hover:border-primary/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'active' ? 'border-primary shadow-md shadow-primary/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">{{ $t('subs.stats.active') }}</span>
          <p class="text-xl font-black font-mono text-primary">{{ activeSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
          <Antenna :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'completed'" 
        class="bg-base-100 p-3 sm:p-4 rounded-2xl border transition-all cursor-pointer hover:border-success/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'completed' ? 'border-success shadow-md shadow-success/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">{{ $t('subs.stats.completed') }}</span>
          <p class="text-xl font-black font-mono text-success">{{ completedSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-success/10 text-success flex items-center justify-center">
          <Check :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'stalled'" 
        class="bg-base-100 p-3 sm:p-4 rounded-2xl border transition-all cursor-pointer hover:border-warning/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'stalled' ? 'border-warning shadow-md shadow-warning/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">{{ $t('subs.stats.stalled') }}</span>
          <p class="text-xl font-black font-mono text-warning">{{ stalledSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-warning/10 text-warning flex items-center justify-center">
          <AlertTriangle :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'recycle'" 
        class="bg-base-100 p-3 sm:p-4 rounded-2xl border transition-all cursor-pointer hover:border-error/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'recycle' ? 'border-error shadow-md shadow-error/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">{{ $t('subs.filter.recycle') }}</span>
          <p class="text-xl font-black font-mono" :class="deletedSubsCount > 0 ? 'text-error' : 'text-base-content/40'">{{ deletedSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-error/10 text-error flex items-center justify-center">
          <Trash2 :size="20" />
        </div>
      </div>
    </div>

    <!-- 搜索与排序工具栏 -->
    <div class="flex flex-wrap items-center gap-4 bg-base-100 p-3 rounded-[2rem] border border-base-200/50 shadow-sm">
      <div class="relative w-full sm:w-80 group">
        <div class="absolute inset-y-0 left-4 flex items-center pointer-events-none text-base-content/20 group-focus-within:text-primary transition-colors">
          <Search :size="20" />
        </div>
        <input 
          v-model="filterText" 
          type="text" 
          :placeholder="$t('subs.searchPlaceholder')" 
          class="input w-full bg-base-200/50 border-transparent focus:border-primary/30 focus:bg-base-100 focus:ring-0 rounded-2xl pl-12 transition-all font-bold text-sm h-12"
        />
      </div>

      <div class="flex p-1.5 bg-base-200/50 rounded-2xl gap-1 w-fit overflow-x-auto no-scrollbar">
        <button 
          v-for="t in [
            { key: 'all', label: $t('subs.filter.all') || '全部' },
            { key: 'active', label: $t('subs.filter.active') || '连载中' },
            { key: 'completed', label: $t('subs.filter.completed') || '已完结' },
            { key: 'stalled', label: $t('subs.filter.stalled') || '超时异常' },
            { key: 'recycle', label: $t('subs.filter.recycle') || '回收站' }
          ]" 
          :key="t.key"
          class="px-5 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all whitespace-nowrap"
          :class="filterType === t.key ? 'bg-base-100 text-primary shadow-sm ring-1 ring-base-300 font-bold' : 'text-base-content/40 hover:text-base-content'"
          @click="filterType = t.key as any"
        >
          {{ t.label }}
          <span v-if="t.key === 'recycle' && deletedSubsCount > 0" class="badge badge-error badge-xs ml-1 font-mono text-[9px] text-white">
            {{ deletedSubsCount }}
          </span>
        </button>
      </div>

        <div class="flex items-center gap-2 px-3 h-11 bg-base-200/50 rounded-2xl border border-transparent focus-within:border-primary/30 transition-all">
          <LayoutGrid :size="16" class="opacity-30" />
          <select v-model="sortBy" class="select select-ghost select-sm rounded-xl text-[10px] font-black uppercase tracking-widest bg-transparent border-none h-8 px-1 focus:ring-0">
            <option value="created_at">{{ $t('subs.sort.createdAt') }}</option>
            <option value="title">{{ $t('subs.sort.title') }}</option>
            <option value="progress">{{ $t('subs.sort.progress') }}</option>
            <option value="year">{{ $t('subs.sort.year') }}</option>
          </select>
        </div>

        <div class="hidden sm:flex ml-auto px-4 items-center gap-2">
           <span class="text-[10px] font-black text-base-content/20 uppercase tracking-widest ml-2">{{ $t('subs.itemsTracked', { count: subs.length }) }}</span>
        </div>
      </div>

    <!-- Status Alerts -->
    <div v-if="error" class="alert bg-error/10 border-error/20 text-error rounded-3xl p-6 flex items-start gap-4">
      <div class="p-3 bg-error/20 rounded-2xl">
        <AlertTriangle :size="24" />
      </div>
      <div class="flex-1">
        <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('subs.error.op') }}</h3>
        <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
      </div>
      <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
        <X :size="16" />
      </button>
    </div>

    <!-- Main Content Section -->
    <div v-if="loading" class="grid gap-6 sm:gap-8 grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 3xl:grid-cols-7 4xl:grid-cols-8 animate-pulse">
      <div v-for="i in 16" :key="i" class="aspect-[3/5] bg-base-200 rounded-[2.5rem]"></div>
    </div>

    <div v-else-if="filteredSubs.length === 0" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200">
      <div class="w-32 h-32 bg-base-200/50 rounded-full flex items-center justify-center mb-8 rotate-12">
        <Trash2 v-if="filterType === 'recycle'" :size="64" class="opacity-10" />
        <LayoutGrid v-else :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">
        {{ filterType === 'recycle' ? $t('subs.recycle.emptyTitle') : (subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs')) }}
      </h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ filterType === 'recycle' ? $t('subs.recycle.emptyDesc') : (subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc')) }}
      </p>
      <button 
        v-if="filterType === 'recycle'"
        class="btn btn-ghost btn-md rounded-2xl px-10 gap-3 border-base-300 font-black text-xs uppercase tracking-wider"
        @click="filterType = 'all'"
      >
        <RotateCcw :size="18" />
        <span>返回活跃订阅</span>
      </button>
      <button 
        v-else-if="subs.length === 0" 
        class="btn btn-primary btn-lg rounded-3xl px-12 shadow-2xl shadow-lg gap-4" 
        @click="router.push('/search')"
      >
        <Search :size="24" />
        <span class="font-black uppercase tracking-widest">{{ $t('subs.empty.discover') }}</span>
      </button>
      <button 
        v-else 
        class="btn btn-ghost btn-md rounded-2xl px-10 gap-4 border-base-300" 
        @click="filterText = ''; filterType = 'all'"
      >
        <RefreshCw :size="20" />
        <span class="font-black uppercase tracking-widest text-xs">{{ $t('subs.empty.clear') }}</span>
      </button>
    </div>

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
      <TransitionGroup
        name="list"
        tag="div"
        class="grid gap-3 sm:gap-6 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7 3xl:grid-cols-8"
      >
        <SubscriptionCard
          v-for="sub in filteredSubs"
          :key="sub.id"
          :sub="sub"
          :deleting="deletingId === sub.id"
          :pending="isPending(sub.id)"
          :batch-delete-mode="batchDeleteMode && !isPending(sub.id)"
          :batch-selected="batchDeleteSelected.has(sub.id)"
          @click="batchDeleteMode ? (!isPending(sub.id) && toggleBatchSelect(sub.id)) : router.push(`/subscriptions/${sub.id}`)"
          @toggle="toggleEnabled(sub)"
          @delete="isPending(sub.id) ? null : handleDelete(sub)"
          @supplement="triggerSupplement(sub)"
        />
      </TransitionGroup>
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
    <div v-if="taskCenter.active" class="fixed bottom-6 right-6 z-[100] pointer-events-none w-80">
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

.list-enter-active,
.list-leave-active {
  transition: all 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: scale(0.9) translateY(20px);
}

.progress-list-enter-active, .progress-list-leave-active { transition: all 0.5s ease; }
.progress-list-enter-from { opacity: 0; transform: translateX(30px); }
.progress-list-leave-to { opacity: 0; transform: scale(0.9); }
</style>
