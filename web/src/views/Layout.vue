<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Menu, User, LogOut, Antenna, Languages, ExternalLink,
  Calendar, LayoutGrid, Search, Download, Settings, Sparkles, X, TriangleAlert
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
  <div class="drawer lg:drawer-open min-h-screen bg-base-300/30 relative overflow-x-hidden">
    <!-- Ambient Background Lights (Subtle Futuristic Anime Glow) -->
    <div class="fixed inset-0 pointer-events-none overflow-hidden z-0">
      <div class="absolute -top-32 left-1/4 w-[500px] h-[500px] bg-primary/[0.04] rounded-full blur-[140px]"></div>
      <div class="absolute top-1/2 -right-32 w-[600px] h-[600px] bg-secondary/[0.03] rounded-full blur-[160px]"></div>
      <div class="absolute -bottom-32 left-1/3 w-[500px] h-[500px] bg-primary/[0.03] rounded-full blur-[140px]"></div>
    </div>

    <input id="drawer-toggle" type="checkbox" class="drawer-toggle" v-model="isDrawerOpen" />

    <div class="drawer-content flex flex-col relative z-10">
      <!-- Top navbar (Mobile & Tablet) -->
      <div class="sticky top-0 z-30 flex h-16 w-full justify-center bg-base-100/80 backdrop-blur-2xl transition-all duration-200 lg:hidden border-b border-base-200/80 shadow-sm">
        <div class="navbar w-full max-w-[1400px] lg:max-w-[1600px] xl:max-w-[2000px] 2xl:max-w-[2400px] mx-auto px-3 sm:px-4">
          <div class="flex-none">
            <label for="drawer-toggle" class="btn btn-square btn-ghost rounded-2xl hover:bg-base-200/60 transition-colors">
              <Menu :size="22" />
            </label>
          </div>
          <div class="flex-1 px-2 flex items-center gap-2.5 cursor-pointer" @click="router.push('/schedule')">
            <div class="relative">
              <img src="/logo.png" alt="Ani-Go" class="w-8 h-8 rounded-xl object-cover shadow-sm shadow-primary/20" />
              <div class="absolute -inset-0.5 rounded-xl bg-primary/20 blur-sm -z-10"></div>
            </div>
            <span class="text-xl font-black bg-gradient-to-r from-primary via-secondary to-accent bg-clip-text text-transparent italic tracking-tight">Ani-Go</span>
            <span class="badge badge-neutral badge-xs font-mono font-bold opacity-60">v{{ CURRENT_VERSION }}</span>
          </div>
          <div class="flex-none gap-2">
            <div class="dropdown dropdown-end">
              <label tabindex="0" class="btn btn-ghost btn-circle avatar online transition-transform active:scale-95">
                <div v-if="userAvatar" class="w-9 h-9 rounded-xl ring-2 ring-primary/20 bg-cover bg-center shadow-inner" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
                <div v-else class="w-9 h-9 rounded-xl ring-2 ring-primary/20 bg-primary/10 flex items-center justify-center text-primary shadow-inner">
                  <User :size="18" />
                </div>
              </label>
              <ul tabindex="0" class="mt-3 z-[1] p-2 shadow-2xl menu menu-sm dropdown-content bg-base-100/95 backdrop-blur-xl rounded-2xl w-56 border border-base-200/80">
                <li class="menu-title opacity-50 px-4 py-2 text-xs uppercase font-black tracking-widest">{{ username }}</li>
                <li>
                  <a @click="toggleLanguage" class="flex items-center gap-2.5 py-2 rounded-xl font-bold">
                    <Languages :size="16" class="text-primary" />
                    <span>{{ locale === 'zh' ? 'English' : '中文' }}</span>
                  </a>
                </li>
                <div class="divider my-1 opacity-20"></div>
                <li>
                  <a @click="logout" class="text-error flex items-center gap-2.5 py-2 rounded-xl font-bold hover:bg-error/10">
                    <LogOut :size="16" />
                    <span>{{ $t('nav.logout') }}</span>
                  </a>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Main content area -->
      <main class="flex-1 p-3 sm:p-5 md:p-8 max-w-[1400px] lg:max-w-[1600px] xl:max-w-[2000px] 2xl:max-w-[2400px] mx-auto w-full pb-28 lg:pb-10 overflow-x-hidden">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>

      <!-- Footer -->
      <footer class="py-8 px-4 text-center text-xs space-y-2 mt-auto border-t border-base-content/5 opacity-70 hover:opacity-100 transition-opacity pb-28 lg:pb-8">
        <div class="font-semibold text-base-content/85 flex items-center justify-center gap-2 flex-wrap">
          <span class="tracking-wide">Ani-Go &copy; 2026 • 倾心打造</span>
          <span class="opacity-30">•</span>
          <span>by <a href="https://github.com/xiaoyueRX" target="_blank" rel="noopener noreferrer" class="text-primary font-bold hover:underline">xiaoyue</a></span>
        </div>
        <div class="text-[11px] opacity-70 flex items-center justify-center gap-2.5 flex-wrap">
          <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" 
             class="inline-flex items-center gap-1.5 hover:text-primary transition-colors font-mono font-medium bg-base-200/60 px-3 py-1 rounded-full border border-base-300/40 hover:border-primary/40">
            <svg class="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
            <span>GitHub: xiaoyueRX/Ani-Go</span>
          </a>
          <span class="opacity-30">•</span>
          <span class="font-mono badge badge-sm badge-ghost font-bold">v{{ CURRENT_VERSION }}</span>
        </div>
      </footer>

      <!-- Modern Mobile Floating Dock Navigation (Phones & Tablets) -->
      <div class="fixed bottom-3 left-3 right-3 z-40 lg:hidden pointer-events-none">
        <nav class="max-w-md mx-auto bg-base-100/90 backdrop-blur-2xl border border-white/10 rounded-[2rem] shadow-[0_12px_40px_rgba(0,0,0,0.35)] px-2.5 py-1.5 flex items-center justify-around pointer-events-auto pb-[env(safe-area-inset-bottom,4px)]">
          <router-link
            v-for="item in nav"
            :key="item.path"
            :to="item.path"
            class="flex flex-col items-center justify-center py-1.5 px-3 rounded-2xl transition-all duration-300 min-w-[54px] relative"
            :class="route.path.startsWith(item.path) ? 'text-primary scale-105 font-black' : 'text-base-content/40 hover:text-base-content/80 font-semibold'"
          >
            <div class="relative flex items-center justify-center">
              <component 
                :is="item.icon" 
                :size="20" 
                class="transition-transform duration-300"
                :class="{ 'scale-110 text-primary drop-shadow-[0_0_8px_rgba(var(--p),0.5)]': route.path.startsWith(item.path) }" 
              />
            </div>
            <span class="text-[10px] tracking-tight mt-0.5 transition-all whitespace-nowrap">
              {{ item.label }}
            </span>
            <div v-if="route.path.startsWith(item.path)" class="w-1.5 h-1.5 rounded-full bg-primary mt-0.5 shadow-sm shadow-primary"></div>
          </router-link>
        </nav>
      </div>

      <!-- Global Toasts -->
      <div class="toast toast-end toast-bottom p-4 z-[100] mb-20 lg:mb-4">
        <transition-group name="page">
          <div 
            v-for="t in toasts" :key="t.id"
            class="alert shadow-2xl border-0 mb-2 rounded-2xl min-w-[280px] animate-in slide-in-from-right duration-300 backdrop-blur-xl"
            :class="{
              'bg-primary text-primary-content shadow-primary/20': t.type === 'success',
              'bg-error text-error-content shadow-error/20': t.type === 'error',
              'bg-info text-info-content shadow-info/20': t.type === 'info'
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

    <!-- Sidebar (Desktop & Expanded Drawer) -->
    <div class="drawer-side z-40">
      <label for="drawer-toggle" class="drawer-overlay" aria-label="close sidebar"></label>
      <aside class="w-72 min-h-screen bg-base-100/95 backdrop-blur-2xl border-r border-base-200/80 flex flex-col shadow-2xl lg:shadow-none">
        <!-- Logo Brand Section -->
        <div class="px-7 py-9">
          <div class="flex items-center gap-3.5 group cursor-pointer" @click="router.push('/schedule')">
            <div class="relative">
              <img src="/logo.png" alt="Ani-Go Logo" class="w-12 h-12 rounded-2xl shadow-xl shadow-primary/25 -rotate-2 group-hover:rotate-0 transition-all duration-500 object-cover" />
              <div class="absolute -inset-1 rounded-2xl bg-gradient-to-tr from-primary to-secondary opacity-30 blur-md group-hover:opacity-60 transition-opacity -z-10"></div>
            </div>
            <div>
              <div class="flex items-center gap-2">
                <h1 class="text-2xl font-black tracking-tight leading-none italic bg-gradient-to-r from-base-content via-base-content to-primary bg-clip-text text-transparent">Ani-Go</h1>
                <span class="badge badge-primary badge-xs font-mono font-bold">v{{ CURRENT_VERSION }}</span>
              </div>
              <p class="text-[9px] font-black tracking-[0.25em] opacity-35 uppercase mt-1.5 ml-0.5">Automated Anime Sync</p>
            </div>
          </div>
        </div>

        <!-- Navigation Menu -->
        <div class="flex-1 px-4 overflow-y-auto space-y-1">
          <div class="px-4 py-2">
             <span class="text-[10px] font-black tracking-[0.2em] opacity-30 uppercase">{{ $t('nav.navigation') }}</span>
          </div>
          <ul class="menu p-0 gap-2">
            <li v-for="item in nav" :key="item.path">
              <router-link
                :to="item.path"
                class="px-4 py-3.5 rounded-2xl transition-all duration-300 group flex items-center gap-4 relative overflow-hidden"
                :class="route.path.startsWith(item.path) ? 'bg-primary text-primary-content font-black shadow-xl shadow-primary/25 scale-[1.02]' : 'hover:bg-base-200/70 text-base-content/70 hover:text-base-content hover:translate-x-1'"
                @click="closeDrawer"
              >
                <div 
                  class="w-9 h-9 rounded-xl flex items-center justify-center transition-all duration-300 shadow-sm"
                  :class="route.path.startsWith(item.path) ? 'bg-white/20 text-white' : 'bg-base-200 group-hover:bg-base-300/80 text-base-content/60 group-hover:text-primary'"
                >
                  <component :is="item.icon" :size="20" />
                </div>
                <span class="tracking-tight text-sm">{{ item.label }}</span>
                
                <!-- Active Indicator Pill -->
                <div v-if="route.path.startsWith(item.path)" class="absolute right-0 top-1/2 -translate-y-1/2 w-1.5 h-8 bg-white/60 rounded-l-full"></div>
              </router-link>
            </li>
          </ul>
        </div>

        <!-- User Profile Card (Bottom) -->
        <div class="p-4 m-5 rounded-3xl bg-base-200/50 border border-base-200/80 mt-auto relative overflow-hidden group backdrop-blur-md shadow-sm">
          <!-- Background glow decoration -->
          <div class="absolute -right-6 -bottom-6 w-24 h-24 bg-primary/10 rounded-full blur-2xl group-hover:scale-150 transition-transform duration-700 pointer-events-none"></div>
          
          <div class="flex items-center gap-3.5 mb-4 relative z-10">
            <div class="avatar online">
              <div v-if="userAvatar" class="w-11 h-11 rounded-2xl ring-2 ring-primary/20 bg-cover bg-center shadow-inner" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
              <div v-else class="bg-primary/10 text-primary rounded-2xl w-11 h-11 ring-2 ring-primary/20 flex items-center justify-center shadow-inner">
                <span class="text-sm font-black">{{ username?.slice(0, 1).toUpperCase() || 'A' }}</span>
              </div>
            </div>
            <div class="overflow-hidden flex-1">
              <p class="text-sm font-black truncate">{{ username }}</p>
              <div class="flex items-center gap-1.5 mt-0.5">
                <div class="w-1.5 h-1.5 rounded-full bg-success animate-pulse"></div>
                <p class="text-[9px] font-bold opacity-45 uppercase tracking-widest">{{ $t('nav.activeNow') }}</p>
              </div>
            </div>
          </div>
          <div class="flex flex-col gap-1.5 relative z-10">
            <button @click="toggleLanguage" class="btn btn-ghost btn-sm w-full justify-start gap-3 rounded-xl hover:bg-base-100 transition-all font-bold">
              <Languages :size="16" class="text-primary" />
              <span class="text-[11px] font-black uppercase tracking-wider">{{ locale === 'zh' ? 'English' : '中文' }}</span>
            </button>
            <button @click="logout" class="btn btn-ghost btn-sm w-full justify-start gap-3 hover:bg-error/15 hover:text-error rounded-xl transition-all group/btn font-bold">
              <LogOut :size="16" class="group-hover/btn:-translate-x-1 transition-transform" />
              <span class="text-[11px] font-black uppercase tracking-wider">{{ $t('nav.terminate') }}</span>
            </button>
          </div>
        </div>
      </aside>
    </div>

    <!-- Update Notification Toast -->
    <div v-if="hasNewVersion" class="fixed bottom-6 right-6 z-[100] animate-in fade-in slide-in-from-bottom-10 duration-500">
      <div class="bg-base-100/95 backdrop-blur-2xl border border-primary/20 shadow-2xl rounded-3xl p-4 flex items-center gap-4 max-w-sm">
        <div class="w-10 h-10 rounded-2xl bg-primary/10 flex items-center justify-center text-primary shrink-0 shadow-inner">
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
