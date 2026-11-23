<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { loginDestination } from './utils/loginRedirect'

const route = useRoute()
const router = useRouter()

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

onMounted(() => window.addEventListener('storage', syncSession))
onUnmounted(() => window.removeEventListener('storage', syncSession))

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
</template>
