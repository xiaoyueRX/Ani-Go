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

  const origin = typeof window !== 'undefined' && window.location?.origin ? window.location.origin : ''
  return `${origin}/api/proxy/image?url=${encodeURIComponent(target)}`
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
    target.src = 'data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'300\' height=\'450\' viewBox=\'0 0 300 450\'%3E%3Crect width=\'300\' height=\'450\' fill=\'%23141417\'/%3E%3Ctext x=\'50%25\' y=\'50%25\' dominant-baseline=\'middle\' text-anchor=\'middle\' fill=\'%23a1a1aa\' font-size=\'13\' font-family=\'sans-serif\'%3E暂无封面%3C/text%3E%3C/svg%3E'
  }
}

const progress = computed(() => {
  if (props.sub.total_episodes <= 0) return null
  return Math.min(100, (props.sub.current_episodes / props.sub.total_episodes) * 100)
})

const progressBarColor = computed(() => {
  if (props.sub.completed) return 'bg-emerald-500'
  if (props.sub.stalled_episodes > 0) return 'bg-amber-500'
  return 'bg-[var(--v2-accent)]'
})
</script>

<template>
  <div
    class="group relative flex flex-col v2-panel overflow-hidden hover:border-[var(--v2-accent)]/50 transition-all duration-200 cursor-pointer h-full shadow-sm hover:shadow-xl"
    :class="{ 'opacity-75 grayscale-[0.3]': !sub.enabled, 'opacity-40 pointer-events-none': pending }"
    :inert="pending"
    @click="!pending && emit('click')"
  >
    <!-- 2:3 满幅电影感海报区 -->
    <div class="relative aspect-[2/3] overflow-hidden bg-[var(--v2-surface-2)]">
      <div v-if="coverSrc && !isImageLoaded" class="absolute inset-0 [content-visibility:auto]" aria-hidden="true">
        <div class="skeleton h-full w-full rounded-none"></div>
      </div>

      <img
        v-if="coverSrc"
        :src="coverSrc"
        :alt="sub.title_cn"
        class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-[1.04]"
        :class="{ 'opacity-0': !isImageLoaded, 'opacity-100': isImageLoaded }"
        @load="coverLoaded"
        @error="coverFailed"
        loading="lazy"
      />
      <div
        v-else
        class="w-full h-full flex flex-col items-center justify-center gap-2 text-[var(--v2-ink-subtle)] px-3 text-center"
        style="background: radial-gradient(circle at 30% 20%, color-mix(in srgb, var(--v2-accent) 18%, transparent), transparent 65%), var(--v2-surface-2);"
      >
        <Image :size="28" class="opacity-65" />
        <span class="text-[11px] font-medium line-clamp-2 opacity-75">{{ sub.title_cn }}</span>
      </div>

      <!-- 左上状态标签 -->
      <div class="absolute top-2.5 left-2.5 flex flex-col gap-1 z-10">
        <div v-if="sub.completed" class="bg-emerald-500/90 backdrop-blur-md text-white text-[10px] font-bold py-0.5 px-2 rounded-full shadow-sm flex items-center gap-1">
          <Check :size="10" />
          <span>{{ $t('card.completed') }}</span>
        </div>
        <div v-if="!sub.enabled" class="bg-black/75 backdrop-blur-md text-white text-[10px] font-semibold py-0.5 px-2 rounded-full border border-white/15 shadow-sm flex items-center gap-1">
          <Pause :size="10" />
          <span>{{ $t('card.paused') }}</span>
        </div>
        <div v-if="sub.stalled_episodes > 0" class="bg-amber-500/95 backdrop-blur-md text-zinc-950 text-[10px] font-bold py-0.5 px-2 rounded-full shadow-sm flex items-center gap-1">
          <AlertTriangle :size="10" />
          <span>{{ $t('card.stalled', { count: sub.stalled_episodes }) }}</span>
        </div>
      </div>

      <!-- 批量删除遮罩 -->
      <div
        v-if="batchDeleteMode"
        class="absolute inset-0 z-20 pointer-events-none transition-colors"
        :class="batchSelected ? 'ring-2 ring-inset ring-red-500 bg-red-500/20' : ''"
      >
        <div v-if="batchSelected" class="absolute top-2.5 right-2.5 bg-red-500 text-white rounded-full p-1 shadow">
          <Check :size="12" />
        </div>
      </div>

      <!-- 底部电影渐变字幕叠层 + 细进度条 -->
      <div class="absolute bottom-0 inset-x-0 px-3 pb-2.5 pt-12 bg-gradient-to-t from-black/90 via-black/45 to-transparent">
        <div class="flex items-baseline justify-between gap-2 mb-1.5">
          <span class="text-[11px] font-mono font-bold text-white tabular-nums leading-none">
            S{{ sub.season || 1 }} · {{ sub.current_episodes }}<span class="text-white/55 font-normal">/{{ sub.total_episodes || '?' }}</span>
          </span>
          <span v-if="progress !== null" class="text-[10px] text-white/85 font-mono tabular-nums leading-none">
            {{ Math.round(progress) }}%
          </span>
        </div>
        <div v-if="progress !== null" class="h-1 w-full bg-white/20 rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-500"
            :class="progressBarColor"
            :style="{ width: `${progress}%` }"
          ></div>
        </div>
      </div>
    </div>

    <!-- 卡片信息区 -->
    <div class="p-3 flex flex-col gap-2 flex-1">
      <h3 class="font-semibold text-[13.5px] leading-snug line-clamp-2 min-h-[2.35rem] text-[var(--v2-ink)]" :title="sub.title_cn">
        <button
          v-if="batchDeleteMode"
          type="button"
          class="cursor-pointer text-left hover:text-[var(--v2-accent)] transition-colors bg-transparent border-0 p-0"
          :aria-pressed="!!batchSelected"
          :disabled="pending"
          @click.stop="emit('click')"
        >
          {{ sub.title_cn }}
        </button>
        <a
          v-else
          :href="pending ? undefined : `/subscriptions/${sub.id}`"
          class="hover:text-[var(--v2-accent)] transition-colors"
          @click.stop="openTitle"
        >{{ sub.title_cn }}</a>
      </h3>

      <!-- 右上角悬浮操作按钮（保持在标题之后、绝对定位到封面右上角） -->
      <div
        class="absolute top-2.5 right-2.5 flex flex-col gap-1.5 opacity-0 translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 group-focus-within:opacity-100 group-focus-within:translate-x-0 transition-all duration-200 z-10"
        @click.stop
      >
        <button
          class="w-7 h-7 rounded-full bg-black/75 backdrop-blur-md border border-white/15 flex items-center justify-center text-white hover:bg-[var(--v2-accent)] hover:border-[var(--v2-accent)] shadow transition-colors cursor-pointer"
          type="button"
          :aria-label="$t(sub.enabled ? 'detail.actions.suspend' : 'detail.actions.resume')"
          :disabled="pending"
          @click="emit('toggle')"
        >
          <component :is="sub.enabled ? Pause : Play" :size="13" />
        </button>
        <button
          class="w-7 h-7 rounded-full bg-black/75 backdrop-blur-md border border-white/15 flex items-center justify-center text-white hover:bg-red-500 hover:border-red-500 shadow transition-colors cursor-pointer"
          type="button"
          :aria-label="$t('common.delete')"
          @click="emit('delete')"
          :disabled="deleting || pending"
        >
          <span v-if="deleting" class="loading loading-spinner loading-xs"></span>
          <Trash2 v-else :size="13" />
        </button>
      </div>

      <div class="flex items-center gap-1.5 flex-wrap mt-auto pt-0.5">
        <span class="inline-flex items-center gap-1 text-[11px] font-semibold text-[var(--v2-accent)] bg-[var(--v2-accent)]/12 px-2 py-0.5 rounded-md max-w-[140px]">
          <User :size="10" class="shrink-0" />
          <span class="truncate">{{ sub.subgroup_name || $t('card.generic') }}</span>
        </span>
        <span v-if="sub.anime_type" class="text-[10px] font-mono text-[var(--v2-ink-muted)] bg-[var(--v2-surface-2)] px-1.5 py-0.5 rounded border border-[var(--v2-border)]">
          {{ sub.anime_type }}
        </span>
        <span v-if="sub.year" class="text-[11px] text-[var(--v2-ink-subtle)] font-mono tabular-nums ml-auto">
          {{ sub.year }}
        </span>
      </div>

      <div
        v-if="sub.enabled && !sub.completed"
        class="pt-1.5 border-t border-[var(--v2-border)] opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity"
        @click.stop
      >
        <button
          class="text-[11px] font-semibold text-[var(--v2-accent)] hover:underline flex items-center gap-1 bg-transparent border-0 p-0 cursor-pointer"
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
  outline: 2px solid var(--v2-accent);
  outline-offset: -2px;
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
