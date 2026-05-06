<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number; season: number
  bangumi_id: string; subgroup_name: string
  cover_url: string; anime_type: string
  total_episodes: number; current_episodes: number
  stalled_episodes: number
  enabled: boolean; completed: boolean
  created_at: string; updated_at: string
}

const router = useRouter()
const subs = ref<Subscription[]>([])
const loading = ref(true)
const error = ref('')
const deletingId = ref<number | null>(null)
const filterText = ref('')
const filterType = ref<'all' | 'active' | 'completed'>('all')

const filteredSubs = computed(() => {
  let list = subs.value
  // 状态筛选
  if (filterType.value === 'active') list = list.filter(s => s.enabled)
  else if (filterType.value === 'completed') list = list.filter(s => s.completed)
  // 文字搜索
  const q = filterText.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      s.title_cn.toLowerCase().includes(q) ||
      (s.title_en && s.title_en.toLowerCase().includes(q)) ||
      (s.subgroup_name && s.subgroup_name.toLowerCase().includes(q))
    )
  }
  return list
})

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
      <div class="flex gap-2">
        <button class="btn btn-ghost btn-sm gap-1" @click="router.push('/search')">
          <IconSax name="search" ::size="16" />
          搜索番剧
        </button>
        <button class="btn btn-primary btn-sm gap-1" @click="router.push('/subscriptions/new')">
          <IconSax name="add" ::size="16" />
          添加订阅
        </button>
      </div>
    </div>

    <!-- 搜索/筛选栏 -->
    <div class="flex flex-col sm:flex-row gap-3 mb-4">
      <label class="input input-bordered input-sm flex items-center gap-2 flex-1">
        <IconSax name="search" :size="16" class="opacity-50" />
        <input v-model="filterText" type="text" class="grow" placeholder="搜索订阅名称/字幕组..." />
      </label>
      <div class="flex gap-1">
        <button class="btn btn-xs" :class="filterType === 'all' ? 'btn-primary' : 'btn-ghost'" @click="filterType = 'all'">全部</button>
        <button class="btn btn-xs" :class="filterType === 'active' ? 'btn-primary' : 'btn-ghost'" @click="filterType = 'active'">进行中</button>
        <button class="btn btn-xs" :class="filterType === 'completed' ? 'btn-primary' : 'btn-ghost'" @click="filterType = 'completed'">已完结</button>
      </div>
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

    <div v-else-if="filteredSubs.length === 0" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="search" :size="48" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50 text-lg">{{ subs.length > 0 ? '未找到匹配的订阅' : '暂无订阅' }}</p>
        <p class="text-base-content/40 text-sm mt-1">{{ subs.length > 0 ? '尝试其他关键词' : '通过搜索添加番剧订阅' }}</p>
        <button v-if="subs.length === 0" class="btn btn-primary mt-4 gap-1" @click="router.push('/search')">
          <IconSax name="search" :size="16" />
          搜索番剧
        </button>
      </div>
    </div>

    <!-- 订阅卡片网格 -->
    <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <div
        v-for="sub in filteredSubs" :key="sub.id"
        class="card bg-base-100 shadow-sm hover:shadow-lg transition-all duration-200 cursor-pointer border border-base-200 hover:border-primary/20"
        :class="{ 'opacity-60': !sub.enabled }"
        @click="router.push(`/subscriptions/${sub.id}`)"
      >
        <div class="card-body">
          <!-- 标题行 -->
          <div class="flex items-start justify-between gap-2">
            <h3 class="card-title text-base truncate" :title="sub.title_cn">{{ sub.title_cn }}</h3>
            <div class="flex gap-1 shrink-0" @click.stop>
              <button
                class="btn btn-ghost btn-xs btn-square"
                :class="{ 'text-success': sub.enabled }"
                @click="toggleEnabled(sub)"
                :title="sub.enabled ? '暂停' : '启用'"
              >
                <IconSax :name="sub.enabled ? 'pause' : 'play'" ::size="16" />
              </button>
              <button
                class="btn btn-ghost btn-xs btn-square text-error"
                @click="handleDelete(sub)"
                :disabled="deletingId === sub.id"
              >
                <span v-if="deletingId === sub.id" class="loading loading-spinner loading-xs"></span>
                <IconSax v-else name="trash" ::size="16" />
              </button>
            </div>
          </div>

          <!-- 字幕组 -->
          <div v-if="sub.subgroup_name" class="flex items-center gap-1 text-sm text-base-content/50">
            <IconSax name="user" ::size="14" />
            {{ sub.subgroup_name }}
          </div>

          <!-- 超时告警 -->
          <div v-if="sub.stalled_episodes > 0">
            <span class="badge badge-warning badge-sm gap-1">
              <IconSax name="warning" ::size="12" />
              {{ sub.stalled_episodes }} 集超时
            </span>
          </div>

          <!-- 进度条 -->
          <div v-if="sub.total_episodes > 0" class="mt-1">
            <div class="flex justify-between text-xs text-base-content/50 mb-1">
              <span>进度</span>
              <span>{{ sub.current_episodes }} / {{ sub.total_episodes }}</span>
            </div>
            <progress
              class="progress progress-primary w-full h-2"
              :value="sub.current_episodes"
              :max="sub.total_episodes"
            ></progress>
          </div>

          <!-- 标签行 -->
          <div class="flex flex-wrap gap-2 mt-1" @click.stop>
            <span v-if="sub.anime_type" class="badge badge-sm badge-ghost gap-1">
              <IconSax name="document" ::size="12" />
              {{ sub.anime_type }}
            </span>
            <span v-if="sub.year" class="badge badge-sm badge-ghost">{{ sub.year }}</span>
            <span v-if="sub.completed" class="badge badge-success badge-sm gap-1">
              <IconSax name="check" ::size="12" />
              已完结
            </span>
          </div>

          <!-- 补全按钮 -->
          <button
            v-if="sub.enabled"
            class="btn btn-outline btn-xs mt-2 gap-1"
            @click.stop="triggerSupplement(sub)"
          >
            <IconSax name="history" ::size="14" />
            补全历史集数
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
