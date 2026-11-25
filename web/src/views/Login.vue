<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { loginDestination } from '../utils/loginRedirect'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { useTheme } from '../composables/useTheme'
import {
  AlertTriangle, User, Lock, LogIn, Languages, Sun, Moon,
  Search, FolderCheck, Sparkles
} from 'lucide-vue-next'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const { isDark, toggleTheme } = useTheme()
const username = ref(localStorage.getItem('remembered_username') || '')
const password = ref('')
const remember = ref(!!username.value)
const error = ref('')
const loading = ref(false)

function toggleLanguage() {
  locale.value = locale.value === 'zh' ? 'en' : 'zh'
  localStorage.setItem('lang', locale.value)
}

async function handleLogin() {
  if (loading.value) return
  if (!username.value || !password.value) {
    error.value = t('login.errorEmpty')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.post('/login', {
      username: username.value,
      password: password.value,
    })
    localStorage.setItem('token', data.token)
    if (remember.value) {
      localStorage.setItem('remembered_username', username.value)
    } else {
      localStorage.removeItem('remembered_username')
    }
    await router.replace(loginDestination(router, route.query.redirect))
  } catch (e: any) {
    error.value = e.response?.data?.error || t('login.errorFailed')
  } finally {
    loading.value = false
  }
}

const highlights = [
  {
    icon: Search,
    zhTitle: '多源并发检索',
    enTitle: 'Multi-Source Discovery',
    zhDesc: '聚合 Mikan、ACG.RIP、Nyaa 与 AnimeTosho 实时索引，毫秒级直达磁力与 RSS 源。',
    enDesc: 'Aggregate Mikan, ACG.RIP, Nyaa, and AnimeTosho feeds in one unified search workbench.',
  },
  {
    icon: FolderCheck,
    zhTitle: '无感洗版与硬链入库',
    enTitle: 'Zero-Copy Library Sync',
    zhDesc: '精准识别季度、集数与字幕组标签，硬链接秒级入库适配 Emby / Jellyfin / Plex。',
    enDesc: 'Parse season, episode, and release groups automatically for Emby, Jellyfin, and Plex.',
  },
  {
    icon: Sparkles,
    zhTitle: '低占轻量级守护',
    enTitle: 'Lean NAS Footprint',
    zhDesc: '纯 Go 单二进制引擎驱动，空载极低功耗与内存占用，全天候守护追番日历。',
    enDesc: 'Single Go binary built for 24/7 NAS deployment with minimal CPU and memory overhead.',
  },
]
</script>

