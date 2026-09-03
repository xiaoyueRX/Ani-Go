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
  <div class="min-h-screen bg-base-300/30 flex text-base-content antialiased">
    <!-- Desktop Sticky Sidebar (100% 视口固定，时间表再长也绝对不随页面滚动) -->
    <aside class="hidden lg:flex w-64 h-screen sticky top-0 flex-col bg-base-100 border-r border-base-200/80 shrink-0 z-30 select-none">
      <!-- Logo Brand Section -->
      <div class="px-6 py-6 border-b border-base-200/60">
        <div class="flex items-center gap-3 group cursor-pointer" @click="router.push('/schedule')">
          <div class="relative">
            <img src="/logo.png" alt="Ani-Go" class="w-10 h-10 rounded-xl object-cover shadow-md shadow-primary/20 group-hover:scale-105 transition-transform" />
            <div class="absolute -inset-0.5 rounded-xl bg-primary/20 blur-sm -z-10 group-hover:opacity-100 opacity-50 transition-opacity"></div>
          </div>
          <div class="flex flex-col">
            <div class="flex items-center gap-1.5">
              <span class="text-xl font-black bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent italic tracking-tight">Ani-Go</span>
              <span class="badge badge-primary badge-xs font-mono font-bold">v{{ CURRENT_VERSION }}</span>
            </div>
            <span class="text-[9px] font-bold text-base-content/40 uppercase tracking-[0.2em]">Anime Sync Engine</span>
          </div>
        </div>
      </div>

      <!-- Navigation Links (自适应高度内部滚动，保证小屏笔记本也全功能可用) -->
      <div class="flex-1 px-3 py-4 overflow-y-auto space-y-1">
        <div class="px-3 py-1.5 text-[10px] font-black tracking-widest text-base-content/30 uppercase">
          {{ $t('nav.navigation') }}
        </div>
        <router-link
          v-for="item in nav"
          :key="item.path"
          :to="item.path"
          class="flex items-center gap-3.5 px-3.5 py-3 rounded-xl transition-all duration-200 group font-bold text-sm relative"
          :class="route.path.startsWith(item.path) 
            ? 'bg-primary text-primary-content font-black shadow-md shadow-primary/25' 
            : 'text-base-content/70 hover:text-base-content hover:bg-base-200/70 hover:translate-x-0.5'"
        >
          <component :is="item.icon" :size="19" class="shrink-0 transition-transform group-hover:scale-110" />
          <span class="tracking-tight">{{ item.label }}</span>
          <div v-if="route.path.startsWith(item.path)" class="absolute right-0 top-1/2 -translate-y-1/2 w-1.5 h-6 bg-white/70 rounded-l-full"></div>
        </router-link>
      </div>

      <!-- User Profile Card (永远固定锁定在侧边栏底部) -->
      <div class="p-3.5 m-3 rounded-2xl bg-base-200/60 border border-base-200/80 shadow-sm">
        <div class="flex items-center gap-3 mb-3">
          <div class="avatar online shrink-0">
            <div v-if="userAvatar" class="w-10 h-10 rounded-xl bg-cover bg-center ring-1 ring-base-content/10 shadow-sm" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
            <div v-else class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center font-black ring-1 ring-base-content/10 shadow-sm">
              {{ username?.slice(0, 1).toUpperCase() || 'A' }}
            </div>
          </div>
          <div class="overflow-hidden min-w-0 flex-1">
            <div class="text-sm font-black truncate">{{ username }}</div>
            <div class="flex items-center gap-1.5 mt-0.5">
              <span class="w-1.5 h-1.5 rounded-full bg-success animate-pulse"></span>
              <span class="text-[9px] font-bold text-base-content/40 uppercase tracking-wider">{{ $t('nav.activeNow') }}</span>
            </div>
          </div>
        </div>
        
        <div class="grid grid-cols-2 gap-1.5 pt-2 border-t border-base-200/80">
          <button @click="toggleLanguage" class="btn btn-ghost btn-xs rounded-lg font-bold gap-1.5 justify-center hover:bg-base-100">
            <Languages :size="13" class="text-primary" />
            <span class="text-[10px]">{{ locale === 'zh' ? 'English' : '中文' }}</span>
          </button>
          <button @click="logout" class="btn btn-ghost btn-xs rounded-lg font-bold gap-1.5 justify-center hover:bg-error/15 hover:text-error">
            <LogOut :size="13" />
            <span class="text-[10px]">{{ $t('nav.logout') }}</span>
          </button>
        </div>
      </div>
    </aside>

    <!-- Main Content Container (Right Side) -->
    <div class="flex-1 flex flex-col min-w-0 min-h-screen">
      <!-- Mobile / Tablet Top Header -->
      <header class="sticky top-0 z-30 flex h-16 w-full items-center justify-between px-4 bg-base-100/90 backdrop-blur-xl border-b border-base-200/80 lg:hidden shadow-sm">
        <button class="btn btn-square btn-ghost btn-sm rounded-xl" @click="isDrawerOpen = true">
          <Menu :size="20" />
        </button>
        <div class="flex items-center gap-2 cursor-pointer" @click="router.push('/schedule')">
          <img src="/logo.png" alt="Ani-Go" class="w-7 h-7 rounded-lg object-cover shadow-sm" />
          <span class="text-lg font-black bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent italic">Ani-Go</span>
          <span class="badge badge-neutral badge-xs font-mono font-bold opacity-60">v{{ CURRENT_VERSION }}</span>
        </div>
        <div class="dropdown dropdown-end">
          <label tabindex="0" class="btn btn-ghost btn-circle btn-sm avatar online">
            <div v-if="userAvatar" class="w-8 h-8 rounded-lg bg-cover bg-center" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
            <div v-else class="w-8 h-8 rounded-lg bg-primary/10 text-primary flex items-center justify-center font-black text-xs">
              {{ username?.slice(0, 1).toUpperCase() || 'A' }}
            </div>
          </label>
          <ul tabindex="0" class="mt-3 z-50 p-2 shadow-2xl menu menu-sm dropdown-content bg-base-100 rounded-2xl w-48 border border-base-200">
            <li class="menu-title text-xs font-black uppercase tracking-wider text-base-content/40 px-3 py-1.5">{{ username }}</li>
            <li>
              <a @click="toggleLanguage" class="flex items-center gap-2 font-bold py-2">
                <Languages :size="15" class="text-primary" />
                <span>{{ locale === 'zh' ? 'English' : '中文' }}</span>
              </a>
            </li>
            <div class="divider my-1 opacity-20"></div>
            <li>
              <a @click="logout" class="text-error flex items-center gap-2 font-bold py-2 hover:bg-error/10">
                <LogOut :size="15" />
                <span>{{ $t('nav.logout') }}</span>
              </a>
            </li>
          </ul>
        </div>
      </header>

      <!-- Main Page Router View -->
      <main class="flex-1 p-3 sm:p-5 md:p-8 max-w-[2000px] mx-auto w-full pb-24 lg:pb-8 overflow-x-hidden">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>

      <!-- Footer -->
      <footer class="py-6 px-4 text-center text-xs space-y-1.5 mt-auto border-t border-base-content/5 opacity-60 hover:opacity-100 transition-opacity pb-24 lg:pb-6">
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

      <!-- Mobile Edge-to-Edge Bottom Navigation Bar (标准贴底，绝不与页面浮动栏碰撞) -->
      <nav class="fixed bottom-0 left-0 right-0 z-40 bg-base-100/95 backdrop-blur-xl border-t border-base-200/80 px-2 py-1 flex items-center justify-around lg:hidden shadow-lg pb-[env(safe-area-inset-bottom,6px)]">
        <router-link
          v-for="item in nav"
          :key="item.path"
          :to="item.path"
          class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition-colors min-w-[56px]"
          :class="route.path.startsWith(item.path) ? 'text-primary font-black' : 'text-base-content/50 hover:text-base-content font-medium'"
        >
          <component :is="item.icon" :size="20" class="transition-transform" :class="{ 'scale-110 text-primary': route.path.startsWith(item.path) }" />
          <span class="text-[10px] tracking-tight mt-0.5 whitespace-nowrap">{{ item.label }}</span>
        </router-link>
      </nav>

      <!-- Global Toasts -->
      <div class="toast toast-end toast-bottom p-4 z-[100] mb-16 lg:mb-2">
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

    <!-- Mobile Slide-over Drawer Backdrop & Panel -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="isDrawerOpen" class="fixed inset-0 z-50 lg:hidden flex">
          <!-- Backdrop -->
          <div class="fixed inset-0 bg-black/60 backdrop-blur-sm" @click="closeDrawer"></div>
          
          <!-- Drawer Content -->
          <div class="relative w-72 max-w-[80vw] h-full bg-base-100 shadow-2xl flex flex-col z-10 animate-in slide-in-from-left duration-200">
            <div class="px-6 py-6 border-b border-base-200/60 flex items-center justify-between">
              <div class="flex items-center gap-3">
                <img src="/logo.png" alt="Ani-Go" class="w-9 h-9 rounded-xl object-cover shadow-sm" />
                <span class="text-xl font-black bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent italic">Ani-Go</span>
              </div>
              <button class="btn btn-ghost btn-circle btn-sm" @click="closeDrawer">
                <X :size="18" />
              </button>
            </div>

            <div class="flex-1 px-3 py-4 overflow-y-auto space-y-1">
              <router-link
                v-for="item in nav"
                :key="item.path"
                :to="item.path"
                class="flex items-center gap-3.5 px-4 py-3 rounded-xl transition-all font-bold text-sm"
                :class="route.path.startsWith(item.path) ? 'bg-primary text-primary-content font-black shadow-md' : 'text-base-content/70 hover:bg-base-200'"
                @click="closeDrawer"
              >
                <component :is="item.icon" :size="20" />
                <span>{{ item.label }}</span>
              </router-link>
            </div>

            <!-- Drawer Bottom User Profile -->
            <div class="p-4 m-3 rounded-2xl bg-base-200/60 border border-base-200/80">
              <div class="flex items-center gap-3 mb-3">
                <div class="avatar online">
                  <div v-if="userAvatar" class="w-9 h-9 rounded-lg bg-cover bg-center" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
                  <div v-else class="w-9 h-9 rounded-lg bg-primary/10 text-primary flex items-center justify-center font-black text-xs">
                    {{ username?.slice(0, 1).toUpperCase() || 'A' }}
                  </div>
                </div>
                <div class="overflow-hidden min-w-0">
                  <div class="text-sm font-black truncate">{{ username }}</div>
                  <div class="text-[9px] font-bold text-success">{{ $t('nav.activeNow') }}</div>
                </div>
              </div>
              <div class="flex items-center gap-2 pt-2 border-t border-base-200/80">
                <button @click="toggleLanguage" class="btn btn-ghost btn-xs rounded-lg font-bold flex-1 justify-center">
                  <Languages :size="14" class="text-primary" />
                  <span class="text-[10px]">{{ locale === 'zh' ? 'English' : '中文' }}</span>
                </button>
                <button @click="logout" class="btn btn-ghost btn-xs rounded-lg font-bold flex-1 justify-center text-error hover:bg-error/10">
                  <LogOut :size="14" />
                  <span class="text-[10px]">{{ $t('nav.logout') }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Update Notification Toast -->
    <div v-if="hasNewVersion" class="fixed bottom-6 right-6 z-[100] animate-in fade-in slide-in-from-bottom-10 duration-500">
      <div class="bg-base-100 border border-primary/20 shadow-2xl rounded-2xl p-4 flex items-center gap-4 max-w-sm">
        <div class="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary shrink-0">
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
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.page-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.page-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* Custom scrollbar for sidebar */
aside > div::-webkit-scrollbar {
  width: 4px;
}
aside > div::-webkit-scrollbar-track {
  background: transparent;
}
aside > div::-webkit-scrollbar-thumb {
  background: hsl(var(--bc) / 0.1);
  border-radius: 10px;
}
</style>
