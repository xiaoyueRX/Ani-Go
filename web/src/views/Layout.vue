<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Menu, User, LogOut, Antenna, Languages, ExternalLink,
  Calendar, LayoutGrid, Search, Download, Settings, Sparkles, X
} from 'lucide-vue-next'
import { useVersion, CURRENT_VERSION } from '../composables/useVersion'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const username = ref('')
const userAvatar = ref('')
const isDrawerOpen = ref(false)

const { latestVersion, hasNewVersion, checkGitHubUpdate } = useVersion()

interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

const toasts = ref<Toast[]>([])
let toastId = 0

function showToast(message: string, type: 'success' | 'error' | 'info' = 'success') {
  const id = ++toastId
  toasts.value.push({ id, message, type })
  setTimeout(() => {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }, 3000)
}

// 暴露出全局 Toast 给子组件使用
window.showToast = showToast

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.startsWith('/api/') || url.startsWith('data:') || url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

function toggleLanguage() {
  locale.value = locale.value === 'zh' ? 'en' : 'zh'
  localStorage.setItem('lang', locale.value)
}

onMounted(async () => {
  try {
    const { data } = await request.get('/me')
    username.value = data.username
    userAvatar.value = data.avatar_url || ''
  } catch { /* 401 handled by interceptor */ }

  window.addEventListener('avatar-updated', ((e: CustomEvent) => {
    if (e.detail) {
      userAvatar.value = e.detail
    }
  }) as EventListener)

  // Check for updates if enabled in backend settings
  try {
    const { data: settings } = await request.get('/settings')
    if (settings.AUTO_CHECK_UPDATE === 'true') {
      checkGitHubUpdate()
    }
  } catch (e) {
    console.error('Failed to fetch settings for update check:', e)
  }
})

function logout() {
  localStorage.removeItem('token')
  router.push('/login')
}

const nav = computed(() => [
  { path: '/schedule', label: t('nav.schedule'), icon: Calendar },
  { path: '/subscriptions', label: t('nav.subscriptions'), icon: LayoutGrid },
  { path: '/search', label: t('nav.search'), icon: Search },
  { path: '/downloads', label: t('nav.downloads'), icon: Download },
  { path: '/settings', label: t('nav.settings'), icon: Settings },
])

function closeDrawer() {
  isDrawerOpen.value = false
}
</script>