<template>
  <div class="min-h-screen grid lg:grid-cols-12 bg-[var(--v2-bg)] text-[var(--v2-ink)] relative overflow-hidden">
    <!-- 右上角主题与语言开关 -->
    <div class="fixed top-5 right-5 z-30 flex items-center gap-2">
      <button
        type="button"
        class="w-9 h-9 rounded-xl border border-[var(--v2-border)] bg-[var(--v2-surface)]/85 backdrop-blur-md flex items-center justify-center text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:border-[var(--v2-border-strong)] transition-colors cursor-pointer"
        :title="isDark ? $t('nav.themeLight') : $t('nav.themeDark')"
        :aria-label="isDark ? $t('nav.themeLight') : $t('nav.themeDark')"
        @click="toggleTheme"
      >
        <Sun v-if="isDark" :size="16" />
        <Moon v-else :size="16" />
      </button>
      <button
        type="button"
        class="h-9 px-3 rounded-xl border border-[var(--v2-border)] bg-[var(--v2-surface)]/85 backdrop-blur-md flex items-center gap-1.5 text-xs font-semibold text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:border-[var(--v2-border-strong)] transition-colors cursor-pointer"
        @click="toggleLanguage"
      >
        <Languages :size="14" class="text-[var(--v2-accent)]" />
        <span>{{ locale === 'zh' ? 'EN' : '中文' }}</span>
      </button>
    </div>

    <!-- 左侧编辑部放映厅品牌展示区（桌面端） -->
    <section class="hidden lg:flex lg:col-span-7 flex-col justify-between p-12 xl:p-16 relative border-r border-[var(--v2-border)] overflow-hidden">
      <div
        class="pointer-events-none absolute inset-0"
        style="background: radial-gradient(ellipse 85% 65% at 15% 15%, color-mix(in srgb, var(--v2-accent) 18%, transparent), transparent 65%), radial-gradient(circle at 85% 85%, color-mix(in srgb, var(--v2-emerald) 10%, transparent), transparent 55%);"
        aria-hidden="true"
      />
      <div
        class="pointer-events-none absolute inset-0 opacity-[0.06]"
        style="background-image: linear-gradient(to right, currentColor 1px, transparent 1px), linear-gradient(to bottom, currentColor 1px, transparent 1px); background-size: 48px 48px;"
        aria-hidden="true"
      />

      <!-- 顶部徽标与运行态标识 -->
      <div class="relative z-10 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <img
            src="/logo.png"
            alt="Ani-Go Logo"
            class="w-10 h-10 rounded-xl object-cover border border-[var(--v2-border-strong)] shadow-md"
          />
          <div>
            <div class="text-base font-bold tracking-tight">Ani-Go</div>
            <div class="text-[11px] font-mono uppercase tracking-widest text-[var(--v2-ink-subtle)]">Editorial Cinema Workbench</div>
          </div>
        </div>
        <span class="v2-pill font-mono text-[11px]">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          {{ locale === 'zh' ? '本地自动化引擎就绪' : 'Engine Ready' }}
        </span>
      </div>

      <!-- 中部主宣言与 3 张特性卡 -->
      <div class="relative z-10 max-w-2xl space-y-8 my-auto py-8">
        <div class="space-y-4">
          <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--v2-accent)]/12 border border-[var(--v2-accent)]/25 text-[var(--v2-accent)] text-xs font-semibold">
            <span>{{ locale === 'zh' ? '新一代 NAS 追番控制台' : 'Next-Gen Anime Tracking Workbench' }}</span>
          </div>
          <h1 class="text-4xl xl:text-5xl font-extrabold tracking-tight leading-[1.12]">
            {{ locale === 'zh' ? '追番，从未如此纯粹。' : 'Anime tracking, refined.' }}
          </h1>
          <p class="text-base text-[var(--v2-ink-muted)] leading-relaxed max-w-xl">
            {{ locale === 'zh'
              ? '告别繁冗的后台表格堆砌。为新番时间表、海报藏书架与多源搜索打造的高对比影院工作台。'
              : 'A high-contrast cinema workbench built around broadcast schedules, poster libraries, and multi-source torrent search.' }}
          </p>
        </div>

        <div class="grid gap-3.5">
          <div
            v-for="item in highlights"
            :key="item.zhTitle"
            class="v2-panel p-4 flex items-start gap-3.5 backdrop-blur-md bg-[var(--v2-surface)]/80"
          >
            <div class="w-9 h-9 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0 border border-[var(--v2-accent)]/20">
              <component :is="item.icon" :size="17" />
            </div>
            <div class="min-w-0">
              <h2 class="text-sm font-semibold text-[var(--v2-ink)]">
                {{ locale === 'zh' ? item.zhTitle : item.enTitle }}
              </h2>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5 leading-relaxed">
                {{ locale === 'zh' ? item.zhDesc : item.enDesc }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部链接 -->
      <div class="relative z-10 flex items-center justify-between text-xs text-[var(--v2-ink-subtle)]">
        <span>Ani-Go &copy; 2026</span>
        <div class="flex items-center gap-5">
          <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" class="hover:text-[var(--v2-ink)] transition-colors">GitHub</a>
          <a :href="locale === 'zh' ? 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT.md' : 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT_EN.md'" target="_blank" rel="noopener noreferrer" class="hover:text-[var(--v2-ink)] transition-colors">Documentation</a>
        </div>
      </div>
    </section>

    <!-- 右侧登录表单面板 -->
    <section class="lg:col-span-5 flex flex-col items-center justify-center px-5 py-16 relative">
      <div class="w-full max-w-[390px] space-y-6">
        <div class="v2-panel p-7 sm:p-8 shadow-2xl border border-[var(--v2-border-strong)]">
          <!-- 头部 -->
          <div class="flex items-center gap-3 mb-7">
            <img
              src="/logo.png"
              alt="Ani-Go Logo"
              class="w-11 h-11 rounded-xl object-cover border border-[var(--v2-border-strong)] shadow-xs lg:hidden"
            />
            <div>
              <h2 class="text-xl font-bold tracking-tight text-[var(--v2-ink)]">
                {{ locale === 'zh' ? '登录控制台' : $t('login.title') }}
              </h2>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('login.subtitle') }}</p>
            </div>
          </div>

          <!-- 错误提示 -->
          <Transition name="fade">
            <div
              v-if="error"
              role="alert"
              class="bg-red-500/10 border border-red-500/25 text-red-400 rounded-xl px-3.5 py-2.5 mb-5 flex items-center gap-2.5 text-xs font-medium"
            >
              <AlertTriangle :size="16" class="shrink-0" />
              <span>{{ error }}</span>
            </div>
          </Transition>

          <!-- 登录表单 -->
          <form @submit.prevent="handleLogin" class="space-y-4">
            <div>
              <label for="login-username" class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1.5">
                {{ $t('login.username') }}
              </label>
              <div class="relative group">
                <div class="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-[var(--v2-ink-subtle)] group-focus-within:text-[var(--v2-accent)] transition-colors">
                  <User :size="15" />
                </div>
                <input
                  id="login-username"
                  v-model="username"
                  type="text"
                  :placeholder="$t('login.usernamePlaceholder')"
                  class="v2-input pl-10"
                  autocomplete="username"
                />
              </div>
            </div>

            <div>
              <label for="login-password" class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1.5">
                {{ $t('login.password') }}
              </label>
              <div class="relative group">
                <div class="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-[var(--v2-ink-subtle)] group-focus-within:text-[var(--v2-accent)] transition-colors">
                  <Lock :size="15" />
                </div>
                <input
                  id="login-password"
                  v-model="password"
                  type="password"
                  :placeholder="$t('login.passwordPlaceholder')"
                  class="v2-input pl-10"
                  autocomplete="current-password"
                />
              </div>
            </div>

            <div class="flex items-center justify-between pt-0.5">
              <label class="flex items-center gap-2 cursor-pointer select-none">
                <input v-model="remember" type="checkbox" class="checkbox checkbox-primary checkbox-sm" />
                <span class="text-xs text-[var(--v2-ink-muted)]">{{ $t('login.remember') }}</span>
              </label>
            </div>

            <button
              type="submit"
              class="v2-btn v2-btn-primary w-full h-10 text-sm font-semibold mt-2 shadow-lg shadow-[var(--v2-accent)]/20"
              :disabled="loading"
            >
              <span v-if="loading" class="loading loading-spinner loading-sm"></span>
              <template v-else>
                <span>{{ $t('login.submit') }}</span>
                <LogIn :size="16" />
              </template>
            </button>
          </form>

          <div class="mt-6 pt-4 border-t border-[var(--v2-border)] text-center">
            <p class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('login.securing') }}</p>
          </div>
        </div>

        <!-- 移动端底部链接 -->
        <div class="flex lg:hidden justify-center gap-5 text-xs text-[var(--v2-ink-subtle)]">
          <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" class="hover:text-[var(--v2-ink)]">GitHub</a>
          <a :href="locale === 'zh' ? 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT.md' : 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT_EN.md'" target="_blank" rel="noopener noreferrer" class="hover:text-[var(--v2-ink)]">Documentation</a>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
input:-webkit-autofill,
input:-webkit-autofill:hover,
input:-webkit-autofill:focus {
  -webkit-text-fill-color: var(--v2-ink);
  -webkit-box-shadow: 0 0 0 1000px var(--v2-surface-2) inset;
  transition: background-color 5000s ease-in-out 0s;
}
</style>
