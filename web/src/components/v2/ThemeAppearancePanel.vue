<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Palette, Check, Download, Trash2, Upload,
  Sparkles, FileCode2, AlertTriangle, Sun, Moon
} from 'lucide-vue-next'
import request from '../../utils/request'
import {
  useTheme,
  THIRD_PARTY_THEME_TEMPLATE,
  type ThemePack,
} from '../../composables/useTheme'

const { locale } = useI18n()
const {
  allThemes,
  activePack,
  activeStyle,
  activePackId,
  activeStyleId,
  selectTheme,
  importCustomTheme,
  removeCustomTheme,
  exportThemeJson,
  getServerSettingsPayload,
} = useTheme()

const jsonInput = ref('')
const importError = ref('')
const importSuccess = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)

async function persistThemesToServer() {
  if (!localStorage.getItem('token')) return
  try {
    await request.put('/settings', { settings: getServerSettingsPayload() })
  } catch {
    // 忽略临时保存异常，本地持久化已立即完成
  }
}

function pickThemeStyle(packId: string, styleId: string) {
  selectTheme(packId, styleId)
  void persistThemesToServer()
  window.showToast?.(
    locale.value === 'zh'
      ? `已切换：${activePack.value.name} · ${activeStyle.value.name}`
      : `Applied: ${activePack.value.name} · ${activeStyle.value.name}`,
    'success',
  )
}

function previewColorsFor(pack: ThemePack) {
  if (pack.id === activePackId.value) return activeStyle.value.colors
  return pack.styles[0].colors
}

function fillTemplateJson() {
  importError.value = ''
  importSuccess.value = ''
  jsonInput.value = JSON.stringify(THIRD_PARTY_THEME_TEMPLATE, null, 2)
}

function handleImportJson(autoApply = true) {
  importError.value = ''
  importSuccess.value = ''
  const trimmed = jsonInput.value.trim()
  if (!trimmed) {
    importError.value = '请先粘贴主题 JSON，或点击「填入多色彩示例模板」体验。'
    return
  }
  const result = importCustomTheme(trimmed)
  if (!result.ok) {
    importError.value = result.error
    return
  }
  if (autoApply) {
    selectTheme(result.pack.id, result.pack.styles[0].id)
  }
  void persistThemesToServer()
  importSuccess.value = `成功导入第三方主题「${result.pack.name}」（含 ${result.pack.styles.length} 套色彩风格）并已保存。`
  window.showToast?.(importSuccess.value, 'success')
}

function triggerFileImport() {
  fileInputRef.value?.click()
}

function onFileSelected(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  input.value = ''
  const reader = new FileReader()
  reader.onload = () => {
    jsonInput.value = String(reader.result || '')
    handleImportJson(true)
  }
  reader.onerror = () => {
    importError.value = '读取主题文件失败，请确认文件为 UTF-8 编码的 JSON。'
  }
  reader.readAsText(file, 'utf-8')
}

