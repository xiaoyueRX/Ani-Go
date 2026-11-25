<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import {
  Antenna, LayoutGrid, RefreshCw,
  AlertTriangle, X, Calendar,
  Image, Check, Clock, Loader2, SlidersHorizontal,
  Search, WifiOff, Info, History, Sparkles, ListChecks, CheckCircle, XCircle
} from 'lucide-vue-next'
import OnboardingModal from '../components/OnboardingModal.vue'
import ChangelogModal from '../components/ChangelogModal.vue'
import { useVersion } from '../composables/useVersion'
import { useAuth } from '../composables/useAuth'
import { useI18n } from 'vue-i18n'
import { resolveMikanSelectionPayload } from '../utils/subscriptionRules'

const { t, locale } = useI18n()
const { isAuthenticated, isOfflineMode, initAuth, checkAuth } = useAuth()

interface TorrentItem {
  title: string
  url: string
  source: string
  bangumi_id: string
  info_hash: string
  cover_url: string
  aired_time?: string
  aired_date?: string
  total_episodes?: number
  is_finished?: boolean
}

interface BangumiEpisode {
  number: number
  title: string
}

interface BangumiAnime {
  id: string
  title_cn: string
  year: number
  total_eps: number
  description: string
  cover_url: string
}

interface WeekDay {
  day_of_week: number
  label: string
  items: TorrentItem[]
}

const router = useRouter()
const weekDays = ref<WeekDay[]>([])
const subscribedIds = ref<Record<string, boolean>>({})
const loading = ref(true)
const scheduleSource = ref<'mikan' | 'bangumi' | 'yuc'>('mikan')
const isYucPluginEnabled = ref(false)
const error = ref('')
const fallbackSourceNotice = ref('')
const activeTab = ref<'schedule' | 'mysub'>('schedule')
const selectedDayFilter = ref<number | 'all'>('all')
const selectedItem = ref<TorrentItem | null>(null)
const selectedMetadata = ref<{ anime: BangumiAnime; episodes: BangumiEpisode[] } | null>(null)
const metaLoading = ref(false)
const subStats = ref<Record<number, { downloaded: number; total: number }>>({})
const subIdByKey = ref<Record<string, number>>({})

interface MikanGroupOption {
  name: string
  rss_url: string
}
const modalGroups = ref<MikanGroupOption[]>([])
const modalGroupsLoading = ref(false)
const modalGroupsError = ref('')
const modalSelectedSubgroups = ref<Set<string>>(new Set())
const modalSubscribing = ref(false)
const modalSubError = ref('')
let modalGroupsController: AbortController | null = null

const normKey = (v?: string) => (v || '').trim().toLowerCase()

function registerSubId(id: unknown, keys: Array<string | undefined>) {
  if (typeof id !== 'number' || id <= 0) return
  const next = { ...subIdByKey.value }
  for (const key of keys) {
    const clean = normKey(key)
    if (clean) next[clean] = id
  }
  subIdByKey.value = next
}

async function loadSubscribedMap() {
  if (!isAuthenticated.value) return
  try {
    const { data } = await request.get('/subscriptions')
    if (Array.isArray(data)) {
      const next: Record<string, number> = {}
      for (const s of data) {
        if (typeof s?.id !== 'number' || s.id <= 0) continue
        for (const k of [s.mikan_id, s.bangumi_id, s.metadata_id, s.title_cn]) {
          const clean = normKey(k)
          if (clean) next[clean] = s.id
        }
      }
      subIdByKey.value = next
    }
  } catch {}
}

function resolveSubscribedId(item: TorrentItem | null): string | number | null {
  if (!item) return null
  if (item.info_hash && item.info_hash !== 'subscribed') return item.info_hash
  const mapped = subIdByKey.value[normKey(item.bangumi_id)] ?? subIdByKey.value[normKey(item.title)]
  if (mapped) return mapped
  const flag = subscribedIds.value[item.bangumi_id]
  if (flag && typeof flag !== 'boolean' && flag !== 'subscribed') return flag
  return null
}

function isItemSubscribed(item: TorrentItem | null): boolean {
  if (!item) return false
  if (resolveSubscribedId(item)) return true
  return Boolean(item.info_hash || subscribedIds.value[item.bangumi_id])
}

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
  '桜都字幕组', '千夏字幕组', '喵萌奶茶屋', 'LoliHouse',
  'Airota', 'ANi', '极影字幕社', 'SweetSub',
  'Haruhana', 'MingY', '星空字幕组',
]

const batchResultModal = ref(false)
const batchResult = ref<{ success: { title: string; id: number }[]; failed: { title: string; error: string }[] }>({ success: [], failed: [] })
const batchSubmitting = ref(false)

async function checkYucPlugin() {
  try {
    const { data } = await request.get('/plugins')
    if (Array.isArray(data)) {
      const p = data.find((item: any) => item.id === 'yuc_schedule')
      isYucPluginEnabled.value = !!(p && p.enabled)
      if (!isYucPluginEnabled.value && scheduleSource.value === 'yuc') {
        scheduleSource.value = 'mikan'
      }
    }
  } catch {}
}

function getSubId(item: TorrentItem): number | null {
  if (item.info_hash && item.info_hash !== 'subscribed') {
    return parseInt(item.info_hash)
  }
  return null
}

