<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'
import { 
  ChevronLeft, Search, LogIn, 
  LayoutGrid, Timer, Calendar, 
  AlertTriangle, X, Antenna, 
  Check, Plus, User 
} from 'lucide-vue-next'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

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
  aired_time?: string
  aired_date?: string
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
  const q = query.value.trim()
  if (!q) return
  loading.value = true
  error.value = ''
  const start = Date.now()
  try {
    const { data } = await request.get('/search', {
      params: { q },
      timeout: 30000,
    })
    results.value = data || []
    lastSearchTime.value = new Date().toLocaleTimeString()
    searchDuration.value = Date.now() - start
  } catch (e: any) {
    if (e.code === 'ECONNABORTED') {
      error.value = t('search.error.timeout')
    } else {
      error.value = e.response?.data?.error || t('search.error.failed')
    }
  } finally {
    loading.value = false
  }
}

async function openSubscribe(item: TorrentItem) {
  if (!item.bangumi_id) {
    await subscribe(item, '')
    return
  }
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
    groupError.value = e.code === 'ECONNABORTED' ? t('schedule.error.timeout') : t('schedule.error.failed')
  } finally {
    groupLoading.value = false
  }
}

async function subscribe(item: TorrentItem, rssUrl: string) {
  try {
    await request.post('/subscriptions', {
      title_cn: item.title,
      bangumi_id: item.bangumi_id,
      rss_url: rssUrl || undefined,
      filter_json: JSON.stringify({ source_url: item.url }),
      cover_url: item.cover_url || '',
    })
    subscribed.value.add(item.title)
    showGroupModal.value = false
    alert(t('search.subscribed', { title: item.title }))
  } catch (e: any) {
    alert(t('search.subscribeFailed', { error: e.response?.data?.error || e.message }))
  }
}

function formatSize(size: number): string {
  if (!size || size <= 0) return '0 B'
  const mb = size / 1024 / 1024
  return mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(1) + ' MB'
}

function sourceBadge(source: string): string {
  const map: Record<string, string> = {
    Mikan: 'bg-primary/10 text-primary border-primary/20',
    Nyaa: 'bg-secondary/10 text-secondary border-secondary/20',
    'ACG.RIP': 'bg-accent/10 text-accent border-accent/20',
    AnimeTosho: 'bg-info/10 text-info border-info/20',
  }
  return map[source] || 'bg-base-200 text-base-content/40 border-base-300'
}
</script>

