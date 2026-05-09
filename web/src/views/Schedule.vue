<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Antenna, LayoutGrid, RefreshCw, 
  AlertTriangle, X, Calendar, 
  Image, Check, Clock, 
  Search, ChevronLeft
} from 'lucide-vue-next'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

interface TorrentItem {
  title: string; url: string; source: string; bangumi_id: string; info_hash: string; cover_url: string
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

const weekOrder = [1, 2, 3, 4, 5, 6, 7, 0, 8]
const dayNames = computed<Record<number, string>>(() => ({
  1: t('schedule.days.monday'), 2: t('schedule.days.tuesday'), 3: t('schedule.days.wednesday'), 4: t('schedule.days.thursday'),
  5: t('schedule.days.friday'), 6: t('schedule.days.saturday'), 7: t('schedule.days.sunday'), 0: t('schedule.days.others'), 8: t('schedule.days.tbd'),
}))

const sortedDays = computed(() =>
  [...weekDays.value]
    .map(d => ({ ...d, label: dayNames.value[d.day_of_week] || d.label }))
    .sort((a, b) => weekOrder.indexOf(a.day_of_week) - weekOrder.indexOf(b.day_of_week))
)

const subscribedSchedule = computed(() => {
  const map: Record<string, TorrentItem[]> = {}
  for (const day of weekDays.value) {
    const items = day.items.filter(i => i.info_hash === 'subscribed' || subscribedIds.value[i.bangumi_id])
    if (items.length > 0) map[dayNames.value[day.day_of_week] || day.label] = items
  }
  return map
})

const subscribedCount = computed(() => Object.keys(subscribedIds.value).length)

async function fetchSchedule() {
  loading.value = true; error.value = ''
  try {
    const { data } = await request.get('/schedule', { timeout: 30000 })
    weekDays.value = data.days || []
    subscribedIds.value = data.subscribed || {}
  } catch (e: any) {
    error.value = e.code === 'ECONNABORTED' ? t('schedule.error.timeout') : t('schedule.error.failed')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchSchedule()
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
         <div class="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 gap-4">
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
        <div v-for="day in sortedDays" :key="day.day_of_week" class="space-y-6">
          <div class="flex items-center gap-4 group">
            <div class="w-1.5 h-6 bg-primary rounded-full shadow-[0_0_12px_rgba(var(--p),0.5)] group-hover:h-8 transition-all"></div>
            <h2 class="text-2xl font-black tracking-tight italic">{{ day.label }}</h2>
            <span class="text-[10px] font-black uppercase tracking-widest text-base-content/20 mt-1">{{ day.items.length }} {{ $t('schedule.entries') }}</span>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4 sm:gap-6">
            <div v-for="item in day.items" :key="item.bangumi_id"
              class="group relative aspect-video bg-base-100/50 rounded-[1.8rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:border-primary/30 transition-all duration-500 cursor-pointer active:scale-95"
              @click="selectedItem = item">
              
              <!-- Poster -->
              <div class="absolute inset-0 z-0 flex items-center justify-center bg-base-300">
                <img v-if="item.cover_url" :src="item.cover_url" :alt="item.title" class="absolute inset-0 w-full h-full object-cover transition-transform duration-1000 group-hover:scale-105" loading="lazy" referrerpolicy="no-referrer"
                  @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" />
                <div class="absolute inset-0 flex items-center justify-center text-base-content/5" v-if="!item.cover_url">
                  <Image :size="48" />
                </div>
                <!-- Glass Overlay -->
                <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent opacity-60 group-hover:opacity-100 transition-opacity pointer-events-none"></div>
              </div>

              <!-- Status Badge -->
              <div v-if="item.info_hash === 'subscribed' || subscribedIds[item.bangumi_id]" class="absolute top-3 right-3 z-10">
                 <div class="w-8 h-8 rounded-full bg-primary/90 backdrop-blur-md flex items-center justify-center text-primary-content shadow-lg border border-white/20 scale-0 group-hover:scale-100 transition-transform duration-500">
                    <Check :size="16" />
                 </div>
              </div>

              <!-- Title -->
              <div class="absolute bottom-0 left-0 w-full p-4 z-10">
                 <p class="text-[10px] font-black leading-tight text-white line-clamp-2 uppercase tracking-wide group-hover:text-primary transition-colors">{{ item.title }}</p>
              </div>
            </div>
          </div>
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

          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4 sm:gap-6">
            <div v-for="item in items" :key="item.bangumi_id"
              class="group relative aspect-video bg-base-100/50 rounded-[1.8rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:border-success/30 transition-all duration-500 cursor-pointer active:scale-95"
              @click="selectedItem = item">
              
              <!-- Poster -->
              <div class="absolute inset-0 z-0 flex items-center justify-center bg-base-300">
                <img v-if="item.cover_url" :src="item.cover_url" :alt="item.title" class="absolute inset-0 w-full h-full object-cover transition-transform duration-1000 group-hover:scale-105" loading="lazy" referrerpolicy="no-referrer" />
                <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent opacity-60 pointer-events-none"></div>
              </div>

              <div class="absolute top-3 right-3 z-10">
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

    <!-- Anime Detail Modal -->
    <dialog :class="['modal', { 'modal-open': selectedItem }]">
      <div v-if="selectedItem" class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] p-0 overflow-hidden max-w-sm w-full shadow-[0_0_40px_rgba(0,0,0,0.5)]">
        <!-- Header Image -->
        <div class="relative w-full aspect-video bg-base-300">
          <img v-if="selectedItem.cover_url" :src="selectedItem.cover_url" class="w-full h-full object-cover" referrerpolicy="no-referrer" />
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
              <span class="flex items-center gap-1.5 bg-base-100/50 px-3 py-1.5 rounded-xl border border-base-content/5"><Clock :size="14" /> {{ selectedItem.info_hash || 'TBA' }}</span>
              <span class="flex items-center gap-1.5 bg-base-100/50 px-3 py-1.5 rounded-xl border border-base-content/5"><Calendar :size="14" /> {{ selectedItem.bangumi_id || 'TBA' }}</span>
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