function getItemStats(item: TorrentItem) {
  const id = getSubId(item)
  if (id !== null && subStats.value[id]) return subStats.value[id]
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

const selectedYear = ref(currentYear)
const selectedSeason = ref(currentSeason)
const { changelog, showChangelog, checkVersion } = useVersion()

const years = computed(() => {
  const arr = []
  const maxYear = currentSeason === 4 ? currentYear + 1 : currentYear
  for (let y = maxYear; y >= 2000; y--) arr.push(y)
  return arr
})

const allSeasons = computed(() => [
  { value: 1, label: t('schedule.seasons.winter') },
  { value: 2, label: t('schedule.seasons.spring') },
  { value: 3, label: t('schedule.seasons.summer') },
  { value: 4, label: t('schedule.seasons.autumn') },
])

const maxSeason = computed(() => (selectedYear.value > currentYear ? 4 : Math.min(currentSeason + 1, 4)))
const seasons = computed(() => (selectedYear.value < currentYear ? allSeasons.value : allSeasons.value.filter(s => s.value <= maxSeason.value)))

const weekOrder = [1, 2, 3, 4, 5, 6, 7]
const dayNames = computed<Record<number, string>>(() => ({
  1: t('schedule.days.monday'),
  2: t('schedule.days.tuesday'),
  3: t('schedule.days.wednesday'),
  4: t('schedule.days.thursday'),
  5: t('schedule.days.friday'),
  6: t('schedule.days.saturday'),
  7: t('schedule.days.sunday'),
}))

const todayWeekday = computed(() => {
  const d = new Date().getDay()
  return d === 0 ? 7 : d
})

const sortedDays = computed(() =>
  [...weekDays.value]
    .filter(d => d.day_of_week >= 1 && d.day_of_week <= 7)
    .map(d => ({
      ...d,
      label: dayNames.value[d.day_of_week] || d.label,
      items: (d.items || []).filter(item => item && item.title && item.title.trim().length > 0)
    }))
    .sort((a, b) => weekOrder.indexOf(a.day_of_week) - weekOrder.indexOf(b.day_of_week))
)

const dayCounts = computed(() => {
  const map: Record<number, number> = {}
  for (const d of sortedDays.value) map[d.day_of_week] = d.items.length
  return map
})

const totalEntriesCount = computed(() => sortedDays.value.reduce((acc, d) => acc + d.items.length, 0))

const scheduleSearch = ref('')

function setScheduleSource(src: 'mikan' | 'bangumi' | 'yuc') {
  if (scheduleSource.value === src) return
  scheduleSource.value = src
  fallbackSourceNotice.value = ''
  void fetchSchedule(false)
}

const filteredDays = computed(() => {
  let days = sortedDays.value
  if (selectedDayFilter.value !== 'all') {
    days = days.filter(d => d.day_of_week === selectedDayFilter.value)
  }
  if (scheduleSearch.value.trim()) {
    const q = scheduleSearch.value.toLowerCase()
    days = days.map(d => ({
      ...d,
      items: d.items.filter(i => i.title.toLowerCase().includes(q) || (i.bangumi_id && i.bangumi_id.includes(q)))
    })).filter(d => d.items.length > 0)
  }
  return days
})

const subscribedSchedule = computed(() => {
  const map: Record<string, TorrentItem[]> = {}
  for (const day of weekDays.value) {
    if (day.day_of_week < 1 || day.day_of_week > 7) continue
    if (selectedDayFilter.value !== 'all' && day.day_of_week !== selectedDayFilter.value) continue
    let items = (day.items || []).filter(i => i && i.title && isItemSubscribed(i))
    if (scheduleSearch.value.trim()) {
      const q = scheduleSearch.value.toLowerCase()
      items = items.filter(i => i.title.toLowerCase().includes(q) || (i.bangumi_id && i.bangumi_id.includes(q)))
    }
    if (items.length > 0) map[dayNames.value[day.day_of_week] || day.label] = items
  }
  return map
})

const subscribedCount = computed(() => Object.values(subscribedSchedule.value).reduce((acc, items) => acc + items.length, 0))

async function handleItemClick(item: TorrentItem) {
  const subId = resolveSubscribedId(item)
  if (subId) {
    router.push(`/subscriptions/${subId}`)
    return
  }
  modalGroupsController?.abort()
  modalGroupsController = null
  selectedItem.value = item
  modalGroups.value = []
  modalGroupsError.value = ''
  modalSubError.value = ''
  modalSelectedSubgroups.value = new Set()
  modalSubscribing.value = false

  if (item.bangumi_id) {
    metaLoading.value = true
    selectedMetadata.value = null
    const tasks: Promise<void>[] = []

    tasks.push((async () => {
      try {
        const { data } = await request.get(`/bangumi/subject/${item.bangumi_id}`)
        selectedMetadata.value = data
      } catch (e) {
        console.error('Failed to fetch bangumi metadata:', e)
      } finally {
        metaLoading.value = false
      }
    })())

    if (item.source === 'Mikan' && isAuthenticated.value) {
      modalGroupsLoading.value = true
      const controller = new AbortController()
      modalGroupsController = controller
      tasks.push((async () => {
        try {
          const { data } = await request.get(`/mikan/groups?bangumi_id=${encodeURIComponent(item.bangumi_id)}`, {
            signal: controller.signal,
            timeout: 15000,
          })
          if (controller.signal.aborted) return
          modalGroups.value = Array.isArray(data) ? data : []
        } catch (e: any) {
          if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
          modalGroupsError.value = e?.response?.data?.error || t('schedule.modal.groupsFailed')
        } finally {
          if (!controller.signal.aborted) modalGroupsLoading.value = false
        }
      })())
    } else {
      modalGroupsLoading.value = false
    }
    await Promise.all(tasks)
  } else {
    modalGroupsLoading.value = false
  }
}

function toggleModalSubgroup(name: string) {
  const next = new Set(modalSelectedSubgroups.value)
  if (next.has(name)) next.delete(name)
  else next.add(name)
  modalSelectedSubgroups.value = next
}

async function subscribeFromModal() {
  const item = selectedItem.value
  if (!item || modalSubscribing.value) return
  modalSubscribing.value = true
  modalSubError.value = ''

  const picked = Array.from(modalSelectedSubgroups.value)
  const isMikan = item.source === 'Mikan'
  const mikanPayload = isMikan
    ? resolveMikanSelectionPayload(modalGroups.value, picked)
    : null

  const subgroupName = isMikan
    ? mikanPayload?.subgroup_name
    : (picked.length === 1 ? picked[0] : (picked.length > 1 ? '' : undefined))
  const subgroupNames = isMikan
    ? mikanPayload?.subgroup_names
    : (picked.length ? picked : undefined)
  const allowedSubgroups = isMikan
    ? mikanPayload?.allowed_subgroups
    : (picked.length ? picked : undefined)
  const rssUrl = isMikan ? (mikanPayload?.rss_url || undefined) : undefined

  try {
    const title = selectedMetadata.value?.anime?.title_cn?.trim() || item.title
    const { data } = await request.post<{ id?: number }>('/subscriptions', {
      title_cn: title,
      bangumi_id: isMikan ? undefined : item.bangumi_id || undefined,
      mikan_id: isMikan ? item.bangumi_id || undefined : undefined,
      metadata_id: item.source === 'BGM.tv' ? item.bangumi_id : undefined,
      metadata_provider: item.source === 'BGM.tv' ? 'BGM.tv' : undefined,
      cover_url: item.cover_url || '',
      rss_url: rssUrl,
      subgroup_name: subgroupName,
      subgroup_names: subgroupNames,
      allowed_subgroups: allowedSubgroups,
    })
    registerSubId(data?.id, [item.bangumi_id, item.title, title])
    if (item.bangumi_id) {
      subscribedIds.value = { ...subscribedIds.value, [item.bangumi_id]: true }
    }
    window.showToast?.(t('schedule.modal.subscribeSuccess', { title }), 'success')
  } catch (e: any) {
    modalSubError.value = e?.response?.data?.error || t('schedule.modal.subscribeFailed')
  } finally {
    modalSubscribing.value = false
  }
}

function openCustomFormFromModal() {
  const item = selectedItem.value
  if (!item) return
  const title = selectedMetadata.value?.anime?.title_cn?.trim() || item.title
  const picked = Array.from(modalSelectedSubgroups.value)
  selectedItem.value = null
  router.push({
    path: '/subscriptions/new',
    query: {
      title,
      subgroup: picked[0] || undefined,
      bangumi_id: item.bangumi_id || undefined,
    },
  })
}

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const seasonNotice = ref('')
const seasonNoticeDetail = ref('')

function seasonLabel(season: number): string {
  const map: Record<number, string> = {
    1: t('schedule.seasons.winter'),
    2: t('schedule.seasons.spring'),
    3: t('schedule.seasons.summer'),
    4: t('schedule.seasons.autumn'),
  }
  return map[Number(season)] || ''
}

function applyScheduleData(data: any) {
  weekDays.value = data?.days || []
  subscribedIds.value = data?.subscribed || {}
  subStats.value = data?.stats || data?.sub_stats || {}

  const fbYear = Number(data?.fallback_year || 0)
  const fbSeason = Number(data?.fallback_season || 0)
  if (fbYear > 0 && (fbYear !== selectedYear.value || fbSeason !== selectedSeason.value)) {
    const reqYear = Number(data?.requested_year || selectedYear.value)
    const reqSeason = Number(data?.requested_season || selectedSeason.value)
    selectedYear.value = fbYear
    selectedSeason.value = fbSeason
    seasonNoticeDetail.value = t('schedule.fallback.detail', {
      reqYear, reqSeason: seasonLabel(reqSeason), fbYear, fbSeason: seasonLabel(fbSeason),
    })
    seasonNotice.value = t('schedule.fallback.title')
  } else {
    seasonNotice.value = ''
    seasonNoticeDetail.value = ''
  }
}

function gotoPreviousSeason() {
  if (selectedSeason.value > 1) selectedSeason.value -= 1
  else { selectedSeason.value = 4; selectedYear.value -= 1 }
  void fetchSchedule(false)
}

async function fetchSchedule(allowMikanFallback = false) {
  loading.value = true
  error.value = ''
  try {
    let endpoint = '/schedule'
    const params: Record<string, any> = { year: selectedYear.value, season: selectedSeason.value }
    if (scheduleSource.value === 'bangumi') endpoint = '/schedule/bangumi'
    else if (scheduleSource.value === 'yuc') params.source = 'yuc'
    else params.source = 'mikan'

    const { data } = await request.get(endpoint, { params, timeout: 30000 })
    applyScheduleData(data)
  } catch (e: any) {
    if (e.response?.status === 401) {
      await checkAuth()
      if (isAuthenticated.value) {
        try {
          const { data } = await request.get('/schedule', { params: { year: selectedYear.value, season: selectedSeason.value }, timeout: 30000 })
          applyScheduleData(data)
          return
        } catch {}
      } else {
        error.value = t('schedule.error.offlineMode')
        loading.value = false
        return
      }
    }

    // 默认蜜柑源首次加载不可达时，自动温和回退至 Bangumi 每日放送
    if (allowMikanFallback && scheduleSource.value === 'mikan') {
      try {
        scheduleSource.value = 'bangumi'
        const { data } = await request.get('/schedule/bangumi', {
          params: { year: selectedYear.value, season: selectedSeason.value },
          timeout: 30000
        })
        applyScheduleData(data)
        fallbackSourceNotice.value = locale.value === 'zh'
          ? '蜜柑计划 (mikanani.me) 当前网络不可达，已自动切换至 Bangumi 每日放送日历。'
          : 'Mikan is currently unreachable; switched automatically to Bangumi daily calendar.'
        return
      } catch {
        scheduleSource.value = 'mikan'
      }
    }

    error.value = e.code === 'ECONNABORTED' ? t('schedule.error.timeout') : t('schedule.error.failed')
  } finally {
    loading.value = false
  }
}

const MAX_BATCH_SUBSCRIBE = 20

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
  if (newSet.size >= MAX_BATCH_SUBSCRIBE) return
  if (defaultSubgroups.value.length > 0) {
    newSet.add(id)
    selectedItems.value = newSet
    const newMap = new Map(selectedSubgroups.value)
    newMap.set(id, [...defaultSubgroups.value])
    selectedSubgroups.value = newMap
    return
  }
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
  if (newSet.has(name)) newSet.delete(name)
  else newSet.add(name)
  currentSubgroupSet.value = newSet
}

