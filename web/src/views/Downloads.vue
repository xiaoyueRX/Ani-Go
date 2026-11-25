<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import {
  RefreshCw, AlertTriangle, X,
  Download, Pause, History,
  Upload, Check, MoreVertical,
  Folder, Search, Zap,
  Play, Trash2, ArrowUp, ArrowDown, Clock, Plus, Settings
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()
const router = useRouter()

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

const showDeleteModal = ref(false)
const taskToDelete = ref<DownloadTask | null>(null)
const deleteLocalFiles = ref(false)
const deleting = ref(false)

const activeReads = ref(0)
let nextRequest = 0
let lastApplied = 0

const isDownloaderUnconfigured = computed(() => {
  const msg = (error.value || '').toLowerCase()
  return (
    msg.includes('connection refused') ||
    msg.includes('connect') ||
    msg.includes('登录') ||
    msg.includes('未配置') ||
    msg.includes('下载器') ||
    msg.includes('无法取得')
  )
})

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

const totalSpeedDown = computed(() => tasks.value.reduce((acc, cur) => acc + (cur.speed_down || 0), 0))
const totalSpeedUp = computed(() => tasks.value.reduce((acc, cur) => acc + (cur.speed_up || 0), 0))
const downloadingCount = computed(() => tasks.value.filter(t => t.display_status === 'downloading').length)
const seedingCount = computed(() => tasks.value.filter(t => t.display_status === 'seeding').length)
const completedCount = computed(() => tasks.value.filter(t => t.complete).length)

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

function calcPercent(task: DownloadTask): number {
  if (task.complete) return 100
  const fraction = Number.isFinite(task.progress) ? task.progress : (task.size > 0 ? task.done / task.size : 0)
  return Math.min(99, Math.max(0, Math.round(fraction * 100)))
}

function statusInfo(status: string): { label: string; icon: any; cls: string } {
  const m: Record<string, { label: string; icon: any; cls: string }> = {
    downloading: { label: t('downloads.status.downloading') || '下载中', icon: Download, cls: 'bg-[var(--v2-accent)]/15 text-[var(--v2-accent)] border-[var(--v2-accent)]/25' },
    paused: { label: t('downloads.status.paused') || '已暂停', icon: Pause, cls: 'bg-amber-500/15 text-amber-400 border-amber-500/25' },
    queued: { label: t('downloads.status.queued') || '排队中', icon: History, cls: 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)]' },
    checking: { label: t('downloads.status.checking') || '校验中', icon: RefreshCw, cls: 'bg-sky-500/15 text-sky-400 border-sky-500/25' },
    seeding: { label: t('downloads.status.seeding') || '做种中', icon: Upload, cls: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/25' },
    completed: { label: t('downloads.status.completed') || '已完成', icon: Check, cls: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/25' },
    error: { label: t('downloads.status.error') || '错误', icon: AlertTriangle, cls: 'bg-red-500/15 text-red-400 border-red-500/25' },
  }
  return m[status] || { label: status, icon: MoreVertical, cls: 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)]' }
}

const changingTasks = ref(new Set<string>())
const batchActionLoading = ref(false)

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

function openDeleteModal(task: DownloadTask) {
  if (changingTasks.value.has(task.hash) || batchActionLoading.value || deleting.value) return
  deleteError.value = ''
  taskToDelete.value = task
  deleteLocalFiles.value = false
  showDeleteModal.value = true
}

const deleteError = ref('')
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

const showAddModal = ref(false)
const addUrl = ref('')
const addTitle = ref('')
const addSavePath = ref('')
const adding = ref(false)
const addError = ref('')
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
  if (val) timer = setInterval(pollDownloads, 4000)
  else if (timer) { clearInterval(timer); timer = null }
})

