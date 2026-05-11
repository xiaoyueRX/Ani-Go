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
const isDrawerOpen = ref(false)

const { latestVersion, hasNewVersion, checkGitHubUpdate } = useVersion()

function toggleLanguage() {
  locale.value = locale.value === 'zh' ? 'en' : 'zh'
  localStorage.setItem('lang', locale.value)
}

onMounted(async () => {
  try {
    const { data } = await request.get('/me')
    username.value = data.username
  } catch { /* 401 handled by interceptor */ }

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
        <div class="navbar w-full max-w-7xl">
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
                <div class="w-10 rounded-full bg-primary/10 flex items-center justify-center">
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
      <main class="flex-1 p-4 sm:p-6 md:p-8 max-w-7xl mx-auto w-full">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>

      <!-- Footer -->
      <footer class="p-6 text-center text-[10px] font-bold tracking-[0.2em] uppercase opacity-20 mt-auto">
        {{ $t('footer.copy') }}
      </footer>
    </div>

    <!-- Sidebar -->
    <div class="drawer-side z-40">
      <label for="drawer-toggle" class="drawer-overlay" aria-label="close sidebar"></label>
      <aside class="w-72 min-h-screen bg-base-100 border-r border-base-200/50 flex flex-col shadow-2xl lg:shadow-none">
        <!-- Logo Section -->
        <div class="px-8 py-10">
          <div class="flex items-center gap-4 group">
            <div class="w-12 h-12 rounded-2xl bg-primary flex items-center justify-center shadow-lg shadow-lg -rotate-3 group-hover:rotate-0 transition-all duration-500">
               <Antenna :size="28" class="text-primary-content" />
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
              <div class="bg-primary/10 text-primary rounded-full w-11 border border-primary/20 shadow-inner">
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