function addCustomSubgroup() {
  const val = customSubgroupInput.value.trim()
  if (!val || val.length > 20 || currentSubgroupSet.value.has(val)) return
  const newSet = new Set(currentSubgroupSet.value)
  newSet.add(val)
  currentSubgroupSet.value = newSet
  customSubgroupInput.value = ''
}

function isSelected(item: TorrentItem): boolean {
  return selectedItems.value.has(item.title)
}

function enterBatchMode() {
  batchMode.value = true
  selectedItems.value = new Set()
  selectedSubgroups.value = new Map()
  defaultSubgroups.value = []
  currentSubgroupSet.value = new Set()
  customSubgroupInput.value = ''
  defaultSubgroupPickerOpen.value = true
}

function exitBatchMode() {
  batchMode.value = false
  selectedItems.value = new Set()
  selectedSubgroups.value = new Map()
  defaultSubgroups.value = []
}

async function confirmBatchSubscribe() {
  if (selectedItems.value.size === 0) return
  const allItems: { item: TorrentItem; title: string; bangumi_id: string; cover_url: string; subgroups: string[] }[] = []
  for (const day of weekDays.value) {
    for (const item of day.items) {
      if (selectedItems.value.has(item.title)) {
        allItems.push({
          item, title: item.title, bangumi_id: item.bangumi_id,
          cover_url: item.cover_url || '', subgroups: selectedSubgroups.value.get(item.title) || [],
        })
      }
    }
  }
  if (allItems.length > MAX_BATCH_SUBSCRIBE) {
    batchResult.value = { success: [], failed: [{ title: `批量订阅最多 ${MAX_BATCH_SUBSCRIBE} 部`, error: '' }] }
    batchResultModal.value = true
    return
  }
  batchSubmitting.value = true
  try {
    const payload = {
      items: allItems.map(i => ({
        title_cn: i.title,
        bangumi_id: i.bangumi_id,
        mikan_id: i.item.source === 'Mikan' ? i.bangumi_id : undefined,
        metadata_id: i.item.source === 'BGM.tv' ? i.bangumi_id : undefined,
        metadata_provider: i.item.source === 'BGM.tv' ? 'BGM.tv' : undefined,
        cover_url: i.cover_url,
        subgroups: i.subgroups,
        total_episodes: i.item.total_episodes || 0,
        is_finished: i.item.is_finished || false,
      }))
    }
    const { data } = await request.post('/subscriptions/batch', payload, { timeout: 30000 })
    batchResult.value = data
    await fetchSchedule(false)
    batchMode.value = false
    selectedItems.value = new Set()
    batchResultModal.value = true
  } catch (e: any) {
    batchResult.value = { success: [], failed: [{ title: '批量订阅失败', error: e.response?.data?.error || e.message }] }
    batchResultModal.value = true
  } finally {
    batchSubmitting.value = false
  }
}

