<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'
import SubscriptionEditForm from '../components/SubscriptionEditForm.vue'

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
const editDialog = ref<HTMLDialogElement | null>(null)
const updatingEps = ref<Set<number>>(new Set())

const statusCycle: Record<string, string> = {
  pending: 'downloading',
  downloading: 'completed',
  completed: 'pending',
  failed: 'pending',
}

async function cycleEpisodeStatus(ep: Episode) {
  const nextStatus = statusCycle[ep.status] || 'pending'
  updatingEps.value.add(ep.id)
  try {
    await request.put(`/episodes/${ep.id}/status`, { status: nextStatus })
    ep.status = nextStatus
  } catch { /* ignore */ } finally {
    updatingEps.value.delete(ep.id)
  }
}

function openEditDialog() {
  showEdit.value = true
  editDialog.value?.showModal()
}
function closeEditDialog() {
  showEdit.value = false
  editDialog.value?.close()
}

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
    closeEditDialog()
  } catch (e: any) {
    alert(e.response?.data?.error || '更新失败')
  }
}

const statusCfg: Record<string, { label: string; icon: string; cls: string }> = {
  pending: { label: '待下载', icon: 'history', cls: 'badge-ghost' },
  downloading: { label: '下载中', icon: 'download', cls: 'badge-warning' },
  completed: { label: '已完成', icon: 'check', cls: 'badge-success' },
  failed: { label: '失败', icon: 'warning', cls: 'badge-error' },
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
    <button class="btn btn-ghost btn-sm mb-4 gap-1" @click="router.push('/')">
      <IconSax name="chevron-left" :size="16" />
      返回订阅列表
    </button>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else-if="error" class="alert alert-error">
      <IconSax name="warning" class="shrink-0" />
      <span>{{ error }}</span>
    </div>

    <template v-else-if="sub">
      <!-- 订阅信息卡片 -->
      <div class="card bg-base-100 shadow-sm border border-base-200 mb-6">
        <div class="card-body">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <h1 class="text-2xl font-bold">{{ sub.title_cn }}</h1>
              <div v-if="sub.title_en" class="text-base-content/50 mt-0.5">{{ sub.title_en }}</div>
              <div v-if="sub.title_jp" class="text-base-content/40 text-sm">{{ sub.title_jp }}</div>
            </div>
            <div class="flex gap-2 shrink-0">
              <button class="btn btn-outline btn-sm gap-1" @click="openEditDialog">
                <IconSax name="edit" :size="14" />
                编辑
              </button>
              <button
                class="btn btn-sm gap-1"
                :class="sub.enabled ? 'btn-warning' : 'btn-success'"
                @click="handleSaveEdit({ enabled: !sub.enabled })"
              >
                <IconSax :name="sub.enabled ? 'pause' : 'play'" :size="14" />
                {{ sub.enabled ? '暂停' : '启用' }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mt-4">
            <div>
              <span class="text-xs opacity-50 flex items-center gap-1">
                <IconSax name="document" :size="12" /> 类型
              </span>
              <div class="font-medium mt-0.5">{{ sub.anime_type || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50">年份</span>
              <div class="font-medium mt-0.5">{{ sub.year || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50">季</span>
              <div class="font-medium mt-0.5">{{ sub.season || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50 flex items-center gap-1">
                <IconSax name="user" :size="12" /> 字幕组
              </span>
              <div class="font-medium mt-0.5">{{ sub.subgroup_name || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50">Bangumi ID</span>
              <div class="font-medium mt-0.5 font-mono text-sm">{{ sub.bangumi_id || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50">元数据源</span>
              <div class="font-medium mt-0.5">{{ sub.metadata_provider || '-' }}</div>
            </div>
            <div>
              <span class="text-xs opacity-50">状态</span>
              <div class="flex gap-1 mt-1">
                <span v-if="sub.completed" class="badge badge-success badge-sm gap-1">
                  <IconSax name="check" :size="12" /> 已完结
                </span>
                <span v-else class="badge badge-sm">连载中</span>
                <span v-if="!sub.enabled" class="badge badge-warning badge-sm gap-1">
                  <IconSax name="pause" :size="12" /> 已暂停
                </span>
              </div>
            </div>
            <div>
              <span class="text-xs opacity-50">添加时间</span>
              <div class="font-medium mt-0.5 text-sm">{{ new Date(sub.created_at).toLocaleDateString('zh-CN') }}</div>
            </div>
          </div>

          <div v-if="sub.total_episodes > 0" class="mt-4">
            <div class="flex justify-between text-sm text-base-content/50 mb-1">
              <span>下载进度</span>
              <span>{{ sub.current_episodes }} / {{ sub.total_episodes }}</span>
            </div>
            <progress
              class="progress progress-primary w-full h-2"
              :value="sub.current_episodes"
              :max="sub.total_episodes"
            ></progress>
          </div>

          <div v-if="sub.custom_path" class="mt-3 text-sm">
            <span class="opacity-50">自定义路径：</span>
            <code class="bg-base-300 px-1.5 py-0.5 rounded text-xs">{{ sub.custom_path }}</code>
          </div>
        </div>
      </div>

      <!-- 超时告警 -->
      <div v-if="sub.stalled_episodes > 0" class="alert alert-warning mb-6">
        <IconSax name="warning" class="shrink-0" />
        <span>{{ sub.stalled_episodes }} 集超过 48 小时未完成下载，建议检查种子健康度或更换字幕组</span>
      </div>

      <!-- 编辑弹窗 (DaisyUI modal) -->
      <dialog ref="editDialog" class="modal" @click.self="closeEditDialog">
        <div class="modal-box">
          <div class="flex items-center gap-2 mb-4">
            <IconSax name="edit" :size="20" />
            <h3 class="text-lg font-bold">编辑订阅</h3>
          </div>
          <SubscriptionEditForm
            v-if="showEdit"
            :sub="sub"
            @save="handleSaveEdit"
            @cancel="closeEditDialog"
          />
        </div>
        <form method="dialog" class="modal-backdrop">
          <button @click="closeEditDialog">关闭</button>
        </form>
      </dialog>

      <!-- 剧集列表 -->
      <div class="card bg-base-100 shadow-sm border border-base-200">
        <div class="card-body">
          <h2 class="card-title text-base mb-3">
            <IconSax name="document" :size="18" />
            剧集列表 ({{ episodes.length }})
          </h2>
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
                    <button class="badge badge-sm gap-1 cursor-pointer hover:opacity-70 transition-opacity"
                      :class="(statusCfg[ep.status] || {}).cls || 'badge-ghost'"
                      @click="cycleEpisodeStatus(ep)"
                      :disabled="updatingEps.has(ep.id)"
                      :title="'点击切换为: ' + ((statusCfg[statusCycle[ep.status]] || {}).label || statusCycle[ep.status])">
                      <span v-if="updatingEps.has(ep.id)" class="loading loading-spinner loading-xs"></span>
                      <IconSax v-else :name="(statusCfg[ep.status] || {}).icon || 'more'" :size="12" />
                      {{ (statusCfg[ep.status] || {}).label || ep.status }}
                    </button>
                  </td>
                  <td>
                    <span v-if="ep.is_stalled" class="badge badge-warning badge-sm gap-1">
                      <IconSax name="warning" :size="12" /> 超时
                    </span>
                    <span v-else class="text-sm opacity-40">-</span>
                  </td>
                  <td class="text-sm opacity-60">{{ formatSize(ep.file_size) }}</td>
                  <td class="text-sm opacity-60">{{ new Date(ep.created_at).toLocaleDateString('zh-CN') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
