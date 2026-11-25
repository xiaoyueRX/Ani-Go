import { computed, ref } from 'vue'

export type ThemeName = 'anigo-dark' | 'anigo-light'
export type ThemeBgPattern = 'glow' | 'dots' | 'grid' | 'aurora' | 'none'

export interface ThemeChrome {
  radiusPanel: string
  radiusMuted: string
  radiusBtn: string
  radiusBtnSm: string
  radiusInput: string
  dockRadius: string
  blur: string
  bgPattern: ThemeBgPattern
}

export interface ThemeColorTokens {
  bg: string
  surface: string
  surface2: string
  surface3: string
  ink: string
  inkMuted: string
  inkSubtle: string
  accent: string
  accentHover: string
  accentInk: string
  emerald: string
  info: string
  success: string
  warning: string
  error: string
  border: string
  borderStrong: string
  dockBg: string
  glow?: string
}

export interface ThemeColorStyle {
  id: string
  name: string
  mode: ThemeName
  colors: ThemeColorTokens
}

export interface ThemePack {
  schema: 'anigo-theme-v1'
  id: string
  name: string
  description: string
  author: string
  version: string
  builtin?: boolean
  chrome: ThemeChrome
  styles: ThemeColorStyle[]
}

export type ThemeValidationResult =
  | { ok: true; pack: ThemePack }
  | { ok: false; error: string }

export const THEME_STORAGE_KEY = 'ani-go-theme'
export const THEME_PACK_STORAGE_KEY = 'ani-go-theme-pack'
export const THEME_STYLE_STORAGE_KEY = 'ani-go-theme-style'
export const CUSTOM_THEMES_STORAGE_KEY = 'ani-go-custom-themes'
export const THEME_VARS_STORAGE_KEY = 'ani-go-theme-vars'

export const DEFAULT_PACK_ID = 'cinema'
export const DEFAULT_DARK_STYLE_ID = 'obsidian-vermilion'
export const DEFAULT_LIGHT_STYLE_ID = 'warm-paper'

export const THEME_COLORS: Record<ThemeName, string> = {
  'anigo-dark': '#09090b',
  'anigo-light': '#f5f3ee',
}

function buildDarkTokens(overrides: Partial<ThemeColorTokens> & Pick<ThemeColorTokens, 'bg' | 'surface' | 'ink' | 'accent'>): ThemeColorTokens {
  const { bg, surface, ink, accent } = overrides
  return {
    bg,
    surface,
    surface2: overrides.surface2 || `color-mix(in srgb, ${surface} 86%, ${ink})`,
    surface3: overrides.surface3 || `color-mix(in srgb, ${surface} 74%, ${ink})`,
    ink,
    inkMuted: overrides.inkMuted || '#a1a1aa',
    inkSubtle: overrides.inkSubtle || '#71717a',
    accent,
    accentHover: overrides.accentHover || `color-mix(in srgb, ${accent} 82%, #ffffff)`,
    accentInk: overrides.accentInk || '#ffffff',
    emerald: overrides.emerald || '#10b981',
    info: overrides.info || '#0ea5e9',
    success: overrides.success || '#22c55e',
    warning: overrides.warning || '#f59e0b',
    error: overrides.error || '#ef4444',
    border: overrides.border || 'rgba(255, 255, 255, 0.08)',
    borderStrong: overrides.borderStrong || 'rgba(255, 255, 255, 0.16)',
    dockBg: overrides.dockBg || `color-mix(in srgb, ${surface} 84%, transparent)`,
    glow: overrides.glow || accent,
  }
}

