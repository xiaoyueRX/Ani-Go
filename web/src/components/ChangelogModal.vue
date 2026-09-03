<script setup lang="ts">
import { ref } from 'vue'
import { X, Zap, CheckCircle2 } from 'lucide-vue-next'
import { CURRENT_VERSION } from '../composables/useVersion'

const props = defineProps<{
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
    <div class="modal-box bg-base-200/95 backdrop-blur-3xl border border-white/5 rounded-[2.5rem] p-0 overflow-hidden max-w-md w-full shadow-[0_0_50px_rgba(0,0,0,0.3)]">
      <!-- Header -->
      <div class="relative p-8 pb-4">
        <div class="flex items-center gap-4 mb-2">
          <div class="w-12 h-12 rounded-2xl bg-primary/20 flex items-center justify-center text-primary">
            <Zap :size="24" />
          </div>
          <div>
            <h3 class="text-2xl font-black tracking-tighter italic">🎉 {{ CURRENT_VERSION }} 更新内容</h3>
            <p class="text-[10px] font-black uppercase tracking-[0.2em] opacity-30 mt-1">What's New in Ani-Go</p>
          </div>
        </div>
        <button class="btn btn-circle btn-sm btn-ghost absolute top-6 right-6 opacity-30 hover:opacity-100" @click="close">
          <X :size="20" />
        </button>
      </div>

      <!-- Content -->
      <div class="px-8 pb-8 space-y-4">
        <div class="space-y-3">
          <div v-for="(item, index) in changelog" :key="index" 
            class="flex items-start gap-4 p-4 rounded-2xl bg-base-100/50 border border-base-content/5 group hover:border-primary/20 transition-colors">
            <CheckCircle2 :size="18" class="text-primary shrink-0 mt-0.5" />
            <p class="text-sm font-bold text-base-content/70 leading-relaxed">{{ item }}</p>
          </div>
        </div>

        <button class="btn btn-primary w-full rounded-2xl font-black tracking-widest text-xs h-14 shadow-lg shadow-primary/20 mt-4" @click="close">
          我知道了 (GOT IT)
        </button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop" @click="close">
      <button class="cursor-default">close</button>
    </form>
  </dialog>
</template>
