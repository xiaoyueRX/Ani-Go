import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'

const isAuthenticated = ref(false)
const token = ref<string | null>(null)
const isLoading = ref(true)

let initPromise: Promise<void> | null = null

function getToken(): string | null {
  if (token.value) return token.value
  const stored = localStorage.getItem('token')
  if (stored) {
    token.value = stored
  }
  return token.value
}

function setToken(newToken: string | null) {
  token.value = newToken
  if (newToken) {
    localStorage.setItem('token', newToken)
    isAuthenticated.value = true
  } else {
    localStorage.removeItem('token')
    isAuthenticated.value = false
  }
}

async function checkAuth(): Promise<boolean> {
  const t = getToken()
  if (!t) {
    isAuthenticated.value = false
    isLoading.value = false
    return false
  }

  try {
    // 验证 token 有效性
    const response = await fetch('/api/me', {
      headers: {
        'Authorization': `Bearer ${t}`,
        'Content-Type': 'application/json'
      }
    })
    
    if (response.ok) {
      isAuthenticated.value = true
      return true
    } else if (response.status === 401) {
      // Token 过期
      setToken(null)
      return false
    }
    return false
  } catch {
    // 网络错误或服务不可用，降级为离线模式
    // 保留 token 但不标记为已认证
    isAuthenticated.value = false
    return false
  } finally {
    isLoading.value = false
  }
}

async function initAuth(): Promise<void> {
  if (initPromise) return initPromise
  
  initPromise = (async () => {
    await checkAuth()
  })()
  
  return initPromise
}

function logout() {
  setToken(null)
  const router = useRouter()
  router.push('/login')
}

// 兜底模式：隐私模式/离线模式/token过期时仍可浏览公开数据
const isOfflineMode = computed(() => !isAuthenticated.value && !isLoading.value)

export function useAuth() {
  return {
    isAuthenticated: computed(() => isAuthenticated.value),
    isLoading: computed(() => isLoading.value),
    isOfflineMode,
    token: computed(() => token.value),
    initAuth,
    checkAuth,
    setToken,
    logout,
    getToken
  }
}