<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  value: string | number
  tone?: 'default' | 'primary' | 'success' | 'warning' | 'error'
  active?: boolean
  clickable?: boolean
}>(), {
  tone: 'default',
  active: false,
  clickable: false,
})

const emit = defineEmits<{
  (e: 'click'): void
}>()

const valueColorClass = computed(() => {
  switch (props.tone) {
    case 'primary':
      return 'text-primary'
    case 'success':
      return 'text-success'
    case 'warning':
      return 'text-warning'
    case 'error':
      return 'text-error'
    default:
      return 'text-base-content'
  }
})

const iconBoxClass = computed(() => {
  switch (props.tone) {
    case 'primary':
      return 'bg-primary/12 text-primary'
    case 'success':
      return 'bg-success/12 text-success'
    case 'warning':
      return 'bg-warning/12 text-warning'
    case 'error':
      return 'bg-error/12 text-error'
    default:
      return 'bg-base-200 text-base-content/65'
  }
})

const containerClass = computed(() => [
  'app-stat-card',
  props.clickable ? 'cursor-pointer hover:border-primary/45 text-left w-full' : '',
  props.active ? 'border-primary ring-1 ring-primary/25 bg-primary/[0.04]' : '',
])
</script>

<template>
  <button
    v-if="clickable"
    type="button"
    :class="containerClass"
    :aria-pressed="active"
    @click="emit('click')"
  >
    <div class="min-w-0 space-y-0.5">
      <span class="block text-xs font-medium text-base-content/60 truncate">{{ label }}</span>
      <p class="text-lg sm:text-xl font-bold font-mono tabular-nums truncate" :class="valueColorClass">
        {{ value }}
      </p>
    </div>
    <div
      v-if="$slots.icon"
      class="w-9 h-9 rounded-lg flex items-center justify-center shrink-0"
      :class="iconBoxClass"
    >
      <slot name="icon" />
    </div>
  </button>

  <div v-else :class="containerClass">
    <div class="min-w-0 space-y-0.5">
      <span class="block text-xs font-medium text-base-content/60 truncate">{{ label }}</span>
      <p class="text-lg sm:text-xl font-bold font-mono tabular-nums truncate" :class="valueColorClass">
        {{ value }}
      </p>
    </div>
    <div
      v-if="$slots.icon"
      class="w-9 h-9 rounded-lg flex items-center justify-center shrink-0"
      :class="iconBoxClass"
    >
      <slot name="icon" />
    </div>
  </div>
</template>
