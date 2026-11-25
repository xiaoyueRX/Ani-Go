import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { installAssetRecovery } from './router/assetRecovery'
import i18n, { ensureLocale } from './i18n'
import { useTheme } from './composables/useTheme'

useTheme().syncTheme()
installAssetRecovery(router)

async function bootstrap() {
  const initialLocale = localStorage.getItem('lang') || 'zh'
  try {
    await ensureLocale(initialLocale)
  } catch {
    // 英文包拉取失败时保留内置中文兜底，避免阻塞应用挂载
  }
  createApp(App).use(router).use(i18n).mount('#app')
}

void bootstrap()
