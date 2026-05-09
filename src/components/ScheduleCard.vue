<script setup lang="ts">
import IconSax from './IconSax.vue'

interface TorrentItem {
  title: string
  url: string
  source: string
  bangumi_id: string
  info_hash: string
  cover_url: string
}

defineProps<{
  item: TorrentItem
  isSubscribed?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

function proxyImage(url: string): string {
  if (!url) return ''
  if (url.startsWith('http') || url.startsWith('//')) {
    const target = url.startsWith('//') ? 'https:' + url : url
    return `/api/proxy/image?url=${encodeURIComponent(target)}`
  }
  return url
}
</script>

<template>
  <div 
    class="group relative flex flex-col bg-base-100 rounded-2xl overflow-hidden border border-base-200/60 shadow-sm hover:shadow-xl hover:border-primary/30 transition-all duration-300 cursor-pointer active:scale-[0.98]"
    @click="emit('click')"
  >
    <!-- Poster -->
    <div class="relative aspect-[3/4] bg-base-300 overflow-hidden">
      <img 
        v-if="item.cover_url" 
        :src="proxyImage(item.cover_url)" 
        :alt="item.title" 
        class="w-full h-full object-cover transition-transform duration-700 group-hover:scale-110"
        loading="lazy"
        @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'" 
      />
      <div v-else class="w-full h-full flex items-center justify-center text-base-content/10">
        <IconSax name="antenna" :size="32" />
      </div>

      <!-- Subscribed Badge -->
      <div v-if="isSubscribed || item.info_hash === 'subscribed'" class="absolute top-2 right-2">
         <div class="badge badge-primary shadow-lg gap-1 py-3 px-3">
            <IconSax name="check" :size="12" />
            <span class="text-[10px] font-black uppercase tracking-widest">Subscribed</span>
         </div>
      </div>
      
      <!-- Overlay Title (Bottom) -->
      <div class="absolute bottom-0 left-0 w-full p-3 pt-8 bg-gradient-to-t from-black/80 to-transparent">
         <p class="text-white text-xs font-black leading-tight line-clamp-2">{{ item.title }}</p>
      </div>
    </div>
  </div>
</template>
