<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import request from '../utils/request'
import { 
  RefreshCw, AlertTriangle, X, 
  Download, Pause, History, 
  Upload, Check, MoreVertical, 
  Folder 
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

async function fetchDownloads() {
  error.value = ''
  try {
    const { data } = await request.get('/downloads')
    tasks.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '获取下载列表失败'
  } finally {
    loading.value = false
  }
}

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

function statusInfo(status: string): { label: string; icon: string; cls: string } {
  const m: Record<string, { label: string; icon: any; cls: string }> = {
    downloading: { label: t('downloads.status.downloading'), icon: Download, cls: 'bg-primary/20 text-primary border-primary/20' },
    paused: { label: t('downloads.status.paused'), icon: Pause, cls: 'bg-warning/20 text-warning border-warning/20' },
    queued: { label: t('downloads.status.queued'), icon: History, cls: 'bg-base-300 text-base-content/40 border-base-300' },
    checking: { label: t('downloads.status.checking'), icon: RefreshCw, cls: 'bg-info/20 text-info border-info/20' },
    seeding: { label: t('downloads.status.seeding'), icon: Upload, cls: 'bg-success/20 text-success border-success/20' },
    completed: { label: t('downloads.status.completed'), icon: Check, cls: 'bg-success/20 text-success border-success/20' },
    error: { label: t('downloads.status.error'), icon: AlertTriangle, cls: 'bg-error/20 text-error border-error/20' },
  }
  return m[status] || { label: status, icon: MoreVertical, cls: 'bg-base-300 text-base-content/40 border-base-300' }
}

let timer: ReturnType<typeof setInterval>

onMounted(() => {
  fetchDownloads()
  timer = setInterval(fetchDownloads, 5000)
})

onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="space-y-10">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('downloads.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('downloads.subtitle') }}</p>
      </div>
      
      <button 
        class="btn btn-ghost border-base-300 rounded-2xl gap-3 px-6 hover:bg-base-200 transition-all active:scale-95" 
        @click="fetchDownloads"
      >
        <RefreshCw :size="20" />
        <span class="text-xs font-black uppercase tracking-widest">{{ $t('downloads.refresh') }}</span>
      </button>
    </div>

    <!-- Error Alert -->
    <div v-if="error" class="alert bg-error/10 border-error/20 text-error rounded-[2rem] p-6 shadow-xl shadow-lg">
      <AlertTriangle :size="24" class="shrink-0" />
      <div class="flex-1">
        <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('downloads.error.title') }}</h3>
        <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
      </div>
      <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
        <X :size="16" />
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading && tasks.length === 0" class="space-y-4">
      <div v-for="i in 5" :key="i" class="h-24 bg-base-100 rounded-[2rem] border border-base-200/50 animate-pulse"></div>
    </div>

    <!-- Empty State -->
    <div v-else-if="tasks.length === 0" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200">
      <div class="w-24 h-24 bg-base-200/50 rounded-full flex items-center justify-center mb-8 rotate-[-12deg]">
        <Download :size="48" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">{{ $t('downloads.empty.title') }}</h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ $t('downloads.empty.desc') }}
      </p>
    </div>

    <!-- Download List -->
    <div v-else class="grid gap-6">
      <div
        v-for="t in tasks" :key="t.hash"
        class="group relative bg-base-100 rounded-[2rem] border border-base-200/60 shadow-sm hover:shadow-xl hover:shadow-lg hover:border-primary/20 transition-all duration-500 overflow-hidden"
      >
        <!-- Background Decoration -->
        <div class="absolute -right-12 -top-12 w-32 h-32 bg-primary/5 rounded-full blur-3xl group-hover:scale-150 transition-transform duration-1000"></div>
        
        <div class="p-6 sm:p-8 relative z-10">
          <div class="flex flex-col sm:flex-row sm:items-center gap-6">
            <!-- Icon & Status -->
            <div class="flex items-center gap-4">
               <div class="w-14 h-14 rounded-2xl bg-base-200 flex items-center justify-center text-primary shadow-inner">
                  <component :is="statusInfo(t.status).icon" :size="28" />
               </div>
               <div class="sm:hidden">
                  <span class="text-[9px] font-black uppercase tracking-widest px-3 py-1.5 rounded-full border" :class="statusInfo(t.status).cls">
                    {{ statusInfo(t.status).label }}
                  </span>
               </div>
            </div>

            <!-- Meta Information -->
            <div class="flex-1 min-w-0 space-y-2">
              <h3 class="text-lg font-black tracking-tight truncate group-hover:text-primary transition-colors" :title="t.name">
                {{ t.name }}
              </h3>
              <div class="flex items-center gap-2 text-[10px] font-bold text-base-content/30 bg-base-200/50 py-1.5 px-3 rounded-lg self-start uppercase tracking-widest truncate max-w-md">
                <Folder :size="14" />
                <span class="truncate">{{ t.save_path }}</span>
              </div>
            </div>

            <!-- Status Badge (Desktop) -->
            <div class="hidden sm:flex flex-col items-end gap-2 shrink-0">
               <span class="text-[10px] font-black uppercase tracking-widest px-4 py-2 rounded-xl border transition-all duration-500 group-hover:scale-110 group-hover:rotate-2" :class="statusInfo(t.status).cls">
                 {{ statusInfo(t.status).label }}
               </span>
               <p class="text-[9px] font-black text-base-content/20 tracking-widest uppercase truncate max-w-[120px]">#{{ t.hash.slice(0, 12) }}</p>
            </div>
          </div>

          <!-- Progress Section -->
          <div class="mt-8 space-y-4">
            <div class="flex items-end justify-between">
              <div class="space-y-1">
                 <p class="text-[9px] font-black uppercase tracking-widest text-base-content/40">{{ $t('downloads.processed') }}</p>
                 <p class="text-sm font-black">{{ formatSize(t.done) }} <span class="text-base-content/20 mx-1">/</span> {{ formatSize(t.size) }}</p>
              </div>
              
              <div v-if="t.status === 'downloading'" class="flex flex-col items-end space-y-1">
                 <p class="text-[9px] font-black uppercase tracking-widest text-primary animate-pulse">{{ $t('downloads.speed') }}</p>
                 <p class="text-sm font-black text-primary">{{ formatSpeed(t.speed_down) }}</p>
              </div>
              
              <div class="text-right space-y-1">
                 <p class="text-[9px] font-black uppercase tracking-widest text-base-content/40">{{ $t('downloads.efficiency') }}</p>
                 <p class="text-sm font-black">{{ Math.round((t.done / t.size) * 100) || 0 }}%</p>
              </div>
            </div>

            <div class="h-3 w-full bg-base-200 rounded-full overflow-hidden p-[2px] border border-base-300/30">
               <div 
                 class="h-full rounded-full transition-all duration-1000 ease-out relative"
                 :class="t.status === 'downloading' ? 'bg-primary' : 'bg-success'"
                 :style="{ width: `${(t.done / t.size) * 100}%` }"
               >
                  <!-- Shine effect -->
                  <div class="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent -translate-x-full group-hover:animate-[shimmer_2s_infinite]"></div>
               </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes shimmer {
  100% {
    transform: translateX(100%);
  }
}
</style>
