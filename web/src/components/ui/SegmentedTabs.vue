<script setup lang="ts">
import type { Component } from 'vue'

export interface SegmentedTabOption {
  value: string
  label: string
  count?: number | string
  icon?: Component
}

defineProps<{
  modelValue: string
  options: SegmentedTabOption[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()
</script>

<template>
  <div class="app-segmented max-w-full" role="tablist">
    <button
      v-for="opt in options"
      :key="opt.value"
      type="button"
      role="tab"
      class="app-segmented-btn"
      :aria-selected="modelValue === opt.value"
      :data-active="modelValue === opt.value ? 'true' : 'false'"
      @click="emit('update:modelValue', opt.value)"
    >
      <component :is="opt.icon" v-if="opt.icon" :size="14" class="shrink-0" />
      <span>{{ opt.label }}</span>
      <span
        v-if="opt.count !== undefined && opt.count !== ''"
        class="inline-flex items-center justify-center px-1.5 py-0.2 rounded text-[11px] font-mono tabular-nums"
        :class="modelValue === opt.value ? 'bg-primary/15 text-primary' : 'bg-base-content/10 text-base-content/65'"
      >
        {{ opt.count }}
      </span>
    </button>
  </div>
</template>
