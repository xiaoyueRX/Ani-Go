import { computed, ref } from 'vue'

export type ThemeName = 'anigo-dark' | 'anigo-light'

export const THEME_STORAGE_KEY = 'ani-go-theme'

export const THEME_COLORS: Record<ThemeName, string> = {
  'anigo-dark': '#09090b',
  'anigo-light': '#f5f3ee',
}

export function normalizeTheme(raw: unknown): ThemeName {
  return raw === 'anigo-light' ? 'anigo-light' : 'anigo-dark'
}

export function readStoredTheme(): ThemeName {
  if (typeof localStorage === 'undefined') return 'anigo-dark'
  try {
    return normalizeTheme(localStorage.getItem(THEME_STORAGE_KEY))
  } catch {
    return 'anigo-dark'
  }
}

export function applyTheme(next: ThemeName) {
  if (typeof document !== 'undefined') {
    document.documentElement?.setAttribute('data-theme', next)
    const meta = document.querySelector?.('meta[name="theme-color"]')
    if (meta) {
      meta.setAttribute('content', THEME_COLORS[next])
    }
  }
  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next)
    } catch {
      // 忽略受限环境写入失败，保持内存主题状态
    }
  }
}

const currentTheme = ref<ThemeName>(readStoredTheme())

export function useTheme() {
  const isDark = computed(() => currentTheme.value === 'anigo-dark')

  function setTheme(next: ThemeName) {
    const normalized = normalizeTheme(next)
    currentTheme.value = normalized
    applyTheme(normalized)
    return normalized
  }

  function syncTheme() {
    const stored = readStoredTheme()
    currentTheme.value = stored
    applyTheme(stored)
    return stored
  }

  function toggleTheme() {
    return setTheme(currentTheme.value === 'anigo-dark' ? 'anigo-light' : 'anigo-dark')
  }

  return {
    theme: currentTheme,
    isDark,
    setTheme,
    syncTheme,
    toggleTheme,
  }
}
