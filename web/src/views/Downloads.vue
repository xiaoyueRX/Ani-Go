<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import request from '../utils/request'
import { 
  RefreshCw, AlertTriangle, X, 
  Download, Pause, History, 
  Upload, Check, MoreVertical, 
  Folder, Search, Zap, Activity,
  Play, Trash2, ArrowUp, ArrowDown, Clock, Plus
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface DownloadTask {
  hash: string
  name: string
  save_path: string
  status: string
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

async function fetchDownloads() {
  error.value = ''
  try {
    const { data } = await request.get('/downloads')
    tasks.value = Array.isArray(data) ? data : []
  } catch (e: any) {
    error.value = e.response?.data?.error || '获取下载列表失败'
  } finally {
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
  return tasks.value.filter(t => t.status === 'downloading').length
})

const seedingCount = computed(() => {
  return tasks.value.filter(t => t.status === 'seeding').length
})

const completedCount = computed(() => {
  return tasks.value.filter(t => t.status === 'completed' || t.status === 'seeding').length
})

// 过滤后的列表
const filteredTasks = computed(() => {
  let list = tasks.value

  if (currentFilter.value !== 'all') {
    list = list.filter(t => t.status === currentFilter.value)
  }

  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase()
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
  if (t.size > 0 && t.done >= 0) {
    const p = Math.round((t.done / t.size) * 100)
    return Math.min(100, Math.max(0, p))
  }
  if (typeof t.progress === 'number') {
    const p = t.progress > 1 ? Math.round(t.progress) : Math.round(t.progress * 100)
    return Math.min(100, Math.max(0, p))
  }
  return 0
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
async function pauseTask(t: DownloadTask) {
  try {
    await request.post(`/downloads/${t.hash}/pause`)
    window.showToast?.(t('downloads.action.pausedSuccess') || '任务已暂停', 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '暂停任务失败', 'error')
  }
}

async function resumeTask(t: DownloadTask) {
  try {
    await request.post(`/downloads/${t.hash}/resume`)
    window.showToast?.(t('downloads.action.resumedSuccess') || '任务已恢复', 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '恢复任务失败', 'error')
  }
}

function openDeleteModal(t: DownloadTask) {
  taskToDelete.value = t
  deleteLocalFiles.value = false
  showDeleteModal.value = true
}

async function confirmDelete() {
  if (!taskToDelete.value) return
  deleting.value = true
  try {
    await request.delete(`/downloads/${taskToDelete.value.hash}`, {
      params: { delete_files: deleteLocalFiles.value }
    })
    window.showToast?.(t('downloads.action.deletedSuccess') || '任务已删除', 'success')
    showDeleteModal.value = false
    taskToDelete.value = null
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '删除任务失败', 'error')
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

async function submitNewDownload() {
  const target = addUrl.value.trim()
  if (!target) {
    window.showToast?.('请提供磁力链接或种子下载链接', 'warning')
    return
  }
  adding.value = true
  try {
    const isMagnet = target.startsWith('magnet:')
    await request.post('/downloads', {
      url: isMagnet ? '' : target,
      magnet: isMagnet ? target : '',
      title: addTitle.value.trim() || undefined,
      save_path: addSavePath.value.trim() || undefined,
    })
    window.showToast?.('已成功推送到下载核心', 'success')
    showAddModal.value = false
    addUrl.value = ''
    addTitle.value = ''
    addSavePath.value = ''
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '添加下载任务失败', 'error')
  } finally {
    adding.value = false
  }
}

// 批量暂停 / 恢复所有任务
const batchActionLoading = ref(false)

async function pauseAllTasks() {
  if (!confirm('确定要暂停所有活跃下载任务吗？')) return
  batchActionLoading.value = true
  try {
    const { data } = await request.post('/downloads/pause-all')
    window.showToast?.(data.message || '已暂停所有任务', 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '批量暂停失败', 'error')
  } finally {
    batchActionLoading.value = false
  }
}

async function resumeAllTasks() {
  batchActionLoading.value = true
  try {
    const { data } = await request.post('/downloads/resume-all')
    window.showToast?.(data.message || '已恢复所有任务', 'success')
    fetchDownloads()
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '批量恢复失败', 'error')
  } finally {
    batchActionLoading.value = false
  }
}

watch(autoRefresh, (val) => {
  if (val) {
    timer = setInterval(fetchDownloads, 4000)
  } else if (timer) {
    clearInterval(timer)
    timer = null
  }
})

onMounted(() => {
  fetchDownloads()
  if (autoRefresh.value) {
    timer = setInterval(fetchDownloads, 4000)
  }
})

onUnmounted(() => {
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
          <span class="badge badge-neutral text-xs font-mono font-bold">{{ tasks.length }} 个任务</span>
        </div>
        <p class="text-xs text-base-content/60 font-medium">{{ $t('downloads.subtitle') }} · 实时同步下载器任务进度</p>
      </div>
      
      <div class="flex items-center gap-2.5 flex-wrap">
        <button 
          class="btn btn-outline btn-sm rounded-xl gap-1.5 px-3 border-base-300 hover:bg-base-200"
          @click="pauseAllTasks"
          :disabled="batchActionLoading || downloadingCount === 0"
          title="暂停全部下载任务"
        >
          <Pause :size="14" />
          <span class="text-xs font-bold">全部暂停</span>
        </button>

        <button 
          class="btn btn-outline btn-sm rounded-xl gap-1.5 px-3 border-base-300 hover:bg-base-200"
          @click="resumeAllTasks"
          :disabled="batchActionLoading"
          title="恢复全部暂停任务"
        >
          <Play :size="14" />
          <span class="text-xs font-bold">全部继续</span>
        </button>

        <button 
          class="btn btn-primary btn-sm rounded-xl gap-2 px-4 shadow-sm"
          @click="showAddModal = true"
        >
          <Plus :size="15" />
          <span class="text-xs font-bold">新建下载</span>
        </button>

        <label class="flex items-center gap-1.5 cursor-pointer text-xs font-bold border border-base-300/60 px-3 py-2 rounded-xl hover:bg-base-200/40 transition-colors">
          <input type="checkbox" v-model="autoRefresh" class="checkbox checkbox-primary checkbox-xs rounded" />
          <span>自动轮询 (4s)</span>
        </label>
        <button 
          class="btn btn-ghost btn-sm rounded-xl gap-2 px-3 border border-base-300/60 shadow-sm" 
          @click="fetchDownloads"
          :disabled="loading"
        >
          <RefreshCw :size="14" :class="{ 'animate-spin': loading }" />
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
          全部 ({{ tasks.length }})
        </button>
        <button 
          @click="currentFilter = 'downloading'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'downloading' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          下载中 ({{ downloadingCount }})
        </button>
        <button 
          @click="currentFilter = 'seeding'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'seeding' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          做种中 ({{ seedingCount }})
        </button>
        <button 
          @click="currentFilter = 'completed'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'completed' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          已完成 ({{ tasks.filter(t => t.status === 'completed').length }})
        </button>
        <button 
          @click="currentFilter = 'paused'" 
          class="btn btn-xs sm:btn-sm rounded-xl font-bold"
          :class="currentFilter === 'paused' ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
        >
          已暂停 ({{ tasks.filter(t => t.status === 'paused').length }})
        </button>
      </div>

      <div class="relative w-full sm:w-64">
        <input 
          v-model="searchQuery" 
          type="text" 
          placeholder="按番剧名称或 Hash 筛选..." 
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
    <div v-else-if="filteredTasks.length === 0" class="flex flex-col items-center justify-center py-24 text-center bg-base-100 rounded-3xl border border-base-200/80 shadow-sm">
      <div class="w-16 h-16 bg-base-200/70 rounded-2xl flex items-center justify-center mb-4 text-base-content/30">
        <Download :size="32" />
      </div>
      <h3 class="text-lg font-black tracking-tight mb-1">{{ searchQuery || currentFilter !== 'all' ? '未找到符合条件的下载任务' : $t('downloads.empty.title') }}</h3>
      <p class="text-xs text-base-content/50 max-w-sm mx-auto leading-relaxed">
        {{ searchQuery || currentFilter !== 'all' ? '请尝试更换搜索关键字或清除状态过滤器' : $t('downloads.empty.desc') }}
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
              :class="t.status === 'downloading' ? 'text-primary' : (t.status === 'seeding' || t.status === 'completed' ? 'text-success' : 'text-base-content/50')">
              <component :is="statusInfo(t.status).icon" :size="22" />
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
            <div v-if="t.speed_down > 0 || t.status === 'downloading'" class="text-right">
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

            <span class="text-[10px] font-black uppercase px-2.5 py-1 rounded-lg border font-mono" :class="statusInfo(t.status).cls">
              {{ statusInfo(t.status).label }}
            </span>

            <!-- Actions: Pause/Resume, Delete -->
            <div class="flex items-center gap-1">
              <button 
                v-if="t.status === 'paused'"
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 text-primary hover:bg-primary/10"
                :title="$t('downloads.action.resume')"
                @click="resumeTask(t)"
              >
                <Play :size="13" />
              </button>
              <button 
                v-else
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 hover:bg-base-200"
                :title="$t('downloads.action.pause')"
                @click="pauseTask(t)"
              >
                <Pause :size="13" />
              </button>

              <button 
                class="btn btn-ghost btn-xs btn-square rounded-lg border border-base-200 text-base-content/50 hover:bg-error/10 hover:text-error hover:border-error/20"
                :title="$t('downloads.action.delete')"
                @click="openDeleteModal(t)"
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
              :class="t.status === 'downloading' ? 'bg-primary' : (t.status === 'seeding' || t.status === 'completed' ? 'bg-success' : 'bg-warning')"
              :style="{ width: `${calcPercent(t)}%` }"
            ></div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Modal -->
    <Transition name="scale">
      <div v-if="showDeleteModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-md">
        <div class="w-full max-w-md bg-base-100 rounded-3xl shadow-2xl border border-base-200 overflow-hidden animate-in zoom-in-95 duration-200">
          <div class="p-6 sm:p-7 space-y-5">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-xl bg-error/10 text-error flex items-center justify-center">
                  <Trash2 :size="18" />
                </div>
                <h3 class="text-base font-black tracking-tight">{{ $t('downloads.action.deleteTitle') }}</h3>
              </div>
              <button class="btn btn-ghost btn-circle btn-xs" @click="showDeleteModal = false">
                <X :size="14" />
              </button>
            </div>

            <p class="text-xs text-base-content/70 leading-relaxed">
              {{ $t('downloads.action.deleteDesc', { name: taskToDelete?.name || '' }) }}
            </p>

            <div class="bg-base-200/50 p-3.5 rounded-2xl border border-base-200">
              <label class="flex items-center gap-2.5 cursor-pointer text-xs font-bold select-none text-base-content/80">
                <input type="checkbox" v-model="deleteLocalFiles" class="checkbox checkbox-error checkbox-xs rounded" />
                <span>{{ $t('downloads.action.deleteFiles') }}</span>
              </label>
            </div>

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
      </div>
    </Transition>

    <!-- New Download Modal -->
    <Transition name="scale">
      <div v-if="showAddModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-md">
        <div class="w-full max-w-lg bg-base-100 rounded-3xl shadow-2xl border border-base-200 overflow-hidden animate-in zoom-in-95 duration-200">
          <div class="p-6 sm:p-7 space-y-5">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                  <Plus :size="18" />
                </div>
                <div>
                  <h3 class="text-base font-black tracking-tight">新建下载任务</h3>
                  <p class="text-[11px] text-base-content/60">直接向当前下载器核心推送 Magnet 或 Torrent 链接</p>
                </div>
              </div>
              <button class="btn btn-ghost btn-circle btn-xs" @click="showAddModal = false">
                <X :size="14" />
              </button>
            </div>

            <div class="space-y-4">
              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70">下载链接 / 磁力链 *</label>
                <textarea 
                  v-model="addUrl"
                  rows="3"
                  class="textarea textarea-bordered w-full rounded-2xl text-xs font-mono resize-none focus:textarea-primary"
                  placeholder="magnet:?xt=urn:btih:... 或 http(s)://... .torrent"
                ></textarea>
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70">自定义任务标题 (选填)</label>
                <input 
                  type="text" 
                  v-model="addTitle"
                  class="input input-bordered input-sm w-full rounded-xl text-xs focus:input-primary"
                  placeholder="如：葬送的芙莉莲 01 [1080p]"
                />
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-bold text-base-content/70">自定义保存路径 (选填)</label>
                <input 
                  type="text" 
                  v-model="addSavePath"
                  class="input input-bordered input-sm w-full rounded-xl text-xs focus:input-primary"
                  placeholder="留空则使用下载器默认下载目录"
                />
              </div>
            </div>

            <div class="flex items-center justify-end gap-3 pt-2">
              <button 
                class="btn btn-ghost btn-sm rounded-xl text-xs font-bold" 
                @click="showAddModal = false"
                :disabled="adding"
              >
                {{ $t('downloads.action.cancel') }}
              </button>
              <button 
                class="btn btn-primary btn-sm rounded-xl text-xs font-bold px-6 gap-1.5 shadow-sm"
                @click="submitNewDownload"
                :disabled="adding || !addUrl.trim()"
              >
                <span v-if="adding" class="loading loading-spinner loading-xs"></span>
                <span v-else>立即下载</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>
