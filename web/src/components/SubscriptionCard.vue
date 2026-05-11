<script setup lang="ts">
import { computed, ref } from 'vue'
import { 
  Image, Check, Pause, Play, AlertTriangle, 
  Trash2, User, History 
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number
  season: number
  bangumi_id: string
  subgroup_name: string
  cover_url: string
  anime_type: string
  total_episodes: number
  current_episodes: number
  stalled_episodes: number
  enabled: boolean
  completed: boolean
  created_at: string
  updated_at: string
}

const props = defineProps<{
  sub: Subscription
  deleting?: boolean
  pending?: boolean
  batchDeleteMode?: boolean
  batchSelected?: boolean
}>()

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const emit = defineEmits<{
  (e: 'click'): void
  (e: 'toggle'): void
  (e: 'delete'): void
  (e: 'supplement'): void
}>()

const isImageLoaded = ref(false)

const progress = computed(() => {
  if (!props.sub.total_episodes) return 0
  return Math.min(100, (props.sub.current_episodes / props.sub.total_episodes) * 100)
})

const progressColor = computed(() => {
  if (props.sub.completed) return 'bg-success'
  if (props.sub.stalled_episodes > 0) return 'bg-warning'
  return 'bg-primary'
})
</script>

<template>
  <div
    class="group relative flex flex-col bg-base-100 rounded-[2.5rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:shadow-primary hover:border-primary/30 transition-all duration-500 cursor-pointer active:scale-[0.98] h-full"
    :class="{ 'opacity-80 grayscale-[0.4]': !sub.enabled, 'opacity-40 pointer-events-none': pending }"
    @click="emit('click')"
  >
    <!-- Cover Image Wrapper -->
    <div class="relative aspect-[3/4.2] overflow-hidden bg-base-300">
      <!-- Loading Skeleton -->
      <div v-if="!isImageLoaded" class="absolute inset-0 skeleton rounded-none"></div>
      
      <img
        v-if="sub.cover_url"
        :src="proxyImage(sub.cover_url)"
        :alt="sub.title_cn"
        class="w-full h-full object-cover transition-all duration-1000 group-hover:scale-110 group-hover:rotate-1"
        :class="{ 'opacity-0': !isImageLoaded, 'opacity-100': isImageLoaded }"
        @load="isImageLoaded = true"
        loading="lazy"
      />
      <div v-else class="w-full h-full flex items-center justify-center text-base-content/5">
        <Image :size="64" />
      </div>

      <!-- Glass Badges -->
      <div class="absolute top-4 left-4 flex flex-col gap-2 z-10">
        <div v-if="sub.completed" class="bg-success/80 backdrop-blur-md text-success-content text-[10px] font-black uppercase tracking-widest py-1.5 px-3.5 rounded-full shadow-lg flex items-center gap-2 border border-white/20">
          <Check :size="12" />
          {{ $t('card.completed') }}
        </div>
        <div v-if="!sub.enabled" class="bg-base-100/60 backdrop-blur-md text-base-content text-[10px] font-black uppercase tracking-widest py-1.5 px-3.5 rounded-full shadow-lg flex items-center gap-2 border border-white/10">
          <Pause :size="12" />
          {{ $t('card.paused') }}
        </div>
        <div v-if="sub.stalled_episodes > 0" class="bg-warning/80 backdrop-blur-md text-warning-content text-[10px] font-black uppercase tracking-widest py-1.5 px-3.5 rounded-full shadow-lg flex items-center gap-2 border border-white/20">
          <AlertTriangle :size="12" />
          {{ $t('card.stalled', { count: sub.stalled_episodes }) }}
        </div>
      </div>

      <!-- Batch Delete Selection Mode Visual -->
      <div v-if="batchDeleteMode" class="absolute inset-0 z-20 pointer-events-none rounded-[2.5rem] transition-all duration-300"
           :class="batchSelected ? 'border-4 border-error bg-error/10' : 'border-4 border-transparent'">
         <div v-if="batchSelected" class="absolute top-4 right-4 bg-error text-error-content rounded-full p-1 shadow-lg">
           <Check :size="20" />
         </div>
      </div>

      <!-- Floating Actions (Hover) -->
      <div class="absolute top-4 right-4 flex flex-col gap-2 opacity-0 translate-x-4 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300 z-10" @click.stop>
        <button
          class="w-11 h-11 rounded-full bg-base-100/80 backdrop-blur-md border border-white/20 flex items-center justify-center text-base-content hover:bg-primary hover:text-primary-content hover:scale-110 shadow-xl transition-all active:scale-95"
          @click="emit('toggle')"
        >
          <component :is="sub.enabled ? Pause : Play" :size="20" />
        </button>
        <button
          class="w-11 h-11 rounded-full bg-base-100/80 backdrop-blur-md border border-white/20 flex items-center justify-center text-base-content hover:bg-error hover:text-error-content hover:scale-110 shadow-xl transition-all active:scale-95"
          @click="emit('delete')"
          :disabled="deleting"
        >
          <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
          <Trash2 v-else :size="20" />
        </button>
      </div>
      
      <!-- Gradient Overlay & Progress -->
      <div class="absolute bottom-0 left-0 w-full p-5 pt-16 bg-gradient-to-t from-black/90 via-black/40 to-transparent">
         <div class="flex justify-between items-end mb-3">
            <div>
               <p class="text-[9px] text-white/50 font-black uppercase tracking-[0.2em] leading-none mb-1.5">{{ $t('card.episodes') }}</p>
               <p class="text-xl text-white font-black leading-none">{{ sub.current_episodes }}<span class="text-white/30 text-xs ml-1.5 font-bold">/ {{ sub.total_episodes || '?' }}</span></p>
            </div>
            <div class="text-right">
               <p class="text-[9px] text-white/50 font-black uppercase tracking-[0.2em] leading-none mb-1.5">{{ $t('card.ratio') }}</p>
               <p class="text-sm text-white font-black leading-none">{{ Math.round(progress) }}%</p>
            </div>
         </div>
         <div class="h-2.5 w-full bg-white/10 rounded-full overflow-hidden border border-white/5 p-[1px]">
            <div 
              class="h-full rounded-full transition-all duration-1000 ease-out relative"
              :class="progressColor"
              :style="{ width: `${progress}%` }"
            >
               <!-- Shine effect -->
               <div class="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent -translate-x-full group-hover:animate-[shimmer_2s_infinite]"></div>
            </div>
         </div>
      </div>
    </div>

    <!-- Content Area -->
    <div class="p-6 flex flex-col gap-4 flex-1 bg-gradient-to-b from-transparent to-base-200/20">
      <h3 class="font-black text-lg leading-tight line-clamp-2 min-h-[3rem] group-hover:text-primary transition-colors tracking-tight" :title="sub.title_cn">
        {{ sub.title_cn }}
      </h3>
      
      <div class="flex items-center gap-2.5 text-[10px] font-black text-base-content/40 bg-base-200/50 py-2 px-4 rounded-xl self-start uppercase tracking-widest border border-base-300/30">
        <User :size="14" class="text-primary/60" />
        <span class="truncate max-w-[140px]">{{ sub.subgroup_name || $t('card.generic') }}</span>
      </div>

      <div class="flex flex-wrap gap-2 mt-auto pt-2">
        <span v-if="sub.anime_type" class="text-[9px] font-black uppercase tracking-[0.2em] bg-primary/10 text-primary py-1.5 px-3 rounded-lg border border-primary/10 shadow-sm shadow-primary">
          {{ sub.anime_type }}
        </span>
        <span v-if="sub.year" class="text-[9px] font-black uppercase tracking-[0.2em] bg-base-200 text-base-content/50 py-1.5 px-3 rounded-lg border border-base-300/50">
          {{ sub.year }}
        </span>
        <span v-if="sub.season" class="text-[9px] font-black uppercase tracking-[0.2em] bg-base-200 text-base-content/50 py-1.5 px-3 rounded-lg border border-base-300/50">
          S{{ sub.season }}
        </span>
      </div>

      <!-- Quick Action Footer -->
      <div class="flex items-center justify-between mt-2 opacity-0 translate-y-2 group-hover:opacity-100 group-hover:translate-y-0 transition-all duration-500" @click.stop>
         <button
          v-if="sub.enabled && !sub.completed"
          class="btn btn-ghost btn-xs text-primary font-black uppercase tracking-[0.2em] hover:bg-primary/10 px-0 h-auto min-h-0 py-1"
          @click="emit('supplement')"
        >
          <History :size="14" class="mr-2" />
          {{ $t('card.supplement') }}
        </button>
        <div v-else></div>
        <span class="text-[9px] font-black text-base-content/10 tracking-[0.3em] uppercase">Ref:{{ sub.id }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes shimmer {
  100% {
    transform: translateX(100%);
  }
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
