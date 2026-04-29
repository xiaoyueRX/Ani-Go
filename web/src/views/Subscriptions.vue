<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number
  season: number
  bangumi_id: string
  subgroup_name: string
  cover_url: string
  anime_type: string
  total_episodes: number
  current_episodes: number
  stalled_episodes: number
  enabled: boolean
  completed: boolean
  created_at: string
  updated_at: string
}

const router = useRouter()
const subs = ref<Subscription[]>([])
const loading = ref(true)
const error = ref('')
const deletingId = ref<number | null>(null)

async function fetchSubscriptions() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get('/subscriptions')
    subs.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载订阅列表失败'
  } finally {
    loading.value = false
  }
}

async function toggleEnabled(sub: Subscription) {
  try {
    await request.put(`/subscriptions/${sub.id}`, { enabled: !sub.enabled })
    sub.enabled = !sub.enabled
  } catch (e: any) {
    error.value = e.response?.data?.error || '操作失败'
  }
}

async function handleDelete(sub: Subscription) {
  if (!confirm(`确定要删除「${sub.title_cn}」吗？关联的剧集记录也会一并删除。`)) return
  deletingId.value = sub.id
  try {
    await request.delete(`/subscriptions/${sub.id}`)
    subs.value = subs.value.filter(s => s.id !== sub.id)
  } catch (e: any) {
    error.value = e.response?.data?.error || '删除失败'
  } finally {
    deletingId.value = null
  }
}

async function triggerSupplement(sub: Subscription) {
  try {
    await request.post(`/subscriptions/${sub.id}/trigger-supplement`)
    alert('补全任务已触发，将在后台执行')
  } catch (e: any) {
    error.value = e.response?.data?.error || '触发补全失败'
  }
}

onMounted(fetchSubscriptions)
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">订阅管理</h1>
      <button class="btn btn-primary" @click="router.push('/subscriptions/new')">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        添加订阅
      </button>
    </div>

    <div v-if="error" class="alert alert-error mb-4">
      <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">✕</button>
    </div>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else-if="subs.length === 0" class="card bg-base-100 shadow">
      <div class="card-body text-center py-16">
        <p class="text-base-content/50 text-lg">暂无订阅</p>
        <p class="text-base-content/40 text-sm mt-1">添加 Mikan RSS 后，订阅会自动同步到这里</p>
      </div>
    </div>

    <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <div
        v-for="sub in subs" :key="sub.id"
        class="card bg-base-100 shadow hover:shadow-lg transition-shadow cursor-pointer"
        :class="{ 'opacity-60': !sub.enabled }"
        @click="router.push(`/subscriptions/${sub.id}`)"
      >
        <div class="card-body">
          <div class="flex items-start justify-between">
            <h3 class="card-title text-lg">{{ sub.title_cn }}</h3>
            <div class="flex gap-1" @click.stop>
              <button
                class="btn btn-ghost btn-xs"
                :class="{ 'text-success': sub.enabled, 'text-error': !sub.enabled }"
                @click="toggleEnabled(sub)"
                :title="sub.enabled ? '暂停' : '启用'"
              >
                {{ sub.enabled ? '⏸' : '▶' }}
              </button>
              <button
                class="btn btn-ghost btn-xs text-error"
                @click="handleDelete(sub)"
                :disabled="deletingId === sub.id"
              >
                <span v-if="deletingId === sub.id" class="loading loading-spinner loading-xs"></span>
                <span v-else>✕</span>
              </button>
            </div>
          </div>

          <div v-if="sub.subgroup_name" class="text-sm text-base-content/60">
            {{ sub.subgroup_name }}
          </div>

          <div v-if="sub.stalled_episodes > 0" class="mt-1">
            <span class="badge badge-warning badge-sm">
              ⚠ {{ sub.stalled_episodes }} 集超时未完成
            </span>
          </div>

          <div v-if="sub.total_episodes > 0" class="mt-2">
            <div class="flex justify-between text-sm mb-1">
              <span>进度</span>
              <span>{{ sub.current_episodes }} / {{ sub.total_episodes }}</span>
            </div>
            <progress
              class="progress progress-primary w-full"
              :value="sub.current_episodes"
              :max="sub.total_episodes"
            ></progress>
          </div>

          <div class="flex gap-2 mt-2" @click.stop>
            <span v-if="sub.anime_type" class="badge badge-sm">{{ sub.anime_type }}</span>
            <span v-if="sub.year" class="badge badge-sm">{{ sub.year }}</span>
            <span v-if="sub.completed" class="badge badge-success badge-sm">已完结</span>
          </div>

          <button
            v-if="sub.enabled"
            class="btn btn-outline btn-sm mt-2"
            @click.stop="triggerSupplement(sub)"
          >
            补全历史集数
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
