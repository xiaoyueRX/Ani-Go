<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import {
  Menu, LogOut, Antenna, Languages, ExternalLink,
  Calendar, LayoutGrid, Search, Download, Settings, Sparkles, X, TriangleAlert, Sun, Moon
} from 'lucide-vue-next'
import { loadCurrentVersion, useVersion } from '../composables/useVersion'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const username = ref('')
const userAvatar = ref('')
const isDrawerOpen = ref(false)
const isDefaultPassword = ref(false)
const dismissDefaultPasswordAlert = ref(false)

// version.test.mjs 在仅注入有限模块的 VM 中运行 Layout，主题读写在此内联以避免额外相对导入
type ThemeName = 'anigo-dark' | 'anigo-light'
const currentTheme = ref<ThemeName>(
  typeof localStorage !== 'undefined' && localStorage.getItem('ani-go-theme') === 'anigo-light'
    ? 'anigo-light'
    : 'anigo-dark'
)

function applyTheme(next: ThemeName) {
  currentTheme.value = next
  if (typeof document !== 'undefined') {
    document.documentElement?.setAttribute('data-theme', next)
    const meta = document.querySelector?.('meta[name="theme-color"]')
    if (meta) {
      meta.setAttribute('content', next === 'anigo-light' ? '#f6f7f9' : '#0b0d12')
    }
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('ani-go-theme', next)
  }
}

function toggleTheme() {
  applyTheme(currentTheme.value === 'anigo-dark' ? 'anigo-light' : 'anigo-dark')
}

const { currentVersion, latestVersion, hasNewVersion, checkGitHubUpdate, updating, updateProgress, triggerUpdate, canSelfUpdate } = useVersion()

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
    toasts.value = toasts.value.filter(item => item.id !== id)
  }, 3000)
}

// 暴露全局 Toast 供子组件调用
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

function updateAvatar(event: Event) {
  const url = (event as CustomEvent).detail
  if (typeof url === 'string') userAvatar.value = url
}

