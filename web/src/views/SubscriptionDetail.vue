<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  ChevronLeft, RefreshCw, AlertTriangle, 
  Image, Edit3, Pause, Play, 
  History, Download, Check, 
  MoreVertical, FileText, X,
  Trash2,
  Shield
} from 'lucide-vue-next'
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
  avatar_url?: string
  excluded_keywords?: string
  skip_bulk_update?: boolean
  allowed_subgroups?: string
  stall_timeout_hours: number
}

interface Episode {
  id: number; subscription_id: number; season: number; number: number
  title: string; status: string; manual_status?: string; torrent_hash: string; torrent_url: string
  original_name: string; final_path: string; file_size: number
  is_stalled: boolean; group_name?: string
  review_reason?: string; review_kind?: string; source_file?: string
  can_retry_organize?: boolean; can_assign_episode?: boolean; can_confirm_movie?: boolean
  download_started_at: string; created_at: string
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const { t, locale } = useI18n({ useScope: 'global' })

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const sub = ref<Subscription | null>(null)
const episodes = ref<Episode[]>([])
const loading = ref(true)
const error = ref('')
const actionError = ref('')
let detailRead = 0
const showEdit = ref(false)
const editDialog = ref<HTMLDialogElement | null>(null)
const updatingEps = ref<Set<number>>(new Set())
const selectedEpisodes = ref<Set<number>>(new Set())
const showBulkActions = ref(false)
const bulkActionLoading = ref(false)
const reviewErrors = ref<Record<number, string>>({})
const assignment = ref<{ id: number; sourceFile: string; season: number | ''; number: number | '' } | null>(null)
const validAssignment = computed(() => {
  if (!assignment.value) return false
  const { season, number } = assignment.value
  return typeof season === 'number' && Number.isInteger(season) && season >= 0 && season <= 99 &&
    typeof number === 'number' && number > 0 && number <= 2000
})

function reviewConfirmation(ep: Episode) {
  if (ep.status !== 'needs_review' || !ep.source_file) return
  if (ep.review_kind === 'episode_identity' && ep.can_assign_episode) return 'assign'
  if (ep.review_kind === 'movie_selection' && ep.can_confirm_movie) return 'movie'
}

function editAssignment(ep: Episode) {
  if (reviewConfirmation(ep) !== 'assign' || !ep.source_file || updatingEps.value.has(ep.id) || bulkActionLoading.value) return
  assignment.value = { id: ep.id, sourceFile: ep.source_file, season: ep.season, number: ep.number > 0 ? ep.number : '' }
}

async function retryWithAssignment(ep: Episode) {
  const draft = assignment.value
  if (!ep.can_assign_episode || !draft || ep.id !== draft.id || ep.source_file !== draft.sourceFile || !validAssignment.value) return
  await retryOrganize(ep, { season: Number(draft.season), number: Number(draft.number) })
}

async function retryOrganize(ep: Episode, correction?: { season: number; number: number } | { confirm_movie: true }) {
  if (ep.status !== 'needs_review' || updatingEps.value.has(ep.id) || bulkActionLoading.value) return
  if (correction) {
    if (reviewConfirmation(ep) !== ('confirm_movie' in correction ? 'movie' : 'assign')) return
  } else if (!ep.can_retry_organize) return
  updatingEps.value.add(ep.id)
  delete reviewErrors.value[ep.id]
  actionError.value = ''
  try {
    const { data } = await request.post(`/episodes/${ep.id}/retry-organize`, correction)
    const current = episodes.value.find(row => row.id === ep.id)
    if (current) {
      current.status = data.status
      current.review_reason = ''
      if (correction && 'season' in correction) Object.assign(current, correction)
    }
    if (assignment.value?.id === ep.id) assignment.value = null
    await fetchDetail()
  } catch (e: any) {
    const message = e.response?.data?.error || t(!correction && ep.review_kind ? 'detail.review.recheckFailed' : 'detail.review.failed')
    if ([404, 409].includes(e.response?.status)) await fetchDetail()
    if (episodes.value.some(row => row.id === ep.id && row.status === 'needs_review')) reviewErrors.value[ep.id] = message
    else actionError.value = message
  } finally {
    updatingEps.value.delete(ep.id)
  }
}

async function setEpisodeMark(ep: Episode, event: Event) {
  const select = event.target as HTMLSelectElement
  const mark = select.value
  select.value = ep.manual_status || ''
  if (updatingEps.value.has(ep.id) || bulkActionLoading.value || mark === select.value) return
  updatingEps.value.add(ep.id)
  actionError.value = ''
  try {
    const { data } = await request.put(`/episodes/${ep.id}/status`, { status: mark })
    ep.manual_status = data.manual_status
    await fetchDetail()
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
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
  const sequence = ++detailRead
  loading.value = true
  try {
    const { data } = await request.get(`/subscriptions/${id}`)
    if (sequence !== detailRead) return
    error.value = ''
    sub.value = data.subscription
    episodes.value = data.episodes || []
    const visible = new Set(episodes.value.map(ep => ep.id))
    selectedEpisodes.value = new Set([...selectedEpisodes.value].filter(id => visible.has(id)))
    showBulkActions.value = selectedEpisodes.value.size > 0
  } catch (e: any) {
    if (sequence !== detailRead) return
    error.value = e.response?.data?.error || t('detail.error.load')
    if ([401, 403, 404].includes(e.response?.status)) {
      sub.value = null
      episodes.value = []
      clearSelection()
    }
  } finally {
    if (sequence === detailRead) loading.value = false
  }
}

async function handleSaveEdit(updated: Record<string, any>) {
  try {
    const { data } = await request.put(`/subscriptions/${id}`, updated)
    ++detailRead
    loading.value = false
    sub.value = data
    closeEditDialog()
    return true
  } catch (e: any) {
    alert(e.response?.data?.error || t('detail.error.update'))
    return false
  }
}

const statusCfg = computed<Record<string, { label: string; icon: any; cls: string }>>(() => ({
  pending: { label: t('detail.logs.status.pending'), icon: History, cls: 'bg-base-200 text-base-content/40 border-base-300' },
  submitting: { label: t('detail.logs.status.submitting'), icon: Download, cls: 'bg-primary/10 text-primary border-primary/20' },
  retrying: { label: t('detail.logs.status.retrying'), icon: RefreshCw, cls: 'bg-warning/10 text-warning border-warning/20' },
  downloaded: { label: t('detail.logs.status.downloaded'), icon: Check, cls: 'bg-primary/10 text-primary border-primary/20' },
  renaming: { label: t('detail.logs.status.organizing'), icon: FileText, cls: 'bg-primary/10 text-primary border-primary/20' },
  needs_review: { label: t('detail.logs.status.needsReview'), icon: AlertTriangle, cls: 'bg-warning/10 text-warning border-warning/20' },
  downloading: { label: t('detail.logs.status.active'), icon: Download, cls: 'bg-primary/10 text-primary border-primary/20' },
  completed: { label: t('detail.logs.status.finished'), icon: Check, cls: 'bg-success/10 text-success border-success/20' },
  organized: { label: t('detail.logs.status.organized'), icon: Check, cls: 'bg-success/10 text-success border-success/20' },
  failed: { label: t('detail.logs.status.failed'), icon: AlertTriangle, cls: 'bg-error/10 text-error border-error/20' },
}))

function formatSize(bytes: number): string {
  if (!bytes) return '-'
  if (bytes > 1e9) return (bytes / 1e9).toFixed(2) + ' GB'
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(0) + ' KB'
}

// 按字幕组分组的剧集列表
const groupedEpisodes = computed(() => {
  const groups: Record<string, Episode[]> = {}
  for (const ep of episodes.value) {
    const key = ep.group_name || t('detail.logs.ungrouped')
    if (!groups[key]) groups[key] = []
    groups[key].push(ep)
  }
  // 按组名排序，未分组放最后
  const sorted = Object.entries(groups).sort(([a], [b]) => {
    if (a === t('detail.logs.ungrouped')) return 1
    if (b === t('detail.logs.ungrouped')) return -1
    return a.localeCompare(b)
  })
  return sorted
})

const progress = computed(() => {
  if (!sub.value || sub.value.total_episodes <= 0) return null
  return Math.min(100, (sub.value.current_episodes / sub.value.total_episodes) * 100)
})

// 多选相关函数
function toggleEpisodeSelect(epId: number) {
  if (bulkActionLoading.value) return
  if (selectedEpisodes.value.has(epId)) {
    selectedEpisodes.value.delete(epId)
  } else {
    selectedEpisodes.value.add(epId)
  }
  showBulkActions.value = selectedEpisodes.value.size > 0
}

function toggleSelectAll(groupEps: Episode[]) {
  if (bulkActionLoading.value) return
  const allSelected = groupEps.every(ep => selectedEpisodes.value.has(ep.id))
  if (allSelected) {
    groupEps.forEach(ep => selectedEpisodes.value.delete(ep.id))
  } else {
    groupEps.forEach(ep => selectedEpisodes.value.add(ep.id))
  }
  showBulkActions.value = selectedEpisodes.value.size > 0
}

function clearSelection() {
  selectedEpisodes.value.clear()
  showBulkActions.value = false
}

async function bulkUpdateStatus(newStatus: string) {
  if (!selectedEpisodes.value.size || bulkActionLoading.value || updatingEps.value.size) return
  const ids = [...selectedEpisodes.value]
  bulkActionLoading.value = true
  actionError.value = ''
  try {
    const failures = new Set<number>()
    for (const epId of ids) {
      try {
        await request.put(`/episodes/${epId}/status`, { status: newStatus })
      } catch {
        failures.add(epId)
      }
    }
    selectedEpisodes.value = failures
    showBulkActions.value = failures.size > 0
    if (failures.size) actionError.value = t('detail.error.partialUpdate', { count: failures.size, total: ids.length })
    await fetchDetail()
  } finally {
    bulkActionLoading.value = false
  }
}

async function bulkDeleteEpisodes(deleteFiles: boolean) {
  if (!selectedEpisodes.value.size || bulkActionLoading.value || updatingEps.value.size) return
  if (!confirm(t(deleteFiles ? 'detail.bulk.confirmFiles' : 'detail.bulk.confirmRecords', { count: selectedEpisodes.value.size }))) return
  
  bulkActionLoading.value = true
  try {
    const ids = Array.from(selectedEpisodes.value)
    await request.post(`/subscriptions/${id}/episodes/batch-delete`, { ids, delete_files: deleteFiles })
    await fetchDetail()
    clearSelection()
  } catch (e: any) {
    await fetchDetail()
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
    bulkActionLoading.value = false
  }
}

async function addToBlacklist() {
  if (!selectedEpisodes.value.size || !sub.value || bulkActionLoading.value || updatingEps.value.size) return
  
  const selectedEps = episodes.value.filter(ep => selectedEpisodes.value.has(ep.id))
  const keywords = new Set<string>()
  
  selectedEps.forEach(ep => {
    // 从文件名提取关键词
    const name = ep.original_name || ep.title
    const parts = name.split(/[\s\.\-_\[\]\(\)]+/)
    parts.forEach(p => {
      if (p.length > 3 && !/^\d+$/.test(p)) {
        keywords.add(p.toLowerCase())
      }
    })
  })
  
  let currentExcluded = []
  if (sub.value.excluded_keywords) {
    try {
      currentExcluded = JSON.parse(sub.value.excluded_keywords)
      if (!Array.isArray(currentExcluded)) currentExcluded = []
    } catch (e) {
      currentExcluded = []
    }
  }
  const merged = Array.from(new Set([...currentExcluded, ...keywords]))
  
  try {
    bulkActionLoading.value = true
    if (!await handleSaveEdit({ excluded_keywords: JSON.stringify(merged) })) return
    window.showToast?.(t('detail.bulk.blocked', { count: merged.length - new Set(currentExcluded).size }), 'success')
    clearSelection()
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
    bulkActionLoading.value = false
  }
}

function refreshVisibleDetail() {
  if (!document.hidden && !loading.value && !showEdit.value && !bulkActionLoading.value && !updatingEps.value.size) {
    void fetchDetail()
  }
}

let refreshTimer: number
onMounted(() => {
  void fetchDetail()
  refreshTimer = window.setInterval(refreshVisibleDetail, 30_000)
  window.addEventListener('focus', refreshVisibleDetail)
  document.addEventListener('visibilitychange', refreshVisibleDetail)
})
onUnmounted(() => {
  ++detailRead
  window.clearInterval(refreshTimer)
  window.removeEventListener('focus', refreshVisibleDetail)
  document.removeEventListener('visibilitychange', refreshVisibleDetail)
})
</script>

<template>
  <div class="space-y-10 pb-20">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <button class="btn btn-ghost border-base-300 rounded-2xl px-6 hover:bg-base-200 transition-all active:scale-95 group" @click="router.push('/subscriptions')">
        <ChevronLeft :size="20" class="group-hover:-translate-x-1 transition-transform" />
        <span class="text-xs font-black uppercase tracking-widest">{{ t('detail.back') }}</span>
      </button>
      
      <div class="flex gap-2">
         <button class="btn btn-ghost btn-circle hover:bg-base-200" :aria-label="t('common.refresh')" @click="fetchDetail" :disabled="loading">
            <RefreshCw :size="20" class="opacity-40" />
         </button>
      </div>
    </div>

    <div v-if="actionError" role="alert" class="alert alert-error break-words">{{ actionError }}</div>
    <div v-if="loading && !sub" class="flex justify-center py-32">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <div v-if="error" role="alert" class="max-w-4xl mx-auto">
       <div class="alert bg-error/10 border-error/20 text-error rounded-[2rem] p-6">
          <AlertTriangle :size="24" class="shrink-0" />
          <div class="flex-1">
             <h3 class="font-black text-sm uppercase tracking-widest">{{ t('detail.error.load') }}</h3>
             <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
          </div>
       </div>
    </div>

    <template v-if="sub">
      <!-- Main Content Card -->
      <div class="group relative bg-base-100 rounded-[3rem] border border-base-200/60 shadow-xl overflow-hidden">
        <!-- Hero Background -->
        <div class="absolute inset-x-0 top-0 h-64 z-0 overflow-hidden">
           <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-cover blur-3xl opacity-20 scale-125" />
           <div class="absolute inset-0 bg-gradient-to-b from-transparent to-base-100"></div>
        </div>

        <div class="p-4 sm:p-8 lg:p-12 relative z-10">
          <div class="flex flex-col lg:flex-row gap-6 lg:gap-12">
             <!-- Left: Poster -->
             <div class="w-full lg:w-1/3 xl:w-1/4 shrink-0 flex justify-center lg:justify-start">
                <div class="w-full max-w-[240px] sm:max-w-sm lg:max-w-none aspect-[3/3.8] rounded-2xl sm:rounded-[2.5rem] bg-base-200 overflow-hidden shadow-2xl border border-white/5 relative group/poster">
                   <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-contain bg-black transition-transform duration-1000 group-hover/poster:scale-110" />
                   <div v-else class="w-full h-full flex items-center justify-center text-base-content/10">
                      <Image :size="80" />
                   </div>
                   <!-- Overlay badge -->
                   <div class="absolute top-4 left-4">
                      <span class="bg-primary/90 backdrop-blur-md text-primary-content text-[10px] font-black uppercase tracking-widest px-4 py-2 rounded-full shadow-lg border border-white/10">
                        #{{ sub.id }}
                      </span>
                   </div>
                </div>
             </div>

             <!-- Right: Information -->
            <div class="flex-1 space-y-6 sm:space-y-8">
               <div class="space-y-3 sm:space-y-4">
              <div class="flex flex-wrap items-center gap-2 sm:gap-3">
                  <span v-if="sub.anime_type && sub.anime_type !== t('detail.type.unknown')" class="text-[10px] font-black uppercase tracking-[0.3em] text-primary bg-primary/10 px-3 py-1 sm:px-4 sm:py-1.5 rounded-full border border-primary/20">
                    {{ sub.anime_type }}
                  </span>
                  <span v-if="sub.completed" class="text-[10px] font-black uppercase tracking-[0.3em] text-success bg-success/10 px-3 py-1 sm:px-4 sm:py-1.5 rounded-full border border-success/20">
                    {{ t('detail.status.finished') }}
                  </span>
                  <span v-if="!sub.enabled" class="text-[10px] font-black uppercase tracking-[0.3em] text-warning bg-warning/10 px-3 py-1 sm:px-4 sm:py-1.5 rounded-full border border-warning/20">
                    {{ t('detail.status.paused') }}
                  </span>
               </div>
                   
                   <div class="space-y-2">
                      <h1 class="text-2xl sm:text-4xl lg:text-5xl font-black tracking-tight leading-tight">{{ sub.title_cn }}</h1>
                      <p v-if="sub.title_jp" class="text-sm sm:text-lg font-bold opacity-30 tracking-tight">{{ sub.title_jp }}</p>
                      <p v-if="sub.title_en" class="text-xs sm:text-sm font-bold opacity-20 tracking-widest uppercase">{{ sub.title_en }}</p>
                   </div>
                </div>

                <!-- Stats Grid -->
                <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 sm:gap-6 p-4 sm:p-8 bg-base-200/50 rounded-2xl sm:rounded-[2rem] border border-base-300/30">
                   <div class="space-y-1">
                      <p class="text-[9px] font-black uppercase tracking-widest opacity-30">{{ t('detail.stats.releaseYear') }}</p>
                      <p class="text-lg font-black">{{ sub.year || 'N/A' }}</p>
                   </div>
                   <div class="space-y-1">
                      <p class="text-[9px] font-black uppercase tracking-widest opacity-30">{{ t('detail.stats.season') }}</p>
                      <p class="text-lg font-black">{{ sub.season || '1' }}</p>
                   </div>
                   <div class="space-y-1">
                      <p class="text-[9px] font-black uppercase tracking-widest opacity-30">{{ t('detail.stats.metadata') }}</p>
                      <p class="text-lg font-black">{{ sub.metadata_provider?.trim() || t('detail.stats.metadataNotRecorded') }}</p>
                   </div>
                </div>

                <!-- Progress Section -->
                <div class="space-y-4">
                   <div class="flex items-end justify-between px-2">
                      <div class="space-y-1">
                         <p class="text-[10px] font-black uppercase tracking-widest opacity-30 leading-none">{{ t('detail.progress.title') }}</p>
                         <p class="text-2xl font-black leading-none mt-2">{{ sub.current_episodes }} <span class="text-base-content/20 mx-1">/</span> {{ sub.total_episodes || '?' }}</p>
                      </div>
                      <div v-if="progress !== null" class="text-right">
                         <p class="text-3xl font-black tracking-tighter text-primary">{{ Math.round(progress) }}%</p>
                      </div>
                   </div>
                   <div v-if="progress !== null" class="h-4 w-full bg-base-300 rounded-full overflow-hidden p-1 border border-base-300 shadow-inner">
                      <div 
                        class="h-full rounded-full bg-gradient-to-r from-primary to-secondary transition-all duration-1000 ease-out shadow-[0_0_12px_rgba(var(--p),0.4)]"
                        :style="{ width: `${progress}%` }"
                      ></div>
                   </div>
                </div>

                <!-- Actions -->
                <div class="flex flex-wrap gap-4 pt-4">
                   <button class="btn btn-primary h-14 rounded-2xl px-10 shadow-xl shadow-lg gap-3 group/btn transition-all active:scale-95" @click="openEditDialog">
                      <Edit3 :size="20" />
                      <span class="text-xs font-black uppercase tracking-widest">{{ t('detail.actions.modify') }}</span>
                   </button>
                   <button 
                     class="btn h-14 rounded-2xl px-10 gap-3 transition-all active:scale-95"
                     :class="sub.enabled ? 'btn-ghost bg-warning/10 text-warning border-warning/20 hover:bg-warning hover:text-warning-content' : 'btn-ghost bg-success/10 text-success border-success/20 hover:bg-success hover:text-success-content'"
                     @click="handleSaveEdit({ enabled: !sub.enabled })"
                   >
                      <component :is="sub.enabled ? Pause : Play" :size="20" />
                      <span class="text-xs font-black uppercase tracking-widest">{{ sub.enabled ? t('detail.actions.suspend') : t('detail.actions.resume') }}</span>
                   </button>
                </div>

                <!-- Description -->
                <div v-if="sub.description" class="mt-12 space-y-4">
                   <div class="flex items-center gap-4">
                      <div class="w-1.5 h-4 bg-primary rounded-full"></div>
                      <h3 class="text-sm font-black uppercase tracking-[0.2em] opacity-30 italic">{{ t('detail.synopsis') }}</h3>
                   </div>
                   <p class="text-base-content/70 leading-relaxed text-sm font-bold">{{ sub.description }}</p>
                </div>

                <!-- Subgroup List -->
                <div class="mt-10 space-y-4">
                   <div class="flex items-center gap-4">
                      <div class="w-1.5 h-4 bg-secondary rounded-full"></div>
                      <h3 class="text-sm font-black uppercase tracking-[0.2em] opacity-30 italic">{{ t('detail.stats.subgroup') }}</h3>
                   </div>
                   <div class="flex flex-wrap gap-2">
                      <span v-if="sub.subgroup_name" class="px-4 py-2 bg-base-200 rounded-xl text-sm font-bold border border-base-300">
                         {{ sub.subgroup_name }}
                      </span>
                      <span v-else class="px-4 py-2 bg-base-200/50 rounded-xl text-sm font-bold opacity-30 border border-base-300/50">
                         {{ t('card.generic') }}
                      </span>
                   </div>
                </div>
             </div>
            </div>
          </div>
        </div>
      <!-- Warning Panel -->
      <Transition name="fade">
       <div v-if="sub.stalled_episodes > 0" class="max-w-4xl mx-auto">
          <div class="alert bg-warning/10 border border-warning/20 text-warning rounded-[2.5rem] p-8 flex items-start gap-6 shadow-2xl shadow-lg">
             <div class="w-16 h-16 rounded-2xl bg-warning/20 flex items-center justify-center shrink-0">
                <AlertTriangle :size="32" />
             </div>
             <div class="flex-1 pt-1">
                <h3 class="text-lg font-black uppercase tracking-widest">{{ t('detail.warning.title') }}</h3>
                <p class="text-sm font-bold opacity-80 mt-1 leading-relaxed">
                  {{ t('detail.warning.desc', { count: sub.stalled_episodes } ) }}
                  <span v-if="sub.stall_timeout_hours > 0" class="text-[10px] font-black opacity-40 ml-2">(超时阈值: {{ sub.stall_timeout_hours }}h)</span>
                </p>
             </div>
          </div>
       </div>
      </Transition>

      <!-- Episode List Container -->
      <div class="space-y-6">
         <!-- Bulk Actions Bar -->
         <Transition name="slide">
           <fieldset v-if="showBulkActions" :disabled="bulkActionLoading || updatingEps.size > 0" class="fixed bottom-16 lg:bottom-6 left-3 right-3 sm:left-auto sm:right-6 z-50 bg-base-100/95 backdrop-blur-xl rounded-2xl sm:rounded-[2rem] border border-primary/30 shadow-2xl p-3 sm:p-4 flex flex-col sm:flex-row gap-2 sm:gap-3 animate-in slide-in-from-bottom-4 max-h-[55dvh] overflow-y-auto max-w-2xl">
             <span class="text-xs sm:text-sm font-black text-primary self-center px-2 whitespace-nowrap shrink-0">
               {{ t('detail.bulk.selected', { count: selectedEpisodes.size }) }}
             </span>
             <div class="flex flex-wrap items-center justify-center gap-1.5">
               <button class="btn btn-sm btn-ghost gap-2" @click="bulkUpdateStatus('pending')">
                 <History :size="16" /> {{ t('detail.logs.status.pending') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2" @click="bulkUpdateStatus('downloading')">
                 <Download :size="16" /> {{ t('detail.logs.status.active') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2" @click="bulkUpdateStatus('completed')">
                 <Check :size="16" /> {{ t('detail.logs.status.finished') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2 text-warning" @click="bulkUpdateStatus('failed')">
                 <AlertTriangle :size="16" /> {{ t('detail.logs.status.failed') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2" @click="bulkUpdateStatus('')">
                 <X :size="16" /> {{ t('detail.clearMarks') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2 text-error" @click="bulkDeleteEpisodes(false)">
                 <Trash2 :size="16" /> {{ t('detail.bulk.deleteRecords') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2 text-error" @click="bulkDeleteEpisodes(true)">
                 <Trash2 :size="16" /> {{ t('detail.bulk.deleteFiles') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2 text-warning" @click="addToBlacklist">
                 <Shield :size="16" /> {{ t('detail.bulk.blacklist') }}
               </button>
               <button class="btn btn-sm btn-ghost gap-2" @click="clearSelection">
                 <X :size="16" /> {{ t('detail.bulk.cancel') }}
               </button>
             </div>
           </fieldset>
         </Transition>
         
         <div class="flex items-center gap-4 px-4 group">
            <div class="w-1.5 h-6 bg-primary rounded-full shadow-[0_0_12px_rgba(var(--p),0.5)] group-hover:h-8 transition-all"></div>
            <h2 class="text-2xl font-black tracking-tight italic uppercase">{{ t('detail.logs.title') }}</h2>
            <span class="text-[10px] font-black uppercase tracking-widest text-base-content/20 mt-1">{{ t('detail.logs.count', { count: episodes.length }) }}</span>
         </div>
         <div v-if="episodes.length === 0" class="bg-base-100 rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden">
            <div class="flex flex-col items-center justify-center py-24 gap-4">
               <div class="w-16 h-16 rounded-full bg-base-200 flex items-center justify-center">
                  <FileText :size="32" class="opacity-10" />
               </div>
               <p class="text-[10px] font-black uppercase tracking-widest opacity-20">{{ t('detail.logs.empty') }}</p>
            </div>
         </div>
         <div v-else class="space-y-4">
            <!-- Grouped episodes by subgroup -->
            <div v-for="([groupName, groupEps], gIdx) in groupedEpisodes" :key="groupName"
                 class="collapse collapse-arrow bg-base-100 rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden">
              <input type="checkbox" :checked="gIdx === 0" />
              <div class="collapse-title text-lg font-black tracking-tight flex items-center gap-3">
                <label class="cursor-pointer flex items-center gap-2">
                  <input type="checkbox" class="checkbox checkbox-primary" :disabled="bulkActionLoading" :checked="groupEps.every(ep => selectedEpisodes.has(ep.id))" @change="toggleSelectAll(groupEps)" />
                  <span>{{ groupName }}</span>
                </label>
                <span class="text-[10px] font-black uppercase tracking-widest opacity-30 bg-base-200 px-3 py-1 rounded-full">{{ groupEps.length }} {{ t('detail.logs.episodes') }}</span>
                <span v-if="groupEps.some(ep => ep.status === 'needs_review')" class="badge badge-warning badge-sm">{{ t('detail.logs.status.needsReview') }}</span>
              </div>
              <div class="collapse-content overflow-x-auto p-0">
                <table class="table table-lg w-full">
                  <thead>
                    <tr class="border-b border-base-200 bg-base-200/30">
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pl-10 w-12">
                        <input type="checkbox" class="checkbox checkbox-primary" :disabled="bulkActionLoading" :checked="groupEps.every(ep => selectedEpisodes.has(ep.id))" @change="toggleSelectAll(groupEps)" />
                      </th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.id') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.name') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.status') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.integrity') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.capacity') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pr-10">{{ t('detail.logs.table.timestamp') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-base-200/50">
                    <tr v-for="ep in groupEps" :key="ep.id" class="hover:bg-base-200/30 transition-colors group/row" :class="{ 'bg-primary/5': selectedEpisodes.has(ep.id) }">
                      <td class="pl-10">
                        <label class="cursor-pointer">
                          <input type="checkbox" class="checkbox checkbox-primary" :disabled="bulkActionLoading" :checked="selectedEpisodes.has(ep.id)" @change="toggleEpisodeSelect(ep.id)" />
                        </label>
                      </td>
                      <td class="pl-10">
                        <span class="text-xs font-black font-mono bg-base-200 py-1.5 px-3 rounded-lg group-hover/row:bg-primary group-hover/row:text-primary-content transition-colors">
                          {{ ep.season === 0 || ep.season > 1 ? 'S' + ep.season : '' }}E{{ ep.number }}
                        </span>
                      </td>
                      <td class="max-w-md">
                        <div class="text-sm font-bold truncate transition-all group-hover/row:translate-x-1" :title="ep.original_name">
                          {{ ep.original_name || ep.title || 'Untitled Stream' }}
                        </div>
                      </td>
                      <td>
                        <span class="inline-flex items-center gap-2 rounded-lg border px-2 py-1 text-xs" :class="statusCfg[ep.status]?.cls">
                          <component :is="statusCfg[ep.status]?.icon || MoreVertical" :size="14" />
                          {{ statusCfg[ep.status]?.label || ep.status }}
                        </span>
                        <label class="mt-1 flex items-center gap-1 text-xs">
                          {{ t('detail.manualMark') }}
                          <select class="select select-bordered select-xs" :value="ep.manual_status || ''"
                            :disabled="updatingEps.has(ep.id) || bulkActionLoading" @change="setEpisodeMark(ep, $event)">
                            <option value="">{{ t('detail.noMark') }}</option>
                            <option v-for="mark in ['pending', 'downloading', 'completed', 'failed']" :key="mark" :value="mark">
                              {{ statusCfg[mark]?.label }}
                            </option>
                          </select>
                        </label>
                      </td>
                      <td>
                        <div v-if="ep.status === 'needs_review'" class="min-w-48 max-w-sm space-y-2">
                          <p class="text-xs whitespace-normal break-words">{{ ep.review_reason || t('detail.review.noReason') }}</p>
                          <p v-if="ep.source_file" class="text-xs break-all"><span class="font-semibold">{{ t('detail.review.source') }}</span> {{ ep.source_file }}</p>
                          <p v-if="reviewErrors[ep.id]" role="alert" class="text-xs text-error break-words">{{ reviewErrors[ep.id] }}</p>
                          <button v-if="ep.can_retry_organize" type="button" class="btn btn-outline btn-warning btn-xs" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="retryOrganize(ep)">{{ t(ep.review_kind ? 'detail.review.recheck' : 'detail.review.retry') }}</button>
                          <form v-if="reviewConfirmation(ep) === 'assign' && assignment && assignment.id === ep.id && assignment.sourceFile === ep.source_file"
                            @submit.prevent="retryWithAssignment(ep)">
                            <fieldset class="space-y-2" :disabled="updatingEps.has(ep.id) || bulkActionLoading">
                              <div class="flex gap-2">
                                <label class="text-xs">{{ t('detail.review.season') }}
                                  <input v-model.number="assignment.season" type="number" min="0" max="99" step="1" required class="input input-bordered input-xs block w-20">
                                </label>
                                <label class="text-xs">{{ t('detail.review.episode') }}
                                  <input v-model.number="assignment.number" type="number" min="0" max="2000" step="any" required class="input input-bordered input-xs block w-20">
                                </label>
                              </div>
                              <div class="flex flex-wrap gap-2">
                                <button type="submit" class="btn btn-outline btn-warning btn-xs" :disabled="!validAssignment">{{ t('detail.review.assignAndRetry') }}</button>
                                <button type="button" class="btn btn-ghost btn-xs" @click="assignment = null">{{ t('detail.bulk.cancel') }}</button>
                              </div>
                            </fieldset>
                          </form>
                          <div v-else class="flex flex-wrap gap-2">
                            <button v-if="reviewConfirmation(ep) === 'assign'" type="button" class="btn btn-outline btn-warning btn-xs" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="editAssignment(ep)">{{ t('detail.review.assign') }}</button>
                            <button v-else-if="reviewConfirmation(ep) === 'movie'" type="button" class="btn btn-outline btn-warning btn-xs h-auto min-h-6 whitespace-normal" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="retryOrganize(ep, { confirm_movie: true })">{{ t('detail.review.confirmMovie') }}</button>
                            <p v-else-if="ep.review_kind === 'episode_identity' || ep.review_kind === 'movie_selection'" class="text-xs text-base-content/70">{{ t('detail.review.missingSource') }}</p>
                          </div>
                        </div>
                        <span v-else-if="ep.is_stalled" class="flex items-center gap-1.5 text-warning font-black uppercase text-[9px] tracking-widest bg-warning/5 px-3 py-1.5 rounded-full border border-warning/10">
                          <AlertTriangle :size="12" /> {{ t('detail.logs.integrity.stalled') }}
                        </span>
                        <span v-else class="text-[10px] font-black opacity-10 tracking-widest uppercase pl-2">{{ t('detail.logs.integrity.verified') }}</span>
                      </td>
                      <td>
                        <span class="text-xs font-bold opacity-40">{{ formatSize(ep.file_size) }}</span>
                      </td>
                      <td class="pr-10">
                        <span class="text-[10px] font-black opacity-30 uppercase tracking-tighter">{{ new Date(ep.created_at).toLocaleString(locale, {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'}) }}</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
         </div>
      </div>

      <!-- Edit Dialog -->
      <dialog ref="editDialog" class="modal bg-black/80 backdrop-blur-xl" @click.self="closeEditDialog" @close="showEdit = false">
        <div class="modal-box max-w-2xl bg-base-100 rounded-[3rem] p-10 border border-white/5 overflow-y-auto max-h-[85vh]">
         <div class="flex items-center justify-between mb-10">
            <div class="space-y-1">
                <h3 class="text-3xl font-black tracking-tighter italic">{{ t('detail.edit.title') }}</h3>
                <p class="text-[10px] font-black tracking-widest uppercase opacity-30">{{ t('detail.edit.subtitle') }}</p>
            </div>
            <button class="btn btn-ghost btn-circle" @click="closeEditDialog">
                <X :size="24" />
             </button>
          </div>
          
          <SubscriptionEditForm
            v-if="showEdit"
            :sub="sub"
            @save="handleSaveEdit"
            @cancel="closeEditDialog"
          />
        </div>
      </dialog>
    </template>
  </div>
</template>

<style scoped>
.modal-box {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.modal-box::-webkit-scrollbar {
  display: none;
}
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.5s ease, transform 0.5s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
  transform: translateY(20px);
}

.slide-enter-active, .slide-leave-active {
  transition: all 0.3s ease;
}
.slide-enter-from {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}
.slide-leave-to {
  opacity: 0;
  transform: translateY(-20px) scale(0.95);
}

.table th, .table td {
  border-color: rgba(var(--bc), 0.05);
}

::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: hsl(var(--bc) / 0.1);
  border-radius: 10px;
}
</style>
