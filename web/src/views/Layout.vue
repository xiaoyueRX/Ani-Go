<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import {
  Menu, LogOut, Antenna, Languages, ExternalLink,
  Calendar, LayoutGrid, Search, Download, Settings,
  Sparkles, X, TriangleAlert, Sun, Moon, Command, Palette
} from 'lucide-vue-next'
import { loadCurrentVersion, useVersion } from '../composables/useVersion'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const username = ref('')
const userAvatar = ref('')
const avatarBroken = ref(false)
const isDrawerOpen = ref(false)
const isUserMenuOpen = ref(false)
const isDefaultPassword = ref(false)
const dismissDefaultPasswordAlert = ref(false)

// version.test.mjs 在仅注入有限模块的 VM 中运行 Layout，主题与头像校验在此内联
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
      meta.setAttribute('content', next === 'anigo-light' ? '#f5f3ee' : '#09090b')
    }
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('ani-go-theme', next)
  }
}

function toggleTheme() {
  const before = typeof document !== 'undefined' ? document.documentElement?.getAttribute('data-theme') : null
  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent?.(new CustomEvent('anigo-toggle-theme-mode'))
  }
  const after = typeof document !== 'undefined' ? document.documentElement?.getAttribute('data-theme') : null
  if (!after || after === before) {
    applyTheme(currentTheme.value === 'anigo-dark' ? 'anigo-light' : 'anigo-dark')
  }
}

function openThemeStudio() {
  isUserMenuOpen.value = false
  isDrawerOpen.value = false
  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent?.(new CustomEvent('anigo-open-theme-studio'))
  }
}

function onThemeChanged(event: Event) {
  const mode = (event as CustomEvent).detail?.mode
  if (mode === 'anigo-light' || mode === 'anigo-dark') {
    currentTheme.value = mode
  }
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

window.showToast = showToast

function isValidAvatarUrl(raw?: string): boolean {
  const v = (raw || '').trim()
  if (!v) return false
  return /^(?:\/|https?:\/\/|data:image\/)/i.test(v)
}

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  const trimmed = url.trim()
  if (!trimmed) return ''
  if (trimmed.startsWith('/api/') || trimmed.startsWith('data:image/') || trimmed.includes('api/proxy/image')) {
    return trimmed
  }
  const target = trimmed.startsWith('//') ? 'https:' + trimmed : trimmed
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const resolvedAvatarSrc = computed(() => {
  if (avatarBroken.value || !isValidAvatarUrl(userAvatar.value)) return ''
  return proxyImage(userAvatar.value)
})

const userInitial = computed(() => (username.value?.trim().slice(0, 1).toUpperCase() || 'A'))

function toggleLanguage() {
  locale.value = locale.value === 'zh' ? 'en' : 'zh'
  localStorage.setItem('lang', locale.value)
}

function openCommandPalette() {
  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent?.(new CustomEvent('anigo-open-command-palette'))
  }
}

function updateAvatar(event: Event) {
  const url = (event as CustomEvent).detail
  if (typeof url === 'string') {
    userAvatar.value = url
    avatarBroken.value = false
  }
}

function handleDocClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  if (target && !target.closest('[data-user-menu]')) {
    isUserMenuOpen.value = false
  }
}

onUnmounted(() => {
  window.removeEventListener?.('avatar-updated', updateAvatar)
  window.removeEventListener?.('anigo-theme-changed', onThemeChanged)
  if (typeof document !== 'undefined') {
    document.removeEventListener?.('click', handleDocClick)
  }
})

onMounted(async () => {
  window.addEventListener('avatar-updated', updateAvatar)
  window.addEventListener('anigo-theme-changed', onThemeChanged)
  if (typeof document !== 'undefined') {
    document.addEventListener?.('click', handleDocClick)
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
    avatarBroken.value = false
    isDefaultPassword.value = !!data.is_default_password
  } catch { /* 401 handled by interceptor */ }

  // 若后台配置开启了自动检查更新，则在启动后静默探测，并水合主题配置
  try {
    const { data: settings } = await request.get('/settings')
    if (settings.AUTO_CHECK_UPDATE === 'true') {
      checkGitHubUpdate()
    }
    if ((settings.UI_THEME_PACK || settings.UI_THEME_STYLE || settings.UI_CUSTOM_THEMES) &&
        typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
      window.dispatchEvent?.(new CustomEvent('anigo-hydrate-theme-settings', { detail: settings }))
    }
  } catch (e) {
    console.error('Failed to fetch settings for update check:', e)
  }
})

