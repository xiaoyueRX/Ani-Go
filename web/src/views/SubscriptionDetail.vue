<script setup lang="ts">
import { ref, onMounted, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  ChevronLeft, RefreshCw, AlertTriangle, 
  Image, Edit3, Pause, Play, 
  History, Download, Check, 
  MoreVertical, FileText, X 
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
}

interface Episode {
  id: number; subscription_id: number; season: number; number: number
  title: string; status: string; torrent_hash: string; torrent_url: string
  original_name: string; final_path: string; file_size: number
  is_stalled: boolean; group_name?: string
  download_started_at: string; created_at: string
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const { t } = useI18n({ useScope: 'global' })

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
    if (sub.value) {
       // Refresh sub to get updated current_episodes count
       const { data } = await request.get(`/subscriptions/${id}`)
       sub.value = data.subscription
    }
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
    error.value = e.response?.data?.error || t('detail.error.load')
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
    alert(e.response?.data?.error || t('detail.error.update'))
  }
}
const statusCfg = computed<Record<string, { label: string; icon: any; cls: string }>>(() => ({
  pending: { label: t('detail.logs.status.pending'), icon: History, cls: 'bg-base-200 text-base-content/40 border-base-300' },
  downloading: { label: t('detail.logs.status.active'), icon: Download, cls: 'bg-primary/10 text-primary border-primary/20' },
  completed: { label: t('detail.logs.status.finished'), icon: Check, cls: 'bg-success/10 text-success border-success/20' },
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
  if (!sub.value?.total_episodes) return 0
  return Math.min(100, (sub.value.current_episodes / sub.value.total_episodes) * 100)
})

onMounted(fetchDetail)
</script>

<template>
  <div class="space-y-10 pb-20">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <button class="btn btn-ghost border-base-300 rounded-2xl px-6 hover:bg-base-200 transition-all active:scale-95 group" @click="router.push('/')">
        <ChevronLeft :size="20" class="group-hover:-translate-x-1 transition-transform" />
        <span class="text-xs font-black uppercase tracking-widest">{{ t('detail.back') }}</span>
      </button>
      
      <div v-if="sub" class="flex gap-2">
         <button class="btn btn-ghost btn-circle hover:bg-base-200" @click="fetchDetail" :disabled="loading">
            <RefreshCw :size="20" class="opacity-40" />
         </button>
      </div>
    </div>

    <div v-if="loading && !sub" class="flex justify-center py-32">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <div v-else-if="error" class="max-w-4xl mx-auto">
       <div class="alert bg-error/10 border-error/20 text-error rounded-[2rem] p-6">
          <AlertTriangle :size="24" class="shrink-0" />
          <div class="flex-1">
             <h3 class="font-black text-sm uppercase tracking-widest">{{ t('detail.error.accessDenied') }}</h3>
             <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
          </div>
       </div>
    </div>

    <template v-else-if="sub">
      <!-- Main Content Card -->
      <div class="group relative bg-base-100 rounded-[3rem] border border-base-200/60 shadow-xl overflow-hidden">
        <!-- Hero Background -->
        <div class="absolute inset-x-0 top-0 h-64 z-0 overflow-hidden">
           <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-cover blur-3xl opacity-20 scale-125" />
           <div class="absolute inset-0 bg-gradient-to-b from-transparent to-base-100"></div>
        </div>

        <div class="p-8 sm:p-12 relative z-10">
          <div class="flex flex-col lg:flex-row gap-12">
             <!-- Left: Poster -->
             <div class="w-full lg:w-1/3 xl:w-1/4 shrink-0 flex justify-center lg:justify-start">
                <div class="w-full max-w-[280px] sm:max-w-sm lg:max-w-none aspect-[3/4.2] rounded-[2.5rem] bg-base-200 overflow-hidden shadow-2xl border border-white/5 relative group/poster">
                   <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-cover transition-transform duration-1000 group-hover/poster:scale-110" />
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
            <div class="flex-1 space-y-8">
               <div class="space-y-4">
              <div class="flex flex-wrap items-center gap-3">
                  <span v-if="sub.anime_type && sub.anime_type !== t('detail.type.unknown')" class="text-[10px] font-black uppercase tracking-[0.3em] text-primary bg-primary/10 px-4 py-1.5 rounded-full border border-primary/20">
                    {{ sub.anime_type }}
                  </span>
                  <span v-if="sub.completed" class="text-[10px] font-black uppercase tracking-[0.3em] text-success bg-success/10 px-4 py-1.5 rounded-full border border-success/20">
                    {{ t('detail.status.finished') }}
                  </span>
                  <span v-if="!sub.enabled" class="text-[10px] font-black uppercase tracking-[0.3em] text-warning bg-warning/10 px-4 py-1.5 rounded-full border border-warning/20">
                    {{ t('detail.status.paused') }}
                  </span>
               </div>
                   
                   <div class="space-y-2">
                      <h1 class="text-4xl sm:text-5xl font-black tracking-tighter leading-none">{{ sub.title_cn }}</h1>
                      <p v-if="sub.title_jp" class="text-lg font-bold opacity-30 tracking-tight">{{ sub.title_jp }}</p>
                      <p v-if="sub.title_en" class="text-sm font-bold opacity-20 tracking-widest uppercase">{{ sub.title_en }}</p>
                   </div>
                </div>

                <!-- Stats Grid -->
                <div class="grid grid-cols-2 sm:grid-cols-3 gap-6 p-8 bg-base-200/50 rounded-[2rem] border border-base-300/30">
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
                      <p class="text-lg font-black uppercase">{{ sub.metadata_provider || 'yuc.wiki' }}</p>
                   </div>
                </div>

                <!-- Progress Section -->
                <div class="space-y-4">
                   <div class="flex items-end justify-between px-2">
                      <div class="space-y-1">
                         <p class="text-[10px] font-black uppercase tracking-widest opacity-30 leading-none">{{ t('detail.progress.title') }}</p>
                         <p class="text-2xl font-black leading-none mt-2">{{ sub.current_episodes }} <span class="text-base-content/20 mx-1">/</span> {{ sub.total_episodes || '?' }}</p>
                      </div>
                      <div class="text-right">
                         <p class="text-3xl font-black tracking-tighter text-primary">{{ Math.round(progress) }}%</p>
                      </div>
                   </div>
                   <div class="h-4 w-full bg-base-300 rounded-full overflow-hidden p-1 border border-base-300 shadow-inner">
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
                <p class="text-sm font-bold opacity-80 mt-1 leading-relaxed" v-html="t('detail.warning.desc', { count: sub.stalled_episodes } )"></p>
             </div>
          </div>
       </div>
      </Transition>

      <!-- Episode List Container -->
      <div class="space-y-6">
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
                <span>{{ groupName }}</span>
                <span class="text-[10px] font-black uppercase tracking-widest opacity-30 bg-base-200 px-3 py-1 rounded-full">{{ groupEps.length }} {{ t('detail.logs.episodes') }}</span>
              </div>
              <div class="collapse-content overflow-x-auto p-0">
                <table class="table table-lg w-full">
                  <thead>
                    <tr class="border-b border-base-200 bg-base-200/30">
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pl-10">{{ t('detail.logs.table.id') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.name') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.status') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.integrity') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ t('detail.logs.table.capacity') }}</th>
                      <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pr-10">{{ t('detail.logs.table.timestamp') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-base-200/50">
                    <tr v-for="ep in groupEps" :key="ep.id" class="hover:bg-base-200/30 transition-colors group/row">
                      <td class="pl-10">
                        <span class="text-xs font-black font-mono bg-base-200 py-1.5 px-3 rounded-lg group-hover/row:bg-primary group-hover/row:text-primary-content transition-colors">
                          {{ ep.season > 1 ? 'S' + ep.season : '' }}E{{ ep.number?.toFixed(1).replace('.0', '') }}
                        </span>
                      </td>
                      <td class="max-w-md">
                        <div class="text-sm font-bold truncate transition-all group-hover/row:translate-x-1" :title="ep.original_name">
                          {{ ep.original_name || ep.title || 'Untitled Stream' }}
                        </div>
                      </td>
                      <td>
                        <button 
                          class="px-4 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest border transition-all active:scale-95 flex items-center gap-2 group/btn-s shadow-sm"
                          :class="statusCfg[ep.status]?.cls || 'bg-base-200 text-base-content/40'"
                          @click="cycleEpisodeStatus(ep)"
                          :disabled="updatingEps.has(ep.id)"
                        >
                          <span v-if="updatingEps.has(ep.id)" class="loading loading-spinner loading-xs"></span>
                          <component v-else :is="statusCfg[ep.status]?.icon || MoreVertical" :size="14" />
                          {{ statusCfg[ep.status]?.label || ep.status }}
                        </button>
                      </td>
                      <td>
                        <span v-if="ep.is_stalled" class="flex items-center gap-1.5 text-warning font-black uppercase text-[9px] tracking-widest bg-warning/5 px-3 py-1.5 rounded-full border border-warning/10">
                          <AlertTriangle :size="12" /> {{ t('detail.logs.integrity.stalled') }}
                        </span>
                        <span v-else class="text-[10px] font-black opacity-10 tracking-widest uppercase pl-2">{{ t('detail.logs.integrity.verified') }}</span>
                      </td>
                      <td>
                        <span class="text-xs font-bold opacity-40">{{ formatSize(ep.file_size) }}</span>
                      </td>
                      <td class="pr-10">
                        <span class="text-[10px] font-black opacity-30 uppercase tracking-tighter">{{ new Date(ep.created_at).toLocaleString('zh-CN', {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'}) }}</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
         </div>
      </div>

      <!-- Edit Dialog -->
      <dialog ref="editDialog" class="modal bg-black/80 backdrop-blur-xl" @click.self="closeEditDialog">
        <div class="modal-box max-w-2xl bg-base-100 rounded-[3rem] p-10 border border-white/5 overflow-hidden">
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
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.5s ease, transform 0.5s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
  transform: translateY(20px);
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
