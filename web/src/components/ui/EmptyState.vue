<script setup lang="ts">
defineProps<{
  title: string
  description?: string
  hint?: string
}>()
</script>

<template>
  <div class="app-empty-state">
    <div
      v-if="$slots.icon"
      class="w-11 h-11 rounded-xl bg-base-200 text-base-content/50 flex items-center justify-center mb-3"
    >
      <slot name="icon" />
    </div>
    <h3 class="text-base font-semibold text-base-content">{{ title }}</h3>
    <p v-if="description" class="text-sm text-base-content/60 mt-1 max-w-md leading-relaxed">
      {{ description }}
    </p>
    <p v-if="hint" class="text-xs text-base-content/45 mt-2 max-w-md leading-relaxed">
      {{ hint }}
    </p>
    <div v-if="$slots.actions" class="mt-5 flex items-center gap-2 flex-wrap justify-center">
      <slot name="actions" />
    </div>
  </div>
</template>