<template>
  <div class="space-y-10 pb-20">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('search.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('search.subtitle') }}</p>
      </div>
      
      <button 
        class="btn btn-ghost border-base-300 rounded-2xl gap-3 px-6 hover:bg-base-200 transition-all active:scale-95" 
        @click="router.push('/')"
      >
        <ChevronLeft :size="20" />
        <span class="text-xs font-black uppercase tracking-widest">{{ $t('search.back') }}</span>
      </button>
    </div>

    <!-- Modern Search Interface -->
    <div class="relative group max-w-4xl mx-auto w-full">
      <div class="absolute -inset-1 bg-gradient-to-r from-primary to-secondary rounded-[2.5rem] blur opacity-10 group-focus-within:opacity-30 transition-opacity duration-500"></div>
      <div class="relative bg-base-100 rounded-[2.2rem] border border-base-200/50 shadow-2xl flex items-center p-2 overflow-hidden">
         <div class="pl-6 text-base-content/20 group-focus-within:text-primary transition-colors">
            <Search :size="28" />
         </div>
         <input 
           v-model="query" 
           type="text"
           class="flex-1 bg-transparent border-none outline-none px-6 py-4 font-bold text-lg placeholder:text-base-content/20 placeholder:font-bold"
           :placeholder="$t('search.placeholder')"
           @keyup.enter="handleSearch"
         />
         <button 
           class="btn btn-primary h-14 min-h-0 px-10 rounded-[1.8rem] shadow-xl shadow-lg gap-3 group/btn transition-all active:scale-95"
           :disabled="loading || !query.trim()" 
           @click="handleSearch"
         >
           <span v-if="loading" class="loading loading-spinner loading-md"></span>
           <template v-else>
              <span class="text-xs font-black uppercase tracking-widest">{{ $t('search.execute') }}</span>
              <LogIn :size="20" class="group-hover/btn:translate-x-1 transition-transform" />
           </template>
         </button>
      </div>
    </div>

    <!-- Search Metadata -->
    <Transition name="fade">
      <div v-if="results.length > 0" class="flex items-center justify-center gap-6 text-[10px] font-black uppercase tracking-[0.2em] opacity-30">
        <div class="flex items-center gap-2">
           <LayoutGrid :size="14" />
           {{ $t('search.resultsCount', { count: results.length }) }}
        </div>
        <div class="w-1 h-1 rounded-full bg-base-content/20"></div>
        <div class="flex items-center gap-2">
           <Timer :size="14" />
           {{ $t('search.duration', { duration: (searchDuration / 1000).toFixed(2) }) }}
        </div>
        <div class="w-1 h-1 rounded-full bg-base-content/20"></div>
        <div class="flex items-center gap-2">
           <Calendar :size="14" />
           {{ $t('search.updatedAt', { time: lastSearchTime }) }}
        </div>
      </div>
    </Transition>

    <!-- Error Alert -->
    <div v-if="error" class="max-w-4xl mx-auto">
      <div class="alert bg-error/10 border-error/20 text-error rounded-[2rem] p-6">
        <AlertTriangle :size="24" class="shrink-0" />
        <div class="flex-1">
          <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('search.error.title') }}</h3>
          <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
        </div>
        <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
          <X :size="16" />
        </button>
      </div>
    </div>

    <!-- Results Grid -->
    <div v-if="results.length > 0" class="grid gap-4 max-w-5xl mx-auto">
       <div 
         v-for="(item, idx) in results" 
         :key="idx"
         class="group bg-base-100 rounded-[2rem] border border-base-200/60 shadow-sm hover:shadow-2xl hover:shadow-lg hover:border-primary/20 transition-all duration-500 overflow-hidden active:scale-[0.99]"
       >
         <div class="p-4 sm:p-5 flex items-center gap-6">
            <!-- Poster -->
            <div class="w-16 h-24 sm:w-20 sm:h-28 rounded-2xl bg-base-200 shrink-0 overflow-hidden relative shadow-lg shadow-black/10 group-hover:rotate-2 transition-transform duration-500">
               <img 
                 v-if="item.cover_url" 
                 :src="proxyImage(item.cover_url)" 
                 :alt="item.title" 
                 class="w-full h-full object-cover group-hover:scale-110 transition-transform duration-700" 
                 loading="lazy" 
                 @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" 
               />
               <div class="absolute inset-0 flex items-center justify-center text-base-content/10" v-else>
                  <Antenna :size="32" />
               </div>
            </div>

            <!-- Meta -->
            <div class="flex-1 min-w-0 space-y-3">
               <h3 class="text-lg font-black tracking-tight line-clamp-2 leading-snug group-hover:text-primary transition-colors">
                 {{ item.title }}
               </h3>
               
               <div class="flex flex-wrap gap-2">
                  <span class="text-[9px] font-black uppercase tracking-widest py-1.5 px-3 rounded-lg border transition-all" :class="sourceBadge(item.source)">
                    {{ item.source }}
                  </span>
                  <span v-if="item.bangumi_id" class="text-[9px] font-black uppercase tracking-widest bg-base-200 text-base-content/40 py-1.5 px-3 rounded-lg border border-base-300/50">
                    ID: {{ item.bangumi_id }}
                  </span>
                  <span v-if="item.size > 0" class="text-[9px] font-black uppercase tracking-widest bg-base-200 text-base-content/40 py-1.5 px-3 rounded-lg border border-base-300/50">
                    {{ formatSize(item.size) }}
                  </span>
               </div>
            </div>

            <!-- Action -->
            <div class="shrink-0 pl-2">
               <button 
                 class="btn btn-circle w-14 h-14 rounded-2xl transition-all duration-500 shadow-xl"
                 :class="subscribed.has(item.title) ? 'btn-success shadow-lg' : 'btn-primary shadow-lg hover:scale-110'"
                 :disabled="subscribed.has(item.title)" 
                 @click="openSubscribe(item)"
               >
                 <component :is="subscribed.has(item.title) ? Check : Plus" :size="28" />
               </button>
            </div>
         </div>
       </div>
    </div>

    <!-- Empty/Welcome State -->
    <div v-else-if="!loading" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200 max-w-4xl mx-auto">
      <div class="w-32 h-32 bg-base-200/50 rounded-full flex items-center justify-center mb-8" :class="query ? 'rotate-12' : ''">
        <Search :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">
        {{ query ? $t('search.empty.noResults') : $t('search.empty.welcome') }}
      </h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ query ? $t('search.empty.noResultsDesc') : $t('search.empty.welcomeDesc') }}
      </p>
    </div>

    <!-- 字幕组选择弹窗 -->
    <Transition name="scale">
       <div v-if="showGroupModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-xl">
          <div class="w-full max-w-lg bg-base-100 rounded-[3rem] shadow-2xl border border-white/5 overflow-hidden animate-in zoom-in-95 duration-300">
             <div class="p-8 sm:p-10 space-y-8">
                <div class="flex items-center justify-between">
                   <div class="space-y-1">
                      <h3 class="text-2xl font-black tracking-tighter italic">{{ $t('search.modal.title') }}</h3>
                      <p class="text-[10px] font-black tracking-widest uppercase opacity-30">{{ $t('search.modal.subtitle') }}</p>
                   </div>
                   <button class="btn btn-ghost btn-circle" @click="showGroupModal = false">
                      <X :size="24" />
                   </button>
                </div>

                <div class="bg-base-200/50 p-4 rounded-2xl border border-base-300/30">
                   <p class="text-sm font-black tracking-tight leading-snug">{{ selectedItem?.title }}</p>
                </div>

                <div v-if="groupLoading" class="flex flex-col items-center justify-center py-12 gap-4">
                   <span class="loading loading-spinner loading-lg text-primary"></span>
                   <p class="text-[10px] font-black uppercase tracking-widest opacity-30">{{ $t('search.modal.fetching') }}</p>
                </div>

                <div v-else-if="groupError" class="bg-error/10 border border-error/20 text-error rounded-2xl p-6 flex flex-col items-center gap-3">
                   <AlertTriangle :size="32" />
                   <p class="text-sm font-bold">{{ groupError }}</p>
                </div>

                <div v-else-if="subgroups.length === 0" class="text-center py-12 space-y-6">
                   <div class="w-20 h-20 bg-base-200 rounded-full flex items-center justify-center mx-auto">
                      <User :size="32" class="opacity-10" />
                   </div>
                   <div class="space-y-2">
                      <p class="text-lg font-black tracking-tight">{{ $t('search.modal.empty') }}</p>
                      <p class="text-xs font-bold text-base-content/30">{{ $t('search.modal.emptyDesc') }}</p>
                   </div>
                   <div class="flex flex-col gap-3">
                      <button class="btn btn-primary h-14 rounded-2xl font-black uppercase tracking-widest shadow-xl shadow-lg" @click="selectedItem && subscribe(selectedItem, '')">
                        {{ $t('search.modal.proceed') }}
                      </button>
                      <button class="btn btn-ghost" @click="showGroupModal = false">{{ $t('search.modal.cancel') }}</button>
                   </div>
                </div>

                <div v-else class="space-y-3 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                   <button 
                     v-for="g in subgroups" :key="g.rss_url"
                     class="group/item w-full bg-base-200/30 hover:bg-primary/10 border border-base-300/30 hover:border-primary/30 rounded-2xl p-4 flex items-center justify-between transition-all duration-300"
                     @click="selectedItem && subscribe(selectedItem, g.rss_url)"
                   >
                     <div class="flex items-center gap-4">
                        <div class="w-10 h-10 rounded-xl bg-base-300 flex items-center justify-center text-base-content/30 group-hover/item:bg-primary group-hover/item:text-primary-content transition-colors">
                           <User :size="18" />
                        </div>
                        <span class="font-black text-sm tracking-tight group-hover/item:text-primary transition-colors">{{ g.name }}</span>
                     </div>
                     <Plus :size="20" class="opacity-0 group-hover/item:opacity-100 group-hover/item:translate-x-0 -translate-x-2 transition-all" />
                  </button>
                </div>

                <div class="text-center pt-4">
                   <p class="text-[9px] font-black text-base-content/10 uppercase tracking-[0.3em]">{{ $t('search.modal.end') }}</p>
                </div>
             </div>
          </div>
       </div>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.5s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}
.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: hsl(var(--bc) / 0.1);
  border-radius: 10px;
}
</style>
