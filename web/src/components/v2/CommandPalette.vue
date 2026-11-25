<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  Calendar, LayoutGrid, Search, Download, Settings,
  Sun, Moon, Languages, Plus, ArrowRight, Command, Palette
} from 'lucide-vue-next'
import { useTheme } from '../../composables/useTheme'
import { setAppLocale } from '../../i18n'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const { t, locale } = useI18n()
const { isDark, toggleTheme } = useTheme()

const keyword = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

watch(() => props.open, async (val) => {
  if (val) {
    keyword.value = ''
    await nextTick()
    inputRef.value?.focus()
  }
})

const actions = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  const base = [
    { id: 'subscriptions', label: t('nav.subscriptions'), group: locale.value === 'zh' ? '页面导航' : 'Navigate', icon: LayoutGrid, run: () => go('/subscriptions') },
    { id: 'schedule', label: t('nav.schedule'), group: locale.value === 'zh' ? '页面导航' : 'Navigate', icon: Calendar, run: () => go('/schedule') },
    { id: 'search', label: t('nav.search'), group: locale.value === 'zh' ? '页面导航' : 'Navigate', icon: Search, run: () => go('/search') },
    { id: 'downloads', label: t('nav.downloads'), group: locale.value === 'zh' ? '页面导航' : 'Navigate', icon: Download, run: () => go('/downloads') },
    { id: 'settings', label: t('nav.settings'), group: locale.value === 'zh' ? '页面导航' : 'Navigate', icon: Settings, run: () => go('/settings') },
    { id: 'new-sub', label: locale.value === 'zh' ? '手动新增订阅' : 'Add Subscription', group: locale.value === 'zh' ? '快捷动作' : 'Quick Actions', icon: Plus, run: () => go('/subscriptions/new') },
    {
      id: 'theme-studio',
      label: locale.value === 'zh' ? '主题与色彩工坊 (预设 / 第三方)' : 'Theme & Color Studio',
      group: locale.value === 'zh' ? '外观与语言' : 'Appearance',
      icon: Palette,
      run: () => {
        window.dispatchEvent?.(new CustomEvent('anigo-open-theme-studio'))
        emit('close')
      },
    },
    {
      id: 'theme',
      label: isDark.value ? t('nav.themeLight') : t('nav.themeDark'),
      group: locale.value === 'zh' ? '外观与语言' : 'Appearance',
      icon: isDark.value ? Sun : Moon,
      run: () => { toggleTheme(); emit('close') },
    },
    {
      id: 'lang',
      label: locale.value === 'zh' ? 'Switch to English' : '切换为简体中文',
      group: locale.value === 'zh' ? '外观与语言' : 'Appearance',
      icon: Languages,
      run: () => {
        const next = locale.value === 'zh' ? 'en' : 'zh'
        void setAppLocale(next)
        emit('close')
      },
    },
  ]
  if (!q) return base
  return base.filter(item => item.label.toLowerCase().includes(q) || item.id.includes(q))
})

function go(path: string) {
  router.push(path)
  emit('close')
}

function submitSearch() {
  const q = keyword.value.trim()
  if (q) {
    router.push({ path: '/search', query: { q } })
    emit('close')
    return
  }
  actions.value[0]?.run()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) {
    emit('close')
  }
}

onMounted(() => window.addEventListener('keydown', handleKeydown))
onUnmounted(() => window.removeEventListener('keydown', handleKeydown))
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-[130] flex items-start justify-center pt-[14vh] px-4">
      <div class="fixed inset-0 bg-black/65 backdrop-blur-xs" @click="emit('close')"></div>
      <div
        role="dialog"
        aria-modal="true"
        class="relative w-full max-w-lg rounded-2xl bg-[var(--v2-surface)] border border-[var(--v2-border-strong)] shadow-2xl overflow-hidden z-10"
      >
        <div class="flex items-center gap-3 px-4 h-12 border-b border-[var(--v2-border)]">
          <Search :size="16" class="text-[var(--v2-accent)] shrink-0" />
          <input
            ref="inputRef"
            v-model="keyword"
            type="text"
            :placeholder="locale === 'zh' ? '输入页面名称、命令或直接按回车搜番...' : 'Jump to view, toggle theme, or press Enter to search anime...'"
            class="w-full bg-transparent text-sm text-[var(--v2-ink)] placeholder:text-[var(--v2-ink-subtle)] focus:outline-none"
            @keydown.enter.prevent="submitSearch"
          />
          <kbd class="px-1.5 py-0.5 text-[10px] font-mono rounded bg-[var(--v2-surface-2)] border border-[var(--v2-border)] text-[var(--v2-ink-subtle)]">ESC</kbd>
        </div>

        <div class="max-h-[340px] overflow-y-auto p-2 space-y-1">
          <button
            v-if="keyword.trim()"
            type="button"
            class="w-full flex items-center justify-between gap-3 px-3 py-2.5 rounded-xl bg-[color-mix(in_srgb,var(--v2-accent)_12%,transparent)] text-[var(--v2-accent)] hover:bg-[color-mix(in_srgb,var(--v2-accent)_18%,transparent)] transition-colors text-left"
            @click="submitSearch"
          >
            <span class="flex items-center gap-2.5 text-xs font-semibold">
              <Search :size="15" />
              <span>{{ locale === 'zh' ? `全网多源检索 "${keyword.trim()}"` : `Search anime "${keyword.trim()}"` }}</span>
            </span>
            <ArrowRight :size="14" />
          </button>

          <button
            v-for="item in actions"
            :key="item.id"
            type="button"
            class="w-full flex items-center justify-between gap-3 px-3 py-2 rounded-xl text-xs font-medium text-[var(--v2-ink)] hover:bg-[var(--v2-surface-2)] transition-colors text-left"
            @click="item.run"
          >
            <span class="flex items-center gap-2.5">
              <component :is="item.icon" :size="15" class="text-[var(--v2-ink-muted)]" />
              <span>{{ item.label }}</span>
            </span>
            <span class="text-[11px] text-[var(--v2-ink-subtle)]">{{ item.group }}</span>
          </button>
        </div>

        <div class="flex items-center justify-between px-4 py-2 border-t border-[var(--v2-border)] bg-[var(--v2-surface-2)]/60 text-[11px] text-[var(--v2-ink-subtle)]">
          <span class="flex items-center gap-1.5"><Command :size="12" /> Ani-Go Workbench</span>
          <span>Enter {{ locale === 'zh' ? '执行 / 搜索' : 'Confirm / Search' }}</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>
