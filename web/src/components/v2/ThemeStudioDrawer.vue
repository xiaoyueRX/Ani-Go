<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Palette, X, Check, Sun, Moon, Sparkles, Upload } from 'lucide-vue-next'
import request from '../../utils/request'
import { useTheme, type ThemePack } from '../../composables/useTheme'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const { locale } = useI18n()
const {
  allThemes,
  activePack,
  activeStyle,
  activePackId,
  activeStyleId,
  isDark,
  selectTheme,
  toggleTheme,
  getServerSettingsPayload,
} = useTheme()

async function persistServerTheme() {
  if (!localStorage.getItem('token')) return
  try {
    await request.put('/settings', { settings: getServerSettingsPayload() })
  } catch {
    // 忽略未登录或临时网络异常，本地偏好已立即生效
  }
}

function handlePickStyle(packId: string, styleId: string) {
  selectTheme(packId, styleId)
  void persistServerTheme()
}

function handleQuickToggleMode() {
  toggleTheme()
  void persistServerTheme()
}

function goManageThemes() {
  emit('close')
  router.push('/settings?tab=appearance')
}

function previewColorsFor(pack: ThemePack) {
  if (pack.id === activePackId.value) {
    return activeStyle.value.colors
  }
  return pack.styles[0].colors
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) {
    emit('close')
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="open" class="fixed inset-0 z-[125] flex justify-end">
        <div class="fixed inset-0 bg-black/60 backdrop-blur-xs" @click="emit('close')"></div>

        <aside
          role="dialog"
          aria-modal="true"
          aria-label="Theme Studio"
          class="relative z-10 w-full max-w-md h-full bg-[var(--v2-surface)] border-l border-[var(--v2-border-strong)] shadow-2xl flex flex-col"
        >
          <!-- 抽屉头部 -->
          <div class="px-5 py-4 border-b border-[var(--v2-border)] flex items-center justify-between gap-3 shrink-0">
            <div class="flex items-center gap-2.5 min-w-0">
              <div class="w-9 h-9 rounded-xl bg-[var(--v2-accent)]/15 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
                <Palette :size="18" />
              </div>
              <div class="min-w-0">
                <h2 class="text-sm font-bold text-[var(--v2-ink)] truncate">
                  {{ locale === 'zh' ? '主题与色彩工坊' : 'Theme & Color Studio' }}
                </h2>
                <p class="text-[11px] text-[var(--v2-ink-muted)] truncate">
                  {{ activePack.name }} · {{ activeStyle.name }}
                </p>
              </div>
            </div>

            <div class="flex items-center gap-1.5 shrink-0">
              <button
                type="button"
                class="v2-btn v2-btn-sm"
                :title="isDark ? '切换亮色风格' : '切换暗色风格'"
                @click="handleQuickToggleMode"
              >
                <Sun v-if="isDark" :size="13" />
                <Moon v-else :size="13" />
                <span>{{ isDark ? (locale === 'zh' ? '亮色' : 'Light') : (locale === 'zh' ? '暗色' : 'Dark') }}</span>
              </button>
              <button
                type="button"
                class="w-8 h-8 rounded-lg flex items-center justify-center text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:bg-[var(--v2-surface-2)] cursor-pointer"
                aria-label="Close"
                @click="emit('close')"
              >
                <X :size="16" />
              </button>
            </div>
          </div>

          <!-- 主题与多色彩风格列表 -->
          <div class="flex-1 overflow-y-auto p-4 space-y-4">
            <div
              v-for="pack in allThemes"
              :key="pack.id"
              class="v2-panel-muted p-4 space-y-3 border transition-colors"
              :class="activePackId === pack.id ? 'border-[var(--v2-accent)]/55' : 'border-[var(--v2-border)]'"
            >
              <!-- 主题标题与来源 -->
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <div class="flex items-center gap-2 flex-wrap">
                    <h3 class="text-xs font-bold text-[var(--v2-ink)]">{{ pack.name }}</h3>
                    <span
                      class="text-[10px] font-semibold px-1.5 py-0.5 rounded"
                      :class="pack.builtin ? 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)]' : 'bg-emerald-500/15 text-emerald-400'"
                    >
                      {{ pack.builtin ? (locale === 'zh' ? '官方预设' : 'Official') : (locale === 'zh' ? '第三方' : '3rd-Party') }}
                    </span>
                  </div>
                  <p class="text-[11px] text-[var(--v2-ink-muted)] mt-0.5 leading-snug">{{ pack.description }}</p>
                </div>
                <span class="text-[10px] font-mono text-[var(--v2-ink-subtle)] shrink-0">
                  {{ pack.styles.length }} {{ locale === 'zh' ? '色' : 'styles' }}
                </span>
              </div>

              <!-- 微缩 UI 结构预览条（直接展示圆角、质感与配色） -->
              <div
                class="p-2.5 border transition-all overflow-hidden"
                :style="{
                  backgroundColor: previewColorsFor(pack).bg,
                  borderColor: previewColorsFor(pack).borderStrong,
                  borderRadius: pack.chrome.radiusPanel,
                }"
              >
                <div class="flex items-center justify-between gap-2 mb-2">
                  <div class="flex items-center gap-1.5">
                    <span class="w-2 h-2 rounded-full" :style="{ backgroundColor: previewColorsFor(pack).accent }"></span>
                    <span
                      class="h-2.5 w-16 inline-block"
                      :style="{
                        backgroundColor: previewColorsFor(pack).surface2,
                        borderRadius: pack.chrome.dockRadius,
                        border: `1px solid ${previewColorsFor(pack).border}`,
                      }"
                    ></span>
                  </div>
                  <span
                    class="px-2 py-0.5 text-[9px] font-bold"
                    :style="{
                      backgroundColor: previewColorsFor(pack).accent,
                      color: previewColorsFor(pack).accentInk,
                      borderRadius: pack.chrome.radiusBtnSm,
                    }"
                  >
                    Aa
                  </span>
                </div>
                <div class="grid grid-cols-4 gap-1.5">
                  <div
                    v-for="n in 4"
                    :key="n"
                    class="h-7 border"
                    :style="{
                      backgroundColor: previewColorsFor(pack).surface,
                      borderColor: n === 1 ? previewColorsFor(pack).accent : previewColorsFor(pack).border,
                      borderRadius: pack.chrome.radiusMuted,
                    }"
                  ></div>
                </div>
              </div>

              <!-- 色彩风格胶囊选择器 -->
              <div class="grid grid-cols-2 gap-1.5">
                <button
                  v-for="styleItem in pack.styles"
                  :key="styleItem.id"
                  type="button"
                  @click="handlePickStyle(pack.id, styleItem.id)"
                  class="px-2.5 py-2 rounded-lg border text-left flex items-center justify-between gap-2 transition-all cursor-pointer"
                  :class="activePackId === pack.id && activeStyleId === styleItem.id
                    ? 'bg-[var(--v2-surface)] border-[var(--v2-accent)] shadow-xs'
                    : 'bg-[var(--v2-surface)]/60 border-[var(--v2-border)] hover:border-[var(--v2-border-strong)]'"
                >
                  <div class="flex items-center gap-2 min-w-0">
                    <span class="flex items-center -space-x-1 shrink-0">
                      <span
                        class="w-3.5 h-3.5 rounded-full border border-white/25"
                        :style="{ backgroundColor: styleItem.colors.bg }"
                      ></span>
                      <span
                        class="w-3.5 h-3.5 rounded-full border border-white/25"
                        :style="{ backgroundColor: styleItem.colors.accent }"
                      ></span>
                    </span>
                    <span class="text-[11px] font-semibold text-[var(--v2-ink)] truncate">{{ styleItem.name }}</span>
                  </div>
                  <Check
                    v-if="activePackId === pack.id && activeStyleId === styleItem.id"
                    :size="12"
                    class="text-[var(--v2-accent)] shrink-0"
                  />
                  <span v-else class="text-[9px] text-[var(--v2-ink-subtle)] shrink-0">
                    {{ styleItem.mode === 'anigo-light' ? (locale === 'zh' ? '浅' : 'L') : (locale === 'zh' ? '深' : 'D') }}
                  </span>
                </button>
              </div>
            </div>
          </div>

          <!-- 底部跳转至设置页第三方工坊 -->
          <div class="p-4 border-t border-[var(--v2-border)] bg-[var(--v2-surface-2)]/60 shrink-0">
            <button
              type="button"
              class="v2-btn v2-btn-primary w-full"
              @click="goManageThemes"
            >
              <Upload :size="14" />
              <span>{{ locale === 'zh' ? '导入 / 管理第三方主题包' : 'Import / Manage 3rd-Party Themes' }}</span>
              <Sparkles :size="13" />
            </button>
          </div>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>
