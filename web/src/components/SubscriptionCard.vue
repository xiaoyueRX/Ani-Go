<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Image, Check, Pause, Play, AlertTriangle,
  Trash2, User, History
} from 'lucide-vue-next'

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
  if (url.startsWith('//')) {
    target = 'https:' + url
  }

  const apiPath = window.location.origin + '/api/proxy/image'
  return `${apiPath}?url=${encodeURIComponent(target)}`
}

const emit = defineEmits<{
  (e: 'click'): void
  (e: 'toggle'): void
  (e: 'delete'): void
  (e: 'supplement'): void
}>()

const coverSrc = computed(() => proxyImage(props.sub.cover_url))
function openTitle(event: MouseEvent) {
  if (props.pending || event.defaultPrevented || event.button !== 0 ||
      event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  emit('click')
}

const isImageLoaded = ref(false)
watch(coverSrc, () => { isImageLoaded.value = false })

// 被 v-if 移除的旧图片仍可能收到 load 或 error
function coverLoaded(event: Event) {
  if ((event.currentTarget as HTMLImageElement).isConnected) isImageLoaded.value = true
}

function coverFailed(event: Event) {
  const target = event.currentTarget as HTMLImageElement
  if (target.isConnected && !target.src.startsWith('data:')) {
    target.src = 'data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'300\' height=\'450\' viewBox=\'0 0 300 450\'%3E%3Crect width=\'300\' height=\'450\' fill=\'%2314171f\'/%3E%3Ctext x=\'50%25\' y=\'50%25\' dominant-baseline=\'middle\' text-anchor=\'middle\' fill=\'%237a8299\' font-size=\'13\' font-family=\'sans-serif\'%3E暂无封面%3C/text%3E%3C/svg%3E'
  }
}

const progress = computed(() => {
  if (props.sub.total_episodes <= 0) return null
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
    class="group relative flex flex-col app-surface rounded-xl overflow-hidden hover:border-primary/40 hover:shadow-md transition-all duration-200 cursor-pointer h-full"
    :class="{ 'opacity-75 grayscale-[0.35]': !sub.enabled, 'opacity-40 pointer-events-none': pending }"
    :inert="pending"
    @click="!pending && emit('click')"
  >
    <!-- 封面区域（2:3 海报比例） -->
    <div class="relative aspect-[2/3] overflow-hidden bg-base-200">
      <div v-if="coverSrc && !isImageLoaded" class="absolute inset-0 [content-visibility:auto]" aria-hidden="true">
        <div class="skeleton h-full w-full rounded-none"></div>
      </div>

      <img
        v-if="coverSrc"
        :src="coverSrc"
        :alt="sub.title_cn"
        class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
        :class="{ 'opacity-0': !isImageLoaded, 'opacity-100': isImageLoaded }"
        @load="coverLoaded"
        @error="coverFailed"
        loading="lazy"
      />
      <div v-else class="w-full h-full flex items-center justify-center text-base-content/20">
        <Image :size="30" />
      </div>

      <!-- 状态标识胶囊 -->
      <div class="absolute top-2 left-2 flex flex-col gap-1 z-10">
        <div v-if="sub.completed" class="bg-success/90 backdrop-blur-sm text-success-content text-[11px] font-semibold py-0.5 px-2 rounded-md shadow-sm flex items-center gap-1">
          <Check :size="10" />
          <span>{{ $t('card.completed') }}</span>
        </div>
        <div v-if="!sub.enabled" class="bg-base-100/85 backdrop-blur-sm text-base-content text-[11px] font-semibold py-0.5 px-2 rounded-md border border-base-content/12 shadow-sm flex items-center gap-1">
          <Pause :size="10" />
          <span>{{ $t('card.paused') }}</span>
        </div>
        <div v-if="sub.stalled_episodes > 0" class="bg-warning/90 backdrop-blur-sm text-warning-content text-[11px] font-semibold py-0.5 px-2 rounded-md shadow-sm flex items-center gap-1">
          <AlertTriangle :size="10" />
          <span>{{ $t('card.stalled', { count: sub.stalled_episodes }) }}</span>
        </div>
      </div>

      <!-- 批量删除选中遮罩 -->
      <div
        v-if="batchDeleteMode"
        class="absolute inset-0 z-20 pointer-events-none rounded-xl transition-colors"
        :class="batchSelected ? 'ring-2 ring-inset ring-error bg-error/15' : ''"
      >
        <div v-if="batchSelected" class="absolute top-2 right-2 bg-error text-error-content rounded-full p-1 shadow">
          <Check :size="12" />
        </div>
      </div>

      <!-- 底部集数与进度条 -->
      <div class="absolute bottom-0 inset-x-0 px-3 pb-2.5 pt-8 bg-gradient-to-t from-black/85 via-black/40 to-transparent">
        <div class="flex items-baseline justify-between gap-2 mb-1.5">
          <p class="text-xs text-white font-semibold tabular-nums leading-none">
            {{ sub.current_episodes }}<span class="text-white/55 font-normal ml-0.5">/ {{ sub.total_episodes || '?' }}</span>
          </p>
          <span v-if="progress !== null" class="text-[11px] text-white/85 font-mono tabular-nums leading-none">
            {{ Math.round(progress) }}%
          </span>
        </div>
        <div v-if="progress !== null" class="h-1 w-full bg-white/20 rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-500"
            :class="progressColor"
            :style="{ width: `${progress}%` }"
          ></div>
        </div>
      </div>
    </div>

    <!-- 卡片信息区 -->
    <div class="p-3 flex flex-col gap-2 flex-1">
      <h3 class="font-semibold text-sm leading-snug line-clamp-2 min-h-[2.5rem] text-base-content" :title="sub.title_cn">
        <button
          v-if="batchDeleteMode"
          type="button"
          class="cursor-pointer text-left hover:text-primary transition-colors"
          :aria-pressed="!!batchSelected"
          :disabled="pending"
          @click.stop="emit('click')"
        >
          {{ sub.title_cn }}
        </button>
        <a
          v-else
          :href="pending ? undefined : `/subscriptions/${sub.id}`"
          class="hover:text-primary transition-colors"
          @click.stop="openTitle"
        >{{ sub.title_cn }}</a>
      </h3>

      <!-- 标题之后才进入操作按钮，绝对定位保持按钮位于封面右上角 -->
      <div
        class="absolute top-2 right-2 flex flex-col gap-1.5 opacity-0 translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 group-focus-within:opacity-100 group-focus-within:translate-x-0 transition-all duration-200 z-10"
        @click.stop
      >
        <button
          class="w-7 h-7 rounded-full bg-base-100/90 backdrop-blur-sm border border-base-content/15 flex items-center justify-center text-base-content hover:bg-primary hover:text-primary-content shadow transition-colors"
          type="button"
          :aria-label="$t(sub.enabled ? 'detail.actions.suspend' : 'detail.actions.resume')"
          :disabled="pending"
          @click="emit('toggle')"
        >
          <component :is="sub.enabled ? Pause : Play" :size="13" />
        </button>
        <button
          class="w-7 h-7 rounded-full bg-base-100/90 backdrop-blur-sm border border-base-content/15 flex items-center justify-center text-base-content hover:bg-error hover:text-error-content shadow transition-colors"
          type="button"
          :aria-label="$t('common.delete')"
          @click="emit('delete')"
          :disabled="deleting || pending"
        >
          <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
          <Trash2 v-else :size="13" />
        </button>
      </div>

      <div class="flex items-center gap-1.5 flex-wrap mt-auto pt-1">
        <span class="inline-flex items-center gap-1 text-[11px] font-medium text-primary bg-primary/10 px-2 py-0.5 rounded-md max-w-[140px]">
          <User :size="10" class="shrink-0" />
          <span class="truncate">{{ sub.subgroup_name || $t('card.generic') }}</span>
        </span>
        <span v-if="sub.anime_type" class="text-[11px] text-base-content/60 bg-base-200 px-1.5 py-0.5 rounded">
          {{ sub.anime_type }}
        </span>
        <span v-if="sub.year" class="text-[11px] text-base-content/50 font-mono tabular-nums">
          {{ sub.year }}
        </span>
      </div>

      <div
        v-if="sub.enabled && !sub.completed"
        class="pt-1 border-t border-base-content/6 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity"
        @click.stop
      >
        <button
          class="btn btn-ghost btn-xs h-6 min-h-0 px-0 text-[11px] font-medium text-primary hover:bg-transparent hover:underline gap-1"
          type="button"
          :disabled="pending"
          @click="emit('supplement')"
        >
          <History :size="12" />
          <span>{{ $t('card.supplement') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
:is(a, button):focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: -2px;
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
