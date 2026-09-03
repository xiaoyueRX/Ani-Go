<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import request from '../utils/request'
import { 
  RefreshCw, AlertTriangle, X, 
  Download, Pause, History, 
  Upload, Check, MoreVertical, 
  Folder, Search, Zap, Activity
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
  size: number
  done: number
}

const tasks = ref<DownloadTask[]>([])
const loading = ref(true)
const error = ref('')
const searchQuery = ref('')
const currentFilter = ref<'all' | 'downloading' | 'seeding' | 'completed' | 'paused'>('all')
const autoRefresh = ref(true)
let timer: any = null

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
      
      <div class="flex items-center gap-3 flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer text-xs font-bold border border-base-300/60 px-3 py-2 rounded-xl hover:bg-base-200/40 transition-colors">
          <input type="checkbox" v-model="autoRefresh" class="checkbox checkbox-primary checkbox-xs rounded" />
          <span>自动轮询 (4s)</span>
        </label>
        <button 
          class="btn btn-primary btn-sm rounded-xl gap-2 px-5 shadow-sm" 
          @click="fetchDownloads"
          :disabled="loading"
        >
          <RefreshCw :size="14" :class="{ 'animate-spin': loading }" />
          <span class="text-xs font-bold">{{ $t('downloads.refresh') }}</span>
        </button>
      </div>
    </div>

    <!-- 概览状态卡片群 -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">总下载速率</span>
          <p class="text-lg sm:text-xl font-black font-mono text-primary">{{ formatSpeed(totalSpeedDown) }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
          <Zap :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">正在下载</span>
          <p class="text-lg sm:text-xl font-black font-mono text-primary">{{ downloadingCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
          <Download :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">正在做种</span>
          <p class="text-lg sm:text-xl font-black font-mono text-success">{{ seedingCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-success/10 text-success flex items-center justify-center">
          <Upload :size="20" />
        </div>
      </div>

      <div class="bg-base-100 p-4 sm:p-5 rounded-2xl border border-base-200/80 shadow-sm flex items-center justify-between">
        <div class="space-y-0.5">
          <span class="text-[11px] font-bold text-base-content/50 uppercase tracking-wider">累计完成</span>
          <p class="text-lg sm:text-xl font-black font-mono text-base-content/70">{{ completedCount }}</p>
        </div>
        <div class="w-10 h-10 rounded-xl bg-base-200 text-base-content/50 flex items-center justify-center">
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

          <!-- Status Badge & Speed -->
          <div class="flex items-center gap-3 shrink-0 self-end sm:self-center">
            <div v-if="t.status === 'downloading'" class="text-right">
              <span class="text-[10px] font-black uppercase text-primary tracking-wider block animate-pulse">下载中</span>
              <span class="text-xs font-mono font-black text-primary">{{ formatSpeed(t.speed_down) }}</span>
            </div>
            <span class="text-[10px] font-black uppercase px-3 py-1 rounded-lg border font-mono" :class="statusInfo(t.status).cls">
              {{ statusInfo(t.status).label }}
            </span>
          </div>
        </div>

        <!-- Progress Bar & Details -->
        <div class="space-y-2 pt-1">
          <div class="flex items-center justify-between text-xs font-mono">
            <span class="text-base-content/60 font-medium">
              {{ formatSize(t.done) }} / {{ formatSize(t.size) }}
            </span>
            <span class="font-black" :class="calcPercent(t) === 100 ? 'text-success' : 'text-primary'">
              {{ calcPercent(t) }}%
            </span>
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
  </div>
</template>
