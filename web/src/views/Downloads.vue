<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import request from '../utils/request'

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

function statusLabel(status: string): string {
  const map: Record<string, string> = {
    downloading: '下载中',
    paused: '已暂停',
    queued: '排队中',
    checking: '校验中',
    seeding: '做种中',
    completed: '已完成',
    error: '错误',
  }
  return map[status] || status
}

function statusClass(status: string): string {
  switch (status) {
    case 'downloading': return 'badge-primary'
    case 'completed': case 'seeding': return 'badge-success'
    case 'error': return 'badge-error'
    case 'paused': return 'badge-warning'
    default: return 'badge-ghost'
  }
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
      <button class="btn btn-ghost btn-sm" @click="fetchDownloads">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
        刷新
      </button>
    </div>

    <div v-if="error" class="alert alert-error mb-4">
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">✕</button>
    </div>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else-if="tasks.length === 0" class="card bg-base-100 shadow">
      <div class="card-body text-center py-16">
        <p class="text-base-content/50">暂无下载任务</p>
      </div>
    </div>

    <div v-else class="grid gap-3">
      <div
        v-for="t in tasks" :key="t.hash"
        class="card bg-base-100 shadow"
      >
        <div class="card-body py-4">
          <div class="flex items-center justify-between">
            <div class="flex-1 min-w-0 mr-4">
              <div class="font-medium truncate" :title="t.name">{{ t.name }}</div>
              <div class="text-sm text-base-content/50 mt-1">{{ t.save_path }}</div>
            </div>
            <span class="badge" :class="statusClass(t.status)">{{ statusLabel(t.status) }}</span>
          </div>

          <div v-if="t.size > 0" class="mt-3">
            <div class="flex justify-between text-sm mb-1">
              <span>{{ formatSize(t.done) }} / {{ formatSize(t.size) }}</span>
              <span>{{ formatSpeed(t.speed_down) }}</span>
            </div>
            <progress
              class="progress w-full"
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
