<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { loginDestination } from './utils/loginRedirect'
import CommandPalette from './components/v2/CommandPalette.vue'
import ThemeStudioDrawer from './components/v2/ThemeStudioDrawer.vue'
import { useTheme } from './composables/useTheme'
import request from './utils/request'

const route = useRoute()
const router = useRouter()
const commandPaletteOpen = ref(false)
const themeStudioOpen = ref(false)
const { toggleTheme, hydrateFromServerSettings, getServerSettingsPayload } = useTheme()

function openPalette() {
  commandPaletteOpen.value = true
}

function openThemeStudio() {
  themeStudioOpen.value = true
}

async function handleToggleThemeMode() {
  toggleTheme()
  if (localStorage.getItem('token')) {
    try {
      await request.put('/settings', { settings: getServerSettingsPayload() })
    } catch {}
  }
}

function handleHydrateThemeSettings(e: Event) {
  const detail = (e as CustomEvent).detail
  hydrateFromServerSettings(detail)
}

function handleGlobalShortcut(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    commandPaletteOpen.value = !commandPaletteOpen.value
  }
}

function syncSession(event: StorageEvent) {
  if (event.storageArea !== localStorage || (event.key !== 'token' && event.key !== null)) return
  const token = localStorage.getItem('token')
  if (event.key === 'token' && event.newValue !== token) return
  if (!token) {
    if (route.name !== 'login') window.location.replace('/login?redirect=' + encodeURIComponent(route.fullPath))
  } else if (route.name === 'login') {
    window.location.replace(loginDestination(router, route.query.redirect))
  } else {
    window.location.reload()
  }
}

onMounted(() => {
  window.addEventListener('storage', syncSession)
  window.addEventListener('keydown', handleGlobalShortcut)
  window.addEventListener('anigo-open-command-palette', openPalette)
  window.addEventListener('anigo-open-theme-studio', openThemeStudio)
  window.addEventListener('anigo-toggle-theme-mode', handleToggleThemeMode)
  window.addEventListener('anigo-hydrate-theme-settings', handleHydrateThemeSettings)
})
onUnmounted(() => {
  window.removeEventListener('storage', syncSession)
  window.removeEventListener('keydown', handleGlobalShortcut)
  window.removeEventListener('anigo-open-command-palette', openPalette)
  window.removeEventListener('anigo-open-theme-studio', openThemeStudio)
  window.removeEventListener('anigo-toggle-theme-mode', handleToggleThemeMode)
  window.removeEventListener('anigo-hydrate-theme-settings', handleHydrateThemeSettings)
})

// 已登录用户可能直接打开业务页面，因此在根组件清理旧版保存的凭据。
function migrateRememberedUsername() {
  const legacyLogin = localStorage.getItem('remembered_user')
  if (legacyLogin === null) return
  localStorage.removeItem('remembered_user')
  try {
    const user = JSON.parse(legacyLogin)
    if (typeof user?.username === 'string' && user.username && localStorage.getItem('remembered_username') === null) {
      localStorage.setItem('remembered_username', user.username)
    }
  } catch {
    // 损坏的旧记录已经清除，仍可继续使用现有会话。
  }
}

migrateRememberedUsername()
</script>

<template>
  <router-view />
  <CommandPalette :open="commandPaletteOpen" @close="commandPaletteOpen = false" />
  <ThemeStudioDrawer :open="themeStudioOpen" @close="themeStudioOpen = false" />
</template>
