<script setup lang="ts">
import { X, Zap, CheckCircle2 } from 'lucide-vue-next'
import { currentVersion } from '../composables/useVersion'

defineProps<{
  show: boolean
  changelog: string[]
}>()

const emit = defineEmits(['close'])

const close = () => {
  emit('close')
}
</script>

<template>
  <dialog :class="['modal', { 'modal-open': show }]">
    <div class="modal-box max-w-md w-full p-0 overflow-hidden">
      <div class="relative p-6 pb-4 border-b border-[var(--v2-border)] flex items-center justify-between gap-3">
        <div class="flex items-center gap-3 min-w-0">
          <div class="w-10 h-10 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
            <Zap :size="20" />
          </div>
          <div class="min-w-0">
            <h3 class="text-base font-bold text-[var(--v2-ink)] truncate">{{ currentVersion }} 更新内容</h3>
            <p class="text-[10px] font-mono uppercase tracking-widest text-[var(--v2-ink-subtle)] mt-0.5">What's New in Ani-Go</p>
          </div>
        </div>
        <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm px-2 shrink-0" @click="close">
          <X :size="16" />
        </button>
      </div>

      <div class="p-6 flex flex-col gap-4">
        <div class="space-y-2.5 overflow-y-auto max-h-[55vh] pr-1">
          <div
            v-for="(item, index) in changelog"
            :key="index"
            class="v2-panel-muted p-3.5 flex items-start gap-3 text-xs leading-relaxed"
          >
            <CheckCircle2 :size="16" class="text-[var(--v2-accent)] shrink-0 mt-0.5" />
            <p class="text-[var(--v2-ink)] font-medium">{{ item }}</p>
          </div>
        </div>

        <button type="button" class="v2-btn v2-btn-primary w-full h-10" @click="close">
          我知道了 (GOT IT)
        </button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop" @click="close">
      <button class="cursor-default">close</button>
    </form>
  </dialog>
</template>
