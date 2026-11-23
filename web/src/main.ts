import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { installAssetRecovery } from './router/assetRecovery'
import i18n from './i18n'

installAssetRecovery(router)
createApp(App).use(router).use(i18n).mount('#app')
