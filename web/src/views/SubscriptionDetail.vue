<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import request from '../utils/request'

interface Subscription {
  id: number; title_cn: string; title_en: string; title_jp: string
  year: number; season: number; bangumi_id: string; subgroup_name: string
  metadata_id: string; metadata_provider: string; cover_url: string
  description: string; anime_type: string
  total_episodes: number; current_episodes: number; stalled_episodes: number
  enabled: boolean; completed: boolean
  filter_json: string; custom_path: string
  created_at: string; updated_at: string
}

interface Episode {
  id: number; subscription_id: number; season: number; number: number
  title: string; status: string; torrent_hash: string; torrent_url: string
  original_name: string; final_path: string; file_size: number
  is_stalled: boolean
  download_started_at: string; created_at: string
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const sub = ref<Subscription | null>(null)
const episodes = ref<Episode[]>([])
const loading = ref(true)
const error = ref('')
const showEdit = ref(false)

async function fetchDetail() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get(`/subscriptions/${id}`)
    sub.value = data.subscription
    episodes.value = data.episodes || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载订阅详情失败'
  } finally {
    loading.value = false
  }
}

async function handleSaveEdit(updated: Record<string, any>) {
  try {
    const { data } = await request.put(`/subscriptions/${id}`, updated)
    sub.value = data
    showEdit.value = false
  } catch (e: any) {
    alert(e.response?.data?.error || '更新失败')
  }
}

const statusLabels: Record<string, string> = {
  pending: '待下载',
  downloading: '下载中',
  completed: '已完成',
  failed: '失败',
}

function formatSize(bytes: number): string {
  if (!bytes) return '-'
  if (bytes > 1e9) return (bytes / 1e9).toFixed(1) + ' GB'
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(1) + ' KB'
}

onMounted(fetchDetail)
</script>

<template>
  <div>
    <button class="btn btn-ghost btn-sm mb-4" @click="router.push('/')">
      ← 返回订阅列表
    </button>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else-if="error" class="alert alert-error">
      <span>{{ error }}</span>
    </div>

    <template v-else-if="sub">
      <!-- 订阅信息卡片 -->
      <div class="card bg-base-100 shadow mb-6">
        <div class="card-body">
          <div class="flex items-start justify-between">
            <div>
              <h1 class="text-2xl font-bold">{{ sub.title_cn }}</h1>
              <div v-if="sub.title_en" class="text-base-content/60">{{ sub.title_en }}</div>
              <div v-if="sub.title_jp" class="text-base-content/40 text-sm">{{ sub.title_jp }}</div>
            </div>
            <div class="flex gap-2">
              <button class="btn btn-outline btn-sm" @click="showEdit = true">编辑</button>
              <button
                class="btn btn-sm"
                :class="sub.enabled ? 'btn-warning' : 'btn-success'"
                @click="handleSaveEdit({ enabled: !sub.enabled })"
              >
                {{ sub.enabled ? '暂停' : '启用' }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 md:grid-cols-4 gap-3 mt-4">
            <div><span class="text-sm opacity-60">类型</span><div class="font-medium">{{ sub.anime_type || '-' }}</div></div>
            <div><span class="text-sm opacity-60">年份</span><div class="font-medium">{{ sub.year || '-' }}</div></div>
            <div><span class="text-sm opacity-60">季</span><div class="font-medium">{{ sub.season || '-' }}</div></div>
            <div><span class="text-sm opacity-60">字幕组</span><div class="font-medium">{{ sub.subgroup_name || '-' }}</div></div>
            <div><span class="text-sm opacity-60">Bangumi ID</span><div class="font-medium">{{ sub.bangumi_id || '-' }}</div></div>
            <div><span class="text-sm opacity-60">元数据源</span><div class="font-medium">{{ sub.metadata_provider || '-' }}</div></div>
            <div><span class="text-sm opacity-60">状态</span>
              <div>
                <span v-if="sub.completed" class="badge badge-success">已完结</span>
                <span v-else class="badge">连载中</span>
                <span v-if="!sub.enabled" class="badge badge-warning ml-1">已暂停</span>
              </div>
            </div>
            <div><span class="text-sm opacity-60">添加时间</span><div class="font-medium text-sm">{{ new Date(sub.created_at).toLocaleDateString('zh-CN') }}</div></div>
          </div>

          <div v-if="sub.total_episodes > 0" class="mt-4">
            <div class="flex justify-between text-sm mb-1">
              <span>下载进度</span>
              <span>{{ sub.current_episodes }} / {{ sub.total_episodes }}</span>
            </div>
            <progress
              class="progress progress-primary w-full"
              :value="sub.current_episodes"
              :max="sub.total_episodes"
            ></progress>
          </div>

          <div v-if="sub.custom_path" class="mt-2 text-sm">
            <span class="opacity-60">自定义路径：</span><code class="bg-base-300 px-1 rounded">{{ sub.custom_path }}</code>
          </div>
        </div>
      </div>

      <div v-if="sub.stalled_episodes > 0" class="alert alert-warning mt-3">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
        <span>{{ sub.stalled_episodes }} 集超过 48 小时未完成下载，建议检查种子健康度或更换字幕组</span>
      </div>

      <!-- 编辑弹窗 (DaisyUI modal) -->
      <dialog :open="showEdit" class="modal" @click.self="showEdit = false">
        <div class="modal-box">
          <h3 class="text-lg font-bold mb-4">编辑订阅</h3>
          <SubscriptionEditForm
            :sub="sub"
            @save="handleSaveEdit"
            @cancel="showEdit = false"
          />
        </div>
      </dialog>

      <!-- 剧集列表 -->
      <div class="card bg-base-100 shadow">
        <div class="card-body">
          <h2 class="card-title">剧集列表 ({{ episodes.length }})</h2>
          <div v-if="episodes.length === 0" class="text-center py-8 text-base-content/50">
            暂无剧集记录
          </div>
          <div v-else class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr>
                  <th>#</th>
                  <th>文件名</th>
                  <th>状态</th>
                  <th>超时</th>
                  <th>大小</th>
                  <th>添加时间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ep in episodes" :key="ep.id">
                  <td class="font-mono">{{ ep.season > 1 ? 'S' + ep.season : '' }}{{ ep.number ? 'E' + ep.number : '' }}</td>
                  <td class="max-w-xs truncate" :title="ep.original_name">{{ ep.original_name || ep.title || '-' }}</td>
                  <td>
                    <span class="badge badge-sm" :class="{
                      'badge-ghost': ep.status === 'pending',
                      'badge-warning': ep.status === 'downloading',
                      'badge-success': ep.status === 'completed',
                      'badge-error': ep.status === 'failed',
                    }">{{ statusLabels[ep.status] || ep.status }}</span>
                  </td>
                  <td>
                    <span v-if="ep.is_stalled" class="badge badge-warning badge-sm">⚠ 超时</span>
                    <span v-else class="text-sm opacity-40">-</span>
                  </td>
                  <td class="text-sm opacity-70">{{ formatSize(ep.file_size) }}</td>
                  <td class="text-sm opacity-70">{{ new Date(ep.created_at).toLocaleDateString('zh-CN') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