<template>
  <div class="drawer lg:drawer-open min-h-screen bg-base-300/30">
    <input id="drawer-toggle" type="checkbox" class="drawer-toggle" v-model="isDrawerOpen" />

    <div class="drawer-content flex flex-col">
      <!-- Top navbar (Mobile & Tablet) -->
      <div class="sticky top-0 z-30 flex h-16 w-full justify-center bg-base-100/60 backdrop-blur transition-all duration-100 lg:hidden border-b border-base-200/50">
        <div class="navbar w-full max-w-[1400px] lg:max-w-[1600px] xl:max-w-[2000px] 2xl:max-w-[2400px] mx-auto">
          <div class="flex-none">
            <label for="drawer-toggle" class="btn btn-square btn-ghost">
              <Menu :size="24" />
            </label>
          </div>
          <div class="flex-1 px-2">
            <span class="text-lg font-bold bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent">Ani-Go</span>
          </div>
          <div class="flex-none gap-2">
            <div class="dropdown dropdown-end">
              <label tabindex="0" class="btn btn-ghost btn-circle avatar online">
                <div v-if="userAvatar" class="w-10 rounded-full bg-cover bg-center" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
                <div v-else class="w-10 rounded-full bg-primary/10 flex items-center justify-center">
                  <User :size="20" class="text-primary" />
                </div>
              </label>
              <ul tabindex="0" class="mt-3 z-[1] p-2 shadow-xl menu menu-sm dropdown-content bg-base-100 rounded-box w-52 border border-base-200">
                <li class="menu-title opacity-50 px-4 py-2 text-xs uppercase font-bold tracking-widest">{{ username }}</li>
                <li><a @click="toggleLanguage" class="flex items-center gap-2"><Languages :size="16" /> {{ locale === 'zh' ? 'English' : '中文' }}</a></li>
                <li><a @click="logout" class="text-error"><LogOut :size="16" /> {{ $t('nav.logout') }}</a></li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Main content area -->
      <main class="flex-1 p-4 sm:p-6 md:p-8 max-w-[1400px] lg:max-w-[1600px] xl:max-w-[2000px] 2xl:max-w-[2400px] mx-auto w-full overflow-hidden">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>

      <!-- Footer -->
      <footer class="py-6 px-4 text-center text-xs space-y-1.5 mt-auto border-t border-base-content/5 opacity-60 hover:opacity-100 transition-opacity">
        <div class="font-semibold text-base-content/80 flex items-center justify-center gap-1.5 flex-wrap">
          <span>Ani-Go &copy; 2026 • 倾心打造</span>
          <span class="opacity-40">•</span>
          <span>by <a href="https://github.com/xiaoyueRX" target="_blank" rel="noopener noreferrer" class="text-primary font-bold hover:underline">xiaoyue</a></span>
        </div>
        <div class="text-[11px] opacity-70 flex items-center justify-center gap-2 flex-wrap">
          <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" 
             class="inline-flex items-center gap-1.5 hover:text-primary transition-colors font-mono font-medium">
            <svg class="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
            <span>GitHub: xiaoyueRX/Ani-Go</span>
          </a>
          <span class="opacity-30">•</span>
          <span class="font-mono">v{{ CURRENT_VERSION }}</span>
        </div>
      </footer>

      <!-- Global Toasts -->
      <div class="toast toast-end toast-bottom p-4 z-[100]">
        <transition-group name="page">
          <div 
            v-for="t in toasts" :key="t.id"
            class="alert shadow-2xl border-0 mb-2 rounded-2xl min-w-[280px] animate-in slide-in-from-right duration-300"
            :class="{
              'bg-primary text-primary-content': t.type === 'success',
              'bg-error text-error-content': t.type === 'error',
              'bg-info text-info-content': t.type === 'info'
            }"
          >
            <div class="flex items-center gap-3">
              <Sparkles v-if="t.type === 'success'" :size="18" />
              <TriangleAlert v-else-if="t.type === 'error'" :size="18" />
              <Antenna v-else :size="18" />
              <span class="text-xs font-black uppercase tracking-widest">{{ t.message }}</span>
            </div>
          </div>
        </transition-group>
      </div>
    </div>

    <!-- Sidebar -->
    <div class="drawer-side z-40">
      <label for="drawer-toggle" class="drawer-overlay" aria-label="close sidebar"></label>
      <aside class="w-72 min-h-screen bg-base-100 border-r border-base-200/50 flex flex-col shadow-2xl lg:shadow-none">
        <!-- Logo Section -->
        <div class="px-8 py-10">
          <div class="flex items-center gap-4 group">
            <div class="w-12 h-12 rounded-2xl bg-gradient-to-br from-primary to-primary-focus flex items-center justify-center shadow-xl shadow-primary/20 -rotate-3 group-hover:rotate-0 transition-all duration-500 overflow-hidden">
               <div class="absolute inset-0 bg-[radial-gradient(circle_at_50%_120%,rgba(255,255,255,0.3),transparent)]"></div>
               <Antenna :size="28" class="text-primary-content relative z-10" />
            </div>
            <div>
              <h1 class="text-2xl font-black tracking-tighter leading-none italic">Ani-Go</h1>
              <p class="text-[9px] font-black tracking-[0.3em] opacity-30 uppercase mt-1.5 ml-0.5">Automated Sync {{ CURRENT_VERSION }}</p>
            </div>
          </div>
        </div>

        <!-- Navigation Menu -->
        <div class="flex-1 px-4 overflow-y-auto space-y-1">
          <div class="px-4 py-4">
             <span class="text-[10px] font-black tracking-[0.2em] opacity-20 uppercase">{{ $t('nav.navigation') }}</span>
          </div>
          <ul class="menu p-0 gap-1.5">
            <li v-for="item in nav" :key="item.path">
              <router-link
                :to="item.path"
                class="px-4 py-3.5 rounded-[1.2rem] transition-all duration-300 group flex items-center gap-4 relative overflow-hidden"
                :class="route.path.startsWith(item.path) ? 'bg-primary text-primary-content font-black shadow-xl shadow-lg' : 'hover:bg-base-200 text-base-content/70 hover:text-base-content'"
                @click="closeDrawer"
              >
                <div 
                  class="w-9 h-9 rounded-xl flex items-center justify-center transition-all duration-300"
                  :class="route.path.startsWith(item.path) ? 'bg-white/20' : 'bg-base-200 group-hover:bg-base-300'"
                >
                  <component :is="item.icon" :size="20" />
                </div>
                <span class="tracking-tight">{{ item.label }}</span>
                
                <!-- Active Indicator -->
                <div v-if="route.path.startsWith(item.path)" class="absolute right-0 top-1/2 -translate-y-1/2 w-1.5 h-8 bg-white/40 rounded-l-full"></div>
              </router-link>
            </li>
          </ul>
        </div>

        <!-- User Profile (Bottom) -->
        <div class="p-4 m-6 rounded-[2rem] bg-base-200/40 border border-base-200/50 mt-auto relative overflow-hidden group">
          <!-- Background decoration -->
          <div class="absolute -right-4 -bottom-4 w-20 h-20 bg-primary/5 rounded-full blur-2xl group-hover:scale-150 transition-transform duration-700"></div>
          
          <div class="flex items-center gap-4 mb-5 relative z-10">
            <div class="avatar placeholder">
              <div v-if="userAvatar" class="w-11 h-11 rounded-full bg-cover bg-center border border-primary/20 shadow-inner" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
              <div v-else class="bg-primary/10 text-primary rounded-full w-11 border border-primary/20 shadow-inner">
                <span class="text-sm font-black">{{ username?.slice(0, 1).toUpperCase() || 'A' }}</span>
              </div>
            </div>
            <div class="overflow-hidden">
              <p class="text-sm font-black truncate">{{ username }}</p>
              <div class="flex items-center gap-1.5 mt-0.5">
                <div class="w-1.5 h-1.5 rounded-full bg-success animate-pulse"></div>
                <p class="text-[9px] font-bold opacity-40 uppercase tracking-widest">{{ $t('nav.activeNow') }}</p>
              </div>
            </div>
          </div>
          <div class="flex flex-col gap-2 relative z-10">
            <button @click="toggleLanguage" class="btn btn-ghost btn-sm w-full justify-start gap-3 rounded-xl transition-all duration-300">
              <Languages :size="16" />
              <span class="text-[10px] font-black uppercase tracking-widest">{{ locale === 'zh' ? 'English' : '中文' }}</span>
            </button>
            <button @click="logout" class="btn btn-ghost btn-sm w-full justify-start gap-3 hover:bg-error hover:text-error-content rounded-xl transition-all duration-300 group/btn">
              <LogOut :size="16" class="group-hover/btn:-translate-x-1 transition-transform" />
              <span class="text-[10px] font-black uppercase tracking-widest">{{ $t('nav.terminate') }}</span>
            </button>
          </div>
        </div>
      </aside>
    </div>

    <!-- Update Notification Toast -->
    <div v-if="hasNewVersion" class="fixed bottom-6 right-6 z-[100] animate-in fade-in slide-in-from-bottom-10 duration-500">
      <div class="bg-base-100 border border-primary/20 shadow-2xl rounded-[1.5rem] p-4 flex items-center gap-4 max-w-sm">
        <div class="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary shrink-0">
          <Sparkles :size="20" />
        </div>
        <div class="flex-1">
          <h4 class="text-xs font-black uppercase tracking-widest opacity-40">新版本可用</h4>
          <p class="text-sm font-bold">{{ latestVersion }} 已发布</p>
        </div>
        <div class="flex items-center gap-2">
          <a href="https://github.com/xiaoyueRX/Ani-Go/releases" target="_blank" class="btn btn-primary btn-sm rounded-xl">
            <ExternalLink :size="14" />
          </a>
          <button @click="hasNewVersion = false" class="btn btn-ghost btn-sm btn-circle rounded-xl">
            <X :size="16" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-enter-active,
.page-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.page-enter-from {
  opacity: 0;
  transform: translateY(10px) scale(0.99);
}

.page-leave-to {
  opacity: 0;
  transform: translateY(-10px) scale(0.99);
}

/* Custom scrollbar */
aside::-webkit-scrollbar {
  width: 4px;
}
aside::-webkit-scrollbar-track {
  background: transparent;
}
aside::-webkit-scrollbar-thumb {
  background: hsl(var(--bc) / 0.1);
  border-radius: 10px;
}

.router-link-active {
  transform: translateX(4px);
}
</style>
