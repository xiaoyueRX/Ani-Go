<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  src?: string
  username?: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
  rounded?: 'full' | 'xl' | 'lg'
}>(), {
  src: '',
  username: 'admin',
  size: 'sm',
  rounded: 'lg',
})

const broken = ref(false)

watch(() => props.src, () => {
  broken.value = false
})

function isValidAvatarUrl(raw?: string): boolean {
  const value = (raw || '').trim()
  if (!value) return false
  return /^(?:\/|https?:\/\/|data:image\/)/i.test(value)
}

function proxyImage(url: string): string {
  const trimmed = url.trim()
  if (!trimmed) return ''
  if (trimmed.startsWith('/') || trimmed.startsWith('data:image/') || trimmed.includes('api/proxy/image')) {
    return trimmed
  }
  const target = trimmed.startsWith('//') ? `https:${trimmed}` : trimmed
  return `${window.location.origin}/api/proxy/image?url=${encodeURIComponent(target)}`
}

const resolvedSrc = computed(() => {
  if (!isValidAvatarUrl(props.src)) return ''
  return proxyImage(props.src!)
})

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xs': return 'w-6 h-6 text-[10px]'
    case 'md': return 'w-10 h-10 text-sm'
    case 'lg': return 'w-20 h-20 text-2xl'
    default: return 'w-8 h-8 text-xs'
  }
})

const radiusClass = computed(() => {
  switch (props.rounded) {
    case 'full': return 'rounded-full'
    case 'xl': return 'rounded-2xl'
    default: return 'rounded-lg'
  }
})

const initial = computed(() => {
  const name = (props.username || 'A').trim()
  return name ? name.slice(0, 1).toUpperCase() : 'A'
})
</script>

<template>
  <div
    class="relative inline-flex items-center justify-center overflow-hidden shrink-0 select-none border border-[var(--v2-border-strong)]"
    :class="[sizeClass, radiusClass]"
    :style="!resolvedSrc || broken ? {
      background: 'linear-gradient(135deg, var(--v2-accent) 0%, #fb923c 100%)',
      color: '#ffffff'
    } : undefined"
  >
    <img
      v-if="resolvedSrc && !broken"
      :src="resolvedSrc"
      :alt="username || 'User'"
      class="w-full h-full object-cover"
      @error="broken = true"
    />
    <span v-else class="font-bold tracking-tight drop-shadow-xs">{{ initial }}</span>
  </div>
</template>
