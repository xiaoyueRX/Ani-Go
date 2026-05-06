<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

interface TorrentItem {
  title: string
  url: string
  magnet: string
  size: number
  pub_date: string
  source: string
  bangumi_id: string
  info_hash: string
}

const router = useRouter()
const query = ref('')
const results = ref<TorrentItem[]>([])
const loading = ref(false)
const error = ref('')
const subscribed = ref<Set<string>>(new Set())

async function handleSearch() {
  if (!query.value.trim()) return
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get('/search', { params: { q: query.value } })
    results.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '搜索失败'
  } finally {
    loading.value = false
  }
}

async function subscribe(item: TorrentItem) {
  try {
    await request.post('/subscriptions', {
      title_cn: item.title,
      bangumi_id: item.bangumi_id,
      filter_json: JSON.stringify({ source_url: item.url }),
    })
    subscribed.value.add(item.title)
    alert(`已订阅: ${item.title}`)
  } catch (e: any) {
    alert('订阅失败: ' + (e.response?.data?.error || e.message))
  }
}

function formatSize(size: number): string {
  if (!size || size <= 0) return '-'
  const mb = size / 1024 / 1024
  return mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(2) + ' MB'
}

function sourceBadge(source: string): string {
  const map: Record<string, string> = {
    mikan: 'badge-primary',
    nyaa: 'badge-secondary',
    acgrip: 'badge-accent',
    animetosho: 'badge-info',
  }
  return map[source] || 'badge-ghost'
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">搜索番剧</h1>
      <button class="btn btn-ghost btn-sm gap-1" @click="router.push('/')">
        <IconSax name="chevron-left" :size="16" />
        返回
      </button>
    </div>

    <!-- 搜索框 -->
    <div class="form-control mb-6">
      <div class="join w-full">
        <input
          v-model="query"
          type="text"
          class="input input-bordered join-item flex-1"
          placeholder="输入番剧名称搜索... (如: 鬼灭之刃, 某科学的超电磁炮)"
          @keyup.enter="handleSearch"
        />
        <button
          class="btn btn-primary join-item gap-1"
          :disabled="loading || !query.trim()"
          @click="handleSearch"
        >
          <span v-if="loading" class="loading loading-spinner loading-sm"></span>
          <IconSax v-else name="search" :size="18" />
          搜索
        </button>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="error" class="alert alert-error mb-4">
      <IconSax name="warning" class="shrink-0" />
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">
        <IconSax name="close" :size="16" />
      </button>
    </div>

    <!-- 搜索结果 -->
    <div v-if="results.length > 0" class="mb-4">
      <div class="text-sm text-base-content/50 mb-3 flex items-center gap-1">
        <IconSax name="category" :size="14" />
        找到 {{ results.length }} 个结果
      </div>

      <div class="grid gap-3">
        <div
          v-for="(item, idx) in results"
          :key="idx"
          class="card bg-base-100 shadow-sm hover:shadow-md transition-shadow border border-base-200"
        >
          <div class="card-body py-3 px-4">
            <div class="flex items-start justify-between gap-4">
              <div class="flex-1 min-w-0">
                <div class="font-medium text-sm">{{ item.title }}</div>
                <div class="flex flex-wrap gap-2 mt-2">
                  <span class="badge badge-sm gap-1" :class="sourceBadge(item.source)">
                    <IconSax name="antenna" :size="12" />
                    {{ item.source }}
                  </span>
                  <span v-if="item.bangumi_id" class="badge badge-sm badge-ghost">
                    #{{ item.bangumi_id }}
                  </span>
                  <span v-if="item.size > 0" class="badge badge-sm badge-ghost">
                    {{ formatSize(item.size) }}
                  </span>
                  <span v-if="item.info_hash" class="badge badge-sm badge-ghost font-mono text-xs">
                    {{ item.info_hash.slice(0, 8) }}...
                  </span>
                </div>
              </div>

              <button
                class="btn btn-sm gap-1"
                :class="subscribed.has(item.title) ? 'btn-success' : 'btn-primary'"
                :disabled="subscribed.has(item.title)"
                @click="subscribe(item)"
              >
                <IconSax :name="subscribed.has(item.title) ? 'check' : 'add'" :size="14" />
                {{ subscribed.has(item.title) ? '已订阅' : '订阅' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 空结果 -->
    <div v-else-if="!loading && query" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="search" :size="48" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50">未找到相关番剧</p>
        <p class="text-base-content/40 text-sm mt-1">尝试其他关键词</p>
      </div>
    </div>

    <!-- 初始状态 -->
    <div v-else-if="!loading" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="search" :size="56" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50 text-lg">搜索番剧资源</p>
        <p class="text-base-content/40 text-sm mt-1">
          支持 Mikan、Nyaa、ACG.RIP、AnimeTosho 等资源站
        </p>
      </div>
    </div>
  </div>
</template>
