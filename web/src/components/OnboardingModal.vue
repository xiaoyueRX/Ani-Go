<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { 
  Calendar, 
  LayoutGrid, 
  Star, 
  Bell,
  ChevronRight,
  X
} from 'lucide-vue-next'

const show = ref(false)
const dontShowAgain = ref(false)

const STORAGE_KEY = 'ani-go-onboarding-dismissed'
const SESSION_KEY = 'ani-go-onboarding-shown'

onMounted(() => {
  const dismissed = localStorage.getItem(STORAGE_KEY)
  const shownInSession = sessionStorage.getItem(SESSION_KEY)
  
  if (!dismissed && !shownInSession) {
    show.value = true
  }
})

const closeModal = () => {
  if (dontShowAgain.value) {
    localStorage.setItem(STORAGE_KEY, 'true')
  }
  sessionStorage.setItem(SESSION_KEY, 'true')
  show.value = false
}

const features = [
  {
    icon: Calendar,
    title: '季度选择器',
    desc: '左上角切换年份和季度，浏览历史新番',
    color: 'text-blue-400'
  },
  {
    icon: LayoutGrid,
    title: '全部/我的',
    desc: '切换查看所有新番 / 只看已订阅',
    color: 'text-purple-400'
  },
  {
    icon: Star,
    title: '订阅番剧',
    desc: '点击番剧卡片，搜索并订阅字幕组',
    color: 'text-yellow-400'
  },
  {
    icon: Bell,
    title: '自动追番',
    desc: '订阅后自动下载，在订阅管理中查看进度',
    color: 'text-green-400'
  }
]
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-6">
    <!-- Backdrop -->
    <div class="absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity" @click="closeModal"></div>

    <!-- Modal Card -->
    <div class="relative w-full max-w-lg bg-base-100/90 backdrop-blur-2xl border border-white/10 rounded-[2.5rem] shadow-2xl overflow-hidden animate-in fade-in zoom-in duration-300">
      
      <!-- Content -->
      <div class="p-8 sm:p-10">
        <!-- Header -->
        <div class="text-center mb-10">
          <h2 class="text-3xl font-black tracking-tight mb-2 italic">🎉 欢迎使用 Ani-Go</h2>
          <p class="text-sm font-bold text-base-content/50 uppercase tracking-widest">全自动番剧追番管理系统</p>
        </div>

        <!-- Features List -->
        <div class="space-y-6">
          <div v-for="f in features" :key="f.title" class="flex gap-4 items-start group">
            <div :class="['p-3 rounded-2xl bg-base-200/50 border border-white/5 transition-transform group-hover:scale-110 shadow-sm', f.color]">
              <component :is="f.icon" :size="20" />
            </div>
            <div class="space-y-1">
              <h3 class="font-black text-sm tracking-wide">{{ f.title }}</h3>
              <p class="text-xs font-medium text-base-content/40 leading-relaxed">{{ f.desc }}</p>
            </div>
          </div>
        </div>

        <!-- Footer -->
        <div class="mt-12 space-y-4">
          <div class="flex items-center justify-center gap-3">
            <label class="label cursor-pointer gap-3">
              <input type="checkbox" v-model="dontShowAgain" class="checkbox checkbox-primary checkbox-sm rounded-lg" />
              <span class="label-text font-bold text-xs opacity-60">不再显示</span>
            </label>
          </div>
          
          <button 
            @click="closeModal"
            class="btn btn-primary w-full rounded-2xl h-14 font-black tracking-widest text-sm shadow-lg shadow-primary/20 hover:shadow-primary/40 transition-all flex gap-2"
          >
            开始使用
            <ChevronRight :size="18" />
          </button>
        </div>
      </div>

      <!-- Close Button -->
      <button @click="closeModal" class="btn btn-circle btn-ghost btn-sm absolute top-6 right-6 opacity-30 hover:opacity-100">
        <X :size="18" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.animate-in {
  animation-fill-mode: both;
}
@keyframes fade-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes zoom-in {
  from { transform: scale(0.95); }
  to { transform: scale(1); }
}
.fade-in { animation-name: fade-in; }
.zoom-in { animation-name: zoom-in; }
</style>
