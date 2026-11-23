<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import request from '../utils/request'
import { 
  RefreshCw, AlertTriangle, X, 
  Download, Pause, History, 
  Upload, Check, MoreVertical, 
  Folder, Search, Zap,
  Play, Trash2, ArrowUp, ArrowDown, Clock, Plus
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface DownloadTask {
  hash: string
  name: string
  save_path: string
  status: string
  display_status: string
  can_pause: boolean
  can_resume: boolean
  complete: boolean
  progress: number
  speed_down: number
  speed_up?: number
  size: number
  done: number
  uploaded?: number
  ratio?: number
  seeding_time?: number
}

const tasks = ref<DownloadTask[]>([])
const loading = ref(true)
const error = ref('')
const searchQuery = ref('')
const currentFilter = ref<'all' | 'downloading' | 'seeding' | 'completed' | 'paused'>('all')
const autoRefresh = ref(true)
let timer: any = null

// 删除弹窗状态
const showDeleteModal = ref(false)
const taskToDelete = ref<DownloadTask | null>(null)
const deleteLocalFiles = ref(false)
const deleting = ref(false)

const activeReads = ref(0)
let nextRequest = 0
let lastApplied = 0

async function fetchDownloads() {
  const order = ++nextRequest
  activeReads.value++
  try {
    const { data } = await request.get('/downloads')
    if (order < lastApplied) return
    error.value = ''
    tasks.value = Array.isArray(data) ? data : []
  } catch (e: any) {
    if (order < lastApplied) return
    error.value = e.response?.data?.error || t('downloads.action.loadFailed')
  } finally {
    activeReads.value--
    lastApplied = Math.max(lastApplied, order)
    loading.value = false
  }
}

// 统计数据
const totalSpeedDown = computed(() => {
  return tasks.value.reduce((acc, cur) => acc + (cur.speed_down || 0), 0)
})

const totalSpeedUp = computed(() => {
  return tasks.value.reduce((acc, cur) => acc + (cur.speed_up || 0), 0)
})

const downloadingCount = computed(() => {
  return tasks.value.filter(t => t.display_status === 'downloading').length
})

const seedingCount = computed(() => {
  return tasks.value.filter(t => t.display_status === 'seeding').length
})

const completedCount = computed(() => {
  return tasks.value.filter(t => t.complete).length
})

// 过滤后的列表
const filteredTasks = computed(() => {
  let list = tasks.value

  if (currentFilter.value !== 'all') {
    list = currentFilter.value === 'completed'
      ? list.filter(t => t.complete)
      : list.filter(t => t.display_status === currentFilter.value)
  }

  if (searchQuery.value.trim()) {
    const q = searchQuery.value.trim().toLowerCase()
    list = list.filter(t => 
      t.name.toLowerCase().includes(q) || 
      t.hash.toLowerCase().includes(q) ||
      (t.save_path && t.save_path.toLowerCase().includes(q))
    )
  }

  return list
})

function formatSize(bytes: number): string {
  if (!bytes) return '0 B'
  if (bytes > 1e9) return (bytes / 1e9).toFixed(2) + ' GB'
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(0) + ' KB'
}

function formatSpeed(bytesPerSec: number): string {
  if (!bytesPerSec) return '0 B/s'
  if (bytesPerSec > 1e6) return (bytesPerSec / 1e6).toFixed(1) + ' MB/s'
  return (bytesPerSec / 1e3).toFixed(0) + ' KB/s'
}

function formatDuration(seconds?: number): string {
  if (!seconds || seconds <= 0) return '0s'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function calcPercent(t: DownloadTask): number {
  if (t.complete) return 100
  const fraction = Number.isFinite(t.progress) ? t.progress : (t.size > 0 ? t.done / t.size : 0)
  return Math.min(99, Math.max(0, Math.round(fraction * 100)))
}

function statusInfo(status: string): { label: string; icon: any; cls: string } {
  const m: Record<string, { label: string; icon: any; cls: string }> = {
    downloading: { label: t('downloads.status.downloading') || '下载中', icon: Download, cls: 'bg-primary/20 text-primary border-primary/20' },
    paused: { label: t('downloads.status.paused') || '已暂停', icon: Pause, cls: 'bg-warning/20 text-warning border-warning/20' },
    queued: { label: t('downloads.status.queued') || '排队中', icon: History, cls: 'bg-base-300 text-base-content/40 border-base-300' },
    checking: { label: t('downloads.status.checking') || '校验中', icon: RefreshCw, cls: 'bg-info/20 text-info border-info/20' },
    seeding: { label: t('downloads.status.seeding') || '做种中', icon: Upload, cls: 'bg-success/20 text-success border-success/20' },
    completed: { label: t('downloads.status.completed') || '已完成', icon: Check, cls: 'bg-success/20 text-success border-success/20' },
    error: { label: t('downloads.status.error') || '错误', icon: AlertTriangle, cls: 'bg-error/20 text-error border-error/20' },
  }
  return m[status] || { label: status, icon: MoreVertical, cls: 'bg-base-300 text-base-content/40 border-base-300' }
}

// 任务控制操作
const changingTasks = ref(new Set<string>())

async function changeTask(task: DownloadTask, command: 'pause' | 'resume') {
  if (changingTasks.value.has(task.hash) || batchActionLoading.value) return
  changingTasks.value.add(task.hash)
  try {
    await request.post(`/downloads/${encodeURIComponent(task.hash)}/${command}`)
    window.showToast?.(t(command === 'pause' ? 'downloads.action.pausedSuccess' : 'downloads.action.resumedSuccess'), 'success')
    await fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || t('downloads.action.changeFailed'), 'error')
  } finally {
    changingTasks.value.delete(task.hash)
  }
}

function pauseTask(task: DownloadTask) { return changeTask(task, 'pause') }
function resumeTask(task: DownloadTask) { return changeTask(task, 'resume') }

function openDeleteModal(t: DownloadTask) {
  if (changingTasks.value.has(t.hash) || batchActionLoading.value || deleting.value) return
  deleteError.value = ''
  taskToDelete.value = t
  deleteLocalFiles.value = false
  showDeleteModal.value = true
}

async function confirmDelete() {
  if (!taskToDelete.value || deleting.value) return
  deleteError.value = ''
  deleting.value = true
  try {
    await request.delete(`/downloads/${encodeURIComponent(taskToDelete.value.hash)}`, {
      params: { delete_files: deleteLocalFiles.value }
    })
    window.showToast?.(t('downloads.action.deletedSuccess') || '任务已删除', 'success')
    showDeleteModal.value = false
    taskToDelete.value = null
    fetchDownloads()
  } catch (e: any) {
    deleteError.value = e.response?.data?.error || t('downloads.action.deleteFailed')
  } finally {
    deleting.value = false
  }
}

// 新建下载任务弹窗状态与操作
const showAddModal = ref(false)
const addUrl = ref('')
const addTitle = ref('')
const addSavePath = ref('')
const adding = ref(false)
const addError = ref('')
const deleteError = ref('')
const addDialog = ref<HTMLDialogElement | null>(null)
const deleteDialog = ref<HTMLDialogElement | null>(null)

watch(showAddModal, open => {
  if (open) {
    addError.value = ''
    addDialog.value?.showModal()
  } else {
    addDialog.value?.close()
  }
})
watch(showDeleteModal, open => {
  if (open) deleteDialog.value?.showModal()
  else deleteDialog.value?.close()
})

async function submitNewDownload() {
  if (adding.value) return
  addError.value = ''
  const target = addUrl.value.trim()
  if (!target) {
    addError.value = t('downloads.action.link')
    return
  }
  adding.value = true
  try {
    const isMagnet = /^magnet:/i.test(target)
    await request.post('/downloads', {
      url: isMagnet ? '' : target,
      magnet: isMagnet ? target : '',
      title: addTitle.value.trim() || undefined,
      save_path: addSavePath.value.trim() || undefined,
    })
    window.showToast?.(t('downloads.action.submitted'), 'success')
    showAddModal.value = false
    addUrl.value = ''
    addTitle.value = ''
    addSavePath.value = ''
    fetchDownloads()
  } catch (e: any) {
    addError.value = e.response?.data?.error || t('downloads.action.addFailed')
  } finally {
    adding.value = false
  }
}

// 批量暂停 / 恢复所有任务
const batchActionLoading = ref(false)

async function pauseAllTasks() {
  if (batchActionLoading.value || changingTasks.value.size || !confirm(t('downloads.action.pauseAllConfirm'))) return
  batchActionLoading.value = true
  try {
    const { data } = await request.post('/downloads/pause-all')
    window.showToast?.(data.message || t('downloads.action.pausedSuccess'), 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || t('downloads.action.changeFailed'), 'error')
  } finally {
    batchActionLoading.value = false
  }
}

async function resumeAllTasks() {
  if (batchActionLoading.value || changingTasks.value.size) return
  batchActionLoading.value = true
  try {
    const { data } = await request.post('/downloads/resume-all')
    window.showToast?.(data.message || t('downloads.action.resumedSuccess'), 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || t('downloads.action.changeFailed'), 'error')
  } finally {
    batchActionLoading.value = false
  }
}

function pollDownloads() {
  if (autoRefresh.value && !activeReads.value && !document.hidden) void fetchDownloads()
}

watch(autoRefresh, (val) => {
  if (val) {
    timer = setInterval(pollDownloads, 4000)
  } else if (timer) {
    clearInterval(timer)
    timer = null
  }
})

onMounted(() => {
  document.addEventListener('visibilitychange', pollDownloads)
  fetchDownloads()
  if (autoRefresh.value) {
    timer = setInterval(pollDownloads, 4000)
  }
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', pollDownloads)
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="space-y-8 pb-20 max-w-7xl mx-auto animate-in fade-in duration-300">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-base-100 p-6 sm:p-7 rounded-3xl border border-base-200/80 shadow-sm">
      <div class="space-y-1">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shadow-inner">
            <Download :size="20" />
          </div>
          <h1 class="text-2xl sm:text-3xl font-black tracking-tight italic">{{ $t('downloads.title') }}</h1>
          <span class="badge badge-neutral text-xs font-mono font-bold">{{ tasks.length }} {{ $t('downloads.countUnit') }}</span>
        </div>
        <p class="text-xs text-base-content/60 font-medium">{{ $t('downloads.subtitle') }}</p>
      </div>
      
      <div class="flex items-center gap-2.5 flex-wrap">
        <button 
          class="btn btn-outline btn-sm rounded-xl gap-1.5 px-3 border-base-300 hover:bg-base-200"
          @click="pauseAllTasks"
          :disabled="batchActionLoading || changingTasks.size > 0 || !tasks.some(task => task.can_pause)"
          :title="$t('downloads.action.pauseAll')"
        >
          <Pause :size="14" />
          <span class="text-xs font-bold">{{ $t('downloads.action.pauseAll') }}</span>
        </button>

        <button 
          class="btn btn-outline btn-sm rounded-xl gap-1.5 px-3 border-base-300 hover:bg-base-200"
          @click="resumeAllTasks"
          :disabled="batchActionLoading || changingTasks.size > 0 || !tasks.some(task => task.can_resume)"
          :title="$t('downloads.action.resumeAll')"
        >
          <Play :size="14" />
          <span class="text-xs font-bold">{{ $t('downloads.action.resumeAll') }}</span>
        </button>

        <button 
          class="btn btn-primary btn-sm rounded-xl gap-2 px-4 shadow-sm"
          @click="showAddModal = true"
        >
          <Plus :size="15" />
          <span class="text-xs font-bold">{{ $t('downloads.action.newDownload') }}</span>
        </button>

        <label class="flex items-center gap-1.5 cursor-pointer text-xs font-bold border border-base-300/60 px-3 py-2 rounded-xl hover:bg-base-200/40 transition-colors">
          <input type="checkbox" v-model="autoRefresh" class="checkbox checkbox-primary checkbox-xs rounded" />
          <span>{{ $t('downloads.action.autoRefresh') }} (4s)</span>
        </label>
        <button 
          class="btn btn-ghost btn-sm rounded-xl gap-2 px-3 border border-base-300/60 shadow-sm" 
          @click="fetchDownloads"
          :disabled="activeReads > 0"
        >
          <RefreshCw :size="14" :class="{ 'animate-spin': activeReads > 0 }" />
          <span class="text-xs font-bold">{{ $t('downloads.refresh') }}</span>
        </button>
      </div>
    </div>

    <!-- 概览状态卡片群 -->
    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 sm:gap-4">
      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5 min-w-0">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider block truncate">{{ $t('downloads.stats.speedDown') }}</span>
          <p class="text-base sm:text-xl font-black font-mono text-primary truncate">{{ formatSpeed(totalSpeedDown) }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
          <Zap :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5 min-w-0">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider block truncate">{{ $t('downloads.stats.speedUp') }}</span>
          <p class="text-base sm:text-xl font-black font-mono text-success truncate">{{ formatSpeed(totalSpeedUp) }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-success/10 text-success flex items-center justify-center shrink-0">
          <ArrowUp :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5 min-w-0">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider block truncate">{{ $t('downloads.stats.downloading') }}</span>
          <p class="text-base sm:text-xl font-black font-mono text-primary truncate">{{ downloadingCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
          <Download :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5 min-w-0">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider block truncate">{{ $t('downloads.stats.seeding') }}</span>
          <p class="text-base sm:text-xl font-black font-mono text-success truncate">{{ seedingCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-success/10 text-success flex items-center justify-center shrink-0">
          <Upload :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between col-span-2 sm:col-span-1">
        <div class="space-y-0.5 min-w-0">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider block truncate">{{ $t('downloads.stats.completed') }}</span>
          <p class="text-base sm:text-xl font-black font-mono text-base-content/70 truncate">{{ completedCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-base-200 text-base-content/50 flex items-center justify-center shrink-0">
          <Check :size="20" />
        </div>
      </div>
    </div>

    <!-- 筛选过滤与搜索工具栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-base-100 p-4 rounded-2xl border border-base-200/80 shadow-sm">
      <div class="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0 text-xs">
        <button 
          @click="currentFilter = 'all'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'all' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          {{ $t('downloads.action.all') }} ({{ tasks.length }})
        </button>
        <button 
          @click="currentFilter = 'downloading'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'downloading' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          {{ $t('downloads.status.downloading') }} ({{ downloadingCount }})
        </button>
        <button 
          @click="currentFilter = 'seeding'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'seeding' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          {{ $t('downloads.status.seeding') }} ({{ seedingCount }})
        </button>
        <button 
          @click="currentFilter = 'completed'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'completed' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          {{ $t('downloads.status.completed') }} ({{ completedCount }})
        </button>
        <button 
          @click="currentFilter = 'paused'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'paused' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          {{ $t('downloads.status.paused') }} ({{ tasks.filter(t => t.display_status === 'paused').length }})
        </button>
      </div>

      <div class="relative w-full sm:w-64">
        <input 
          v-model="searchQuery" 
          type="text" 
          :placeholder="$t('downloads.action.search')" :aria-label="$t('downloads.action.search')"
          class="input input-bordered input-sm w-full rounded-xl pl-8 text-xs font-medium"
        />
        <Search :size="13" class="absolute left-2.5 top-1/2 -translate-y-1/2 opacity-40" />
      </div>
    </div>

    <!-- Error Alert -->
    <div v-if="error" class="alert bg-error/10 border-error/20 text-error rounded-2xl p-4 shadow-sm flex items-center justify-between">
      <div class="flex items-center gap-3">
        <AlertTriangle :size="20" class="shrink-0" />
        <div>
          <h4 class="font-black text-xs uppercase tracking-wider">{{ $t('downloads.error.title') }}</h4>
          <p class="text-xs opacity-80 mt-0.5">{{ error }}</p>
        </div>
      </div>
      <button class="btn btn-ghost btn-circle btn-xs" @click="error = ''">
        <X :size="14" />
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading && tasks.length === 0" class="space-y-4">
      <div v-for="i in 4" :key="i" class="h-28 bg-base-100 rounded-3xl border border-base-200/50 animate-pulse"></div>
    </div>

    <!-- Empty State -->
    <div v-else-if="!error && filteredTasks.length === 0" class="flex flex-col items-center justify-center py-24 text-center bg-base-100 rounded-3xl border border-base-200/80 shadow-sm">
      <div class="w-16 h-16 bg-base-200/70 rounded-2xl flex items-center justify-center mb-4 text-base-content/30">
        <Download :size="32" />
      </div>
      <h3 class="text-lg font-black tracking-tight mb-1">{{ searchQuery || currentFilter !== 'all' ? $t('downloads.action.noMatches') : $t('downloads.empty.title') }}</h3>
      <p class="text-xs text-base-content/50 max-w-sm mx-auto leading-relaxed">
        {{ searchQuery || currentFilter !== 'all' ? $t('downloads.action.filterHint') : $t('downloads.empty.desc') }}
      </p>
    </div>

    <!-- Download List -->
    <div v-else class="grid gap-4">
      <div
        v-for="t in filteredTasks" :key="t.hash"
        class="group bg-base-100 rounded-2xl border border-base-200/80 shadow-sm hover:border-primary/30 transition-all p-5 sm:p-6 space-y-4"
      >
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <!-- Icon & Title -->
          <div class="flex items-start gap-3.5 min-w-0 flex-1">
            <div class="w-11 h-11 rounded-xl bg-base-200/70 flex items-center justify-center shrink-0 shadow-inner"
              :class="t.display_status === 'downloading' ? 'text-primary' : (t.display_status === 'seeding' || t.display_status === 'completed' ? 'text-success' : 'text-base-content/50')">
              <component :is="statusInfo(t.display_status).icon" :size="22" />
            </div>

            <div class="min-w-0 flex-1 space-y-1">
              <h3 class="text-sm font-black tracking-tight truncate group-hover:text-primary transition-colors select-all" :title="t.name">
                {{ t.name }}
              </h3>
              <div class="flex items-center gap-2 text-[10px] font-mono opacity-50 truncate">
                <Folder :size="12" class="shrink-0" />
                <span class="truncate">{{ t.save_path }}</span>
                <span>•</span>
                <span class="shrink-0 font-bold uppercase">#{{ t.hash.slice(0, 8) }}</span>
              </div>
            </div>
          </div>

          <!-- Status Badge, Speed & Actions -->
          <div class="flex items-center gap-3 shrink-0 self-end sm:self-center flex-wrap sm:flex-nowrap justify-end">
            <!-- Down speed -->
            <div v-if="t.speed_down > 0 || t.display_status === 'downloading'" class="text-right">
              <span class="text-[9px] font-black uppercase text-primary tracking-wider flex items-center justify-end gap-0.5">
                <ArrowDown :size="10" /> {{ formatSpeed(t.speed_down) }}
              </span>
            </div>
            <!-- Up speed -->
            <div v-if="t.speed_up && t.speed_up > 0" class="text-right">
              <span class="text-[9px] font-black uppercase text-success tracking-wider flex items-center justify-end gap-0.5">
                <ArrowUp :size="10" /> {{ formatSpeed(t.speed_up) }}
              </span>
            </div>

            <span class="text-[10px] font-black uppercase px-2.5 py-1 rounded-lg border font-mono" :class="statusInfo(t.display_status).cls">
              {{ statusInfo(t.display_status).label }}
            </span>

            <!-- Actions: Pause/Resume, Delete -->
            <div class="flex items-center gap-1">
              <button 
                v-if="t.can_resume"
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 text-primary hover:bg-primary/10"
                :title="$t('downloads.action.resume')"
                :disabled="batchActionLoading || changingTasks.has(t.hash)" @click="resumeTask(t)"
              >
                <Play :size="13" />
              </button>
              <button 
                v-else-if="t.can_pause"
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 hover:bg-base-200"
                :title="$t('downloads.action.pause')"
                :disabled="batchActionLoading || changingTasks.has(t.hash)" @click="pauseTask(t)"
              >
                <Pause :size="13" />
              </button>

              <button 
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 text-base-content/50 hover:bg-error/10 hover:text-error hover:border-error/20"
                :title="$t('downloads.action.delete')"
                :disabled="batchActionLoading || changingTasks.has(t.hash)" @click="openDeleteModal(t)"
              >
                <Trash2 :size="13" />
              </button>
            </div>
          </div>
        </div>

        <!-- Progress Bar & Details -->
        <div class="space-y-2 pt-1">
          <div class="flex items-center justify-between text-xs font-mono">
            <span class="text-base-content/60 font-medium">
              {{ formatSize(t.done) }} / {{ formatSize(t.size) }}
            </span>
            <div class="flex items-center gap-3">
              <span v-if="t.uploaded !== undefined && t.uploaded > 0" class="text-[10px] text-base-content/50">
                {{ $t('downloads.stats.uploaded') }}: <span class="font-bold text-base-content/70">{{ formatSize(t.uploaded) }}</span>
              </span>
              <span v-if="t.ratio !== undefined" class="text-[10px] text-base-content/50">
                {{ $t('downloads.stats.ratio') }}: <span class="font-bold text-success">{{ t.ratio.toFixed(2) }}</span>
              </span>
              <span v-if="t.seeding_time && t.seeding_time > 0" class="text-[10px] text-base-content/50 flex items-center gap-0.5">
                <Clock :size="10" /> {{ formatDuration(t.seeding_time) }}
              </span>
              <span class="font-black" :class="calcPercent(t) === 100 ? 'text-success' : 'text-primary'">
                {{ calcPercent(t) }}%
              </span>
            </div>
          </div>

          <div class="h-2 w-full bg-base-200 rounded-full overflow-hidden">
            <div 
              class="h-full rounded-full transition-all duration-500 ease-out"
              :class="t.display_status === 'downloading' ? 'bg-primary' : (t.display_status === 'seeding' || t.display_status === 'completed' ? 'bg-success' : 'bg-warning')"
              :style="{ width: `${calcPercent(t)}%` }"
            ></div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Modal -->
    <dialog ref="deleteDialog" class="modal bg-black/70 backdrop-blur-md" aria-labelledby="download-delete-title"
      @cancel="deleting && $event.preventDefault()" @close="showDeleteModal = false">
        <div class="modal-box min-w-0 max-w-md max-h-[90dvh] overflow-y-auto p-0 bg-base-100 rounded-3xl shadow-2xl border border-base-200 animate-in zoom-in-95 duration-200">
          <div class="p-6 sm:p-7 space-y-5">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-xl bg-error/10 text-error flex items-center justify-center">
                  <Trash2 :size="18" />
                </div>
                <h3 id="download-delete-title" class="text-base font-black tracking-tight">{{ $t('downloads.action.deleteTitle') }}</h3>
              </div>
              <button type="button" class="btn btn-ghost btn-circle btn-xs" :disabled="deleting" :aria-label="$t('downloads.action.cancel')" @click="showDeleteModal = false">
                <X :size="14" />
              </button>
            </div>

            <p class="text-xs text-base-content/70 leading-relaxed">
              {{ $t('downloads.action.deleteDesc', { name: taskToDelete?.name || '' }) }}
            </p>

            <div class="bg-base-200/50 p-3.5 rounded-2xl border border-base-200">
              <label class="flex items-center gap-2.5 cursor-pointer text-xs font-bold select-none text-base-content/80">
                <input type="checkbox" :disabled="deleting" v-model="deleteLocalFiles" class="checkbox checkbox-error checkbox-xs rounded" />
                <span>{{ $t('downloads.action.deleteFiles') }}</span>
              </label>
            </div>

            <p v-if="deleteError" role="alert" class="text-sm text-error">{{ deleteError }}</p>
            <div class="flex items-center justify-end gap-3 pt-2">
              <button 
                class="btn btn-ghost btn-sm rounded-xl text-xs font-bold" 
                @click="showDeleteModal = false"
                :disabled="deleting"
              >
                {{ $t('downloads.action.cancel') }}
              </button>
              <button 
                class="btn btn-error btn-sm rounded-xl text-xs font-bold text-error-content px-5 gap-1.5 shadow-sm"
                @click="confirmDelete"
                :disabled="deleting"
              >
                <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
                <span v-else>{{ $t('downloads.action.confirmDelete') }}</span>
              </button>
            </div>
          </div>
        </div>
    </dialog>

    <!-- New Download Modal -->
    <dialog ref="addDialog" class="modal bg-black/70 backdrop-blur-md" aria-labelledby="download-add-title"
      @cancel="adding && $event.preventDefault()" @close="showAddModal = false">
        <div class="modal-box min-w-0 max-w-lg max-h-[90dvh] overflow-y-auto p-0 bg-base-100 rounded-3xl shadow-2xl border border-base-200 animate-in zoom-in-95 duration-200">
          <form class="p-6 sm:p-7 space-y-5" @submit.prevent="submitNewDownload">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                  <Plus :size="18" />
                </div>
                <div>
                  <h3 id="download-add-title" class="text-base font-black tracking-tight">{{ $t('downloads.action.newDownload') }}</h3>
                  <p class="text-[11px] text-base-content/60">{{ $t('downloads.action.addDescription') }}</p>
                </div>
              </div>
              <button type="button" class="btn btn-ghost btn-circle btn-xs" :aria-label="$t('downloads.action.cancel')" :disabled="adding" @click="showAddModal = false">
                <X :size="14" />
              </button>
            </div>

            <div class="space-y-4">
              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70" for="download-url">{{ $t('downloads.action.link') }}</label>
                <input autofocus id="download-url" type="text" autocomplete="off" :spellcheck="false" :disabled="adding"
                  v-model="addUrl"
                  class="input input-bordered w-full rounded-2xl text-xs font-mono focus:input-primary"
                  placeholder="magnet:?xt=… / https://…/file.torrent"
                >
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70" for="download-title">{{ $t('downloads.action.taskTitle') }}</label>
                <input 
                  type="text" 
                  id="download-title" :disabled="adding" v-model="addTitle"
                  class="input input-bordered input-sm w-full rounded-xl text-xs focus:input-primary"
                  placeholder="Example 01 [1080p]"
                />
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70" for="download-directory">{{ $t('downloads.action.directory') }}</label>
                <input 
                  type="text" 
                  id="download-directory" :disabled="adding" v-model="addSavePath"
                  class="input input-bordered input-sm w-full rounded-xl text-xs focus:input-primary"
                  :placeholder="$t('downloads.action.defaultDirectory')"
                />
              </div>
            </div>

            <p v-if="addError" role="alert" class="text-sm text-error">{{ addError }}</p>
            <div class="flex items-center justify-end gap-3 pt-2">
              <button type="button"
                class="btn btn-ghost btn-sm rounded-xl text-xs font-bold" 
                @click="showAddModal = false"
                :disabled="adding"
              >
                {{ $t('downloads.action.cancel') }}
              </button>
              <button 
                class="btn btn-primary btn-sm rounded-xl text-xs font-bold px-6 gap-1.5 shadow-sm"
                type="submit"
                :disabled="adding || !addUrl.trim()"
              >
                <span v-if="adding" class="loading loading-spinner loading-xs"></span>
                <span v-else>{{ $t('downloads.action.start') }}</span>
              </button>
            </div>
          </form>
        </div>
    </dialog>
  </div>
</template>