function logout() {
  isUserMenuOpen.value = false
  isDrawerOpen.value = false
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
  <div class="min-h-screen flex flex-col bg-[var(--v2-bg)] text-[var(--v2-ink)] antialiased">
    <!-- 桌面端悬浮玻璃顶栏（必须用 nav 而非 header，保证移动端 header 契约） -->
    <nav class="hidden lg:block sticky top-0 z-30 border-b border-[var(--v2-border)] bg-[var(--v2-dock-bg)] backdrop-blur-xl select-none">
      <div class="max-w-[1440px] mx-auto h-16 px-6 flex items-center justify-between gap-4">
        <!-- 左侧品牌 + 脉冲在线指示 + Mono 版本号 -->
        <div class="flex items-center gap-3 min-w-0 shrink-0">
          <button
            type="button"
            class="flex items-center gap-2.5 group cursor-pointer bg-transparent border-0 p-0 text-left"
            @click="router.push('/subscriptions')"
          >
            <img
              src="/logo.png"
              alt="Ani-Go"
              class="w-8 h-8 rounded-xl object-cover border border-[var(--v2-border-strong)] shadow-xs transition-transform group-hover:scale-105"
            />
            <div class="flex items-center gap-2">
              <span class="text-[15px] font-bold tracking-tight text-[var(--v2-ink)]">Ani-Go</span>
              <span class="relative flex h-2 w-2" title="Engine Online">
                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
            </div>
          </button>
          <span
            v-if="currentVersion"
            :title="currentVersion"
            class="v2-pill font-mono text-[11px] max-w-[116px] truncate"
          >
            {{ currentVersion }}
          </span>
        </div>

        <!-- 中央悬浮胶囊导航 Dock（5 个核心路由） -->
        <div class="v2-nav-dock shadow-xs">
          <router-link
            v-for="item in nav"
            :key="item.path"
            :to="item.path"
            class="flex items-center gap-2 px-3.5 py-1.5 rounded-full text-[13px] font-medium transition-all duration-150 whitespace-nowrap"
            :class="route.path.startsWith(item.path)
              ? 'bg-[var(--v2-accent)] text-white font-semibold shadow-sm'
              : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:bg-[var(--v2-surface)]'"
          >
            <component :is="item.icon" :size="15" class="shrink-0" />
            <span>{{ item.label }}</span>
          </router-link>
        </div>

        <!-- 右侧全局指令入口 + 主题/语言开关 + 安全头像菜单 -->
        <div class="flex items-center gap-2 shrink-0">
          <button
            type="button"
            class="h-8 px-2.5 rounded-lg border border-[var(--v2-border)] bg-[var(--v2-surface-2)] hover:border-[var(--v2-border-strong)] text-xs text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] flex items-center gap-2 transition-colors cursor-pointer"
            :title="locale === 'zh' ? '打开快捷命令面板 (Ctrl+K)' : 'Command Palette (Ctrl+K)'"
            @click="openCommandPalette"
          >
            <Command :size="13" />
            <span class="hidden xl:inline">{{ locale === 'zh' ? '快捷跳转' : 'Jump' }}</span>
            <kbd class="font-mono text-[10px] px-1.5 py-0.5 rounded bg-[var(--v2-surface)] border border-[var(--v2-border)] text-[var(--v2-ink-subtle)]">⌘K</kbd>
          </button>

          <button
            type="button"
            class="w-8 h-8 rounded-lg border border-[var(--v2-border)] bg-[var(--v2-surface)] hover:bg-[var(--v2-surface-2)] flex items-center justify-center text-[var(--v2-ink-muted)] hover:text-[var(--v2-accent)] transition-colors cursor-pointer"
            :title="locale === 'zh' ? '主题与配色' : 'Themes & Colors'"
            aria-label="主题与配色"
            @click="openThemeStudio"
          >
            <Palette :size="15" />
          </button>

          <button
            type="button"
            class="w-8 h-8 rounded-lg border border-[var(--v2-border)] bg-[var(--v2-surface)] hover:bg-[var(--v2-surface-2)] flex items-center justify-center text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] transition-colors cursor-pointer"
            :title="themeLabel"
            :aria-label="themeLabel"
            @click="toggleTheme"
          >
            <Sun v-if="currentTheme === 'anigo-dark'" :size="15" />
            <Moon v-else :size="15" />
          </button>

          <button
            type="button"
            class="h-8 px-2.5 rounded-lg border border-[var(--v2-border)] bg-[var(--v2-surface)] hover:bg-[var(--v2-surface-2)] flex items-center gap-1.5 text-xs font-semibold text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] transition-colors cursor-pointer"
            :title="locale === 'zh' ? 'English' : '中文'"
            @click="toggleLanguage"
          >
            <Languages :size="13" class="text-[var(--v2-accent)]" />
            <span>{{ locale === 'zh' ? 'EN' : '中' }}</span>
          </button>

          <!-- 账户菜单 -->
          <div class="relative" data-user-menu>
            <button
              type="button"
              class="flex items-center gap-2 pl-1 pr-2.5 py-1 rounded-full border border-[var(--v2-border)] bg-[var(--v2-surface)] hover:border-[var(--v2-border-strong)] transition-colors cursor-pointer"
              @click.stop="isUserMenuOpen = !isUserMenuOpen"
            >
              <div
                class="w-7 h-7 rounded-full overflow-hidden flex items-center justify-center text-xs font-bold shrink-0"
                :style="!resolvedAvatarSrc ? { background: 'linear-gradient(135deg, var(--v2-accent) 0%, #fb923c 100%)', color: '#fff' } : undefined"
              >
                <img
                  v-if="resolvedAvatarSrc"
                  :src="resolvedAvatarSrc"
                  :alt="username || 'admin'"
                  class="w-full h-full object-cover"
                  @error="avatarBroken = true"
                />
                <span v-else>{{ userInitial }}</span>
              </div>
              <span class="text-xs font-semibold max-w-[84px] truncate text-[var(--v2-ink)]">{{ username || 'admin' }}</span>
            </button>

            <Transition name="fade">
              <div
                v-if="isUserMenuOpen"
                class="absolute right-0 mt-2 w-52 v2-panel p-1.5 shadow-2xl z-50 border border-[var(--v2-border-strong)]"
              >
                <div class="px-3 py-2 border-b border-[var(--v2-border)] mb-1">
                  <div class="text-xs font-semibold text-[var(--v2-ink)] truncate">{{ username || 'admin' }}</div>
                  <div class="text-[11px] text-emerald-500 flex items-center gap-1.5 mt-0.5">
                    <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                    <span>{{ $t('nav.activeNow') }}</span>
                  </div>
                </div>
                <router-link
                  to="/settings?tab=account"
                  class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:bg-[var(--v2-surface-2)] transition-colors"
                  @click="isUserMenuOpen = false"
                >
                  <Settings :size="14" />
                  <span>{{ locale === 'zh' ? '账户与安全' : 'Account & Security' }}</span>
                </router-link>
                <button
                  type="button"
                  class="w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                  @click="logout"
                >
                  <LogOut :size="14" />
                  <span>{{ $t('nav.logout') }}</span>
                </button>
              </div>
            </Transition>
          </div>
        </div>
      </div>
    </nav>

    <!-- 移动端顶栏：第一个按钮触发侧滑抽屉，右侧 .avatar 触发账户菜单 -->
    <header class="sticky top-0 z-30 flex h-14 w-full items-center justify-between px-3 bg-[var(--v2-dock-bg)] backdrop-blur-xl border-b border-[var(--v2-border)] lg:hidden">
      <button type="button" class="w-9 h-9 rounded-lg flex items-center justify-center text-[var(--v2-ink)] hover:bg-[var(--v2-surface-2)]" @click="isDrawerOpen = true" aria-label="Menu">
        <Menu :size="19" />
      </button>

      <div class="flex min-w-0 items-center gap-2 cursor-pointer px-2" @click="router.push('/subscriptions')">
        <img src="/logo.png" alt="Ani-Go" class="w-6 h-6 shrink-0 rounded-md object-cover border border-[var(--v2-border)]" />
        <span class="shrink-0 text-sm font-bold tracking-tight text-[var(--v2-ink)]">Ani-Go</span>
        <span v-if="currentVersion" :title="currentVersion" class="v2-pill font-mono text-[10px] max-w-[110px] truncate">
          {{ currentVersion }}
        </span>
      </div>

      <div class="relative shrink-0" data-user-menu>
        <button
          type="button"
          class="avatar w-8 h-8 rounded-full overflow-hidden flex items-center justify-center text-xs font-bold border border-[var(--v2-border-strong)]"
          :style="!resolvedAvatarSrc ? { background: 'linear-gradient(135deg, var(--v2-accent) 0%, #fb923c 100%)', color: '#fff' } : undefined"
          @click.stop="isUserMenuOpen = !isUserMenuOpen"
        >
          <img
            v-if="resolvedAvatarSrc"
            :src="resolvedAvatarSrc"
            :alt="username || 'admin'"
            class="w-full h-full object-cover"
            @error="avatarBroken = true"
          />
          <span v-else>{{ userInitial }}</span>
        </button>

        <div
          v-if="isUserMenuOpen"
          class="absolute right-0 mt-2 w-48 v2-panel p-1.5 shadow-xl z-50 border border-[var(--v2-border-strong)]"
        >
          <div class="px-3 py-1.5 text-xs font-semibold text-[var(--v2-ink-muted)] truncate">{{ username || 'admin' }}</div>
          <button type="button" class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium hover:bg-[var(--v2-surface-2)]" @click="openThemeStudio">
            <Palette :size="14" class="text-[var(--v2-accent)]" />
            <span>{{ locale === 'zh' ? '主题与配色' : 'Themes' }}</span>
          </button>
          <button type="button" class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium hover:bg-[var(--v2-surface-2)]" @click="toggleTheme">
            <Sun v-if="currentTheme === 'anigo-dark'" :size="14" class="text-[var(--v2-accent)]" />
            <Moon v-else :size="14" class="text-[var(--v2-accent)]" />
            <span>{{ themeLabel }}</span>
          </button>
          <button type="button" class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium hover:bg-[var(--v2-surface-2)]" @click="toggleLanguage">
            <Languages :size="14" class="text-[var(--v2-accent)]" />
            <span>{{ locale === 'zh' ? 'English' : '中文' }}</span>
          </button>
          <div class="my-1 border-t border-[var(--v2-border)]"></div>
          <button type="button" class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10" @click="logout">
            <LogOut :size="14" />
            <span>{{ $t('nav.logout') }}</span>
          </button>
        </div>
      </div>
    </header>

    <!-- 默认密码安全横幅 -->
    <div v-if="isDefaultPassword && !dismissDefaultPasswordAlert" class="bg-amber-500/10 border-b border-amber-500/25 px-4 py-2 text-amber-400 text-xs font-medium">
      <div class="max-w-[1440px] mx-auto flex items-center justify-between gap-3">
        <div class="flex items-center gap-2 min-w-0">
          <TriangleAlert :size="15" class="shrink-0" />
          <span class="truncate">{{ locale === 'zh' ? '安全提醒：当前管理员仍在使用默认密码，建议前往「账户与安全」修改。' : 'Security notice: Please update the default administrator password in Settings.' }}</span>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <router-link to="/settings?tab=account" class="px-2.5 py-1 rounded-md bg-amber-500/20 hover:bg-amber-500/30 text-amber-300 text-xs font-semibold transition-colors">
            {{ locale === 'zh' ? '前往修改' : 'Update' }}
          </router-link>
          <button type="button" @click="dismissDefaultPasswordAlert = true" class="p-1 rounded-md hover:bg-amber-500/20 opacity-75 hover:opacity-100">
            <X :size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- 主路由视口 -->
    <main class="flex-1 px-4 sm:px-6 lg:px-8 pt-6 pb-24 lg:pb-10 max-w-[1440px] mx-auto w-full min-w-0 overflow-x-hidden">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.name === 'subscription-detail' ? route.params.id as string : undefined" />
        </transition>
      </router-view>
    </main>

    <!-- 页脚（保留完整单行版本号 span.font-mono 供移动端校验） -->
    <footer class="py-5 px-4 text-center text-xs text-[var(--v2-ink-subtle)] mt-auto border-t border-[var(--v2-border)] pb-24 lg:pb-6">
      <div class="flex items-center justify-center gap-2 flex-wrap">
        <span>Ani-Go &copy; 2026</span>
        <span class="opacity-40">•</span>
        <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" class="hover:text-[var(--v2-accent)] transition-colors">GitHub</a>
        <template v-if="currentVersion">
          <span class="opacity-40">•</span>
          <span class="max-w-full break-all font-mono text-[var(--v2-ink-muted)]">{{ currentVersion }}</span>
        </template>
      </div>
    </footer>

    <!-- 移动端底部固定导航栏 -->
    <nav class="fixed bottom-0 left-0 right-0 z-40 bg-[var(--v2-dock-bg)] backdrop-blur-xl border-t border-[var(--v2-border)] px-1.5 py-1 flex items-center justify-around lg:hidden pb-[env(safe-area-inset-bottom,6px)]">
      <router-link
        v-for="item in nav"
        :key="item.path"
        :to="item.path"
        class="flex flex-col items-center justify-center py-1 px-2 rounded-lg transition-colors min-w-[52px]"
        :class="route.path.startsWith(item.path) ? 'text-[var(--v2-accent)] font-semibold' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] font-medium'"
      >
        <component :is="item.icon" :size="18" />
        <span class="text-[11px] mt-0.5 whitespace-nowrap">{{ item.label }}</span>
      </router-link>
    </nav>

    <!-- 全局 Toast 提示 -->
    <div class="fixed bottom-20 lg:bottom-5 right-5 z-[100] flex flex-col gap-2 pointer-events-none">
      <transition-group name="page">
        <div
          v-for="item in toasts"
          :key="item.id"
          class="pointer-events-auto v2-panel shadow-2xl px-4 py-3 min-w-[260px] max-w-sm flex items-center gap-2.5 border"
          :class="{
            'border-[var(--v2-accent)]/40': item.type === 'success',
            'border-red-500/40 text-red-400': item.type === 'error',
            'border-sky-500/40 text-sky-400': item.type === 'info'
          }"
        >
          <Sparkles v-if="item.type === 'success'" :size="16" class="text-[var(--v2-accent)] shrink-0" />
          <TriangleAlert v-else-if="item.type === 'error'" :size="16" class="text-red-400 shrink-0" />
          <Antenna v-else :size="16" class="text-sky-400 shrink-0" />
          <span class="text-xs font-semibold text-[var(--v2-ink)]">{{ item.message }}</span>
        </div>
      </transition-group>
    </div>

    <!-- 移动端侧滑导航抽屉（必须不透明底色且包含 a[href='/settings']） -->
    <Teleport to="body">
      <div v-if="isDrawerOpen" class="fixed inset-0 z-[110] lg:hidden flex">
        <div class="fixed inset-0 bg-black/65 backdrop-blur-xs" @click="closeDrawer"></div>

        <div class="relative w-72 max-w-[82vw] h-full bg-[var(--v2-surface)] border-r border-[var(--v2-border-strong)] shadow-2xl flex flex-col z-10">
          <div class="h-14 px-4 border-b border-[var(--v2-border)] flex items-center justify-between shrink-0">
            <div class="flex items-center gap-2.5">
              <img src="/logo.png" alt="Ani-Go" class="w-7 h-7 rounded-lg object-cover border border-[var(--v2-border)]" />
              <span class="text-base font-bold tracking-tight text-[var(--v2-ink)]">Ani-Go</span>
            </div>
            <button type="button" class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-[var(--v2-surface-2)]" @click="closeDrawer" aria-label="Close">
              <X :size="17" />
            </button>
          </div>

          <div class="flex-1 px-2.5 py-3 overflow-y-auto space-y-1">
            <router-link
              v-for="item in nav"
              :key="item.path"
              :to="item.path"
              class="flex items-center gap-3 px-3 py-2.5 rounded-xl transition-colors text-sm relative"
              :class="route.path.startsWith(item.path)
                ? 'bg-[var(--v2-accent)] text-white font-semibold'
                : 'text-[var(--v2-ink-muted)] hover:bg-[var(--v2-surface-2)] hover:text-[var(--v2-ink)] font-medium'"
              @click="closeDrawer"
            >
              <component :is="item.icon" :size="18" />
              <span>{{ item.label }}</span>
            </router-link>
          </div>

          <div class="p-3 m-2.5 rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)] space-y-2.5">
            <div class="flex items-center gap-2.5 min-w-0">
              <div
                class="w-8 h-8 rounded-lg overflow-hidden flex items-center justify-center font-bold text-xs shrink-0"
                :style="!resolvedAvatarSrc ? { background: 'linear-gradient(135deg, var(--v2-accent) 0%, #fb923c 100%)', color: '#fff' } : undefined"
              >
                <img
                  v-if="resolvedAvatarSrc"
                  :src="resolvedAvatarSrc"
                  :alt="username || 'admin'"
                  class="w-full h-full object-cover"
                  @error="avatarBroken = true"
                />
                <span v-else>{{ userInitial }}</span>
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-xs font-semibold truncate text-[var(--v2-ink)]">{{ username || 'admin' }}</div>
                <div class="text-[11px] text-emerald-500">{{ $t('nav.activeNow') }}</div>
              </div>
            </div>
            <div class="grid grid-cols-4 gap-1 pt-2 border-t border-[var(--v2-border)]">
              <button type="button" @click="openThemeStudio" class="v2-btn v2-btn-ghost v2-btn-sm justify-center" :aria-label="locale === 'zh' ? '主题与配色' : 'Themes'">
                <Palette :size="14" class="text-[var(--v2-accent)]" />
              </button>
              <button type="button" @click="toggleTheme" class="v2-btn v2-btn-ghost v2-btn-sm justify-center" :aria-label="themeLabel">
                <Sun v-if="currentTheme === 'anigo-dark'" :size="14" />
                <Moon v-else :size="14" />
              </button>
              <button type="button" @click="toggleLanguage" class="v2-btn v2-btn-ghost v2-btn-sm justify-center gap-1">
                <Languages :size="13" class="text-[var(--v2-accent)]" />
                <span class="text-[11px]">{{ locale === 'zh' ? 'EN' : '中' }}</span>
              </button>
              <button type="button" @click="logout" class="v2-btn v2-btn-ghost v2-btn-sm justify-center text-red-400 hover:bg-red-500/10" :aria-label="$t('nav.logout')">
                <LogOut :size="14" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 新版本提示浮窗 -->
    <div v-if="hasNewVersion" class="fixed bottom-20 lg:bottom-5 right-5 z-[100]">
      <div class="v2-panel border border-[var(--v2-accent)]/35 shadow-2xl p-4 flex flex-col gap-3 max-w-sm">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2.5 min-w-0">
            <div class="w-9 h-9 rounded-lg bg-[var(--v2-accent)]/15 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
              <Sparkles :size="18" />
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="text-[11px] font-semibold text-[var(--v2-accent)]">发现新版本</span>
                <span class="v2-pill font-mono text-[10px]">{{ latestVersion }}</span>
              </div>
              <h4 class="text-sm font-semibold mt-0.5 text-[var(--v2-ink)]">Ani-Go 更新就绪</h4>
            </div>
          </div>
          <button type="button" @click="hasNewVersion = false" class="p-1 rounded-md text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]">
            <X :size="15" />
          </button>
        </div>

        <div v-if="updating" class="v2-panel-muted p-2.5 space-y-1.5">
          <div class="flex items-center justify-between text-xs font-medium">
            <span class="text-[var(--v2-accent)]">{{ updateProgress.message }}</span>
            <span class="font-mono text-[var(--v2-accent)] tabular-nums">{{ updateProgress.progress }}%</span>
          </div>
          <div class="h-1.5 w-full bg-[var(--v2-surface-3)] rounded-full overflow-hidden">
            <div class="h-full bg-[var(--v2-accent)] transition-all" :style="{ width: `${updateProgress.progress}%` }"></div>
          </div>
        </div>

        <div v-else class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">
          <span v-if="canSelfUpdate">发现新版本，升级完成后将校验运行版本号。</span>
          <span v-else>在 Compose 目录运行 <code>docker compose pull ani-go</code> 与 <code>docker compose up -d ani-go</code> 完成升级。</span>
        </div>

        <div class="flex items-center gap-2">
          <button
            v-if="canSelfUpdate"
            type="button"
            @click="triggerUpdate"
            :disabled="updating"
            class="v2-btn v2-btn-primary v2-btn-sm flex-1"
          >
            <Download :size="14" />
            <span>{{ updating ? '正在升级...' : '立即升级' }}</span>
          </button>
          <a
            href="https://github.com/xiaoyueRX/Ani-Go/releases"
            target="_blank"
            rel="noopener noreferrer"
            class="v2-btn v2-btn-ghost v2-btn-sm text-xs"
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
  transition: opacity 0.16s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