onUnmounted(() => window.removeEventListener('avatar-updated', updateAvatar))
onMounted(async () => {
  window.addEventListener('avatar-updated', updateAvatar)
  if (typeof document !== 'undefined') {
    const domTheme = document.documentElement?.getAttribute('data-theme')
    if (domTheme === 'anigo-light' || domTheme === 'anigo-dark') {
      currentTheme.value = domTheme
    } else {
      document.documentElement?.setAttribute('data-theme', currentTheme.value)
    }
  }
  void loadCurrentVersion()
  try {
    const { data } = await request.get('/me')
    username.value = data.username
    userAvatar.value = data.avatar_url || ''
    isDefaultPassword.value = !!data.is_default_password
  } catch { /* 401 handled by interceptor */ }

  // 若后台配置开启了自动检查更新，则在启动后静默探测
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

const themeLabel = computed(() => (
  currentTheme.value === 'anigo-dark' ? t('nav.themeLight') : t('nav.themeDark')
))

function closeDrawer() {
  isDrawerOpen.value = false
}
</script>

<template>
  <div class="min-h-screen bg-base-300 flex text-base-content antialiased">
    <!-- 桌面端固定侧边栏 -->
    <aside class="hidden lg:flex w-60 h-screen sticky top-0 flex-col bg-base-100 border-r border-base-content/8 shrink-0 z-30 select-none">
      <!-- 品牌标识区 -->
      <div class="h-14 px-4 border-b border-base-content/8 flex items-center justify-between gap-2 shrink-0">
        <div class="flex items-center gap-2.5 min-w-0 cursor-pointer" @click="router.push('/schedule')">
          <img src="/logo.png" alt="Ani-Go" class="w-7 h-7 rounded-lg object-cover shrink-0 border border-base-content/10" />
          <span class="text-base font-bold tracking-tight text-base-content truncate">Ani-Go</span>
        </div>
        <span v-if="currentVersion" :title="currentVersion" class="min-w-0 max-w-[96px]">
          <span class="badge badge-neutral badge-xs max-w-full font-mono text-[11px]">
            <span class="truncate">{{ currentVersion }}</span>
          </span>
        </span>
      </div>

      <!-- 主导航菜单 -->
      <div class="flex-1 px-2.5 py-3 overflow-y-auto space-y-0.5">
        <div class="px-3 pb-1.5 text-[11px] font-semibold text-base-content/45">
          {{ $t('nav.navigation') }}
        </div>
        <router-link
          v-for="item in nav"
          :key="item.path"
          :to="item.path"
          class="flex items-center gap-3 px-3 py-2 rounded-lg transition-colors text-[13.5px] relative"
          :class="route.path.startsWith(item.path)
            ? 'bg-primary/12 text-primary font-semibold'
            : 'text-base-content/70 hover:text-base-content hover:bg-base-200/80 font-medium'"
        >
          <span
            v-if="route.path.startsWith(item.path)"
            class="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 bg-primary rounded-r"
            aria-hidden="true"
          />
          <component :is="item.icon" :size="17" class="shrink-0" />
          <span class="truncate">{{ item.label }}</span>
        </router-link>
      </div>

      <!-- 侧边栏底部账户与快捷控制 -->
      <div class="p-2.5 m-2.5 rounded-xl bg-base-200/70 border border-base-content/8 space-y-2.5">
        <div class="flex items-center gap-2.5 min-w-0">
          <div class="avatar online shrink-0">
            <div v-if="userAvatar" class="w-8 h-8 rounded-lg bg-cover bg-center border border-base-content/10" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
            <div v-else class="w-8 h-8 rounded-lg bg-primary/15 text-primary flex items-center justify-center font-bold text-xs border border-primary/20">
              {{ username?.slice(0, 1).toUpperCase() || 'A' }}
            </div>
          </div>
          <div class="min-w-0 flex-1">
            <div class="text-xs font-semibold truncate text-base-content">{{ username || 'admin' }}</div>
            <div class="flex items-center gap-1.5 mt-0.5">
              <span class="w-1.5 h-1.5 rounded-full bg-success shrink-0"></span>
              <span class="text-[11px] text-base-content/55 truncate">{{ $t('nav.activeNow') }}</span>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-3 gap-1 pt-2 border-t border-base-content/8">
          <button
            type="button"
            class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md justify-center text-base-content/70 hover:text-base-content hover:bg-base-100"
            :title="themeLabel"
            :aria-label="themeLabel"
            @click="toggleTheme"
          >
            <Sun v-if="currentTheme === 'anigo-dark'" :size="14" />
            <Moon v-else :size="14" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md font-medium gap-1 justify-center text-base-content/70 hover:text-base-content hover:bg-base-100"
            :title="locale === 'zh' ? 'English' : '中文'"
            @click="toggleLanguage"
          >
            <Languages :size="13" class="text-primary" />
            <span class="text-[11px]">{{ locale === 'zh' ? 'EN' : '中' }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md justify-center text-base-content/70 hover:bg-error/12 hover:text-error"
            :title="$t('nav.logout')"
            :aria-label="$t('nav.logout')"
            @click="logout"
          >
            <LogOut :size="14" />
          </button>
        </div>
      </div>
    </aside>

    <!-- 右侧主内容区域 -->
    <div class="flex-1 flex flex-col min-w-0 min-h-screen">
      <!-- 移动端顶栏：第一个按钮必须触发侧滑抽屉，右侧 .avatar 触发账户菜单 -->
      <header class="sticky top-0 z-30 flex h-14 w-full items-center justify-between px-3 bg-base-100/95 backdrop-blur-md border-b border-base-content/8 lg:hidden">
        <button type="button" class="btn btn-square btn-ghost btn-sm rounded-lg" @click="isDrawerOpen = true" aria-label="Menu">
          <Menu :size="19" />
        </button>

        <div class="flex min-w-0 items-center gap-2 cursor-pointer px-2" @click="router.push('/schedule')">
          <img src="/logo.png" alt="Ani-Go" class="w-6 h-6 shrink-0 rounded-md object-cover border border-base-content/10" />
          <span class="shrink-0 text-sm font-bold tracking-tight text-base-content">Ani-Go</span>
          <span v-if="currentVersion" :title="currentVersion" class="min-w-0 max-w-[120px]">
            <span class="badge badge-neutral badge-xs max-w-full font-mono text-[10px]">
              <span class="truncate">{{ currentVersion }}</span>
            </span>
          </span>
        </div>

        <div class="dropdown dropdown-end shrink-0">
          <label tabindex="0" class="btn btn-ghost btn-circle btn-sm avatar online">
            <div v-if="userAvatar" class="w-7 h-7 rounded-lg bg-cover bg-center" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
            <div v-else class="w-7 h-7 rounded-lg bg-primary/15 text-primary flex items-center justify-center font-bold text-xs">
              {{ username?.slice(0, 1).toUpperCase() || 'A' }}
            </div>
          </label>
          <ul tabindex="0" class="mt-2 z-50 p-1.5 shadow-xl menu menu-sm dropdown-content bg-base-100 rounded-xl w-48 border border-base-content/10">
            <li class="menu-title text-xs font-semibold text-base-content/60 px-3 py-1.5">{{ username }}</li>
            <li>
              <a @click="toggleTheme" class="flex items-center gap-2 font-medium py-2">
                <Sun v-if="currentTheme === 'anigo-dark'" :size="14" class="text-primary" />
                <Moon v-else :size="14" class="text-primary" />
                <span>{{ themeLabel }}</span>
              </a>
            </li>
            <li>
              <a @click="toggleLanguage" class="flex items-center gap-2 font-medium py-2">
                <Languages :size="14" class="text-primary" />
                <span>{{ locale === 'zh' ? 'English' : '中文' }}</span>
              </a>
            </li>
            <div class="divider my-1 opacity-15"></div>
            <li>
              <a @click="logout" class="text-error flex items-center gap-2 font-medium py-2 hover:bg-error/10">
                <LogOut :size="14" />
                <span>{{ $t('nav.logout') }}</span>
              </a>
            </li>
          </ul>
        </div>
      </header>

      <!-- 默认密码安全横幅 -->
      <div v-if="isDefaultPassword && !dismissDefaultPasswordAlert" class="bg-warning/12 border-b border-warning/25 px-4 py-2 flex items-center justify-between text-warning text-xs sm:text-sm font-medium">
        <div class="flex items-center gap-2 max-w-[85%]">
          <TriangleAlert :size="16" class="shrink-0 text-warning" />
          <span>{{ locale === 'zh' ? '安全提醒：当前管理员仍在使用默认密码 (admin)，请立即前往设置修改。' : 'Security Warning: You are using the default password (admin). Please change it immediately.' }}</span>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <router-link to="/settings" class="btn btn-warning btn-xs rounded-md font-semibold">{{ locale === 'zh' ? '前往修改' : 'Change' }}</router-link>
          <button type="button" @click="dismissDefaultPasswordAlert = true" class="btn btn-ghost btn-xs btn-circle opacity-70 hover:opacity-100"><X :size="14" /></button>
        </div>
      </div>

      <!-- 主路由视口 -->
      <main class="flex-1 p-3 sm:p-5 lg:p-6 max-w-[1600px] mx-auto w-full pb-24 lg:pb-8 overflow-x-hidden">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" :key="route.name === 'subscription-detail' ? route.params.id as string : undefined" />
          </transition>
        </router-view>
      </main>

      <!-- 页脚（保留完整单行版本号 span.font-mono 供移动端校验） -->
      <footer class="py-5 px-4 text-center text-xs text-base-content/55 mt-auto border-t border-base-content/8 pb-24 lg:pb-6">
        <div class="flex items-center justify-center gap-2 flex-wrap">
          <span>Ani-Go &copy; 2026</span>
          <span class="opacity-40">•</span>
          <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" class="hover:text-primary transition-colors">GitHub</a>
          <template v-if="currentVersion">
            <span class="opacity-40">•</span>
            <span class="max-w-full break-all font-mono text-base-content/75">{{ currentVersion }}</span>
          </template>
        </div>
      </footer>

      <!-- 移动端底部固定导航栏 -->
      <nav class="fixed bottom-0 left-0 right-0 z-40 bg-base-100/95 backdrop-blur-md border-t border-base-content/8 px-1.5 py-1 flex items-center justify-around lg:hidden pb-[env(safe-area-inset-bottom,6px)]">
        <router-link
          v-for="item in nav"
          :key="item.path"
          :to="item.path"
          class="flex flex-col items-center justify-center py-1 px-2 rounded-lg transition-colors min-w-[52px]"
          :class="route.path.startsWith(item.path) ? 'text-primary font-semibold' : 'text-base-content/55 hover:text-base-content font-medium'"
        >
          <component :is="item.icon" :size="18" />
          <span class="text-[11px] mt-0.5 whitespace-nowrap">{{ item.label }}</span>
        </router-link>
      </nav>

      <!-- 全局 Toast 提示 -->
      <div class="toast toast-end toast-bottom p-4 z-[100] mb-16 lg:mb-2">
        <transition-group name="page">
          <div
            v-for="item in toasts"
            :key="item.id"
            class="alert shadow-lg border mb-2 rounded-xl min-w-[260px] max-w-sm py-3 px-4"
            :class="{
              'bg-base-100 text-base-content border-primary/30': item.type === 'success',
              'bg-base-100 text-error border-error/30': item.type === 'error',
              'bg-base-100 text-info border-info/30': item.type === 'info'
            }"
          >
            <div class="flex items-center gap-2.5">
              <Sparkles v-if="item.type === 'success'" :size="16" class="text-primary shrink-0" />
              <TriangleAlert v-else-if="item.type === 'error'" :size="16" class="text-error shrink-0" />
              <Antenna v-else :size="16" class="text-info shrink-0" />
              <span class="text-xs font-semibold">{{ item.message }}</span>
            </div>
          </div>
        </transition-group>
      </div>
    </div>

    <!-- 移动端侧滑导航抽屉 -->
    <Teleport to="body">
      <div v-if="isDrawerOpen" class="fixed inset-0 z-[110] lg:hidden flex">
        <div class="fixed inset-0 bg-black/55 backdrop-blur-xs" @click="closeDrawer"></div>

        <div class="relative w-72 max-w-[82vw] h-full bg-base-100 border-r border-base-content/10 shadow-2xl flex flex-col z-10">
          <div class="h-14 px-4 border-b border-base-content/8 flex items-center justify-between shrink-0">
            <div class="flex items-center gap-2.5">
              <img src="/logo.png" alt="Ani-Go" class="w-7 h-7 rounded-lg object-cover border border-base-content/10" />
              <span class="text-base font-bold tracking-tight text-base-content">Ani-Go</span>
            </div>
            <button type="button" class="btn btn-ghost btn-square btn-sm rounded-lg" @click="closeDrawer" aria-label="Close">
              <X :size="17" />
            </button>
          </div>

          <div class="flex-1 px-2.5 py-3 overflow-y-auto space-y-0.5">
            <router-link
              v-for="item in nav"
              :key="item.path"
              :to="item.path"
              class="flex items-center gap-3 px-3 py-2.5 rounded-lg transition-colors text-sm relative"
              :class="route.path.startsWith(item.path) ? 'bg-primary/12 text-primary font-semibold' : 'text-base-content/75 hover:bg-base-200/80 font-medium'"
              @click="closeDrawer"
            >
              <span
                v-if="route.path.startsWith(item.path)"
                class="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 bg-primary rounded-r"
                aria-hidden="true"
              />
              <component :is="item.icon" :size="18" />
              <span>{{ item.label }}</span>
            </router-link>
          </div>

          <div class="p-3 m-2.5 rounded-xl bg-base-200/70 border border-base-content/8 space-y-2.5">
            <div class="flex items-center gap-2.5 min-w-0">
              <div class="avatar online shrink-0">
                <div v-if="userAvatar" class="w-8 h-8 rounded-lg bg-cover bg-center" :style="{ backgroundImage: `url(${proxyImage(userAvatar)})` }"></div>
                <div v-else class="w-8 h-8 rounded-lg bg-primary/15 text-primary flex items-center justify-center font-bold text-xs">
                  {{ username?.slice(0, 1).toUpperCase() || 'A' }}
                </div>
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-xs font-semibold truncate">{{ username }}</div>
                <div class="text-[11px] text-success">{{ $t('nav.activeNow') }}</div>
              </div>
            </div>
            <div class="grid grid-cols-3 gap-1 pt-2 border-t border-base-content/8">
              <button type="button" @click="toggleTheme" class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md justify-center" :aria-label="themeLabel">
                <Sun v-if="currentTheme === 'anigo-dark'" :size="14" />
                <Moon v-else :size="14" />
              </button>
              <button type="button" @click="toggleLanguage" class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md font-medium justify-center gap-1">
                <Languages :size="13" class="text-primary" />
                <span class="text-[11px]">{{ locale === 'zh' ? 'EN' : '中' }}</span>
              </button>
              <button type="button" @click="logout" class="btn btn-ghost btn-xs h-7 min-h-0 rounded-md justify-center text-error hover:bg-error/10" :aria-label="$t('nav.logout')">
                <LogOut :size="14" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 新版本提示浮窗 -->
    <div v-if="hasNewVersion" class="fixed bottom-20 lg:bottom-5 right-5 z-[100]">
      <div class="bg-base-100 border border-primary/30 shadow-xl rounded-xl p-4 flex flex-col gap-3 max-w-sm">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2.5 min-w-0">
            <div class="w-9 h-9 rounded-lg bg-primary/12 text-primary flex items-center justify-center shrink-0">
              <Sparkles :size="18" />
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="text-[11px] font-semibold text-primary">发现新版本</span>
                <span class="badge badge-primary badge-xs font-mono">{{ latestVersion }}</span>
              </div>
              <h4 class="text-sm font-semibold mt-0.5 text-base-content">Ani-Go 更新就绪</h4>
            </div>
          </div>
          <button type="button" @click="hasNewVersion = false" class="btn btn-ghost btn-xs btn-circle text-base-content/50 hover:text-base-content">
            <X :size="15" />
          </button>
        </div>

        <div v-if="updating" class="bg-base-200 p-2.5 rounded-lg space-y-1.5 border border-base-content/8">
          <div class="flex items-center justify-between text-xs font-medium">
            <span class="text-primary flex items-center gap-1.5">
              <span class="loading loading-spinner loading-xs"></span>
              {{ updateProgress.message }}
            </span>
            <span class="font-mono text-primary tabular-nums">{{ updateProgress.progress }}%</span>
          </div>
          <progress class="progress progress-primary w-full h-1.5" :value="updateProgress.progress" max="100"></progress>
        </div>

        <div v-else class="text-xs text-base-content/65 leading-relaxed">
          <span v-if="canSelfUpdate">发现新版本，升级完成后将校验运行版本号。</span>
          <span v-else>在 Compose 目录运行 <code>docker compose pull ani-go</code> 与 <code>docker compose up -d ani-go</code> 完成升级。</span>
        </div>

        <div class="flex items-center gap-2">
          <button
            v-if="canSelfUpdate"
            type="button"
            @click="triggerUpdate"
            :disabled="updating"
            class="btn btn-primary btn-sm flex-1 rounded-lg font-medium gap-1.5"
          >
            <Download :size="14" />
            <span>{{ updating ? "正在升级..." : "立即升级" }}</span>
          </button>
          <a
            href="https://github.com/xiaoyueRX/Ani-Go/releases"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-ghost btn-sm rounded-lg font-medium gap-1 text-xs text-base-content/70"
          >
            <span>发行日志</span>
            <ExternalLink :size="12" />
          </a>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-enter-active,
.page-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.page-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