function buildLightTokens(overrides: Partial<ThemeColorTokens> & Pick<ThemeColorTokens, 'bg' | 'surface' | 'ink' | 'accent'>): ThemeColorTokens {
  const { bg, surface, ink, accent } = overrides
  return {
    bg,
    surface,
    surface2: overrides.surface2 || `color-mix(in srgb, ${bg} 82%, ${surface})`,
    surface3: overrides.surface3 || `color-mix(in srgb, ${bg} 92%, ${ink})`,
    ink,
    inkMuted: overrides.inkMuted || '#52525b',
    inkSubtle: overrides.inkSubtle || '#71717a',
    accent,
    accentHover: overrides.accentHover || `color-mix(in srgb, ${accent} 85%, #000000)`,
    accentInk: overrides.accentInk || '#ffffff',
    emerald: overrides.emerald || '#059669',
    info: overrides.info || '#0284c7',
    success: overrides.success || '#16a34a',
    warning: overrides.warning || '#d97706',
    error: overrides.error || '#dc2626',
    border: overrides.border || 'rgba(20, 20, 23, 0.09)',
    borderStrong: overrides.borderStrong || 'rgba(20, 20, 23, 0.18)',
    dockBg: overrides.dockBg || `color-mix(in srgb, ${surface} 86%, transparent)`,
    glow: overrides.glow || accent,
  }
}

export const BUILTIN_THEMES: ThemePack[] = [
  {
    schema: 'anigo-theme-v1',
    id: 'cinema',
    name: '编辑部放映厅',
    description: '高对比影院海报书架搭配胶囊悬浮顶栏与剧场顶光',
    author: 'Ani-Go Official',
    version: '2.0.0',
    builtin: true,
    chrome: {
      radiusPanel: '0.875rem',
      radiusMuted: '0.75rem',
      radiusBtn: '0.625rem',
      radiusBtnSm: '0.5rem',
      radiusInput: '0.625rem',
      dockRadius: '9999px',
      blur: '20px',
      bgPattern: 'glow',
    },
    styles: [
      {
        id: 'obsidian-vermilion',
        name: '曜石朱红',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#09090b',
          surface: '#121215',
          surface2: '#18181c',
          surface3: '#222228',
          ink: '#f4f4f6',
          accent: '#ff5c28',
          accentHover: '#ff7448',
          dockBg: 'rgba(18, 18, 21, 0.84)',
        }),
      },
      {
        id: 'midnight-sakura',
        name: '午夜樱粉',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#0c0a12',
          surface: '#151221',
          surface2: '#1d192d',
          surface3: '#27223b',
          ink: '#f5f2fa',
          accent: '#f472b6',
          accentHover: '#f9a8d4',
          emerald: '#34d399',
          glow: '#ec4899',
        }),
      },
      {
        id: 'abyss-cyan',
        name: '深海电青',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#060d14',
          surface: '#0d1824',
          surface2: '#132232',
          surface3: '#1b2f45',
          ink: '#edf6fc',
          accent: '#22d3ee',
          accentHover: '#67e8f9',
          accentInk: '#06131c',
          glow: '#06b6d4',
        }),
      },
      {
        id: 'warm-paper',
        name: '暖纸画册',
        mode: 'anigo-light',
        colors: buildLightTokens({
          bg: '#f5f3ee',
          surface: '#ffffff',
          surface2: '#eae6de',
          surface3: '#dfd9cd',
          ink: '#141417',
          accent: '#e04414',
          accentHover: '#c9380b',
          dockBg: 'rgba(255, 255, 255, 0.86)',
        }),
      },
    ],
  },
  {
    schema: 'anigo-theme-v1',
    id: 'studio-grid',
    name: '精密控制台',
    description: '锐利工整的棱角与微点阵画布，面向 NAS 极客仪表盘体验',
    author: 'Ani-Go Official',
    version: '2.0.0',
    builtin: true,
    chrome: {
      radiusPanel: '0.375rem',
      radiusMuted: '0.3125rem',
      radiusBtn: '0.3125rem',
      radiusBtnSm: '0.25rem',
      radiusInput: '0.3125rem',
      dockRadius: '0.5rem',
      blur: '8px',
      bgPattern: 'dots',
    },
    styles: [
      {
        id: 'carbon-amber',
        name: '碳黑琥珀',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#0a0b0d',
          surface: '#131519',
          surface2: '#1a1d23',
          surface3: '#242932',
          ink: '#f1f3f5',
          accent: '#f59e0b',
          accentHover: '#fbbf24',
          accentInk: '#14110a',
        }),
      },
      {
        id: 'slate-indigo',
        name: '岩灰靛蓝',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#0b0f19',
          surface: '#131a2a',
          surface2: '#1a2338',
          surface3: '#24304c',
          ink: '#eef2ff',
          accent: '#6366f1',
          accentHover: '#818cf8',
        }),
      },
      {
        id: 'forest-matrix',
        name: '松林翠绿',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#07100c',
          surface: '#0e1c16',
          surface2: '#15271f',
          surface3: '#1e352b',
          ink: '#ecfdf5',
          accent: '#10b981',
          accentHover: '#34d399',
          accentInk: '#051b12',
        }),
      },
      {
        id: 'blueprint-light',
        name: '工程蓝图',
        mode: 'anigo-light',
        colors: buildLightTokens({
          bg: '#f1f5f9',
          surface: '#ffffff',
          surface2: '#e2e8f0',
          surface3: '#cbd5e1',
          ink: '#0f172a',
          accent: '#2563eb',
          accentHover: '#1d4ed8',
        }),
      },
    ],
  },
  {
    schema: 'anigo-theme-v1',
    id: 'acrylic-pop',
    name: '霓虹亚克力',
    description: '高通透毛玻璃、大圆角卡片与双色极光光晕',
    author: 'Ani-Go Official',
    version: '2.0.0',
    builtin: true,
    chrome: {
      radiusPanel: '1.25rem',
      radiusMuted: '1rem',
      radiusBtn: '9999px',
      radiusBtnSm: '9999px',
      radiusInput: '0.875rem',
      dockRadius: '9999px',
      blur: '28px',
      bgPattern: 'aurora',
    },
    styles: [
      {
        id: 'violet-neon',
        name: '星云紫霓',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#0c0918',
          surface: '#161229',
          surface2: '#201b38',
          surface3: '#2b2449',
          ink: '#f5f3ff',
          accent: '#a855f7',
          accentHover: '#c084fc',
          emerald: '#2dd4bf',
          glow: '#9333ea',
        }),
      },
      {
        id: 'sunset-coral',
        name: '日落珊瑚',
        mode: 'anigo-dark',
        colors: buildDarkTokens({
          bg: '#140c10',
          surface: '#21151b',
          surface2: '#2c1c24',
          surface3: '#3a252f',
          ink: '#fff1f2',
          accent: '#fb7185',
          accentHover: '#fda4af',
          accentInk: '#1d0b10',
          emerald: '#fbbf24',
          glow: '#f43f5e',
        }),
      },
      {
        id: 'matcha-mint',
        name: '抹茶薄荷',
        mode: 'anigo-light',
        colors: buildLightTokens({
          bg: '#eef6f1',
          surface: '#ffffff',
          surface2: '#dff0e6',
          surface3: '#cbe4d6',
          ink: '#0f2419',
          accent: '#0d9488',
          accentHover: '#0f766e',
          emerald: '#16a34a',
          glow: '#14b8a6',
        }),
      },
      {
        id: 'sakura-milk',
        name: '樱花奶霜',
        mode: 'anigo-light',
        colors: buildLightTokens({
          bg: '#fdf4f7',
          surface: '#ffffff',
          surface2: '#fbe8ef',
          surface3: '#f6d6e3',
          ink: '#2a121e',
          accent: '#ec4899',
          accentHover: '#db2777',
          emerald: '#10b981',
          glow: '#f472b6',
        }),
      },
    ],
  },
]

