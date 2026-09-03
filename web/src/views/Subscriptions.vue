<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Search, Plus, AlertTriangle, 
  X, LayoutGrid, RefreshCw,
  RotateCcw, Trash2, Antenna, Check
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
}

const router = useRouter()
const subs = ref<Subscription[]>([])
const loading = ref(true)
const error = ref('')
const deletingId = ref<number | null>(null)
const filterText = ref('')
const filterType = ref<'all' | 'active' | 'completed' | 'stalled'>('all')
const sortBy = ref<'created_at' | 'title' | 'progress' | 'year'>('created_at')

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

const filteredSubs = computed(() => {
  let list = [...subs.value]
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
  if (loading.value === false) {
     // Background refresh - don't show loading spinner
  }
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
  setupEventStream()
  refreshTimer = setInterval(fetchSubscriptions, 30000)
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
              {{ batchDeleteSelected.size === filteredSubs.length && filteredSubs.length > 0 ? '取消全选' : '全选本页' }}
            </button>
            <span class="text-xs font-bold opacity-60 whitespace-nowrap">已选 {{ batchDeleteSelected.size }}</span>
            <button class="btn btn-ghost btn-xs rounded-xl h-10 min-h-0 px-3" @click="exitBatchDeleteMode">
              取消
            </button>
            <button class="btn btn-error btn-xs rounded-xl h-10 min-h-0 px-4 gap-1.5 whitespace-nowrap font-bold" :disabled="batchDeleteSelected.size === 0" @click="openBatchDeleteModal">
              <Trash2 :size="14" />
              删除 ({{ batchDeleteSelected.size }})
            </button>
          </div>
        </template>
        <button v-if="!batchDeleteMode"
          class="flex-none btn btn-ghost border border-base-300/50 rounded-2xl gap-2 px-4 h-11 min-h-0 hover:bg-base-200 transition-all active:scale-95 whitespace-nowrap"
          @click="enterBatchDeleteMode">
          <Trash2 :size="16" class="opacity-50" />
          <span class="text-[10px] font-black uppercase tracking-widest">批量删除</span>
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

    <!-- 订阅统计概览仪表盘 -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
      <div 
        @click="filterType = 'all'" 
        class="bg-base-100 p-4 rounded-2xl border transition-all cursor-pointer hover:border-primary/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'all' ? 'border-primary shadow-md shadow-primary/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">全部追番</span>
          <p class="text-xl font-black font-mono text-base-content">{{ subs.length }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-base-200 text-base-content/60 flex items-center justify-center">
          <LayoutGrid :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'active'" 
        class="bg-base-100 p-4 rounded-2xl border transition-all cursor-pointer hover:border-primary/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'active' ? 'border-primary shadow-md shadow-primary/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">连载中</span>
          <p class="text-xl font-black font-mono text-primary">{{ activeSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
          <Antenna :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'completed'" 
        class="bg-base-100 p-4 rounded-2xl border transition-all cursor-pointer hover:border-success/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'completed' ? 'border-success shadow-md shadow-success/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">完结归档</span>
          <p class="text-xl font-black font-mono text-success">{{ completedSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-success/10 text-success flex items-center justify-center">
          <Check :size="20" />
        </div>
      </div>

      <div 
        @click="filterType = 'stalled'" 
        class="bg-base-100 p-4 rounded-2xl border transition-all cursor-pointer hover:border-warning/50 shadow-sm flex items-center justify-between"
        :class="filterType === 'stalled' ? 'border-warning shadow-md shadow-warning/10' : 'border-base-200/80'"
      >
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">剧集超时/异常</span>
          <p class="text-xl font-black font-mono text-warning">{{ stalledSubsCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-warning/10 text-warning flex items-center justify-center">
          <AlertTriangle :size="20" />
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
            { key: 'stalled', label: '超时异常' }
          ]" 
          :key="t.key"
          class="px-5 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all whitespace-nowrap"
          :class="filterType === t.key ? 'bg-base-100 text-primary shadow-sm ring-1 ring-base-300 font-bold' : 'text-base-content/40 hover:text-base-content'"
          @click="filterType = t.key as any"
        >
          {{ t.label }}
        </button>
      </div>

        <div class="flex items-center gap-2 px-3 h-11 bg-base-200/50 rounded-2xl border border-transparent focus-within:border-primary/30 transition-all">
          <LayoutGrid :size="16" class="opacity-30" />
          <select v-model="sortBy" class="select select-ghost select-sm rounded-xl text-[10px] font-black uppercase tracking-widest bg-transparent border-none h-8 px-1 focus:ring-0">
            <option value="created_at">按订阅时间</option>
            <option value="title">按名称</option>
            <option value="progress">按进度</option>
            <option value="year">按年份</option>
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
        <LayoutGrid :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">
        {{ subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs') }}
      </h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc') }}
      </p>
      <button 
        v-if="subs.length === 0" 
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

    <!-- Subscription Grid -->
    <div v-else>
      <TransitionGroup
        name="list"
        tag="div"
        class="grid gap-6 sm:gap-8 grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 3xl:grid-cols-7 4xl:grid-cols-8"
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
      <div v-if="undoBarVisible" class="fixed bottom-0 left-0 right-0 z-50 p-4 pointer-events-none">
        <div class="max-w-lg mx-auto bg-base-300 rounded-2xl shadow-2xl px-6 py-4 flex items-center gap-4 pointer-events-auto">
          <span class="text-sm font-bold flex-1">
            将在 {{ remainingSeconds }} 秒后删除 {{ undoCount }} 个订阅{{ undoDeleteFiles ? '（含文件）' : '' }}
          </span>
          <button class="btn btn-primary btn-sm rounded-xl" @click="undoDelete">撤回</button>
        </div>
      </div>
    </Transition>

    <dialog v-if="deleteModalOpen" class="modal modal-open" @click.self="deleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">删除「{{ deletingSub?.title_cn }}」</h3>
        <p class="text-sm text-base-content/60 mb-6">将在 15 秒后执行删除，可撤回。</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="deleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">同时删除已下载的文件和种子</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="deleteModalOpen = false">取消</button>
          <button class="btn btn-error rounded-xl" @click="confirmSingleDelete">确认删除</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>

    <dialog v-if="batchDeleteModalOpen" class="modal modal-open" @click.self="batchDeleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">确定删除 {{ batchDeleteSelected.size }} 个订阅？</h3>
        <p class="text-sm text-base-content/60 mb-6">将在 15 秒后执行删除，可撤回。</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="batchDeleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">同时删除已下载的文件和种子</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="batchDeleteModalOpen = false">取消</button>
          <button class="btn btn-error rounded-xl" @click="confirmBatchDeleteWithFiles">确认删除</button>
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