onMounted(() => {
  document.addEventListener('visibilitychange', pollDownloads)
  fetchDownloads()
  if (autoRefresh.value) timer = setInterval(pollDownloads, 4000)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', pollDownloads)
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="space-y-5 pb-16">
    <!-- 顶部工作室操作栏 -->
    <section class="v2-panel p-4 sm:p-5 flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <div>
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">{{ $t('downloads.title') }}</h1>
            <span class="v2-pill font-mono text-[11px]">{{ tasks.length }} {{ $t('downloads.countUnit') }}</span>
          </div>
          <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('downloads.subtitle') }}</p>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <button
          type="button"
          class="v2-btn v2-btn-sm"
          @click="pauseAllTasks"
          :disabled="batchActionLoading || changingTasks.size > 0 || !tasks.some(task => task.can_pause)"
        >
          <Pause :size="13" />
          <span>{{ $t('downloads.action.pauseAll') }}</span>
        </button>

        <button
          type="button"
          class="v2-btn v2-btn-sm"
          @click="resumeAllTasks"
          :disabled="batchActionLoading || changingTasks.size > 0 || !tasks.some(task => task.can_resume)"
        >
          <Play :size="13" />
          <span>{{ $t('downloads.action.resumeAll') }}</span>
        </button>

        <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="showAddModal = true">
          <Plus :size="14" />
          <span>{{ $t('downloads.action.newDownload') }}</span>
        </button>

        <label class="flex items-center gap-1.5 cursor-pointer text-xs font-medium border border-[var(--v2-border)] bg-[var(--v2-surface-2)] px-2.5 h-[1.9rem] rounded-lg">
          <input type="checkbox" v-model="autoRefresh" class="checkbox checkbox-primary checkbox-xs" />
          <span>4s</span>
        </label>

        <button type="button" class="v2-btn v2-btn-sm" @click="fetchDownloads" :disabled="activeReads > 0">
          <RefreshCw :size="13" :class="{ 'animate-spin': activeReads > 0 }" />
          <span>{{ $t('downloads.refresh') }}</span>
        </button>
      </div>
    </section>

    <!-- 4 卡实时遥测条 -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3.5">
      <div class="v2-panel p-4 flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-medium text-[var(--v2-ink-muted)]">{{ $t('downloads.stats.speedDown') }}</div>
          <div class="text-xl font-bold font-mono tabular-nums text-[var(--v2-accent)] mt-1">{{ formatSpeed(totalSpeedDown) }}</div>
        </div>
        <div class="w-10 h-10 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
          <Zap :size="18" />
        </div>
      </div>

      <div class="v2-panel p-4 flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-medium text-[var(--v2-ink-muted)]">{{ $t('downloads.stats.speedUp') }}</div>
          <div class="text-xl font-bold font-mono tabular-nums text-emerald-400 mt-1">{{ formatSpeed(totalSpeedUp) }}</div>
        </div>
        <div class="w-10 h-10 rounded-xl bg-emerald-500/12 text-emerald-400 flex items-center justify-center shrink-0">
          <ArrowUp :size="18" />
        </div>
      </div>

      <div class="v2-panel p-4 flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-medium text-[var(--v2-ink-muted)]">{{ $t('downloads.stats.downloading') }} / {{ $t('downloads.stats.seeding') }}</div>
          <div class="text-xl font-bold font-mono tabular-nums text-[var(--v2-ink)] mt-1">
            {{ downloadingCount }} <span class="text-xs font-normal text-[var(--v2-ink-subtle)]">/ {{ seedingCount }}</span>
          </div>
        </div>
        <div class="w-10 h-10 rounded-xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] flex items-center justify-center shrink-0">
          <Download :size="18" />
        </div>
      </div>

      <div class="v2-panel p-4 flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-medium text-[var(--v2-ink-muted)]">{{ $t('downloads.stats.completed') }}</div>
          <div class="text-xl font-bold font-mono tabular-nums text-[var(--v2-ink)] mt-1">{{ completedCount }}</div>
        </div>
        <div class="w-10 h-10 rounded-xl bg-[var(--v2-surface-2)] text-emerald-400 flex items-center justify-center shrink-0">
          <Check :size="18" />
        </div>
      </div>
    </div>

    <!-- 分段筛选与搜索 -->
    <div class="v2-panel p-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
      <div class="inline-flex flex-wrap items-center gap-1 p-1 rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)]">
        <button
          v-for="tab in [
            { id: 'all', label: $t('downloads.action.all'), count: tasks.length },
            { id: 'downloading', label: $t('downloads.status.downloading'), count: downloadingCount },
            { id: 'seeding', label: $t('downloads.status.seeding'), count: seedingCount },
            { id: 'completed', label: $t('downloads.status.completed'), count: completedCount },
            { id: 'paused', label: $t('downloads.status.paused'), count: tasks.filter(t => t.display_status === 'paused').length }
          ]"
          :key="tab.id"
          type="button"
          class="px-3 py-1 rounded-lg text-xs font-semibold transition-all cursor-pointer"
          :class="currentFilter === tab.id ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)] shadow-xs' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
          @click="currentFilter = tab.id as any"
        >
          {{ tab.label }} ({{ tab.count }})
        </button>
      </div>

      <div class="relative w-full sm:w-64">
        <Search :size="14" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
        <input
          v-model="searchQuery"
          type="text"
          :placeholder="$t('downloads.action.search')"
          :aria-label="$t('downloads.action.search')"
          class="v2-input pl-8 h-8 text-xs"
        />
      </div>
    </div>

    <!-- 未配置下载器引导卡 或 普通错误 -->
    <div v-if="error && isDownloaderUnconfigured" class="v2-panel p-8 sm:p-10 text-center space-y-4 border border-amber-500/25">
      <div class="w-12 h-12 rounded-2xl bg-amber-500/15 text-amber-400 flex items-center justify-center mx-auto">
        <Download :size="22" />
      </div>
      <div class="space-y-1 max-w-md mx-auto">
        <h3 class="text-base font-bold text-[var(--v2-ink)]">
          {{ locale === 'zh' ? '尚未连接到下载客户端' : 'Downloader Client Not Connected' }}
        </h3>
        <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">
          {{ locale === 'zh'
            ? '当前无法连接到 qBittorrent / Transmission / Aria2。前往设置填好地址与凭据并完成联通测试，即可在此管理传输队列。'
            : 'Unable to reach your configured downloader client. Configure qBittorrent, Transmission, or Aria2 in Settings.' }}
        </p>
      </div>
      <div class="flex items-center justify-center gap-2.5 pt-1">
        <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="router.push('/settings?tab=downloader')">
          <Settings :size="14" />
          <span>{{ locale === 'zh' ? '前往配置下载器' : 'Configure Downloader' }}</span>
        </button>
        <button type="button" class="v2-btn v2-btn-sm" @click="fetchDownloads">
          <RefreshCw :size="13" />
          <span>{{ locale === 'zh' ? '重试连接' : 'Retry' }}</span>
        </button>
      </div>
      <details class="text-[11px] text-[var(--v2-ink-subtle)] font-mono max-w-xl mx-auto">
        <summary class="cursor-pointer hover:text-[var(--v2-ink-muted)]">{{ locale === 'zh' ? '查看原始诊断信息' : 'Raw diagnostic details' }}</summary>
        <p class="mt-2 p-2.5 rounded-lg bg-[var(--v2-surface-2)] text-left break-all">{{ error }}</p>
      </details>
    </div>

    <div v-else-if="error" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 p-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-3 min-w-0">
        <AlertTriangle :size="18" class="shrink-0" />
        <div class="min-w-0">
          <h4 class="font-semibold text-xs">{{ $t('downloads.error.title') }}</h4>
          <p class="text-xs opacity-85 mt-0.5 truncate">{{ error }}</p>
        </div>
      </div>
      <button type="button" class="p-1 rounded hover:bg-red-500/20" @click="error = ''">
        <X :size="14" />
      </button>
    </div>

    <!-- 加载骨架 -->
    <div v-else-if="loading && tasks.length === 0" class="space-y-3">
      <div v-for="i in 4" :key="i" class="h-20 v2-panel animate-pulse"></div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="filteredTasks.length === 0" class="v2-panel py-14 px-6 text-center space-y-2.5">
      <div class="w-11 h-11 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] flex items-center justify-center mx-auto">
        <Download :size="20" />
      </div>
      <h3 class="text-sm font-bold text-[var(--v2-ink)]">
        {{ searchQuery || currentFilter !== 'all' ? $t('downloads.action.noMatches') : $t('downloads.empty.title') }}
      </h3>
      <p class="text-xs text-[var(--v2-ink-muted)]">
        {{ searchQuery || currentFilter !== 'all' ? $t('downloads.action.filterHint') : $t('downloads.empty.desc') }}
      </p>
    </div>

    <!-- 传输队列表格/高密度行 -->
    <div v-else class="v2-panel overflow-hidden divide-y divide-[var(--v2-border)]">
      <div
        v-for="task in filteredTasks"
        :key="task.hash"
        class="p-4 sm:px-5 hover:bg-[var(--v2-surface-2)]/50 transition-colors space-y-3"
      >
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div class="flex items-start gap-3 min-w-0 flex-1">
            <div
              class="w-9 h-9 rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)] flex items-center justify-center shrink-0"
              :class="task.display_status === 'downloading' ? 'text-[var(--v2-accent)]' : (task.display_status === 'seeding' || task.display_status === 'completed' ? 'text-emerald-400' : 'text-[var(--v2-ink-muted)]')"
            >
              <component :is="statusInfo(task.display_status).icon" :size="17" />
            </div>

            <div class="min-w-0 flex-1 space-y-1">
              <h3 class="text-sm font-semibold truncate text-[var(--v2-ink)] select-all" :title="task.name">{{ task.name }}</h3>
              <div class="flex items-center gap-2 text-[11px] font-mono text-[var(--v2-ink-subtle)] truncate">
                <Folder :size="11" class="shrink-0" />
                <span class="truncate">{{ task.save_path }}</span>
                <span>•</span>
                <span class="shrink-0">#{{ task.hash.slice(0, 8) }}</span>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-2.5 shrink-0 flex-wrap justify-end">
            <span v-if="task.speed_down > 0 || task.display_status === 'downloading'" class="text-xs font-mono font-semibold text-[var(--v2-accent)] flex items-center gap-0.5 tabular-nums">
              <ArrowDown :size="12" /> {{ formatSpeed(task.speed_down) }}
            </span>
            <span v-if="task.speed_up && task.speed_up > 0" class="text-xs font-mono font-semibold text-emerald-400 flex items-center gap-0.5 tabular-nums">
              <ArrowUp :size="12" /> {{ formatSpeed(task.speed_up) }}
            </span>

            <span class="text-[11px] font-mono font-semibold px-2.5 py-0.5 rounded-full border" :class="statusInfo(task.display_status).cls">
              {{ statusInfo(task.display_status).label }}
            </span>

            <div class="flex items-center gap-1">
              <button
                v-if="task.can_resume"
                type="button"
                class="v2-btn v2-btn-sm w-7 h-7 px-0 text-[var(--v2-accent)]"
                :title="$t('downloads.action.resume')"
                :disabled="batchActionLoading || changingTasks.has(task.hash)"
                @click="resumeTask(task)"
              >
                <Play :size="12" />
              </button>
              <button
                v-else-if="task.can_pause"
                type="button"
                class="v2-btn v2-btn-sm w-7 h-7 px-0"
                :title="$t('downloads.action.pause')"
                :disabled="batchActionLoading || changingTasks.has(task.hash)"
                @click="pauseTask(task)"
              >
                <Pause :size="12" />
              </button>
              <button
                type="button"
                class="v2-btn v2-btn-sm w-7 h-7 px-0 hover:text-red-400 hover:border-red-500/40"
                :title="$t('downloads.action.delete')"
                :disabled="batchActionLoading || changingTasks.has(task.hash)"
                @click="openDeleteModal(task)"
              >
                <Trash2 :size="12" />
              </button>
            </div>
          </div>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between text-[11px] font-mono tabular-nums text-[var(--v2-ink-muted)]">
            <span>{{ formatSize(task.done) }} / {{ formatSize(task.size) }}</span>
            <div class="flex items-center gap-3">
              <span v-if="task.uploaded !== undefined && task.uploaded > 0">{{ $t('downloads.stats.uploaded') }}: {{ formatSize(task.uploaded) }}</span>
              <span v-if="task.ratio !== undefined">{{ $t('downloads.stats.ratio') }}: <span class="text-emerald-400">{{ task.ratio.toFixed(2) }}</span></span>
              <span v-if="task.seeding_time && task.seeding_time > 0" class="flex items-center gap-1"><Clock :size="10" /> {{ formatDuration(task.seeding_time) }}</span>
              <span class="font-bold" :class="calcPercent(task) === 100 ? 'text-emerald-400' : 'text-[var(--v2-accent)]'">{{ calcPercent(task) }}%</span>
            </div>
          </div>
          <div class="h-1.5 w-full bg-[var(--v2-surface-3)] rounded-full overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-500"
              :class="task.display_status === 'downloading' ? 'bg-[var(--v2-accent)]' : (task.display_status === 'seeding' || task.display_status === 'completed' ? 'bg-emerald-500' : 'bg-amber-500')"
              :style="{ width: `${calcPercent(task)}%` }"
            ></div>
          </div>
        </div>
      </div>
    </div>

    <!-- 删除任务弹窗 -->
    <dialog ref="deleteDialog" class="modal" aria-labelledby="download-delete-title" @cancel="deleting && $event.preventDefault()" @close="showDeleteModal = false">
      <div class="modal-box max-w-md">
        <div class="flex items-center justify-between mb-3">
          <h3 id="download-delete-title" class="text-base font-bold">{{ $t('downloads.action.deleteTitle') }}</h3>
          <button type="button" class="p-1 rounded hover:bg-[var(--v2-surface-2)]" :disabled="deleting" @click="showDeleteModal = false"><X :size="14" /></button>
        </div>
        <p class="text-xs text-[var(--v2-ink-muted)] mb-4 leading-relaxed">{{ $t('downloads.action.deleteDesc', { name: taskToDelete?.name || '' }) }}</p>
        <label class="flex items-center gap-2.5 p-3 v2-panel-muted cursor-pointer mb-4">
          <input type="checkbox" :disabled="deleting" v-model="deleteLocalFiles" class="checkbox checkbox-error checkbox-sm" />
          <span class="text-xs font-semibold">{{ $t('downloads.action.deleteFiles') }}</span>
        </label>
        <p v-if="deleteError" role="alert" class="text-xs text-red-400 mb-3">{{ deleteError }}</p>
        <div class="flex items-center justify-end gap-2">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="showDeleteModal = false" :disabled="deleting">{{ $t('downloads.action.cancel') }}</button>
          <button type="button" class="v2-btn v2-btn-sm bg-red-500 border-red-500 text-white" @click="confirmDelete" :disabled="deleting">
            <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
            <span v-else>{{ $t('downloads.action.confirmDelete') }}</span>
          </button>
        </div>
      </div>
      <div class="modal-backdrop" @click="!deleting && (showDeleteModal = false)"></div>
    </dialog>

    <!-- 新建下载弹窗 -->
    <dialog ref="addDialog" class="modal" aria-labelledby="download-add-title" @cancel="adding && $event.preventDefault()" @close="showAddModal = false">
      <div class="modal-box max-w-lg">
        <form class="space-y-4" @submit.prevent="submitNewDownload">
          <div class="flex items-center justify-between">
            <div>
              <h3 id="download-add-title" class="text-base font-bold">{{ $t('downloads.action.newDownload') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)]">{{ $t('downloads.action.addDescription') }}</p>
            </div>
            <button type="button" class="p-1 rounded hover:bg-[var(--v2-surface-2)]" :disabled="adding" @click="showAddModal = false"><X :size="14" /></button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1" for="download-url">{{ $t('downloads.action.link') }}</label>
              <input id="download-url" type="text" autocomplete="off" :spellcheck="false" :disabled="adding" v-model="addUrl" class="v2-input font-mono text-xs" placeholder="magnet:?xt=… / https://…/file.torrent" />
            </div>
            <div>
              <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1" for="download-title">{{ $t('downloads.action.taskTitle') }}</label>
              <input id="download-title" type="text" :disabled="adding" v-model="addTitle" class="v2-input text-xs" placeholder="Example 01 [1080p]" />
            </div>
            <div>
              <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1" for="download-directory">{{ $t('downloads.action.directory') }}</label>
              <input id="download-directory" type="text" :disabled="adding" v-model="addSavePath" class="v2-input text-xs" :placeholder="$t('downloads.action.defaultDirectory')" />
            </div>
          </div>
          <p v-if="addError" role="alert" class="text-xs text-red-400">{{ addError }}</p>
          <div class="flex items-center justify-end gap-2 pt-1">
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="showAddModal = false" :disabled="adding">{{ $t('downloads.action.cancel') }}</button>
            <button class="v2-btn v2-btn-primary v2-btn-sm" type="submit" :disabled="adding || !addUrl.trim()">
              <span v-if="adding" class="loading loading-spinner loading-xs"></span>
              <span v-else>{{ $t('downloads.action.start') }}</span>
            </button>
          </div>
        </form>
      </div>
      <div class="modal-backdrop" @click="!adding && (showAddModal = false)"></div>
    </dialog>
  </div>
</template>