export const THIRD_PARTY_THEME_TEMPLATE: ThemePack = {
  schema: 'anigo-theme-v1',
  id: 'neo-tokyo-custom',
  name: '赛博新东京 (示例)',
  description: '带 3 套色彩风格的第三方主题示例，支持自定义圆角、点阵网格与多配色切换',
  author: 'Community Author',
  version: '1.0.0',
  chrome: {
    radiusPanel: '0.75rem',
    radiusMuted: '0.625rem',
    radiusBtn: '0.5rem',
    radiusBtnSm: '0.375rem',
    radiusInput: '0.5rem',
    dockRadius: '0.875rem',
    blur: '16px',
    bgPattern: 'grid',
  },
  styles: [
    {
      id: 'night-city-gold',
      name: '夜之城鎏金',
      mode: 'anigo-dark',
      colors: buildDarkTokens({
        bg: '#090a0f',
        surface: '#12151f',
        surface2: '#191d2b',
        surface3: '#22283a',
        ink: '#f4f6fb',
        accent: '#eab308',
        accentHover: '#facc15',
        accentInk: '#141105',
        glow: '#eab308',
      }),
    },
    {
      id: 'neon-magenta',
      name: '霓虹洋红',
      mode: 'anigo-dark',
      colors: buildDarkTokens({
        bg: '#0d0814',
        surface: '#181124',
        surface2: '#211832',
        surface3: '#2d2144',
        ink: '#faf5ff',
        accent: '#e879f9',
        accentHover: '#f0abfc',
        accentInk: '#1c0922',
        glow: '#d946ef',
      }),
    },
    {
      id: 'silver-daylight',
      name: '银翼白昼',
      mode: 'anigo-light',
      colors: buildLightTokens({
        bg: '#eff3f8',
        surface: '#ffffff',
        surface2: '#e1e8f0',
        surface3: '#cfd9e6',
        ink: '#101827',
        accent: '#0284c7',
        accentHover: '#0369a1',
        glow: '#38bdf8',
      }),
    },
  ],
}

