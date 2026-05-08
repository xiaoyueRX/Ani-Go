<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
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
  cover_url?: string
}

interface SubgroupInfo {
  name: string
  rss_url: string
}

const router = useRouter()
const route = useRoute()
const query = ref('')
const results = ref<TorrentItem[]>([])
const loading = ref(false)
const error = ref('')
const subscribed = ref<Set<string>>(new Set())
const lastSearchTime = ref('')
const searchDuration = ref(0)

// 从查询参数自动搜索
onMounted(() => {
  const q = route.query.q as string
  if (q) {
    query.value = q
    handleSearch()
  }
})

// 字幕组选择弹窗
const showGroupModal = ref(false)
const selectedItem = ref<TorrentItem | null>(null)
const subgroups = ref<SubgroupInfo[]>([])
const groupLoading = ref(false)
const groupError = ref('')

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.startsWith('http') || url.startsWith('//')) {
    const target = url.startsWith('//') ? 'https:' + url : url
    return `/api/proxy/image?url=${encodeURIComponent(target)}`
  }
  return url
}

async function handleSearch() {
  if (!query.value.trim()) return
  loading.value = true
  error.value = ''
  const start = Date.now()
  try {
    const { data } = await request.get('/search', {
      params: { q: query.value },
      timeout: 25000,
    })
    results.value = data || []
    lastSearchTime.value = new Date().toLocaleTimeString('zh-CN')
    searchDuration.value = Date.now() - start
  } catch (e: any) {
    if (e.code === 'ECONNABORTED') {
      error.value = '搜索超时（Mikan 未响应），请稍后重试'
    } else {
      error.value = e.response?.data?.error || '搜索失败'
    }
  } finally {
    loading.value = false
  }
}

async function openSubscribe(item: TorrentItem) {
  if (!item.bangumi_id) {
    // 没有 BangumiID 时直接订阅
    await subscribe(item, '')
    return
  }
  // 有 BangumiID 时弹窗选字幕组
  selectedItem.value = item
  subgroups.value = []
  groupError.value = ''
  showGroupModal.value = true
  groupLoading.value = true
  try {
    const { data } = await request.get('/mikan/groups', {
      params: { bangumi_id: item.bangumi_id },
      timeout: 15000,
    })
    subgroups.value = data || []
  } catch (e: any) {
    groupError.value = e.code === 'ECONNABORTED' ? '获取字幕组列表超时' : '获取字幕组列表失败'
  } finally {
    groupLoading.value = false
  }
}

