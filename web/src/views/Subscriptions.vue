<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Search, Plus, AlertTriangle, 
  X, LayoutGrid, RefreshCw 
} from 'lucide-vue-next'
import SubscriptionCard from '../components/SubscriptionCard.vue'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

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
  if (filterType.value === 'active') list = list.filter(s => s.enabled && !s.completed)
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
  if (loading.value === false) {
     // Background refresh - don't show loading spinner
  }
  error.value = ''
  try {
    const { data } = await request.get('/subscriptions')
    subs.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.load')
  } finally {
    loading.value = false
  }
}

async function toggleEnabled(sub: Subscription) {
  try {
    await request.put(`/subscriptions/${sub.id}`, { enabled: !sub.enabled })
    sub.enabled = !sub.enabled
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.operation')
  }
}

async function handleDelete(sub: Subscription) {
  if (!confirm(t('subscriptions.deleteConfirm', { title: sub.title_cn }))) return
  deletingId.value = sub.id
  try {
    await request.delete(`/subscriptions/${sub.id}`)
    subs.value = subs.value.filter(s => s.id !== sub.id)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.delete')
  } finally {
    deletingId.value = null
  }
}

async function triggerSupplement(sub: Subscription) {
  try {
    await request.post(`/subscriptions/${sub.id}/trigger-supplement`)
    alert(t('subscriptions.supplementTriggered'))
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.supplement')
  }
}

let refreshTimer: ReturnType<typeof setInterval>
onMounted(() => {
  fetchSubscriptions()
  refreshTimer = setInterval(fetchSubscriptions, 30000)
})
onUnmounted(() => clearInterval(refreshTimer))
</script>

<template>
  <div class="space-y-10">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('subs.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('subs.subtitle') }}</p>
      </div>
      
      <div class="flex items-center gap-3">
        <button 
          class="btn btn-ghost border-base-300 rounded-2xl gap-3 px-6 hover:bg-base-200 transition-all active:scale-95" 
          @click="router.push('/search')"
        >
          <Search :size="20" />
          <span class="text-xs font-black uppercase tracking-widest">{{ $t('subs.find') }}</span>
        </button>
        <button 
          class="btn btn-primary rounded-2xl gap-3 px-6 shadow-xl shadow-lg hover:scale-105 active:scale-95 transition-all" 
          @click="router.push('/subscriptions/new')"
        >
          <Plus :size="20" />
          <span class="text-xs font-black uppercase tracking-widest">{{ $t('subs.new') }}</span>
        </button>
      </div>
    </div>

    <!-- Toolbar Section -->
    <div class="flex flex-col lg:flex-row items-center gap-4 bg-base-100 p-3 rounded-[2rem] border border-base-200/50 shadow-sm">
      <div class="relative w-full lg:w-96 group">
        <div class="absolute inset-y-0 left-4 flex items-center pointer-events-none text-base-content/20 group-focus-within:text-primary transition-colors">
          <Search :size="20" />
        </div>
        <input 
          v-model="filterText" 
          type="text" 
          :placeholder="$t('subs.searchPlaceholder')" 
          class="input w-full bg-base-200/50 border-transparent focus:border-primary/30 focus:bg-base-100 focus:ring-0 rounded-2xl pl-12 transition-all font-bold text-sm h-12"
        />
      </div>

      <div class="flex p-1.5 bg-base-200/50 rounded-2xl gap-1 w-full lg:w-auto overflow-x-auto no-scrollbar">
        <button 
          v-for="t in ['all', 'active', 'completed']" 
          :key="t"
          class="flex-1 lg:flex-none px-6 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all whitespace-nowrap"
          :class="filterType === t ? 'bg-base-100 text-primary shadow-sm ring-1 ring-base-300' : 'text-base-content/40 hover:text-base-content'"
          @click="filterType = t as any"
        >
          {{ t === 'all' ? $t('subs.filter.all') : t === 'active' ? $t('subs.filter.active') : $t('subs.filter.completed') }}
        </button>
      </div>

      <div class="hidden lg:flex ml-auto px-4 items-center gap-2">
         <div class="flex -space-x-3">
            <div v-for="i in 3" :key="i" class="w-8 h-8 rounded-full border-2 border-base-100 bg-base-200 flex items-center justify-center overflow-hidden">
               <img :src="`https://api.dicebear.com/7.x/bottts/svg?seed=${i+10}`" class="w-full h-full object-cover" />
            </div>
         </div>
         <span class="text-[10px] font-black text-base-content/20 uppercase tracking-widest ml-2">{{ $t('subs.itemsTracked', { count: subs.length }) }}</span>
      </div>
    </div>

    <!-- Status Alerts -->
    <div v-if="error" class="alert bg-error/10 border-error/20 text-error rounded-3xl p-6 flex items-start gap-4">
      <div class="p-3 bg-error/20 rounded-2xl">
        <AlertTriangle :size="24" />
      </div>
      <div class="flex-1">
        <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('subs.error.op') }}</h3>
        <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
      </div>
      <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
        <X :size="16" />
      </button>
    </div>

    <!-- Main Content Section -->
    <div v-if="loading" class="grid gap-6 grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 animate-pulse">
      <div v-for="i in 8" :key="i" class="aspect-[3/5] bg-base-200 rounded-[2.5rem]"></div>
    </div>

    <div v-else-if="filteredSubs.length === 0" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200">
      <div class="w-32 h-32 bg-base-200/50 rounded-full flex items-center justify-center mb-8 rotate-12">
        <LayoutGrid :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">
        {{ subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs') }}
      </h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc') }}
      </p>
      <button 
        v-if="subs.length === 0" 
        class="btn btn-primary btn-lg rounded-3xl px-12 shadow-2xl shadow-lg gap-4" 
        @click="router.push('/search')"
      >
        <Search :size="24" />
        <span class="font-black uppercase tracking-widest">{{ $t('subs.empty.discover') }}</span>
      </button>
      <button 
        v-else 
        class="btn btn-ghost btn-md rounded-2xl px-10 gap-4 border-base-300" 
        @click="filterText = ''; filterType = 'all'"
      >
        <RefreshCw :size="20" />
        <span class="font-black uppercase tracking-widest text-xs">{{ $t('subs.empty.clear') }}</span>
      </button>
    </div>

    <!-- Subscription Grid -->
    <div v-else>
      <TransitionGroup
        name="list"
        tag="div"
        class="grid gap-6 sm:gap-8 grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5"
      >
        <SubscriptionCard
          v-for="sub in filteredSubs"
          :key="sub.id"
          :sub="sub"
          :deleting="deletingId === sub.id"
          @click="router.push(`/subscriptions/${sub.id}`)"
          @toggle="toggleEnabled(sub)"
          @delete="handleDelete(sub)"
          @supplement="triggerSupplement(sub)"
        />
      </TransitionGroup>
    </div>
  </div>
</template>

<style scoped>
.list-enter-active,
.list-leave-active {
  transition: all 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateY(40px) scale(0.9);
}
.list-move {
  transition: transform 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
</style>
