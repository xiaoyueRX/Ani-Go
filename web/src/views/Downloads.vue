<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

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
  loading.value = true
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
  if (!bytes) return '-'
  if (bytes > 1e9) return (bytes / 1e9).toFixed(1) + ' GB'
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(0) + ' KB'
}

function formatSpeed(bytesPerSec: number): string {
  if (!bytesPerSec) return '-'
  if (bytesPerSec > 1e6) return (bytesPerSec / 1e6).toFixed(1) + ' MB/s'
  return (bytesPerSec / 1e3).toFixed(0) + ' KB/s'
}

function statusInfo(status: string): { label: string; icon: string; cls: string } {
  const m: Record<string, { label: string; icon: string; cls: string }> = {
    downloading: { label: '下载中', icon: 'download', cls: 'badge-primary' },
    paused: { label: '已暂停', icon: 'pause', cls: 'badge-warning' },
    queued: { label: '排队中', icon: 'history', cls: 'badge-ghost' },
    checking: { label: '校验中', icon: 'refresh', cls: 'badge-info' },
    seeding: { label: '做种中', icon: 'upload', cls: 'badge-success' },
    completed: { label: '已完成', icon: 'check', cls: 'badge-success' },
    error: { label: '错误', icon: 'warning', cls: 'badge-error' },
  }
  return m[status] || { label: status, icon: 'more', cls: 'badge-ghost' }
}

let timer: ReturnType<typeof setInterval>

onMounted(() => {
  fetchDownloads()
  timer = setInterval(fetchDownloads, 10000)
})

onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">下载队列</h1>
      <button class="btn btn-ghost btn-sm gap-1" @click="fetchDownloads">
        <IconSax name="refresh" :size="16" />
        刷新
      </button>
    </div>

    <div v-if="error" class="alert alert-error mb-4">
      <IconSax name="warning" class="shrink-0" />
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">
        <IconSax name="close" :size="16" />
      </button>
    </div>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else-if="tasks.length === 0" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="download" :size="56" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50 text-lg">暂无下载任务</p>
        <p class="text-base-content/40 text-sm mt-1">添加订阅后种子会自动出现在这里</p>
      </div>
    </div>

    <div v-else class="grid gap-3">
      <div
        v-for="t in tasks" :key="t.hash"
        class="card bg-base-100 shadow-sm border border-base-200"
      >
        <div class="card-body py-3 px-4">
          <div class="flex items-center justify-between gap-4">
            <div class="flex-1 min-w-0">
              <div class="font-medium text-sm truncate flex items-center gap-2" :title="t.name">
                <IconSax name="download" :size="16" class="text-primary shrink-0" />
                {{ t.name }}
              </div>
              <div class="text-xs text-base-content/40 truncate mt-0.5" :title="t.save_path">
                {{ t.save_path }}
              </div>
            </div>
            <span class="badge badge-sm gap-1 shrink-0" :class="statusInfo(t.status).cls">
              <IconSax :name="statusInfo(t.status).icon" :size="12" />
              {{ statusInfo(t.status).label }}
            </span>
          </div>

          <div v-if="t.size > 0" class="mt-2">
            <div class="flex justify-between text-xs text-base-content/50 mb-1">
              <span>{{ formatSize(t.done) }} / {{ formatSize(t.size) }}</span>
              <span class="flex items-center gap-1">
                <IconSax name="download" :size="12" />
                {{ formatSpeed(t.speed_down) }}
              </span>
            </div>
            <progress
              class="progress w-full h-2"
              :class="t.status === 'downloading' ? 'progress-primary' : 'progress-success'"
              :value="t.done"
              :max="t.size"
            ></progress>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