const BUILTIN_IDS = new Set(BUILTIN_THEMES.map(t => t.id))
const ID_RE = /^[a-z0-9][a-z0-9_-]{1,47}$/
const HEX_COLOR_RE = /^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/
const FUNC_COLOR_RE = /^(?:rgb|rgba|hsl|hsla|oklch)\(\s*[0-9a-zA-Z.%+\-\s,/]+\)$/
const UNIT_RE = /^\d+(?:\.\d+)?(?:px|rem)$/
const PATTERNS = new Set<ThemeBgPattern>(['glow', 'dots', 'grid', 'aurora', 'none'])

export function isSafeColor(val: unknown): val is string {
  if (typeof val !== 'string') return false
  const s = val.trim()
  if (!s || s.length > 80) return false
  if (/[;{}<>\\]|url\(|expression|var\(/i.test(s)) return false
  return HEX_COLOR_RE.test(s) || FUNC_COLOR_RE.test(s)
}

function isSafeUnit(val: unknown): val is string {
  if (typeof val !== 'string') return false
  return UNIT_RE.test(val.trim())
}

function cleanText(val: unknown, fallback: string, maxLen = 64): string {
  if (typeof val !== 'string') return fallback
  const trimmed = val.trim()
  return trimmed ? trimmed.slice(0, maxLen) : fallback
}

export function validateThemePack(raw: unknown): ThemeValidationResult {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return { ok: false, error: '主题文件必须是一个 JSON 对象。' }
  }
  const obj = raw as Record<string, unknown>
  if (obj.schema !== 'anigo-theme-v1') {
    return { ok: false, error: '不支持的主题协议：schema 必须为 "anigo-theme-v1"。' }
  }
  if (typeof obj.id !== 'string' || !ID_RE.test(obj.id)) {
    return { ok: false, error: '主题 id 须为 2–48 位小写字母、数字、连字符或下划线。' }
  }
  if (BUILTIN_IDS.has(obj.id)) {
    return { ok: false, error: `主题 id "${obj.id}" 与官方内置主题冲突，请改用其他唯一标识。` }
  }
  if (typeof obj.name !== 'string' || !obj.name.trim()) {
    return { ok: false, error: '请填写主题名称（name）。' }
  }

  const rawChrome = (obj.chrome && typeof obj.chrome === 'object' && !Array.isArray(obj.chrome))
    ? (obj.chrome as Record<string, unknown>)
    : {}
  const defChrome = BUILTIN_THEMES[0].chrome
  const bgPattern = typeof rawChrome.bgPattern === 'string' && PATTERNS.has(rawChrome.bgPattern as ThemeBgPattern)
    ? (rawChrome.bgPattern as ThemeBgPattern)
    : defChrome.bgPattern

  const chrome: ThemeChrome = {
    radiusPanel: isSafeUnit(rawChrome.radiusPanel) ? rawChrome.radiusPanel.trim() : defChrome.radiusPanel,
    radiusMuted: isSafeUnit(rawChrome.radiusMuted) ? rawChrome.radiusMuted.trim() : defChrome.radiusMuted,
    radiusBtn: isSafeUnit(rawChrome.radiusBtn) ? rawChrome.radiusBtn.trim() : defChrome.radiusBtn,
    radiusBtnSm: isSafeUnit(rawChrome.radiusBtnSm) ? rawChrome.radiusBtnSm.trim() : defChrome.radiusBtnSm,
    radiusInput: isSafeUnit(rawChrome.radiusInput) ? rawChrome.radiusInput.trim() : defChrome.radiusInput,
    dockRadius: isSafeUnit(rawChrome.dockRadius) ? rawChrome.dockRadius.trim() : defChrome.dockRadius,
    blur: isSafeUnit(rawChrome.blur) ? rawChrome.blur.trim() : defChrome.blur,
    bgPattern,
  }

  if (!Array.isArray(obj.styles) || obj.styles.length === 0) {
    return { ok: false, error: '主题必须包含至少 1 套色彩风格（styles 数组）。' }
  }
  if (obj.styles.length > 12) {
    return { ok: false, error: '单个主题最多支持 12 套色彩风格。' }
  }

  const seenStyleIds = new Set<string>()
  const normalizedStyles: ThemeColorStyle[] = []
  for (let i = 0; i < obj.styles.length; i++) {
    const item = obj.styles[i]
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      return { ok: false, error: `第 ${i + 1} 个色彩风格格式无效。` }
    }
    const st = item as Record<string, unknown>
    if (typeof st.id !== 'string' || !ID_RE.test(st.id)) {
      return { ok: false, error: `第 ${i + 1} 个色彩风格 id "${String(st.id || '')}" 格式无效。` }
    }
    if (seenStyleIds.has(st.id)) {
      return { ok: false, error: `存在重复的色彩风格 id "${st.id}"。` }
    }
    seenStyleIds.add(st.id)

    if (typeof st.name !== 'string' || !st.name.trim()) {
      return { ok: false, error: `色彩风格 "${st.id}" 缺少名称（name）。` }
    }
    const mode: ThemeName = st.mode === 'anigo-light' ? 'anigo-light' : 'anigo-dark'
    if (!st.colors || typeof st.colors !== 'object' || Array.isArray(st.colors)) {
      return { ok: false, error: `色彩风格 "${st.name}" 缺少 colors 对象。` }
    }
    const c = st.colors as Record<string, unknown>
    for (const reqKey of ['bg', 'surface', 'ink', 'accent'] as const) {
      if (!isSafeColor(c[reqKey])) {
        return { ok: false, error: `色彩风格 "${st.name}" 的 ${reqKey} 色值无效或不安全（仅允许 Hex / RGB / HSL / OKLCH）。` }
      }
    }

    const opt = (k: keyof ThemeColorTokens) => (isSafeColor(c[k]) ? String(c[k]).trim() : undefined)
    const builder = mode === 'anigo-light' ? buildLightTokens : buildDarkTokens
    const colors = builder({
      bg: String(c.bg).trim(),
      surface: String(c.surface).trim(),
      ink: String(c.ink).trim(),
      accent: String(c.accent).trim(),
      surface2: opt('surface2'),
      surface3: opt('surface3'),
      inkMuted: opt('inkMuted'),
      inkSubtle: opt('inkSubtle'),
      accentHover: opt('accentHover'),
      accentInk: opt('accentInk'),
      emerald: opt('emerald'),
      info: opt('info'),
      success: opt('success'),
      warning: opt('warning'),
      error: opt('error'),
      border: opt('border'),
      borderStrong: opt('borderStrong'),
      dockBg: opt('dockBg'),
      glow: opt('glow'),
    })

    normalizedStyles.push({
      id: st.id,
      name: cleanText(st.name, st.id, 40),
      mode,
      colors,
    })
  }

  return {
    ok: true,
    pack: {
      schema: 'anigo-theme-v1',
      id: obj.id,
      name: cleanText(obj.name, obj.id, 48),
      description: cleanText(obj.description, '第三方自定义主题', 140),
      author: cleanText(obj.author, 'Community', 48),
      version: cleanText(obj.version, '1.0.0', 24),
      builtin: false,
      chrome,
      styles: normalizedStyles,
    },
  }
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

function readStoredCustomThemes(): ThemePack[] {
  if (typeof localStorage === 'undefined') return []
  try {
    const raw = localStorage.getItem(CUSTOM_THEMES_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const out: ThemePack[] = []
    for (const item of parsed) {
      const res = validateThemePack(item)
      if (res.ok) out.push(res.pack)
    }
    return out
  } catch {
    return []
  }
}

function saveStoredCustomThemes(list: ThemePack[]) {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(CUSTOM_THEMES_STORAGE_KEY, JSON.stringify(list))
  } catch {}
}

function buildBackgroundStyles(pattern: ThemeBgPattern, colors: ThemeColorTokens) {
  const glowColor = colors.glow || colors.accent
  if (pattern === 'dots') {
    return {
      bgImage: `radial-gradient(color-mix(in srgb, ${colors.ink} 12%, transparent) 1px, transparent 1px)`,
      bgSize: '22px 22px',
      bgRepeat: 'repeat',
    }
  }
  if (pattern === 'grid') {
    return {
      bgImage: `linear-gradient(to right, color-mix(in srgb, ${colors.ink} 6%, transparent) 1px, transparent 1px), linear-gradient(to bottom, color-mix(in srgb, ${colors.ink} 6%, transparent) 1px, transparent 1px)`,
      bgSize: '32px 32px',
      bgRepeat: 'repeat',
    }
  }
  if (pattern === 'aurora') {
    return {
      bgImage: `radial-gradient(ellipse 65% 42% at 15% -5%, color-mix(in srgb, ${glowColor} 22%, transparent), transparent), radial-gradient(ellipse 55% 40% at 88% 8%, color-mix(in srgb, ${colors.emerald} 14%, transparent), transparent)`,
      bgSize: 'auto',
      bgRepeat: 'no-repeat',
    }
  }
  if (pattern === 'none') {
    return {
      bgImage: 'none',
      bgSize: 'auto',
      bgRepeat: 'no-repeat',
    }
  }
  return {
    bgImage: `radial-gradient(ellipse 70% 34% at 50% -10%, color-mix(in srgb, ${glowColor} 14%, transparent), transparent)`,
    bgSize: 'auto',
    bgRepeat: 'no-repeat',
  }
}

export function buildCssVarMap(pack: ThemePack, style: ThemeColorStyle): Record<string, string> {
  const c = style.colors
  const bg = buildBackgroundStyles(pack.chrome.bgPattern, c)
  const isLight = style.mode === 'anigo-light'
  return {
    '--v2-bg': c.bg,
    '--v2-surface': c.surface,
    '--v2-surface-2': c.surface2,
    '--v2-surface-3': c.surface3,
    '--v2-ink': c.ink,
    '--v2-ink-muted': c.inkMuted,
    '--v2-ink-subtle': c.inkSubtle,
    '--v2-accent': c.accent,
    '--v2-accent-hover': c.accentHover,
    '--v2-accent-ink': c.accentInk,
    '--v2-emerald': c.emerald,
    '--v2-info': c.info,
    '--v2-success': c.success,
    '--v2-warning': c.warning,
    '--v2-error': c.error,
    '--v2-neutral': isLight ? '#27272a' : '#232329',
    '--v2-neutral-ink': isLight ? '#f4f4f5' : '#e4e4e7',
    '--v2-border': c.border,
    '--v2-border-strong': c.borderStrong,
    '--v2-dock-bg': c.dockBg,
    '--v2-radius-panel': pack.chrome.radiusPanel,
    '--v2-radius-muted': pack.chrome.radiusMuted,
    '--v2-radius-btn': pack.chrome.radiusBtn,
    '--v2-radius-btn-sm': pack.chrome.radiusBtnSm,
    '--v2-radius-input': pack.chrome.radiusInput,
    '--v2-dock-radius': pack.chrome.dockRadius,
    '--v2-blur': pack.chrome.blur,
    '--v2-bg-image': bg.bgImage,
    '--v2-bg-size': bg.bgSize,
    '--v2-bg-repeat': bg.bgRepeat,
  }
}

function findPackAndStyle(packs: ThemePack[], packId?: string | null, styleId?: string | null): { pack: ThemePack; style: ThemeColorStyle } {
  const pack = packs.find(p => p.id === packId) || packs[0]
  const style = pack.styles.find(s => s.id === styleId) || pack.styles[0]
  return { pack, style }
}

const customThemesRef = ref<ThemePack[]>(readStoredCustomThemes())
const allThemesRef = computed<ThemePack[]>(() => [...BUILTIN_THEMES, ...customThemesRef.value])

function readInitialPackAndStyle(): { packId: string; styleId: string; mode: ThemeName } {
  if (typeof localStorage === 'undefined') {
    return { packId: DEFAULT_PACK_ID, styleId: DEFAULT_DARK_STYLE_ID, mode: 'anigo-dark' }
  }
  try {
    const rawPack = localStorage.getItem(THEME_PACK_STORAGE_KEY)
    const rawStyle = localStorage.getItem(THEME_STYLE_STORAGE_KEY)
    const rawMode = localStorage.getItem(THEME_STORAGE_KEY)
    if (!rawPack && !rawStyle) {
      const mode = normalizeTheme(rawMode)
      return {
        packId: DEFAULT_PACK_ID,
        styleId: mode === 'anigo-light' ? DEFAULT_LIGHT_STYLE_ID : DEFAULT_DARK_STYLE_ID,
        mode,
      }
    }
    const { pack, style } = findPackAndStyle(allThemesRef.value, rawPack, rawStyle)
    return { packId: pack.id, styleId: style.id, mode: style.mode }
  } catch {
    return { packId: DEFAULT_PACK_ID, styleId: DEFAULT_DARK_STYLE_ID, mode: 'anigo-dark' }
  }
}

const initialSelection = readInitialPackAndStyle()
const currentPackId = ref<string>(initialSelection.packId)
const currentStyleId = ref<string>(initialSelection.styleId)
const currentTheme = ref<ThemeName>(initialSelection.mode)

export function applyThemePack(pack: ThemePack, style: ThemeColorStyle) {
  const varMap = buildCssVarMap(pack, style)
  if (typeof document !== 'undefined') {
    const root = document.documentElement
    root?.setAttribute('data-theme', style.mode)
    root?.setAttribute('data-theme-pack', pack.id)
    root?.setAttribute('data-theme-style', style.id)
    if (root && 'style' in root && root.style?.setProperty) {
      for (const [k, v] of Object.entries(varMap)) {
        root.style.setProperty(k, v)
      }
    }
    const meta = document.querySelector?.('meta[name="theme-color"]')
    if (meta) {
      meta.setAttribute('content', style.colors.bg)
    }
  }
  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, style.mode)
      localStorage.setItem(THEME_PACK_STORAGE_KEY, pack.id)
      localStorage.setItem(THEME_STYLE_STORAGE_KEY, style.id)
      localStorage.setItem(THEME_VARS_STORAGE_KEY, JSON.stringify(varMap))
    } catch {
      // 忽略受限环境写入失败，保持内存主题状态
    }
  }
  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent?.(new CustomEvent('anigo-theme-changed', {
      detail: { mode: style.mode, packId: pack.id, styleId: style.id },
    }))
  }
}

