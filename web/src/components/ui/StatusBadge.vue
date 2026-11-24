<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  tone?: 'neutral' | 'primary' | 'success' | 'warning' | 'error' | 'info'
  dot?: boolean
}>(), {
  tone: 'neutral',
  dot: false,
})

const toneClass = computed(() => {
  switch (props.tone) {
    case 'primary':
      return 'bg-primary/12 text-primary border-primary/20'
    case 'success':
      return 'bg-success/12 text-success border-success/20'
    case 'warning':
      return 'bg-warning/15 text-warning border-warning/25'
    case 'error':
      return 'bg-error/12 text-error border-error/20'
    case 'info':
      return 'bg-info/12 text-info border-info/20'
    default:
      return 'bg-base-200 text-base-content/75 border-base-content/10'
  }
})

const dotClass = computed(() => {
  switch (props.tone) {
    case 'primary':
      return 'bg-primary'
    case 'success':
      return 'bg-success'
    case 'warning':
      return 'bg-warning'
    case 'error':
      return 'bg-error'
    case 'info':
      return 'bg-info'
    default:
      return 'bg-base-content/50'
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md border text-[11px] font-medium whitespace-nowrap"
    :class="toneClass"
  >
    <span v-if="dot" class="w-1.5 h-1.5 rounded-full shrink-0" :class="dotClass" />
    <slot />
  </span>
</template>