const selectedCount = computed(() => selectedItems.value.size)

onMounted(async () => {
  await checkYucPlugin()
  await initAuth()
  await Promise.all([fetchSchedule(true), loadSubscribedMap()])
  checkVersion()
})

onUnmounted(() => {
  modalGroupsController?.abort()
})
</script>

<template>
  <div class="space-y-5 pb-16">
    <!-- 顶部工作室时间表栏 -->
    <section class="v2-panel p-4 sm:p-5 space-y-4">
      <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        <div class="flex items-center gap-3 min-w-0">
          <div>
            <div class="flex items-center gap-2.5 flex-wrap">
              <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">{{ $t('schedule.title') }}</h1>
              <span v-if="isOfflineMode" class="v2-pill text-amber-400 border-amber-500/30 bg-amber-500/10">
                <WifiOff :size="11" />
                <span>{{ $t('schedule.offlineMode') }}</span>
              </span>
            </div>
            <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('schedule.subtitle') }}</p>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <!-- 数据源切换 -->
          <div class="inline-flex items-center p-0.5 rounded-lg bg-[var(--v2-surface-2)] border border-[var(--v2-border)] text-xs">
            <button
              type="button"
              @click="setScheduleSource('bangumi')"
              class="h-7 px-2.5 rounded-md font-semibold flex items-center gap-1 transition-colors cursor-pointer"
              :class="scheduleSource === 'bangumi' ? 'bg-[var(--v2-accent)] text-white' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
            >
              <Calendar :size="12" />
              <span>{{ $t('schedule.sourceBangumi') }}</span>
            </button>
            <button
              type="button"
              @click="setScheduleSource('mikan')"
              class="h-7 px-2.5 rounded-md font-semibold flex items-center gap-1 transition-colors cursor-pointer"
              :class="scheduleSource === 'mikan' ? 'bg-[var(--v2-accent)] text-white' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
            >
              <Sparkles :size="12" />
              <span>{{ $t('schedule.sourceMikan') }}</span>
            </button>
            <button
              v-if="isYucPluginEnabled"
              type="button"
              @click="setScheduleSource('yuc')"
              class="h-7 px-2.5 rounded-md font-semibold flex items-center gap-1 transition-colors cursor-pointer"
              :class="scheduleSource === 'yuc' ? 'bg-[var(--v2-accent)] text-white' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
            >
              <Antenna :size="12" />
              <span>{{ $t('schedule.sourceYuc') }}</span>
            </button>
          </div>

          <!-- 季度选择 -->
          <div v-if="activeTab === 'schedule'" class="flex items-center gap-1 bg-[var(--v2-surface-2)] border border-[var(--v2-border)] rounded-lg px-1.5 h-8">
            <select v-model="selectedYear" @change="fetchSchedule(false)" class="bg-transparent text-xs font-semibold outline-none cursor-pointer">
              <option v-for="y in years" :key="y" :value="y">{{ y }}</option>
            </select>
            <span class="text-[var(--v2-border-strong)]">/</span>
            <select v-model="selectedSeason" @change="fetchSchedule(false)" class="bg-transparent text-xs font-semibold outline-none cursor-pointer">
              <option v-for="s in seasons" :key="s.value" :value="s.value">{{ s.label }}</option>
            </select>
          </div>

          <!-- 全站时间表 / 我的订阅 -->
          <div class="inline-flex items-center p-0.5 rounded-lg bg-[var(--v2-surface-2)] border border-[var(--v2-border)] text-xs">
            <button
              type="button"
              class="h-7 px-2.5 rounded-md font-semibold flex items-center gap-1 cursor-pointer"
              :class="activeTab === 'schedule' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)]' : 'text-[var(--v2-ink-muted)]'"
              @click="activeTab = 'schedule'"
            >
              <Antenna :size="12" />
              <span>{{ $t('schedule.tabs.all') }}</span>
            </button>
            <button
              type="button"
              class="h-7 px-2.5 rounded-md font-semibold flex items-center gap-1 cursor-pointer"
              :class="activeTab === 'mysub' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)]' : 'text-[var(--v2-ink-muted)]'"
              @click="activeTab = 'mysub'"
            >
              <LayoutGrid :size="12" />
              <span>{{ $t('schedule.tabs.mine') }}</span>
              <span v-if="subscribedCount > 0" class="font-mono text-[10px] text-[var(--v2-accent)]">({{ subscribedCount }})</span>
            </button>
          </div>

          <button v-if="!batchMode" type="button" class="v2-btn v2-btn-sm" @click="enterBatchMode">
            <ListChecks :size="14" />
            <span>{{ $t('schedule.batchSubscribe') }}</span>
          </button>

          <button type="button" class="v2-btn v2-btn-sm w-8 px-0" @click="fetchSchedule(false)" :disabled="loading" :aria-label="$t('common.refresh')">
            <RefreshCw :size="14" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
      </div>

      <!-- Horizontal 7-Day Weekday Strip + 筛选输入框 -->
      <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3 pt-3 border-t border-[var(--v2-border)]">
        <div class="flex items-center gap-1.5 overflow-x-auto no-scrollbar pb-0.5">
          <button
            type="button"
            @click="selectedDayFilter = 'all'"
            class="h-8 px-3 rounded-full text-xs font-semibold border whitespace-nowrap flex items-center gap-1.5 transition-all cursor-pointer"
            :class="selectedDayFilter === 'all'
              ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)]'
              : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)] hover:text-[var(--v2-ink)]'"
          >
            <span>{{ locale === 'zh' ? '全周总览' : 'All Week' }}</span>
            <span class="font-mono text-[10px] opacity-80">{{ totalEntriesCount }}</span>
          </button>

          <button
            v-for="d in weekOrder"
            :key="d"
            type="button"
            @click="selectedDayFilter = d"
            class="h-8 px-3 rounded-full text-xs font-semibold border whitespace-nowrap flex items-center gap-1.5 transition-all cursor-pointer"
            :class="selectedDayFilter === d
              ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)]'
              : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)] hover:text-[var(--v2-ink)]'"
          >
            <span
              v-if="d === todayWeekday"
              class="w-1.5 h-1.5 rounded-full shrink-0"
              :class="selectedDayFilter === d ? 'bg-white' : 'bg-[var(--v2-accent)]'"
              title="Today"
            ></span>
            <span>{{ dayNames[d] }}</span>
            <span class="font-mono text-[10px] opacity-75">{{ dayCounts[d] || 0 }}</span>
          </button>
        </div>

        <div class="relative w-full lg:w-64 shrink-0">
          <Search :size="13" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
          <input v-model="scheduleSearch" type="text" :placeholder="$t('schedule.filterPlaceholder')" class="v2-input pl-8 h-8 text-xs" />
        </div>
      </div>
    </section>

    <!-- 温和的数据源自动回退提示（替代红色错误框） -->
    <div v-if="fallbackSourceNotice && !loading" class="v2-panel border-[var(--v2-accent)]/30 bg-[var(--v2-accent)]/8 px-4 py-3 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2.5 text-xs font-medium text-[var(--v2-ink)]">
        <Info :size="16" class="text-[var(--v2-accent)] shrink-0" />
        <span>{{ fallbackSourceNotice }}</span>
      </div>
      <button type="button" class="p-1 rounded hover:bg-black/10" @click="fallbackSourceNotice = ''">
        <X :size="14" />
      </button>
    </div>

    <!-- 季度回退提示 -->
    <div v-if="seasonNotice && !loading" class="v2-panel border-sky-500/30 bg-sky-500/10 text-sky-300 px-4 py-3 flex items-start justify-between gap-3">
      <div class="flex items-start gap-2.5 text-xs">
        <Info :size="16" class="shrink-0 mt-0.5" />
        <div>
          <p class="font-semibold">{{ seasonNotice }}</p>
          <p class="opacity-85 mt-0.5">{{ seasonNoticeDetail }}</p>
        </div>
      </div>
      <button type="button" class="p-1 rounded hover:bg-sky-500/20" @click="seasonNotice = ''">
        <X :size="14" />
      </button>
    </div>

    <!-- 错误提示（仅用户主动选择某个失败数据源时触发） -->
    <div v-if="error && !loading" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 p-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2.5 text-xs font-medium">
        <AlertTriangle :size="18" class="shrink-0" />
        <div>
          <h3 class="font-semibold">{{ $t('schedule.error.sync') }}</h3>
          <p class="opacity-85 mt-0.5">{{ error }}</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button type="button" class="v2-btn v2-btn-sm" @click="setScheduleSource('bangumi')">
          {{ locale === 'zh' ? '切至 Bangumi 每日放送' : 'Use Bangumi' }}
        </button>
        <button type="button" class="p-1 rounded hover:bg-red-500/20" @click="error = ''">
          <X :size="14" />
        </button>
      </div>
    </div>

    <!-- 加载骨架 -->
    <div v-if="loading" class="grid gap-6">
      <div v-for="i in 2" :key="i" class="space-y-3">
        <div class="h-5 w-28 bg-[var(--v2-surface-2)] rounded animate-pulse"></div>
        <div class="grid gap-4 grid-cols-2 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7">
          <div v-for="j in 7" :key="j" class="aspect-[2/3] bg-[var(--v2-surface-2)] rounded-xl animate-pulse"></div>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div
      v-else-if="(activeTab === 'schedule' ? filteredDays.length === 0 : Object.keys(subscribedSchedule).length === 0)"
      class="v2-panel py-14 px-6 text-center space-y-3"
    >
      <div class="w-12 h-12 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] flex items-center justify-center mx-auto">
        <Calendar :size="22" />
      </div>
      <h3 class="text-sm font-bold text-[var(--v2-ink)]">
        {{ scheduleSearch || selectedDayFilter !== 'all' ? $t('schedule.empty.noMatch') : $t('schedule.empty.title') }}
      </h3>
      <p class="text-xs text-[var(--v2-ink-muted)] max-w-md mx-auto">
        {{ scheduleSearch || selectedDayFilter !== 'all' ? $t('schedule.empty.noMatchDesc') : $t('schedule.empty.desc') }}
      </p>
      <div class="pt-1 flex justify-center gap-2">
        <button v-if="!scheduleSearch && selectedDayFilter === 'all'" type="button" class="v2-btn v2-btn-sm" @click="gotoPreviousSeason">
          <History :size="13" />
          <span>{{ $t('schedule.empty.viewPrevious') }}</span>
        </button>
        <button v-else type="button" class="v2-btn v2-btn-sm" @click="scheduleSearch = ''; selectedDayFilter = 'all'">
          {{ locale === 'zh' ? '显示全周番剧' : 'Show Full Week' }}
        </button>
      </div>
    </div>

    <!-- 主时间表密集海报网格 -->
    <div v-else class="space-y-9">
      <template v-if="activeTab === 'schedule'">
        <div v-for="day in filteredDays" :key="day.day_of_week + day.label" class="space-y-3.5">
          <div class="flex items-center justify-between border-b border-[var(--v2-border)] pb-2">
            <div class="flex items-center gap-2.5">
              <span
                class="w-1.5 h-5 rounded-full"
                :class="day.day_of_week === todayWeekday ? 'bg-[var(--v2-accent)]' : 'bg-[var(--v2-border-strong)]'"
              ></span>
              <h2 class="text-base sm:text-lg font-bold tracking-tight text-[var(--v2-ink)]">{{ day.label }}</h2>
              <span v-if="day.day_of_week === todayWeekday" class="px-2 py-0.5 rounded-full bg-[var(--v2-accent)]/15 text-[var(--v2-accent)] text-[10px] font-bold">
                {{ locale === 'zh' ? '今日放送' : 'TODAY' }}
              </span>
            </div>
            <span class="text-xs font-mono text-[var(--v2-ink-subtle)]">{{ day.items.length }} {{ locale === 'zh' ? '部番剧' : 'entries' }}</span>
          </div>

          <div class="grid gap-3.5 sm:gap-4 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
            <div
              v-for="item in day.items"
              :key="item.title"
              class="group relative v2-panel overflow-hidden hover:border-[var(--v2-accent)]/50 transition-all duration-200 cursor-pointer flex flex-col"
              @click.stop="batchMode ? toggleSelect(item) : handleItemClick(item)"
            >
              <div class="relative aspect-[2/3] w-full overflow-hidden bg-[var(--v2-surface-2)]">
                <div
                  v-if="batchMode"
                  class="absolute top-2.5 left-2.5 z-20 w-6 h-6 rounded-full flex items-center justify-center pointer-events-none"
                  :class="isSelected(item) ? 'bg-[var(--v2-accent)] text-white shadow' : 'bg-black/60 border border-white/30'"
                >
                  <Check v-if="isSelected(item)" :size="13" />
                </div>

                <div class="absolute inset-0 flex items-center justify-center text-[var(--v2-ink-subtle)]/40 pointer-events-none">
                  <Image :size="28" />
                </div>

                <img
                  v-if="item.cover_url"
                  :src="proxyImage(item.cover_url)"
                  :alt="item.title"
                  class="relative z-10 w-full h-full object-cover transition-transform duration-500 group-hover:scale-105"
                  loading="lazy"
                  referrerpolicy="no-referrer"
                  @error="(e: Event) => { (e.target as HTMLImageElement).style.opacity = '0' }"
                />

                <div class="absolute inset-0 z-10 bg-gradient-to-t from-black/90 via-black/25 to-transparent"></div>

                <div v-if="isItemSubscribed(item)" class="absolute top-2.5 right-2.5 z-20 flex items-center gap-1">
                  <span v-if="getItemStats(item)" class="px-1.5 py-0.5 rounded bg-black/75 font-mono text-[10px] font-bold text-white">
                    {{ getItemStats(item)?.downloaded }}<span v-if="getItemStats(item)?.total">/{{ getItemStats(item)?.total }}</span>
                  </span>
                  <div class="w-6 h-6 rounded-full bg-[var(--v2-accent)] flex items-center justify-center text-white shadow">
                    <Check :size="13" />
                  </div>
                </div>

                <div class="absolute bottom-0 inset-x-0 p-3 z-20">
                  <p class="text-xs font-semibold leading-snug text-white line-clamp-2 group-hover:text-[var(--v2-accent-hover)] transition-colors">
                    {{ item.title }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <template v-else>
        <div v-for="(items, label) in subscribedSchedule" :key="label" class="space-y-3.5">
          <div class="flex items-center gap-2.5 border-b border-[var(--v2-border)] pb-2">
            <span class="w-1.5 h-5 rounded-full bg-emerald-500"></span>
            <h2 class="text-base sm:text-lg font-bold tracking-tight text-[var(--v2-ink)]">{{ label }}</h2>
          </div>

          <div class="grid gap-3.5 sm:gap-4 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
            <div
              v-for="item in items"
              :key="item.title"
              class="group relative v2-panel overflow-hidden hover:border-emerald-500/50 transition-all duration-200 cursor-pointer flex flex-col"
              @click.stop="batchMode ? toggleSelect(item) : handleItemClick(item)"
            >
              <div class="relative aspect-[2/3] w-full overflow-hidden bg-[var(--v2-surface-2)]">
                <div class="absolute inset-0 flex items-center justify-center text-[var(--v2-ink-subtle)]/40 pointer-events-none">
                  <Image :size="28" />
                </div>
                <img
                  v-if="item.cover_url"
                  :src="proxyImage(item.cover_url)"
                  :alt="item.title"
                  class="relative z-10 w-full h-full object-cover transition-transform duration-500 group-hover:scale-105"
                  loading="lazy"
                  referrerpolicy="no-referrer"
                  @error="(e: Event) => { (e.target as HTMLImageElement).style.opacity = '0' }"
                />
                <div class="absolute inset-0 z-10 bg-gradient-to-t from-black/90 via-black/25 to-transparent"></div>
                <div class="absolute top-2.5 right-2.5 z-20 flex items-center gap-1">
                  <span v-if="getItemStats(item)" class="px-1.5 py-0.5 rounded bg-black/75 font-mono text-[10px] font-bold text-white">
                    {{ getItemStats(item)?.downloaded }}<span v-if="getItemStats(item)?.total">/{{ getItemStats(item)?.total }}</span>
                  </span>
                  <div class="w-6 h-6 rounded-full bg-emerald-500 flex items-center justify-center text-white shadow">
                    <Check :size="13" />
                  </div>
                </div>
                <div class="absolute bottom-0 inset-x-0 p-3 z-20">
                  <p class="text-xs font-semibold leading-snug text-white line-clamp-2">{{ item.title }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 数据来源脚注 -->
    <div class="text-center py-4">
      <p class="text-xs text-[var(--v2-ink-subtle)]">
        {{ scheduleSource === 'bangumi' ? '数据来源：Bangumi 放送日历 (bgm.tv)' : (scheduleSource === 'yuc' ? $t('schedule.credit') : '数据来源：蜜柑计划 (mikanani.me)') }}
      </p>
    </div>

    <!-- 批量订阅浮动条 -->
    <div v-if="batchMode" class="fixed bottom-16 lg:bottom-5 left-0 right-0 z-50 px-4 pointer-events-none">
      <div class="max-w-3xl mx-auto v2-panel border border-[var(--v2-border-strong)] shadow-2xl px-5 py-3 flex items-center justify-between gap-4 pointer-events-auto">
        <div class="flex items-center gap-3 text-xs font-semibold">
          <span>{{ $t('schedule.batch.selected', { count: selectedCount }) }}</span>
          <span v-if="defaultSubgroups.length > 0" class="text-[var(--v2-ink-muted)]">{{ defaultSubgroups.join(', ') }}</span>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="exitBatchMode">{{ $t('common.cancel') }}</button>
          <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" :disabled="selectedCount === 0 || selectedCount > 20 || batchSubmitting" @click="confirmBatchSubscribe">
            {{ $t('schedule.batch.confirm') }} ({{ selectedCount }})
          </button>
        </div>
      </div>
    </div>

    <!-- 默认字幕组 / 单部字幕组 / 批量结果 Modal -->
    <dialog :class="['modal', { 'modal-open': defaultSubgroupPickerOpen }]">
      <div class="modal-box max-w-sm">
        <h3 class="text-base font-bold mb-1">设置默认字幕组</h3>
        <p class="text-xs text-[var(--v2-ink-muted)] mb-4">所有已选番剧将使用该字幕组。如需逐部设置，点跳过即可。</p>
        <div class="space-y-1.5 max-h-48 overflow-y-auto mb-3">
          <button v-for="name in COMMON_SUBGROUPS" :key="name" type="button" class="w-full text-left px-3 py-2 rounded-lg text-xs font-semibold border transition-colors" :class="currentSubgroupSet.has(name) ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)]' : 'bg-[var(--v2-surface-2)] border-[var(--v2-border)]'" @click="toggleSubgroup(name)">
            {{ name }}
          </button>
        </div>
        <div class="flex gap-2 mb-4">
          <input v-model="customSubgroupInput" type="text" placeholder="自定义字幕组" class="v2-input h-8 text-xs flex-1" @keyup.enter="addCustomSubgroup" />
          <button type="button" class="v2-btn v2-btn-sm" @click="addCustomSubgroup">添加</button>
        </div>
        <div class="flex gap-2 justify-end">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="skipDefaultSubgroups">跳过</button>
          <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" :disabled="currentSubgroupSet.size === 0" @click="confirmDefaultSubgroups">确认</button>
        </div>
      </div>
      <div class="modal-backdrop" @click="skipDefaultSubgroups"></div>
    </dialog>

    <dialog :class="['modal', { 'modal-open': subgroupPickerOpen }]">
      <div class="modal-box max-w-sm">
        <h3 class="text-base font-bold mb-1">选择字幕组</h3>
        <p v-if="subgroupPickerItem" class="text-xs text-[var(--v2-ink-muted)] mb-4 truncate">{{ subgroupPickerItem.title }}</p>
        <div class="space-y-1.5 max-h-48 overflow-y-auto mb-3">
          <button v-for="name in COMMON_SUBGROUPS" :key="name" type="button" class="w-full text-left px-3 py-2 rounded-lg text-xs font-semibold border transition-colors" :class="currentSubgroupSet.has(name) ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)]' : 'bg-[var(--v2-surface-2)] border-[var(--v2-border)]'" @click="toggleSubgroup(name)">
            {{ name }}
          </button>
        </div>
        <div class="flex gap-2 mb-4">
          <input v-model="customSubgroupInput" type="text" placeholder="自定义字幕组" class="v2-input h-8 text-xs flex-1" @keyup.enter="addCustomSubgroup" />
          <button type="button" class="v2-btn v2-btn-sm" @click="addCustomSubgroup">添加</button>
        </div>
        <div class="flex gap-2 justify-end">
          <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="cancelSubgroupPicker">取消</button>
          <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="confirmSubgroupPicker">确认</button>
        </div>
      </div>
      <div class="modal-backdrop" @click="cancelSubgroupPicker"></div>
    </dialog>

    <dialog :class="['modal', { 'modal-open': batchResultModal }]">
      <div class="modal-box max-w-sm space-y-4">
        <div class="flex items-center gap-3">
          <CheckCircle v-if="batchResult.failed.length === 0" :size="22" class="text-emerald-400" />
          <XCircle v-else :size="22" class="text-amber-400" />
          <div>
            <h3 class="text-base font-bold">批量订阅完成</h3>
            <p class="text-xs text-[var(--v2-ink-muted)]">成功 {{ batchResult.success.length }} 部，失败 {{ batchResult.failed.length }} 部</p>
          </div>
        </div>
        <button type="button" class="v2-btn v2-btn-primary v2-btn-sm w-full" @click="batchResultModal = false">知道了</button>
      </div>
      <div class="modal-backdrop" @click="batchResultModal = false"></div>
    </dialog>

    <!-- 番剧快速详情弹窗 -->
    <dialog :class="['modal', { 'modal-open': selectedItem }]">
      <div v-if="selectedItem" class="modal-box max-w-sm p-0 overflow-hidden">
        <div class="relative w-full aspect-[3/3.6] bg-[var(--v2-surface-2)]">
          <img v-if="selectedItem.cover_url" :src="proxyImage(selectedItem.cover_url)" class="w-full h-full object-cover" referrerpolicy="no-referrer" />
          <div class="absolute inset-0 bg-gradient-to-t from-[var(--v2-surface)] via-transparent to-transparent"></div>
          <button type="button" class="w-8 h-8 rounded-full bg-black/55 text-white flex items-center justify-center absolute top-3 right-3 hover:bg-black/80" @click="selectedItem = null">
            <X :size="15" />
          </button>
        </div>
        <div class="p-5 space-y-4">
          <div>
            <h3 class="text-base font-bold text-[var(--v2-ink)]">{{ selectedMetadata?.anime?.title_cn || selectedItem.title }}</h3>
            <div v-if="selectedMetadata?.anime?.year" class="text-xs font-mono text-[var(--v2-accent)] mt-1">
              {{ selectedMetadata.anime.year }} • {{ selectedMetadata.anime.total_eps || '?' }} EP
            </div>
            <div class="flex flex-wrap gap-2 mt-2 text-xs text-[var(--v2-ink-muted)]">
              <span class="v2-pill"><Clock :size="11" /> {{ selectedItem.aired_time || 'TBA' }}</span>
              <span class="v2-pill"><Calendar :size="11" /> {{ selectedItem.aired_date || 'TBA' }}</span>
            </div>
          </div>

          <div v-if="metaLoading" class="py-6 flex justify-center">
            <span class="loading loading-spinner loading-md text-[var(--v2-accent)]"></span>
          </div>
          <p v-else-if="selectedMetadata?.anime?.description" class="text-xs text-[var(--v2-ink-muted)] leading-relaxed line-clamp-4">
            {{ selectedMetadata.anime.description }}
          </p>

          <!-- 内联快速订阅与已订阅直达 -->
          <div v-if="isItemSubscribed(selectedItem)" class="space-y-2 pt-1">
            <div class="rounded-xl border border-emerald-500/35 bg-emerald-500/10 px-3 py-2 text-xs font-semibold text-emerald-400 flex items-center gap-2">
              <Check :size="14" />
              <span>{{ $t('schedule.modal.alreadySubscribed') }}</span>
            </div>
            <button
              v-if="resolveSubscribedId(selectedItem)"
              type="button"
              class="v2-btn v2-btn-primary w-full"
              @click="() => { const id = resolveSubscribedId(selectedItem); selectedItem = null; if (id) router.push(`/subscriptions/${id}`) }"
            >
              <span>{{ $t('schedule.modal.viewDetail') }}</span>
            </button>
          </div>

          <div v-else-if="isAuthenticated" class="space-y-2.5 pt-1 border-t border-[var(--v2-border)]">
            <p v-if="modalSubError" role="alert" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 rounded-lg px-2.5 py-1.5">
              {{ modalSubError }}
            </p>

            <!-- Mikan 源拉取专属字幕组列表（多选复选框） -->
            <template v-if="selectedItem.source === 'Mikan'">
              <div v-if="modalGroupsLoading" class="flex items-center gap-2 text-xs text-[var(--v2-ink-muted)] py-1">
                <Loader2 :size="13" class="animate-spin text-[var(--v2-accent)]" />
                <span>{{ $t('schedule.modal.loadingGroups') }}</span>
              </div>
              <div v-else-if="modalGroups.length > 0" class="space-y-1.5">
                <div class="flex items-center justify-between gap-2 text-[11px]">
                  <p class="font-semibold text-[var(--v2-ink-muted)]">{{ $t('schedule.modal.pickMikanGroup') }}</p>
                  <span v-if="modalSelectedSubgroups.size > 0" class="font-mono text-[var(--v2-accent)] font-semibold">
                    {{ $t('schedule.modal.selectedCount', { count: modalSelectedSubgroups.size }) }}
                  </span>
                </div>
                <div class="max-h-36 overflow-y-auto space-y-1 pr-0.5">
                  <label
                    v-for="g in modalGroups"
                    :key="g.rss_url"
                    class="w-full rounded-lg border px-2.5 py-2 flex items-center gap-2.5 text-left text-xs transition-colors cursor-pointer select-none"
                    :class="modalSelectedSubgroups.has(g.name)
                      ? 'bg-[var(--v2-accent)]/12 border-[var(--v2-accent)] text-[var(--v2-ink)]'
                      : 'v2-panel-muted hover:border-[var(--v2-accent)]/40 text-[var(--v2-ink)]'"
                  >
                    <input
                      type="checkbox"
                      class="checkbox checkbox-sm checkbox-primary shrink-0"
                      :checked="modalSelectedSubgroups.has(g.name)"
                      :disabled="modalSubscribing"
                      @change="toggleModalSubgroup(g.name)"
                    />
                    <span class="truncate font-medium flex-1">{{ g.name }}</span>
                  </label>
                </div>
              </div>
              <p v-else-if="modalGroupsError" class="text-[11px] text-amber-400">{{ modalGroupsError }}</p>
            </template>

            <!-- Bangumi / Yuc 或无专属组时的通用字幕组复选框多选 -->
            <div v-if="selectedItem.source !== 'Mikan' || (!modalGroupsLoading && modalGroups.length === 0)" class="space-y-1.5">
              <div class="flex items-center justify-between gap-2 text-[11px]">
                <p class="font-semibold text-[var(--v2-ink-muted)]">{{ $t('schedule.modal.quickSubgroups') }}</p>
                <span v-if="modalSelectedSubgroups.size > 0" class="font-mono text-[var(--v2-accent)] font-semibold">
                  {{ $t('schedule.modal.selectedCount', { count: modalSelectedSubgroups.size }) }}
                </span>
              </div>
              <div class="flex flex-wrap gap-1.5">
                <label
                  v-for="grp in COMMON_SUBGROUPS.slice(0, 6)"
                  :key="grp"
                  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] border transition-colors cursor-pointer select-none"
                  :class="modalSelectedSubgroups.has(grp)
                    ? 'bg-[var(--v2-accent)]/15 border-[var(--v2-accent)] text-[var(--v2-accent)] font-semibold'
                    : 'bg-[var(--v2-surface-2)] border-[var(--v2-border)] text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
                >
                  <input
                    type="checkbox"
                    class="checkbox checkbox-xs checkbox-primary"
                    :checked="modalSelectedSubgroups.has(grp)"
                    :disabled="modalSubscribing"
                    @change="toggleModalSubgroup(grp)"
                  />
                  <span>{{ grp }}</span>
                </label>
              </div>
            </div>

            <div class="flex gap-2">
              <button
                type="button"
                class="v2-btn v2-btn-primary flex-1"
                :disabled="modalSubscribing"
                @click="subscribeFromModal()"
              >
                <Loader2 v-if="modalSubscribing" :size="14" class="animate-spin" />
                <Check v-else :size="14" />
                <span>{{ modalSubscribing ? $t('schedule.modal.subscribing') : $t('schedule.modal.directSub') }}</span>
                <span v-if="!modalSubscribing && modalSelectedSubgroups.size > 0" class="font-mono text-[11px] opacity-85">({{ modalSelectedSubgroups.size }})</span>
              </button>
              <button
                type="button"
                class="v2-btn shrink-0"
                :title="$t('schedule.modal.customRules')"
                @click="openCustomFormFromModal"
              >
                <SlidersHorizontal :size="14" />
              </button>
            </div>
          </div>

          <button type="button" class="v2-btn w-full" :class="{ 'v2-btn-primary': !isAuthenticated && !isItemSubscribed(selectedItem) }" @click="router.push(`/search?q=${encodeURIComponent(selectedItem?.title || '')}`)">
            <Search :size="15" />
            <span>{{ $t('schedule.modal.searchAndSub') }}</span>
          </button>
        </div>
      </div>
      <div class="modal-backdrop" @click="selectedItem = null"></div>
    </dialog>

    <OnboardingModal />
    <ChangelogModal :show="showChangelog" :changelog="changelog" @close="showChangelog = false" />
  </div>
</template>

<style scoped>
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