export function applyTheme(next: ThemeName) {
  const { pack, style } = findPackAndStyle(
    allThemesRef.value,
    DEFAULT_PACK_ID,
    next === 'anigo-light' ? DEFAULT_LIGHT_STYLE_ID : DEFAULT_DARK_STYLE_ID,
  )
  currentPackId.value = pack.id
  currentStyleId.value = style.id
  currentTheme.value = style.mode
  applyThemePack(pack, style)
}

export function useTheme() {
  const activeSelection = computed(() => findPackAndStyle(allThemesRef.value, currentPackId.value, currentStyleId.value))
  const activePack = computed(() => activeSelection.value.pack)
  const activeStyle = computed(() => activeSelection.value.style)
  const isDark = computed(() => currentTheme.value === 'anigo-dark')

  function selectTheme(packId: string, styleId?: string) {
    const { pack, style } = findPackAndStyle(allThemesRef.value, packId, styleId)
    currentPackId.value = pack.id
    currentStyleId.value = style.id
    currentTheme.value = style.mode
    applyThemePack(pack, style)
    return { pack, style }
  }

  function selectStyle(styleId: string) {
    return selectTheme(currentPackId.value, styleId)
  }

  function setTheme(next: ThemeName) {
    const normalized = normalizeTheme(next)
    const pack = activePack.value
    const matching = pack.styles.find(s => s.mode === normalized)
    if (matching) {
      return selectTheme(pack.id, matching.id).style.mode
    }
    return selectTheme(
      DEFAULT_PACK_ID,
      normalized === 'anigo-light' ? DEFAULT_LIGHT_STYLE_ID : DEFAULT_DARK_STYLE_ID,
    ).style.mode
  }

  function syncTheme() {
    customThemesRef.value = readStoredCustomThemes()
    const restored = readInitialPackAndStyle()
    return selectTheme(restored.packId, restored.styleId).style.mode
  }

  function toggleTheme() {
    return setTheme(currentTheme.value === 'anigo-dark' ? 'anigo-light' : 'anigo-dark')
  }

  function importCustomTheme(rawInput: string | unknown): ThemeValidationResult {
    let parsed: unknown = rawInput
    if (typeof rawInput === 'string') {
      try {
        parsed = JSON.parse(rawInput)
      } catch {
        return { ok: false, error: 'JSON 语法解析失败，请检查括号与引号格式。' }
      }
    }
    const validation = validateThemePack(parsed)
    if (!validation.ok) return validation

    const idx = customThemesRef.value.findIndex(t => t.id === validation.pack.id)
    const nextList = [...customThemesRef.value]
    if (idx >= 0) nextList[idx] = validation.pack
    else nextList.push(validation.pack)
    customThemesRef.value = nextList
    saveStoredCustomThemes(nextList)
    return validation
  }

  function removeCustomTheme(packId: string) {
    const nextList = customThemesRef.value.filter(t => t.id !== packId)
    customThemesRef.value = nextList
    saveStoredCustomThemes(nextList)
    if (currentPackId.value === packId) {
      selectTheme(DEFAULT_PACK_ID, DEFAULT_DARK_STYLE_ID)
    }
  }

  function exportThemeJson(packId: string): string {
    const found = allThemesRef.value.find(t => t.id === packId)
    if (!found) return ''
    const { builtin: _builtin, ...clean } = found
    return JSON.stringify(clean, null, 2)
  }

  function getServerSettingsPayload(): Record<string, string> {
    return {
      UI_THEME_PACK: currentPackId.value,
      UI_THEME_STYLE: currentStyleId.value,
      UI_CUSTOM_THEMES: JSON.stringify(customThemesRef.value.map(({ builtin: _b, ...rest }) => rest)),
    }
  }

  function hydrateFromServerSettings(settings: Record<string, string> | undefined | null) {
    if (!settings || typeof settings !== 'object') return
    if (typeof settings.UI_CUSTOM_THEMES === 'string' && settings.UI_CUSTOM_THEMES.trim()) {
      try {
        const parsed = JSON.parse(settings.UI_CUSTOM_THEMES)
        if (Array.isArray(parsed)) {
          const merged = new Map<string, ThemePack>()
          for (const existing of customThemesRef.value) merged.set(existing.id, existing)
          for (const raw of parsed) {
            const res = validateThemePack(raw)
            if (res.ok) merged.set(res.pack.id, res.pack)
          }
          customThemesRef.value = Array.from(merged.values())
          saveStoredCustomThemes(customThemesRef.value)
        }
      } catch {}
    }

    const srvPack = settings.UI_THEME_PACK?.trim()
    const srvStyle = settings.UI_THEME_STYLE?.trim()
    if (srvPack && allThemesRef.value.some(p => p.id === srvPack)) {
      selectTheme(srvPack, srvStyle)
    }
  }

  return {
    theme: currentTheme,
    isDark,
    setTheme,
    syncTheme,
    toggleTheme,
    allThemes: allThemesRef,
    builtinThemes: BUILTIN_THEMES,
    customThemes: customThemesRef,
    activePack,
    activeStyle,
    activePackId: currentPackId,
    activeStyleId: currentStyleId,
    selectTheme,
    selectStyle,
    importCustomTheme,
    removeCustomTheme,
    exportThemeJson,
    hydrateFromServerSettings,
    getServerSettingsPayload,
  }
}
