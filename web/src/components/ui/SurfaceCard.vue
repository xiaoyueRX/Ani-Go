<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title?: string
  description?: string
  padding?: 'none' | 'sm' | 'md' | 'lg'
  muted?: boolean
}>(), {
  padding: 'md',
  muted: false,
})

const bodyPaddingClass = computed(() => {
  switch (props.padding) {
    case 'none':
      return ''
    case 'sm':
      return 'p-3 sm:p-4'
    case 'lg':
      return 'p-5 sm:p-7'
    default:
      return 'p-4 sm:p-5'
  }
})
</script>

<template>
  <section :class="muted ? 'app-surface-muted' : 'app-surface'" class="overflow-hidden">
    <div
      v-if="title || description || $slots.headerActions"
      class="flex items-start justify-between gap-4 px-4 sm:px-5 py-3.5 border-b border-base-content/8"
    >
      <div class="min-w-0">
        <h2 v-if="title" class="text-sm font-semibold text-base-content">{{ title }}</h2>
        <p v-if="description" class="text-xs text-base-content/60 mt-0.5">{{ description }}</p>
      </div>
      <div v-if="$slots.headerActions" class="shrink-0 flex items-center gap-2">
        <slot name="headerActions" />
      </div>
    </div>

    <div :class="bodyPaddingClass">
      <slot />
    </div>

    <div
      v-if="$slots.footer"
      class="px-4 sm:px-5 py-3 border-t border-base-content/8 bg-base-200/40"
    >
      <slot name="footer" />
    </div>
  </section>
</template>