async function subscribe(item: TorrentItem, rssUrl: string) {
  try {
    await request.post('/subscriptions', {
      title_cn: item.title,
      bangumi_id: item.bangumi_id,
      subgroup_name: rssUrl ? '' : undefined,
      rss_url: rssUrl || undefined,
      filter_json: JSON.stringify({ source_url: item.url }),
      cover_url: item.cover_url || '',
    })
    subscribed.value.add(item.title)
    showGroupModal.value = false
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
        <input v-model="query" type="text"
          class="input input-bordered join-item flex-1"
          placeholder="输入番剧名称搜索... (如: 鬼灭之刃, 某科学的超电磁炮)"
          @keyup.enter="handleSearch" />
        <button class="btn btn-primary join-item gap-1"
          :disabled="loading || !query.trim()" @click="handleSearch">
          <span v-if="loading" class="loading loading-spinner loading-sm"></span>
          <IconSax v-else name="search" :size="18" />
          搜索
        </button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error mb-4">
      <IconSax name="warning" class="shrink-0" />
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">
        <IconSax name="close" :size="16" />
      </button>
    </div>

    <!-- 搜索结果 -->
    <div v-if="results.length > 0" class="mb-4">
      <div class="text-sm text-base-content/50 mb-3 flex items-center gap-2 flex-wrap">
        <span class="flex items-center gap-1">
          <IconSax name="category" :size="14" /> {{ results.length }} 个结果
        </span>
        <span v-if="lastSearchTime" class="text-xs opacity-60">
          搜索耗时 {{ (searchDuration / 1000).toFixed(1) }}s · {{ lastSearchTime }}
        </span>
      </div>

      <div class="grid gap-2 sm:gap-3">
        <div v-for="(item, idx) in results" :key="idx"
          class="card bg-base-100 shadow-sm hover:shadow-md transition-shadow border border-base-200 active:scale-[0.99]">
          <div class="card-body py-2.5 px-3 sm:py-3 sm:px-4">
            <div class="flex items-start justify-between gap-2 sm:gap-4">
              <div class="w-12 h-16 sm:w-16 sm:h-24 shrink-0 bg-base-300 rounded overflow-hidden relative">
                <img v-if="item.cover_url" :src="proxyImage(item.cover_url)" :alt="item.title" class="w-full h-full object-cover" loading="lazy" @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" />
                <div class="absolute inset-0 flex items-center justify-center text-base-content/20" v-else>
                  <IconSax name="antenna" :size="24" />
                </div>
              </div>
              <div class="flex-1 min-w-0">
                <div class="font-medium text-sm">{{ item.title }}</div>
                <div class="flex flex-wrap gap-2 mt-2">
                  <span class="badge badge-sm gap-1" :class="sourceBadge(item.source)">
                    <IconSax name="antenna" :size="12" /> {{ item.source }}
                  </span>
                  <span v-if="item.bangumi_id" class="badge badge-sm badge-ghost">#{{ item.bangumi_id }}</span>
                  <span v-if="item.size > 0" class="badge badge-sm badge-ghost">{{ formatSize(item.size) }}</span>
                </div>
              </div>
              <button class="btn btn-sm gap-1"
                :class="subscribed.has(item.title) ? 'btn-success' : 'btn-primary'"
                :disabled="subscribed.has(item.title)" @click="openSubscribe(item)">
                <IconSax :name="subscribed.has(item.title) ? 'check' : 'add'" :size="14" />
                {{ subscribed.has(item.title) ? '已订阅' : '订阅' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="!loading && query" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="search" :size="48" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50">未找到相关番剧</p>
        <p class="text-base-content/40 text-sm mt-1">尝试其他关键词</p>
      </div>
    </div>

    <div v-else-if="!loading" class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body text-center py-16">
        <IconSax name="search" :size="56" class="mx-auto text-base-content/20 mb-4" />
        <p class="text-base-content/50 text-lg">搜索番剧资源</p>
        <p class="text-base-content/40 text-sm mt-1">支持 Mikan、Nyaa、ACG.RIP、AnimeTosho 等资源站</p>
      </div>
    </div>

    <!-- 字幕组选择弹窗 -->
    <dialog :open="showGroupModal" class="modal" @click.self="showGroupModal = false">
      <div class="modal-box max-w-md">
        <div class="flex items-center gap-2 mb-4">
          <IconSax name="antenna" :size="20" class="text-primary" />
          <h3 class="text-lg font-bold">选择字幕组</h3>
        </div>

        <p class="text-sm text-base-content/60 mb-4">{{ selectedItem?.title }}</p>

        <div v-if="groupLoading" class="flex justify-center py-8">
          <span class="loading loading-spinner loading-md"></span>
        </div>

        <div v-else-if="groupError" class="alert alert-error text-sm mb-4">
          <IconSax name="warning" :size="16" class="shrink-0" />
          <span>{{ groupError }}</span>
        </div>

        <div v-else-if="subgroups.length === 0" class="text-center py-6 text-base-content/50">
          未找到字幕组，将使用默认配置订阅
          <div class="mt-4 flex gap-2 justify-center">
            <button class="btn btn-primary btn-sm" @click="selectedItem && subscribe(selectedItem, '')">直接订阅</button>
            <button class="btn btn-ghost btn-sm" @click="showGroupModal = false">取消</button>
          </div>
        </div>

        <div v-else class="space-y-2 max-h-80 overflow-y-auto">
          <button v-for="g in subgroups" :key="g.rss_url"
            class="btn btn-outline btn-sm w-full justify-start gap-2 h-auto py-2 px-3"
            @click="selectedItem && subscribe(selectedItem, g.rss_url)">
            <IconSax name="user" :size="14" class="shrink-0" />
            <span class="truncate">{{ g.name }}</span>
          </button>
        </div>

        <div class="modal-action">
          <button class="btn btn-ghost btn-sm" @click="showGroupModal = false">取消</button>
        </div>
      </div>
    </dialog>
  </div>
</template>