function handleExport(pack: ThemePack) {
  const text = exportThemeJson(pack.id)
  if (!text) return
  const blob = new Blob([text], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `anigo-theme-${pack.id}.json`
  a.click()
  URL.revokeObjectURL(url)
  window.showToast?.(`已导出主题包：anigo-theme-${pack.id}.json`, 'success')
}

function handleRemove(pack: ThemePack) {
  if (!confirm(`确定要移除第三方主题「${pack.name}」吗？`)) return
  removeCustomTheme(pack.id)
  void persistThemesToServer()
  window.showToast?.(`已移除第三方主题「${pack.name}」`, 'info')
}
</script>

<template>
  <div class="space-y-5">
    <!-- 主题与多色彩画廊 -->
    <div class="v2-panel p-5 sm:p-6 space-y-5">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-[var(--v2-border)] pb-4">
        <div>
          <h3 class="text-sm font-bold flex items-center gap-2 text-[var(--v2-ink)]">
            <Palette :size="16" class="text-[var(--v2-accent)]" />
            <span>{{ locale === 'zh' ? '主题包与多色彩风格' : 'Theme Packs & Color Styles' }}</span>
          </h3>
          <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">
            {{ locale === 'zh'
              ? '每套主题拥有独立的圆角、玻璃质感与背景纹理，并内置多种深/浅色彩风格可即时切换。'
              : 'Each theme defines surface geometry and backdrop texture with multiple dark and light palettes.' }}
          </p>
        </div>

        <div class="v2-pill text-xs font-medium">
          <span>{{ locale === 'zh' ? '当前：' : 'Active: ' }}</span>
          <strong class="text-[var(--v2-accent)]">{{ activePack.name }}</strong>
          <span>/</span>
          <span>{{ activeStyle.name }}</span>
        </div>
      </div>

      <div class="grid gap-4">
        <div
          v-for="pack in allThemes"
          :key="pack.id"
          class="v2-panel-muted p-4 sm:p-5 space-y-4 border transition-colors"
          :class="activePackId === pack.id ? 'border-[var(--v2-accent)]/60' : 'border-[var(--v2-border)]'"
        >
          <div class="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
            <div class="space-y-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <h4 class="text-sm font-bold text-[var(--v2-ink)]">{{ pack.name }}</h4>
                <span
                  class="text-[10px] font-semibold px-2 py-0.5 rounded-full"
                  :class="pack.builtin ? 'bg-[var(--v2-accent)]/15 text-[var(--v2-accent)]' : 'bg-emerald-500/15 text-emerald-400'"
                >
                  {{ pack.builtin ? '官方预设' : '第三方主题' }}
                </span>
                <span class="v2-pill font-mono text-[10px]">v{{ pack.version }}</span>
                <span class="text-[11px] text-[var(--v2-ink-subtle)]">by {{ pack.author }}</span>
              </div>
              <p class="text-xs text-[var(--v2-ink-muted)]">{{ pack.description }}</p>
            </div>

            <div class="flex items-center gap-1.5 shrink-0">
              <button type="button" class="v2-btn v2-btn-sm" @click="handleExport(pack)">
                <Download :size="13" />
                <span>导出 JSON</span>
              </button>
              <button
                v-if="!pack.builtin"
                type="button"
                class="v2-btn v2-btn-sm text-red-400 hover:border-red-500/40"
                @click="handleRemove(pack)"
              >
                <Trash2 :size="13" />
                <span>删除</span>
              </button>
            </div>
          </div>

          <div class="grid lg:grid-cols-12 gap-4 items-center">
            <!-- 微缩实时预览框 -->
            <div
              class="lg:col-span-5 p-3 border"
              :style="{
                backgroundColor: previewColorsFor(pack).bg,
                borderColor: previewColorsFor(pack).borderStrong,
                borderRadius: pack.chrome.radiusPanel,
              }"
            >
              <div class="flex items-center justify-between gap-2 mb-2.5">
                <div class="flex items-center gap-1.5">
                  <span class="w-2.5 h-2.5 rounded-full" :style="{ backgroundColor: previewColorsFor(pack).accent }"></span>
                  <span
                    class="h-3 w-20 inline-block"
                    :style="{
                      backgroundColor: previewColorsFor(pack).surface2,
                      borderRadius: pack.chrome.dockRadius,
                      border: `1px solid ${previewColorsFor(pack).border}`,
                    }"
                  ></span>
                </div>
                <span
                  class="px-2 py-0.5 text-[10px] font-bold"
                  :style="{
                    backgroundColor: previewColorsFor(pack).accent,
                    color: previewColorsFor(pack).accentInk,
                    borderRadius: pack.chrome.radiusBtnSm,
                  }"
                >
                  Action
                </span>
              </div>
              <div class="grid grid-cols-3 gap-2">
                <div
                  v-for="idx in 3"
                  :key="idx"
                  class="h-10 p-1.5 border flex flex-col justify-between"
                  :style="{
                    backgroundColor: previewColorsFor(pack).surface,
                    borderColor: idx === 1 ? previewColorsFor(pack).accent : previewColorsFor(pack).border,
                    borderRadius: pack.chrome.radiusMuted,
                  }"
                >
                  <span class="block h-1.5 w-8 rounded-full" :style="{ backgroundColor: previewColorsFor(pack).ink, opacity: 0.75 }"></span>
                  <span class="block h-1 w-full rounded-full" :style="{ backgroundColor: idx === 1 ? previewColorsFor(pack).accent : previewColorsFor(pack).surface3 }"></span>
                </div>
              </div>
            </div>

            <!-- 多色彩风格开关网格 -->
            <div class="lg:col-span-7 grid grid-cols-2 gap-2">
              <button
                v-for="st in pack.styles"
                :key="st.id"
                type="button"
                @click="pickThemeStyle(pack.id, st.id)"
                class="p-2.5 rounded-xl border text-left flex items-center justify-between gap-2 transition-all cursor-pointer"
                :class="activePackId === pack.id && activeStyleId === st.id
                  ? 'bg-[var(--v2-surface)] border-[var(--v2-accent)] ring-2 ring-[var(--v2-accent)]/20'
                  : 'bg-[var(--v2-surface)]/70 border-[var(--v2-border)] hover:border-[var(--v2-border-strong)]'"
              >
                <div class="flex items-center gap-2.5 min-w-0">
                  <div class="flex items-center -space-x-1 shrink-0">
                    <span class="w-4 h-4 rounded-full border border-white/25" :style="{ backgroundColor: st.colors.bg }"></span>
                    <span class="w-4 h-4 rounded-full border border-white/25" :style="{ backgroundColor: st.colors.accent }"></span>
                  </div>
                  <div class="min-w-0">
                    <div class="text-xs font-semibold text-[var(--v2-ink)] truncate">{{ st.name }}</div>
                    <div class="text-[10px] text-[var(--v2-ink-subtle)] flex items-center gap-1">
                      <Sun v-if="st.mode === 'anigo-light'" :size="10" />
                      <Moon v-else :size="10" />
                      <span>{{ st.mode === 'anigo-light' ? '亮色' : '暗色' }}</span>
                    </div>
                  </div>
                </div>
                <Check
                  v-if="activePackId === pack.id && activeStyleId === st.id"
                  :size="14"
                  class="text-[var(--v2-accent)] shrink-0"
                />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 第三方主题导入工坊 -->
    <div class="v2-panel p-5 sm:p-6 space-y-4">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-[var(--v2-border)] pb-4">
        <div>
          <h3 class="text-sm font-bold flex items-center gap-2 text-[var(--v2-ink)]">
            <Sparkles :size="16" class="text-[var(--v2-accent)]" />
            <span>{{ locale === 'zh' ? '第三方主题导入工坊' : '3rd-Party Theme Importer' }}</span>
          </h3>
          <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">
            支持导入遵循 <code class="font-mono text-[var(--v2-accent)]">anigo-theme-v1</code> 规范的 JSON 主题包，单个主题可包含最多 12 套自定义色彩风格。
          </p>
        </div>

        <div class="flex items-center gap-2 shrink-0 flex-wrap">
          <input ref="fileInputRef" type="file" accept=".json,application/json" class="hidden" @change="onFileSelected" />
          <button type="button" class="v2-btn v2-btn-sm" @click="triggerFileImport">
            <Upload :size="13" />
            <span>上传 .json 文件</span>
          </button>
          <button type="button" class="v2-btn v2-btn-sm" @click="fillTemplateJson">
            <FileCode2 :size="13" />
            <span>填入多色彩示例模板</span>
          </button>
        </div>
      </div>

      <textarea
        v-model="jsonInput"
        rows="9"
        spellcheck="false"
        placeholder='粘贴 schema: "anigo-theme-v1" 的主题 JSON，或点击上方「填入多色彩示例模板」查看结构…'
        class="w-full rounded-xl p-3.5 font-mono text-xs bg-[var(--v2-surface-2)] border border-[var(--v2-border-strong)] text-[var(--v2-ink)] focus:outline-none focus:border-[var(--v2-accent)] leading-relaxed"
      ></textarea>

      <div v-if="importError" class="v2-panel-muted border border-red-500/35 bg-red-500/10 text-red-400 p-3 flex items-center gap-2 text-xs">
        <AlertTriangle :size="15" class="shrink-0" />
        <span>{{ importError }}</span>
      </div>

      <div v-if="importSuccess" class="v2-panel-muted border border-emerald-500/35 bg-emerald-500/10 text-emerald-400 p-3 flex items-center gap-2 text-xs">
        <Check :size="15" class="shrink-0" />
        <span>{{ importSuccess }}</span>
      </div>

      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-1">
        <p class="text-[11px] text-[var(--v2-ink-subtle)]">
          安全说明：系统仅解析颜色值与尺寸变量，自动补齐次级界面色，拒绝任意 CSS 注入。
        </p>
        <button type="button" class="v2-btn v2-btn-primary shrink-0" @click="handleImportJson(true)">
          <Sparkles :size="14" />
          <span>导入并立即启用</span>
        </button>
      </div>
    </div>
  </div>
</template>
