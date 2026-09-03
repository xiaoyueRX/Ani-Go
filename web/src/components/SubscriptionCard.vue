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
  // 如果已经是代理链接，直接返回
  if (url.includes('api/proxy/image')) return url
  
  let target = url
  // 处理相对路径
  if (url.startsWith('//')) {
    target = 'https:' + url
  }
  
  // 生产环境下使用绝对路径 API 绕过前端路由拦截
  const apiPath = window.location.origin + '/api/proxy/image'
  return `${apiPath}?url=${encodeURIComponent(target)}`
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
    class="group relative flex flex-col bg-base-100 rounded-[1.2rem] overflow-hidden border border-base-200/60 shadow-sm hover:shadow-2xl hover:shadow-primary hover:border-primary/30 transition-all duration-500 cursor-pointer active:scale-[0.98] h-full"
    :class="{ 'opacity-80 grayscale-[0.4]': !sub.enabled, 'opacity-40 pointer-events-none': pending }"
    @click="emit('click')"
  >
    <!-- Cover Image Wrapper -->
    <div class="relative aspect-[3/3.8] overflow-hidden bg-base-300">
      <!-- Loading Skeleton -->
      <div v-if="!isImageLoaded" class="absolute inset-0 skeleton rounded-none"></div>
      
      <img
        v-if="sub.cover_url"
        :src="proxyImage(sub.cover_url)"
        :alt="sub.title_cn"
        class="w-full h-full object-contain bg-black transition-all duration-1000 group-hover:scale-105"
        :class="{ 'opacity-0': !isImageLoaded, 'opacity-100': isImageLoaded }"
        @load="isImageLoaded = true"
        @error="(e) => { 
          const target = e.target as HTMLImageElement;
          if (!target.src.includes('placeholder')) {
            target.src = 'https://via.placeholder.com/300x400/1a1a1a/ffffff?text=Image+Load+Error';
          }
        }"
        loading="lazy"
      />
      <div v-else class="w-full h-full flex items-center justify-center text-base-content/5">
        <Image :size="32" />
      </div>

      <!-- Glass Badges -->
      <div class="absolute top-2.5 left-2.5 flex flex-col gap-1.5 z-10">
        <div v-if="sub.completed" class="bg-success/90 backdrop-blur-md text-success-content text-[8px] font-black uppercase tracking-widest py-0.5 px-2 rounded-md shadow-lg flex items-center gap-1 border border-white/20">
          <Check :size="8" />
          {{ $t('card.completed') }}
        </div>
        <div v-if="!sub.enabled" class="bg-base-100/70 backdrop-blur-md text-base-content text-[8px] font-black uppercase tracking-widest py-0.5 px-2 rounded-md shadow-lg flex items-center gap-1 border border-white/10">
          <Pause :size="8" />
          {{ $t('card.paused') }}
        </div>
        <div v-if="sub.stalled_episodes > 0" class="bg-warning/90 backdrop-blur-md text-warning-content text-[8px] font-black uppercase tracking-widest py-0.5 px-2 rounded-md shadow-lg flex items-center gap-1 border border-white/20">
          <AlertTriangle :size="8" />
          {{ $t('card.stalled', { count: sub.stalled_episodes }) }}
        </div>
      </div>

      <!-- Batch Delete Selection Mode Visual -->
      <div v-if="batchDeleteMode" class="absolute inset-0 z-20 pointer-events-none rounded-[1.2rem] transition-all duration-300"
           :class="batchSelected ? 'border-4 border-error bg-error/10' : 'border-4 border-transparent'">
         <div v-if="batchSelected" class="absolute top-2 right-2 bg-error text-error-content rounded-full p-1 shadow-lg">
           <Check :size="12" />
         </div>
      </div>

      <!-- Floating Actions (Hover) -->
      <div class="absolute top-2 right-2 flex flex-col gap-1.5 opacity-0 translate-x-4 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300 z-10" @click.stop>
        <button
          class="w-8 h-8 rounded-full bg-base-100/90 backdrop-blur-md border border-white/20 flex items-center justify-center text-base-content hover:bg-primary hover:text-primary-content hover:scale-110 shadow-xl transition-all active:scale-95"
          @click="emit('toggle')"
        >
          <component :is="sub.enabled ? Pause : Play" :size="14" />
        </button>
        <button
          class="w-8 h-8 rounded-full bg-base-100/90 backdrop-blur-md border border-white/20 flex items-center justify-center text-base-content hover:bg-error hover:text-error-content hover:scale-110 shadow-xl transition-all active:scale-95"
          @click="emit('delete')"
          :disabled="deleting"
        >
          <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
          <Trash2 v-else :size="14" />
        </button>
      </div>
      
      <!-- Gradient Overlay & Progress -->
      <div class="absolute bottom-0 left-0 w-full p-3 pt-8 bg-gradient-to-t from-black/80 via-black/30 to-transparent">
         <div class="flex justify-between items-end mb-1.5">
            <div>
               <p class="text-[7px] text-white/50 font-black uppercase tracking-[0.2em] mb-0.5">{{ $t('card.episodes') }}</p>
               <p class="text-sm text-white font-black leading-none">{{ sub.current_episodes }}<span class="text-white/30 text-[9px] ml-1 font-bold">/ {{ sub.total_episodes || '?' }}</span></p>
            </div>
            <div class="text-right">
               <p class="text-[9px] text-white font-black leading-none">{{ Math.round(progress) }}%</p>
            </div>
         </div>
         <div class="h-1 w-full bg-white/10 rounded-full overflow-hidden border border-white/5">
            <div 
              class="h-full rounded-full transition-all duration-1000 ease-out relative"
              :class="progressColor"
              :style="{ width: `${progress}%` }"
            ></div>
         </div>
      </div>
    </div>

    <!-- Content Area -->
    <div class="p-3.5 flex flex-col gap-2.5 flex-1 bg-gradient-to-b from-transparent to-base-200/5">
      <h3 class="font-black text-[13px] leading-tight line-clamp-2 min-h-[2.2rem] group-hover:text-primary transition-colors tracking-tight" :title="sub.title_cn">
        {{ sub.title_cn }}
      </h3>
      
      <div class="flex items-center gap-1.5 text-[8px] font-black text-primary/80 bg-primary/10 py-1 px-2.5 rounded-md self-start uppercase tracking-widest border border-primary/20">
        <User :size="10" class="text-primary" />
        <span class="truncate max-w-[100px]">{{ sub.subgroup_name || $t('card.generic') }}</span>
      </div>

      <div class="flex flex-wrap gap-1.5 mt-auto">
        <span v-if="sub.anime_type" class="text-[7px] font-black uppercase tracking-[0.2em] bg-base-content/5 text-base-content/60 py-0.5 px-2 rounded border border-base-content/10">
          {{ sub.anime_type }}
        </span>
        <span v-if="sub.year" class="text-[7px] font-black uppercase tracking-[0.2em] bg-base-content/5 text-base-content/40 py-0.5 px-2 rounded border border-base-content/5">
          {{ sub.year }}
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
