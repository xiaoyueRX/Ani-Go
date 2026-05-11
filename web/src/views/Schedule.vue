<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Antenna, LayoutGrid, RefreshCw, 
  AlertTriangle, X, Calendar, 
  Image, Check, Clock, ChevronDown,
  Search, ChevronLeft
} from 'lucide-vue-next'
import { ListChecks, CheckCircle, XCircle } from 'lucide-vue-next'
import OnboardingModal from '../components/OnboardingModal.vue'
import ChangelogModal from '../components/ChangelogModal.vue'
import { useVersion } from '../composables/useVersion'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

interface TorrentItem {
  title: string; 
  url: string; 
  source: string; 
  bangumi_id: string; 
  info_hash: string; 
  cover_url: string;
  aired_time?: string;
  aired_date?: string;
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
const selectedItem = ref<TorrentItem | null>(null)

// 订阅下载统计
const subStats = ref<Record<number, {downloaded: number, total: number}>>({})

// 批量订阅
const batchMode = ref(false)
const selectedItems = ref<Set<string>>(new Set())
const subgroupPickerOpen = ref(false)
const subgroupPickerItem = ref<TorrentItem | null>(null)
const selectedSubgroups = ref<Map<string, string[]>>(new Map())
const currentSubgroupSet = ref<Set<string>>(new Set())
const customSubgroupInput = ref('')

const defaultSubgroups = ref<string[]>([])
const defaultSubgroupPickerOpen = ref(false)

const COMMON_SUBGROUPS = [
  '桜都字幕组',
  '千夏字幕组',
  '喵萌奶茶屋',
  'LoliHouse',
  'Airota',
  'ANi',
  '极影字幕社',
  'SweetSub',
  'Haruhana',
  'MingY',
  '星空字幕组',
]

const batchResultModal = ref(false)
const batchResult = ref<{success: {title: string, id: number}[], failed: {title: string, error: string}[]}>({success: [], failed: []})
const batchSubmitting = ref(false)

function getSubId(item: TorrentItem): number | null {
  // info_hash 已被后端复用为 sub_id（字符串形式）
  if (item.info_hash && item.info_hash !== 'subscribed') {
    return parseInt(item.info_hash)
  }
  return null
}

function getItemStats(item: TorrentItem) {
  const id = getSubId(item)
  if (id !== null && subStats.value[id]) {
    return subStats.value[id]
  }
  return null
}

const now = new Date()
const currentYear = now.getFullYear()
const currentMonth = now.getMonth() + 1
let currentSeason = 1
if (currentMonth >= 1 && currentMonth <= 3) currentSeason = 1
else if (currentMonth >= 4 && currentMonth <= 6) currentSeason = 2
else if (currentMonth >= 7 && currentMonth <= 9) currentSeason = 3
else currentSeason = 4

let defaultYear = currentYear
let defaultSeason = currentSeason

const selectedYear = ref(defaultYear)
const selectedSeason = ref(defaultSeason)

const { changelog, showChangelog, checkVersion } = useVersion()

const years = computed(() => {
  const arr = []
  const maxYear = currentSeason === 4 ? currentYear + 1 : currentYear
  for (let y = maxYear; y >= 2000; y--) arr.push(y)
  return arr
})

const allSeasons = [
  { value: 1, label: '1月冬' },
  { value: 2, label: '4月春' },
  { value: 3, label: '7月夏' },
  { value: 4, label: '10月秋' },
]

const maxSeason = computed(() => {
  if (selectedYear.value > currentYear) return 4
  return Math.min(currentSeason + 1, 4)
})

const seasons = computed(() => {
  if (selectedYear.value < currentYear) return allSeasons
  return allSeasons.filter(s => s.value <= maxSeason.value)
})

const weekOrder = [1, 2, 3, 4, 5, 6, 7, 0, 8]
const dayNames = computed<Record<number, string>>(() => ({
  1: t('schedule.days.monday'), 2: t('schedule.days.tuesday'), 3: t('schedule.days.wednesday'), 4: t('schedule.days.thursday'),
  5: t('schedule.days.friday'), 6: t('schedule.days.saturday'), 7: t('schedule.days.sunday'), 0: t('schedule.days.sp'), 8: t('schedule.days.tbd'),
}))

const sortedDays = computed(() =>
  [...weekDays.value]
    .map(d => ({ ...d, label: dayNames.value[d.day_of_week] || d.label }))
    .sort((a, b) => weekOrder.indexOf(a.day_of_week) - weekOrder.indexOf(b.day_of_week))
)

const subscribedSchedule = computed(() => {
  const map: Record<string, TorrentItem[]> = {}
  for (const day of weekDays.value) {
    const items = day.items.filter(i => i.info_hash || subscribedIds.value[i.bangumi_id])
    if (items.length > 0) map[dayNames.value[day.day_of_week] || day.label] = items
  }
  return map
})

const subscribedCount = computed(() => {
  let count = 0
  Object.values(subscribedSchedule.value).forEach(items => {
    count += items.length
  })
  return count
})

function handleItemClick(item: TorrentItem) {
  const subId = item.info_hash || subscribedIds.value[item.bangumi_id]
  if (subId && subId !== 'subscribed') {
    router.push(`/subscriptions/${subId}`)
  } else {
    selectedItem.value = item
  }
}

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

async function fetchSchedule() {
  loading.value = true; error.value = ''
  try {
    const { data } = await request.get('/schedule', { 
      params: { 
        year: selectedYear.value, 
        season: selectedSeason.value 
      },
      timeout: 30000 
    })
    weekDays.value = data.days || []
    subscribedIds.value = data.subscribed || {}
    subStats.value = data.sub_stats || {}
  } catch (e: any) {
    error.value = e.code === 'ECONNABORTED' ? t('schedule.error.timeout') : t('schedule.error.failed')
  } finally {
    loading.value = false
  }
}

const MAX_BATCH_SUBSCRIBE = 20

// 切换勾选
function toggleSelect(item: TorrentItem) {
  const id = item.title
  const newSet = new Set(selectedItems.value)
  if (newSet.has(id)) {
    newSet.delete(id)
    const newMap = new Map(selectedSubgroups.value)
    newMap.delete(id)
    selectedSubgroups.value = newMap
    selectedItems.value = newSet
    return
  }
  if (newSet.size >= MAX_BATCH_SUBSCRIBE) {
    // @ts-ignore
    if (typeof ElMessage !== 'undefined') ElMessage.warning(`最多选择 ${MAX_BATCH_SUBSCRIBE} 部番剧`)
    return
  }
  // 如果设置了默认字幕组 → 直接勾选
  if (defaultSubgroups.value.length > 0) {
    newSet.add(id)
    selectedItems.value = newSet
    const newMap = new Map(selectedSubgroups.value)
    newMap.set(id, [...defaultSubgroups.value])
    selectedSubgroups.value = newMap
    return
  }
  // 否则弹窗逐部选
  subgroupPickerItem.value = item
  currentSubgroupSet.value = new Set()
  subgroupPickerOpen.value = true
}

function confirmSubgroupPicker() {
  if (!subgroupPickerItem.value) return
  const id = subgroupPickerItem.value.title
  const newSet = new Set(selectedItems.value)
  newSet.add(id)
  selectedItems.value = newSet
  const newMap = new Map(selectedSubgroups.value)
  newMap.set(id, [...currentSubgroupSet.value])
  selectedSubgroups.value = newMap
  subgroupPickerOpen.value = false
  subgroupPickerItem.value = null
}

function cancelSubgroupPicker() {
  subgroupPickerOpen.value = false
  subgroupPickerItem.value = null
  currentSubgroupSet.value = new Set()
}

function confirmDefaultSubgroups() {
  defaultSubgroups.value = [...currentSubgroupSet.value]
  defaultSubgroupPickerOpen.value = false
}
function skipDefaultSubgroups() {
  defaultSubgroups.value = []
  defaultSubgroupPickerOpen.value = false
}

function toggleSubgroup(name: string) {
  const newSet = new Set(currentSubgroupSet.value)
  if (newSet.has(name)) {
    newSet.delete(name)
  } else {
    newSet.add(name)
  }
  currentSubgroupSet.value = newSet
}

function addCustomSubgroup() {
  const val = customSubgroupInput.value.trim()
  if (!val) return
  if (val.length > 20) {
    // @ts-ignore
    if (typeof ElMessage !== 'undefined') ElMessage.warning('字幕组名称不超过 20 个字符')
    return
  }
  if (currentSubgroupSet.value.has(val)) return  // 去重
  const newSet = new Set(currentSubgroupSet.value)
  newSet.add(val)
  currentSubgroupSet.value = newSet
  customSubgroupInput.value = ''
}

function isSelected(item: TorrentItem): boolean {
  return selectedItems.value.has(item.title)
}

// 进入批量模式
function enterBatchMode() {
  batchMode.value = true
  selectedItems.value = new Set()
  selectedSubgroups.value = new Map()
  defaultSubgroups.value = []
  currentSubgroupSet.value = new Set()
  customSubgroupInput.value = ''
  defaultSubgroupPickerOpen.value = true
}

// 退出批量模式
function exitBatchMode() {
  batchMode.value = false
  selectedItems.value = new Set()
  selectedSubgroups.value = new Map()
  defaultSubgroups.value = []
}

// 确认批量订阅
async function confirmBatchSubscribe() {
  if (selectedItems.value.size === 0) return
  
  // 构建 payload
  const allItems: {item: TorrentItem, title: string, bangumi_id: string, cover_url: string, subgroups: string[]}[] = []
  for (const day of weekDays.value) {
    for (const item of day.items) {
      if (selectedItems.value.has(item.title)) {
        allItems.push({
          item,
          title: item.title,
          bangumi_id: item.bangumi_id,
          cover_url: item.cover_url || '',
          subgroups: selectedSubgroups.value.get(item.title) || [],
        })
      }
    }
  }
  
  // 前端限制最多 20 部（与后端一致）
  const MAX_BATCH = 20
  if (allItems.length > MAX_BATCH) {
    batchResult.value = {
      success: [],
      failed: [{title: `批量订阅最多 ${MAX_BATCH} 部，已选 ${allItems.length} 部，请减少选择`, error: ''}]
    }
    batchResultModal.value = true
    batchSubmitting.value = false
    return
  }
  
  batchSubmitting.value = true
  
  try {
    const payload = {
      items: allItems.map(i => ({
        title_cn: i.title,
        bangumi_id: i.bangumi_id,
        cover_url: i.cover_url,
        subgroups: i.subgroups,
      }))
    }
    
    const { data } = await request.post('/subscriptions/batch', payload, { timeout: 30000 })
    batchResult.value = data
    
    // 刷新时间表，更新订阅状态
    await fetchSchedule()
    batchMode.value = false
    selectedItems.value = new Set()
    batchResultModal.value = true
  } catch (e: any) {
    batchResult.value = {
      success: [],
      failed: [{title: '批量订阅失败', error: e.response?.data?.error || e.message}]
    }
    batchResultModal.value = true
  } finally {
    batchSubmitting.value = false
  }
}

// 已选数量
const selectedCount = computed(() => selectedItems.value.size)

onMounted(() => {
  fetchSchedule()
  checkVersion()
})
</script>

<template>
  <div class="space-y-10 pb-20">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('schedule.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('schedule.subtitle') }}</p>
      </div>
      
      <div class="flex items-center gap-3">
        <!-- Season Selector -->
        <div v-if="activeTab === 'schedule'" class="flex items-center gap-1.5 p-1 bg-base-200/50 rounded-2xl border border-base-300/30">
          <select v-model="selectedYear" @change="fetchSchedule" class="select select-ghost select-xs focus:bg-transparent font-black text-[10px] w-20 h-8 min-h-0">
            <option v-for="y in years" :key="y" :value="y">{{ y }}</option>
          </select>
          <div class="w-px h-3 bg-base-content/10"></div>
          <select v-model="selectedSeason" @change="fetchSchedule" class="select select-ghost select-xs focus:bg-transparent font-black text-[10px] w-20 h-8 min-h-0">
            <option v-for="s in seasons" :key="s.value" :value="s.value">{{ s.label }}</option>
          </select>
        </div>

        <div class="p-1 bg-base-200/50 rounded-2xl flex gap-1 border border-base-300/30">
          <button 
            v-for="t in [ {id: 'schedule', label: $t('schedule.tabs.all'), icon: Antenna}, {id: 'mysub', label: $t('schedule.tabs.mine'), icon: LayoutGrid} ]" 
            :key="t.id"
            class="px-5 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all flex items-center gap-2"
            :class="activeTab === t.id ? 'bg-primary text-primary-content shadow-lg shadow-lg' : 'text-base-content/40 hover:text-base-content hover:bg-base-200'"
            @click="activeTab = t.id as any"
          >
            <component :is="t.icon" :size="14" />
            {{ t.label }}
            <span v-if="t.id === 'mysub' && subscribedCount > 0" class="badge badge-xs bg-white/20 text-white border-none ml-1">{{ subscribedCount }}</span>
          </button>
        </div>
        <button v-if="!batchMode" class="btn btn-ghost border border-base-300/50 rounded-2xl hover:bg-base-200 text-[10px] font-black uppercase tracking-widest gap-2" @click="enterBatchMode">
          <ListChecks :size="16" />
          批量订阅
        </button>
        <button 
          class="btn btn-ghost btn-circle hover:bg-base-200" 
          @click="fetchSchedule" 
          :disabled="loading"
        >
          <RefreshCw :size="20" class="opacity-50" />
        </button>
      </div>
    </div>

    <!-- Error State -->
    <div v-if="error" class="max-w-4xl mx-auto">
      <div class="alert bg-error/10 border-error/20 text-error rounded-[2rem] p-6">
        <AlertTriangle :size="24" class="shrink-0" />
        <div class="flex-1">
          <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('schedule.error.sync') }}</h3>
          <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
        </div>
        <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
          <X :size="16" />
        </button>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="grid gap-10">
      <div v-for="i in 3" :key="i" class="space-y-4">
         <div class="h-6 w-32 bg-base-200 rounded-full animate-pulse"></div>
         <div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(160px, 1fr))">
            <div v-for="j in 6" :key="j" class="aspect-[3/4.5] bg-base-200 rounded-3xl animate-pulse border border-base-300/30"></div>
         </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="sortedDays.length === 0" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200 max-w-4xl mx-auto">
      <div class="w-32 h-32 bg-base-200/50 rounded-full flex items-center justify-center mb-8 rotate-12">
        <Calendar :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">{{ $t('schedule.empty.title') }}</h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ $t('schedule.empty.desc') }}
      </p>
    </div>

    <!-- ====== Main Timeline ====== -->
    <div v-else class="space-y-12">
      <!-- Section Template -->
      <template v-if="activeTab === 'schedule'">
        <div v-for="(day, index) in sortedDays" :key="day.day_of_week + day.label" class="space-y-6">
          <!-- SP 月份：可折叠 -->
          <template v-if="day.day_of_week === 0">
            <div class="collapse collapse-arrow bg-base-100/30 rounded-2xl border border-base-200/50">
              <input type="checkbox" :checked="index === sortedDays.findIndex(d => d.day_of_week === 0)" />
              <div class="collapse-title text-lg font-black tracking-tight">
                {{ day.label }} 
                <span class="text-[10px] font-black uppercase tracking-widest text-base-content/30 ml-2">{{ day.items.length }} 部</span>
              </div>
              <div class="collapse-content">
                <div class="grid gap-4 sm:gap-6 items-start pt-2" style="grid-template-columns: repeat(auto-fill, minmax(160px, 1fr))">
                  <div v-for="item in day.items" :key="item.title"
                    class="group relative bg-base-100/50 rounded-[1.8rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:border-primary/30 transition-all duration-500 cursor-pointer active:scale-95"
                    @click.stop="batchMode ? toggleSelect(item) : handleItemClick(item)">
                    <div class="z-0 bg-base-200/50 relative">
                      <div v-if="batchMode"
                        class="absolute top-3 left-3 z-20 w-7 h-7 rounded-full flex items-center justify-center cursor-pointer transition-all duration-200 pointer-events-none"
                        :class="isSelected(item) ? 'bg-primary shadow-lg ring-2 ring-primary/50' : 'bg-black/40 backdrop-blur-sm border border-white/30 hover:bg-black/60'">
                        <Check v-if="isSelected(item)" :size="14" class="text-primary-content" />
                      </div>
                      <img v-if="item.cover_url" :src="proxyImage(item.cover_url)" :alt="item.title" class="block w-full h-auto object-contain transition-transform duration-1000 group-hover:scale-105" loading="lazy" referrerpolicy="no-referrer"
                        @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" />
                      <div class="aspect-video flex items-center justify-center text-base-content/5" v-if="!item.cover_url">
                        <Image :size="48" />
                      </div>
                      <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent opacity-60 group-hover:opacity-100 transition-opacity pointer-events-none"></div>
                    </div>
                    <div v-if="item.info_hash || subscribedIds[item.bangumi_id]" class="absolute top-3 right-3 z-10 flex items-center gap-1">
                       <div v-if="getItemStats(item)" class="px-1.5 py-0.5 rounded-md bg-black/70 backdrop-blur-md text-[8px] font-black text-white tracking-wider leading-none">
                         {{ getItemStats(item)?.downloaded }}<span v-if="getItemStats(item)?.total">/{{ getItemStats(item)?.total }}</span>
                       </div>
                       <div class="w-8 h-8 rounded-full bg-primary/90 backdrop-blur-md flex items-center justify-center text-primary-content shadow-lg border border-white/20 shadow-lg">
                          <Check :size="16" />
                       </div>
                    </div>
                    <div class="absolute bottom-0 left-0 w-full p-4 z-10">
                       <p class="text-[10px] font-black leading-tight text-white line-clamp-2 uppercase tracking-wide group-hover:text-primary transition-colors">{{ item.title }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </template>
          
          <!-- 普通星期：保持不变 -->
          <template v-else>
            <div class="flex items-center gap-4 group">
              <div class="w-1.5 h-6 bg-primary rounded-full shadow-[0_0_12px_rgba(var(--p),0.5)] group-hover:h-8 transition-all"></div>
              <h2 class="text-2xl font-black tracking-tight italic">{{ day.label }}</h2>
              <span class="text-[10px] font-black uppercase tracking-widest text-base-content/20 mt-1">{{ day.items.length }} {{ $t('schedule.entries') }}</span>
            </div>

            <div class="grid gap-4 sm:gap-6 items-start" style="grid-template-columns: repeat(auto-fill, minmax(160px, 1fr))">
              <div v-for="item in day.items" :key="item.title"
                class="group relative bg-base-100/50 rounded-[1.8rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:border-primary/30 transition-all duration-500 cursor-pointer active:scale-95"
                @click.stop="batchMode ? toggleSelect(item) : handleItemClick(item)">
                <div class="z-0 bg-base-200/50 relative">
                  <div v-if="batchMode"
                    class="absolute top-3 left-3 z-20 w-7 h-7 rounded-full flex items-center justify-center cursor-pointer transition-all duration-200 pointer-events-none"
                    :class="isSelected(item) ? 'bg-primary shadow-lg ring-2 ring-primary/50' : 'bg-black/40 backdrop-blur-sm border border-white/30 hover:bg-black/60'">
                    <Check v-if="isSelected(item)" :size="14" class="text-primary-content" />
                  </div>
                  <img v-if="item.cover_url" :src="proxyImage(item.cover_url)" :alt="item.title" class="block w-full h-auto object-contain transition-transform duration-1000 group-hover:scale-105" loading="lazy" referrerpolicy="no-referrer"
                    @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" />
                  <div class="aspect-video flex items-center justify-center text-base-content/5" v-if="!item.cover_url">
                    <Image :size="48" />
                  </div>
                  <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent opacity-60 group-hover:opacity-100 transition-opacity pointer-events-none"></div>
                </div>
                <div v-if="item.info_hash || subscribedIds[item.bangumi_id]" class="absolute top-3 right-3 z-10 flex items-center gap-1">
                   <div v-if="getItemStats(item)" class="px-1.5 py-0.5 rounded-md bg-black/70 backdrop-blur-md text-[8px] font-black text-white tracking-wider leading-none">
                     {{ getItemStats(item)?.downloaded }}<span v-if="getItemStats(item)?.total">/{{ getItemStats(item)?.total }}</span>
                   </div>
                   <div class="w-8 h-8 rounded-full bg-primary/90 backdrop-blur-md flex items-center justify-center text-primary-content shadow-lg border border-white/20 shadow-lg">
                      <Check :size="16" />
                   </div>
                </div>
                <div class="absolute bottom-0 left-0 w-full p-4 z-10">
                   <p class="text-[10px] font-black leading-tight text-white line-clamp-2 uppercase tracking-wide group-hover:text-primary transition-colors">{{ item.title }}</p>
                </div>
              </div>
            </div>
          </template>
        </div>
      </template>

      <!-- ====== My Subscriptions Timeline ====== -->
      <template v-else>
        <div v-if="Object.keys(subscribedSchedule).length === 0" class="flex flex-col items-center justify-center py-20 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200 max-w-4xl mx-auto">
           <div class="w-24 h-24 bg-base-200/50 rounded-full flex items-center justify-center mb-6">
              <LayoutGrid :size="40" class="opacity-10" />
           </div>
           <h3 class="text-xl font-black tracking-tight mb-2 text-base-content/40">{{ $t('schedule.mineEmpty.title') }}</h3>
           <button class="btn btn-primary btn-sm rounded-xl px-8 uppercase font-black tracking-widest text-[10px] h-10 min-h-0" @click="router.push('/search')">{{ $t('schedule.mineEmpty.action') }}</button>
        </div>

        <div v-for="(items, label) in subscribedSchedule" :key="label" class="space-y-6">
          <div class="flex items-center gap-4 group">
            <div class="w-1.5 h-6 bg-success rounded-full shadow-[0_0_12px_rgba(var(--s),0.5)] group-hover:h-8 transition-all"></div>
            <h2 class="text-2xl font-black tracking-tight italic">{{ label }}</h2>
          </div>

          <div class="grid gap-4 sm:gap-6 items-start" style="grid-template-columns: repeat(auto-fill, minmax(160px, 1fr))">
            <div v-for="item in items" :key="item.title"
              class="group relative bg-base-100/50 rounded-[1.8rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:border-success/30 transition-all duration-500 cursor-pointer active:scale-95"
              @click.stop="batchMode ? toggleSelect(item) : handleItemClick(item)">
              
              <!-- Poster -->
              <div class="z-0 bg-base-200/50 relative">
                <div v-if="batchMode"
                  class="absolute top-3 left-3 z-20 w-7 h-7 rounded-full flex items-center justify-center cursor-pointer transition-all duration-200 pointer-events-none"
                  :class="isSelected(item) ? 'bg-primary shadow-lg ring-2 ring-primary/50' : 'bg-black/40 backdrop-blur-sm border border-white/30 hover:bg-black/60'">
                  <Check v-if="isSelected(item)" :size="14" class="text-primary-content" />
                </div>
                <img v-if="item.cover_url" :src="proxyImage(item.cover_url)" :alt="item.title" class="block w-full h-auto object-contain transition-transform duration-1000 group-hover:scale-105" loading="lazy" referrerpolicy="no-referrer" />
                <div class="aspect-video flex items-center justify-center text-base-content/5" v-if="!item.cover_url">
                  <Image :size="48" />
                </div>
                <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent opacity-60 pointer-events-none"></div>
              </div>

              <div class="absolute top-3 right-3 z-10 flex items-center gap-1">
                 <div v-if="getItemStats(item)" class="px-1.5 py-0.5 rounded-md bg-black/70 backdrop-blur-md text-[8px] font-black text-white tracking-wider leading-none">
                   {{ getItemStats(item)?.downloaded }}<span v-if="getItemStats(item)?.total">/{{ getItemStats(item)?.total }}</span>
                 </div>
                 <div class="w-8 h-8 rounded-full bg-success/90 backdrop-blur-md flex items-center justify-center text-success-content shadow-lg border border-white/20">
                    <Check :size="16" />
                 </div>
              </div>

              <div class="absolute bottom-0 left-0 w-full p-4 z-10">
                 <p class="text-[10px] font-black leading-tight text-white line-clamp-2 uppercase tracking-wide group-hover:text-success transition-colors">{{ item.title }}</p>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 数据来源 -->
    <div class="text-center py-10">
      <p class="text-xs font-medium text-base-content/25 tracking-wide">
        数据来源：<a href="https://yuc.wiki" target="_blank" rel="noopener" class="hover:text-primary transition-colors underline underline-offset-2">長門番堂 (yuc.wiki)</a>
        — 感谢長門有C 的精心维护
      </p>
    </div>

    <!-- 批量订阅浮动栏 -->
    <Transition name="slide-up">
      <div v-if="batchMode" class="fixed bottom-0 left-0 right-0 z-40 p-4 bg-gradient-to-t from-base-200 via-base-200/95 to-transparent pointer-events-none">
        <div class="max-w-4xl mx-auto pointer-events-auto">
          <div class="bg-base-100 rounded-[2rem] border border-base-200/60 shadow-2xl p-4 flex items-center justify-between gap-4">
            <div class="flex items-center gap-4">
              <span class="text-sm font-black tracking-tight">已选 <span class="text-primary">{{ selectedCount }}</span> 部</span>
              <span v-if="defaultSubgroups.length > 0" class="text-[9px] font-black uppercase tracking-widest opacity-30 ml-2">默认: {{ defaultSubgroups.join(', ') }}</span>
              <span v-if="selectedCount > 20" class="text-[8px] font-black text-error uppercase tracking-widest ml-2">(最多20部)</span>
            </div>
            <div class="flex items-center gap-3">
              <button class="btn btn-ghost btn-sm rounded-xl text-[10px] font-black uppercase tracking-widest" @click="exitBatchMode">取消</button>
              <button class="btn btn-primary btn-sm rounded-xl px-6 text-[10px] font-black uppercase tracking-widest gap-2" :disabled="selectedCount === 0 || selectedCount > 20 || batchSubmitting" @click="confirmBatchSubscribe">
                <span v-if="batchSubmitting" class="loading loading-spinner loading-xs"></span>
                <template v-else>确认订阅 {{ selectedCount > 0 ? `(${selectedCount})` : '' }}</template>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 设置默认字幕组 Modal -->
    <dialog :class="['modal', { 'modal-open': defaultSubgroupPickerOpen }]">
      <div class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] max-w-sm w-full p-8">
        <h3 class="text-xl font-black tracking-tight mb-2">设置默认字幕组</h3>
        <p class="text-[10px] font-bold uppercase tracking-widest opacity-30 mb-6">所有已选番剧将使用该字幕组。如需逐部设置，点跳过即可</p>
        
        <div class="space-y-2 max-h-48 overflow-y-auto">
          <button v-for="name in COMMON_SUBGROUPS" :key="name"
            class="w-full text-left px-5 py-3.5 rounded-xl text-sm font-bold transition-all border"
            :class="currentSubgroupSet.has(name) ? 'bg-primary text-primary-content border-primary shadow-lg' : 'bg-base-100 hover:bg-base-300 border-base-200/50'"
            @click="toggleSubgroup(name)">
            <Check v-if="currentSubgroupSet.has(name)" :size="14" class="inline mr-2" />
            {{ name }}
          </button>
        </div>
        
        <div class="mt-4 flex gap-2">
          <input v-model="customSubgroupInput" type="text" placeholder="输入自定义字幕组" class="flex-1 bg-base-100 border border-base-200/50 rounded-xl px-4 py-3 text-sm font-bold outline-none focus:border-primary/30 transition-all"
            @keyup.enter="addCustomSubgroup" />
          <button class="btn btn-ghost btn-sm rounded-xl text-[10px] font-black uppercase tracking-widest px-4" @click="addCustomSubgroup">添加</button>
        </div>
        
        <div v-if="currentSubgroupSet.size > 0" class="mt-4 p-3 bg-base-100/50 rounded-xl">
          <p class="text-[9px] font-black uppercase tracking-widest opacity-30 mb-1">已选默认字幕组</p>
          <div class="flex flex-wrap gap-1.5">
            <span v-for="name in [...currentSubgroupSet]" :key="name"
              class="px-2 py-1 bg-primary/20 text-primary text-[10px] font-black rounded-lg flex items-center gap-1">
              {{ name }}
              <X :size="12" class="cursor-pointer" @click.stop="toggleSubgroup(name)" />
            </span>
          </div>
        </div>
        
        <div class="mt-6 flex gap-3">
          <button class="flex-1 btn btn-ghost rounded-xl text-[10px] font-black uppercase tracking-widest" @click="skipDefaultSubgroups">跳过，逐部设置</button>
          <button class="flex-1 btn btn-primary rounded-xl text-[10px] font-black uppercase tracking-widest" :disabled="currentSubgroupSet.size === 0" @click="confirmDefaultSubgroups">确认</button>
        </div>
      </div>
      <div class="modal-backdrop bg-black/50" @click="skipDefaultSubgroups"></div>
    </dialog>

    <!-- 字幕组选择器 Modal -->
    <dialog :class="['modal', { 'modal-open': subgroupPickerOpen }]">
      <div class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] max-w-sm w-full p-8">
        <h3 class="text-xl font-black tracking-tight mb-2">选择字幕组</h3>
        <p v-if="subgroupPickerItem" class="text-[10px] font-bold uppercase tracking-widest opacity-30 mb-6 truncate">{{ subgroupPickerItem.title }}</p>
        
        <div class="space-y-2 max-h-48 overflow-y-auto">
          <button v-for="name in COMMON_SUBGROUPS" :key="name"
            class="w-full text-left px-5 py-3.5 rounded-xl text-sm font-bold transition-all border"
            :class="currentSubgroupSet.has(name) ? 'bg-primary text-primary-content border-primary shadow-lg' : 'bg-base-100 hover:bg-base-300 border-base-200/50'"
            @click="toggleSubgroup(name)">
            <Check v-if="currentSubgroupSet.has(name)" :size="14" class="inline mr-2" />
            {{ name }}
          </button>
        </div>
        
        <!-- 自定义输入 -->
        <div class="mt-4 flex gap-2">
          <input v-model="customSubgroupInput" type="text" placeholder="输入自定义字幕组" class="flex-1 bg-base-100 border border-base-200/50 rounded-xl px-4 py-3 text-sm font-bold outline-none focus:border-primary/30 transition-all"
            @keyup.enter="addCustomSubgroup" />
          <button class="btn btn-ghost btn-sm rounded-xl text-[10px] font-black uppercase tracking-widest px-4" @click="addCustomSubgroup">添加</button>
        </div>
        
        <!-- 已选显示 -->
        <div v-if="currentSubgroupSet.size > 0" class="mt-4 p-3 bg-base-100/50 rounded-xl">
          <p class="text-[9px] font-black uppercase tracking-widest opacity-30 mb-1">已选字幕组</p>
          <div class="flex flex-wrap gap-1.5">
            <span v-for="name in [...currentSubgroupSet]" :key="name"
              class="px-2 py-1 bg-primary/20 text-primary text-[10px] font-black rounded-lg flex items-center gap-1">
              {{ name }}
              <X :size="12" class="cursor-pointer" @click.stop="toggleSubgroup(name)" />
            </span>
          </div>
        </div>
        
        <div class="mt-6 flex gap-3">
          <button class="flex-1 btn btn-ghost rounded-xl text-[10px] font-black uppercase tracking-widest" @click="cancelSubgroupPicker">取消</button>
          <button class="flex-1 btn btn-primary rounded-xl text-[10px] font-black uppercase tracking-widest" @click="confirmSubgroupPicker">确认</button>
        </div>
      </div>
      <div class="modal-backdrop bg-black/50" @click="cancelSubgroupPicker"></div>
    </dialog>

    <!-- 批量订阅结果 Modal -->
    <dialog :class="['modal', { 'modal-open': batchResultModal }]">
      <div class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] max-w-sm w-full p-8">
        <div class="flex items-center gap-4 mb-6">
          <div v-if="batchResult.failed.length === 0" class="w-12 h-12 rounded-full bg-success/20 flex items-center justify-center">
            <CheckCircle :size="24" class="text-success" />
          </div>
          <div v-else class="w-12 h-12 rounded-full bg-warning/20 flex items-center justify-center">
            <XCircle :size="24" class="text-warning" />
          </div>
          <div class="space-y-1">
            <h3 class="text-xl font-black tracking-tight">批量订阅完成</h3>
            <p class="text-[10px] font-bold uppercase tracking-widest opacity-30">
              成功 {{ batchResult.success.length }} 部，失败 {{ batchResult.failed.length }} 部
            </p>
          </div>
        </div>
        
        <div v-if="batchResult.success.length > 0" class="mb-4">
          <p class="text-[9px] font-black uppercase tracking-widest text-success mb-2">✅ 成功</p>
          <div class="space-y-1 max-h-24 overflow-y-auto">
            <p v-for="s in batchResult.success" :key="s.id" class="text-xs font-bold px-3 py-1.5 bg-success/10 rounded-lg">
              {{ s.title }}
            </p>
          </div>
        </div>
        
        <div v-if="batchResult.failed.length > 0">
          <p class="text-[9px] font-black uppercase tracking-widest text-error mb-2">❌ 失败</p>
          <div class="space-y-1 max-h-32 overflow-y-auto">
            <div v-for="f in batchResult.failed" :key="f.title" class="px-3 py-1.5 bg-error/10 rounded-lg">
              <p class="text-xs font-bold">{{ f.title }}</p>
              <p class="text-[9px] opacity-60">{{ f.error }}</p>
            </div>
          </div>
        </div>
        
        <button class="btn btn-primary w-full rounded-xl text-[10px] font-black uppercase tracking-widest mt-6" @click="batchResultModal = false">知道了</button>
      </div>
      <form method="dialog" class="modal-backdrop" @click="batchResultModal = false">
        <button class="cursor-default">close</button>
      </form>
    </dialog>

    <!-- Anime Detail Modal -->
    <dialog :class="['modal', { 'modal-open': selectedItem }]">
      <div v-if="selectedItem" class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] p-0 overflow-hidden max-w-sm w-full shadow-[0_0_40px_rgba(0,0,0,0.5)]">
        <!-- Header Image -->
        <div class="relative w-full aspect-video bg-base-300">
          <img v-if="selectedItem.cover_url" :src="proxyImage(selectedItem.cover_url)" class="w-full h-full object-contain" referrerpolicy="no-referrer" />
          <div class="absolute inset-0 bg-gradient-to-t from-base-200 to-transparent"></div>
          <button class="btn btn-circle btn-sm btn-ghost absolute top-4 right-4 text-white bg-black/20 hover:bg-black/40 border-none backdrop-blur-md" @click="selectedItem = null">
            <X :size="16" />
          </button>
        </div>
        
        <!-- Content -->
        <div class="p-6 pt-2 space-y-4">
          <div>
            <h3 class="text-xl font-black leading-tight mb-3 text-base-content">{{ selectedItem.title }}</h3>
            <div class="flex flex-wrap gap-2 text-[10px] font-black uppercase tracking-widest text-base-content/60">
              <span class="flex items-center gap-1.5 bg-base-100/50 px-3 py-1.5 rounded-xl border border-base-content/5"><Clock :size="14" /> {{ selectedItem.aired_time || selectedItem.info_hash || 'TBA' }}</span>
              <span class="flex items-center gap-1.5 bg-base-100/50 px-3 py-1.5 rounded-xl border border-base-content/5"><Calendar :size="14" /> {{ selectedItem.aired_date || selectedItem.bangumi_id || 'TBA' }}</span>
            </div>
          </div>
          
          <!-- Synopsis Placeholder -->
          <p class="text-xs font-medium text-base-content/50 leading-relaxed pb-2">
            {{ $t('schedule.modal.noSynopsis') }}
          </p>

          <!-- Action -->
          <div class="pt-2">
            <button class="btn btn-primary w-full rounded-2xl flex gap-3 items-center justify-center font-black tracking-widest text-xs h-14 shadow-lg shadow-primary/20 hover:shadow-primary/40 transition-shadow" @click="router.push(`/search?q=${encodeURIComponent(selectedItem?.title || '')}`)">
              <Search :size="20" />
              {{ $t('schedule.modal.searchAndSub') }}
            </button>
          </div>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop" @click="selectedItem = null">
        <button class="cursor-default">close</button>
      </form>
    </dialog>

    <!-- Onboarding Modal -->
    <OnboardingModal />

    <!-- Changelog Modal -->
    <ChangelogModal :show="showChangelog" :changelog="changelog" @close="showChangelog = false" />
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
