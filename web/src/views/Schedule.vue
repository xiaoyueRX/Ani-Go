<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

interface TorrentItem {
  title: string; url: string; source: string; bangumi_id: string; info_hash: string
}

interface WeekDay {
  day_of_week: number; label: string; items: TorrentItem[]
}

const router = useRouter()
const weekDays = ref<WeekDay[]>([])
const subscribedIds = ref<Record<string, boolean>>({})
const loading = ref(true)
const error = ref('')
const activeTab = ref<'schedule' | 'mysub'>('schedule')

const weekOrder = [1, 2, 3, 4, 5, 6, 7]
const sortedDays = computed(() =>
  [...weekDays.value].sort((a, b) => weekOrder.indexOf(a.day_of_week) - weekOrder.indexOf(b.day_of_week))
)

// 已订阅的番剧按放送日分组
const subscribedSchedule = computed(() => {
  const map: Record<string, TorrentItem[]> = {}
  for (const day of weekDays.value) {
    const items = day.items.filter(i => subscribedIds.value[i.bangumi_id])
    if (items.length > 0) map[day.label] = items
  }
  return map
})

const subscribedCount = computed(() => Object.keys(subscribedIds.value).length)

async function fetchSchedule() {
  loading.value = true; error.value = ''
  try {
    const { data } = await request.get('/schedule', { timeout: 25000 })
    weekDays.value = data.days || []
    subscribedIds.value = data.subscribed || {}
  } catch (e: any) {
    error.value = e.code === 'ECONNABORTED' ? '获取时间表超时' : '获取时间表失败'
  } finally { loading.value = false }
}

onMounted(fetchSchedule)
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h1 class="text-xl sm:text-2xl font-bold">新番时间表</h1>
      <button class="btn btn-ghost btn-sm gap-1" @click="fetchSchedule" :disabled="loading">
        <IconSax name="refresh" :size="16" />
        刷新
      </button>
    </div>

    <!-- 切换 Tab -->
    <div class="tabs tabs-box mb-4 bg-base-200">
      <button class="tab gap-1" :class="{ 'tab-active': activeTab === 'schedule' }"
        @click="activeTab = 'schedule'">
        <IconSax name="antenna" :size="16" /> 放送表
      </button>
      <button class="tab gap-1" :class="{ 'tab-active': activeTab === 'mysub' }"
        @click="activeTab = 'mysub'">
        <IconSax name="category" :size="16" /> 我的订阅
        <span v-if="subscribedCount > 0" class="badge badge-xs badge-primary">{{ subscribedCount }}</span>
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

    <!-- ====== 放送表 ====== -->
    <template v-else-if="activeTab === 'schedule'">
      <div v-if="sortedDays.length === 0" class="card bg-base-100 shadow-sm border border-base-200">
        <div class="card-body text-center py-16">
          <IconSax name="antenna" :size="56" class="mx-auto text-base-content/20 mb-4" />
          <p class="text-base-content/50">暂无放送数据</p>
        </div>
      </div>

      <div v-else class="space-y-4">
        <div v-for="day in sortedDays" :key="day.day_of_week"
          class="card bg-base-100 shadow-sm border border-base-200">
          <div class="card-body p-3 sm:p-4">
            <h2 class="text-base font-semibold flex items-center gap-2 mb-2">
              <IconSax name="calendar" :size="18" class="text-primary" />
              {{ day.label }}
              <span class="text-xs text-base-content/40 font-normal">({{ day.items.length }} 部)</span>
            </h2>
            <div class="flex flex-wrap gap-2">
              <div v-for="item in day.items" :key="item.bangumi_id"
                class="badge badge-lg gap-1.5 py-3 px-3 cursor-pointer hover:bg-primary/10 transition-colors"
                :class="subscribedIds[item.bangumi_id] ? 'badge-primary' : 'badge-ghost'"
                @click="subscribedIds[item.bangumi_id] ? router.push(`/subscriptions?q=${encodeURIComponent(item.title)}`) : router.push(`/search?q=${encodeURIComponent(item.title)}`)">
                <IconSax :name="subscribedIds[item.bangumi_id] ? 'check' : 'add'" :size="14" />
                <span class="text-sm max-w-[200px] truncate">{{ item.title }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- ====== 我的订阅时间表 ====== -->
    <template v-else>
      <div v-if="Object.keys(subscribedSchedule).length === 0" class="card bg-base-100 shadow-sm border border-base-200">
        <div class="card-body text-center py-16">
          <IconSax name="category" :size="56" class="mx-auto text-base-content/20 mb-4" />
          <p class="text-base-content/50 text-lg">暂无订阅</p>
          <p class="text-base-content/40 text-sm mt-1">去搜索页添加番剧订阅</p>
          <button class="btn btn-primary mt-4 gap-1" @click="router.push('/search')">
            <IconSax name="search" :size="16" /> 搜索番剧
          </button>
        </div>
      </div>

      <div v-else class="space-y-4">
        <div v-for="(items, label) in subscribedSchedule" :key="label"
          class="card bg-base-100 shadow-sm border border-base-200">
          <div class="card-body p-3 sm:p-4">
            <h2 class="text-base font-semibold flex items-center gap-2 mb-2">
              <IconSax name="calendar" :size="18" class="text-success" />
              {{ label }}
              <span class="text-xs text-base-content/40 font-normal">({{ items.length }} 部)</span>
            </h2>
            <div class="flex flex-wrap gap-2">
              <div v-for="item in items" :key="item.bangumi_id"
                class="badge badge-lg gap-1.5 py-3 px-3 badge-primary cursor-pointer hover:opacity-80 transition-opacity"
                @click="router.push(`/subscriptions?q=${encodeURIComponent(item.title)}`)">
                <IconSax name="check" :size="14" />
                <span class="text-sm max-w-[200px] truncate">{{ item.title }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
