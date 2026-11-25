import { createI18n } from 'vue-i18n'
import zh from './locales/zh'

export type AppLocale = 'zh' | 'en'

const initialLocale =
  (typeof localStorage !== 'undefined' && localStorage.getItem('lang') === 'en' ? 'en' : 'zh') as AppLocale

const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: 'zh',
  messages: {
    zh,
  } as Record<AppLocale, typeof zh>,
})

const loadedLocales = new Set<string>(['zh'])

/** 按需加载并注入英文语言包，避免默认中文用户首屏下载双份文案资源 */
export async function ensureLocale(lang: string): Promise<void> {
  if (lang === 'en' && !loadedLocales.has('en')) {
    const mod = await import('./locales/en')
    i18n.global.setLocaleMessage('en', mod.default)
    loadedLocales.add('en')
  }
}

/** 切换界面语言并持久化到 localStorage */
export async function setAppLocale(lang: AppLocale): Promise<void> {
  await ensureLocale(lang)
  i18n.global.locale.value = lang
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('lang', lang)
  }
}

export default i18n
