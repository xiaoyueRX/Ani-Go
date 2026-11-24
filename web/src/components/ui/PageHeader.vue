<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
  badge?: string | number
}>()
</script>

<template>
  <header class="app-page-header">
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-3 min-w-0">
        <slot name="leading" />
        <h1 class="app-page-title truncate">{{ title }}</h1>
        <span
          v-if="badge !== undefined && badge !== ''"
          class="badge badge-neutral badge-sm font-mono font-semibold tabular-nums"
        >
          {{ badge }}
        </span>
        <slot name="meta" />
      </div>
      <p v-if="subtitle" class="app-page-subtitle">
        {{ subtitle }}
      </p>
    </div>

    <div v-if="$slots.actions" class="flex items-center gap-2 flex-wrap shrink-0">
      <slot name="actions" />
    </div>
  </header>
</template>
