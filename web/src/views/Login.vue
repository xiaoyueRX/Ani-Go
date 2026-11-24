<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { loginDestination } from '../utils/loginRedirect'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { useTheme } from '../composables/useTheme'
import {
  AlertTriangle, User, Lock, LogIn, Languages, Sun, Moon
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
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-base-300 text-base-content relative overflow-hidden px-4 py-10">
    <!-- 细腻网格与顶部微光背景 -->
    <div
      class="pointer-events-none absolute inset-0 opacity-45"
      style="background-image: radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--color-primary) 16%, transparent), transparent 60%);"
      aria-hidden="true"
    />

    <!-- 右上角主题与语言开关 -->
    <div class="absolute top-5 right-5 z-20 flex items-center gap-1.5">
      <button
        type="button"
        class="btn btn-ghost btn-sm btn-square rounded-lg border border-base-content/10 bg-base-100/80"
        :title="isDark ? $t('nav.themeLight') : $t('nav.themeDark')"
        :aria-label="isDark ? $t('nav.themeLight') : $t('nav.themeDark')"
        @click="toggleTheme"
      >
        <Sun v-if="isDark" :size="16" />
        <Moon v-else :size="16" />
      </button>
      <button
        type="button"
        class="btn btn-ghost btn-sm rounded-lg border border-base-content/10 bg-base-100/80 gap-1.5 text-xs font-medium"
        @click="toggleLanguage"
      >
        <Languages :size="15" class="text-primary" />
        <span>{{ locale === 'zh' ? 'EN' : '中文' }}</span>
      </button>
    </div>

    <!-- 登录卡片 -->
    <div class="relative z-10 w-full max-w-[390px]">
      <div class="app-surface rounded-2xl shadow-xl p-6 sm:p-8">
        <!-- 品牌标题区 -->
        <div class="flex flex-col items-center text-center mb-7">
          <img
            src="/logo.png"
            alt="Ani-Go Logo"
            class="w-12 h-12 rounded-xl object-cover border border-base-content/10 shadow-sm mb-4"
          />
          <h1 class="text-xl font-bold tracking-tight text-base-content">{{ $t('login.title') }}</h1>
          <p class="text-xs text-base-content/60 mt-1">{{ $t('login.subtitle') }}</p>
        </div>

        <!-- 错误提示 -->
        <Transition name="fade">
          <div
            v-if="error"
            role="alert"
            class="bg-error/10 border border-error/25 text-error rounded-lg px-3.5 py-2.5 mb-5 flex items-center gap-2.5 text-xs font-medium"
          >
            <AlertTriangle :size="16" class="shrink-0" />
            <span>{{ error }}</span>
          </div>
        </Transition>

        <!-- 登录表单 -->
        <form @submit.prevent="handleLogin" class="space-y-4">
          <div>
            <label for="login-username" class="app-field-label">{{ $t('login.username') }}</label>
            <div class="relative group">
              <div class="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-base-content/35 group-focus-within:text-primary transition-colors">
                <User :size="16" />
              </div>
              <input
                id="login-username"
                v-model="username"
                type="text"
                :placeholder="$t('login.usernamePlaceholder')"
                class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg pl-10 pr-3.5 text-sm font-medium placeholder:text-base-content/35"
                autocomplete="username"
              />
            </div>
          </div>

          <div>
            <label for="login-password" class="app-field-label">{{ $t('login.password') }}</label>
            <div class="relative group">
              <div class="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-base-content/35 group-focus-within:text-primary transition-colors">
                <Lock :size="16" />
              </div>
              <input
                id="login-password"
                v-model="password"
                type="password"
                :placeholder="$t('login.passwordPlaceholder')"
                class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg pl-10 pr-3.5 text-sm font-medium placeholder:text-base-content/35"
                autocomplete="current-password"
              />
            </div>
          </div>

          <div class="flex items-center justify-between pt-0.5">
            <label class="flex items-center gap-2 cursor-pointer select-none">
              <input v-model="remember" type="checkbox" class="checkbox checkbox-primary checkbox-sm rounded" />
              <span class="text-xs text-base-content/65">{{ $t('login.remember') }}</span>
            </label>
          </div>

          <button
            type="submit"
            class="btn btn-primary w-full h-10 rounded-lg font-semibold text-sm gap-2 mt-2"
            :disabled="loading"
          >
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            <template v-else>
              <span>{{ $t('login.submit') }}</span>
              <LogIn :size="16" />
            </template>
          </button>
        </form>

        <div class="mt-6 pt-4 border-t border-base-content/8 text-center">
          <p class="text-[11px] text-base-content/45">{{ $t('login.securing') }}</p>
        </div>
      </div>

      <!-- 底部链接 -->
      <div class="mt-5 flex justify-center gap-5 text-xs text-base-content/50">
        <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" class="hover:text-base-content transition-colors">GitHub</a>
        <a :href="locale === 'zh' ? 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT.md' : 'https://github.com/xiaoyueRX/Ani-Go/blob/HEAD/docs/05_DEPLOYMENT_EN.md'" target="_blank" rel="noopener noreferrer" class="hover:text-base-content transition-colors">Documentation</a>
        <a href="https://github.com/xiaoyueRX/Ani-Go/issues" target="_blank" rel="noopener noreferrer" class="hover:text-base-content transition-colors">Support</a>
      </div>
    </div>
  </div>
</template>

<style scoped>
input:-webkit-autofill,
input:-webkit-autofill:hover,
input:-webkit-autofill:focus {
  -webkit-text-fill-color: var(--color-base-content);
  -webkit-box-shadow: 0 0 0 1000px var(--color-base-200) inset;
  transition: background-color 5000s ease-in-out 0s;
}
</style>
