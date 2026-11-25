<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import {
  ChevronLeft, Plus, AlertTriangle,
  X, LayoutGrid
} from 'lucide-vue-next'

const router = useRouter()
const { t } = useI18n()
const titleCN = ref('')
const bangumiId = ref('')
const subgroupName = ref('')
const filterJson = ref('')
const customPath = ref('')
const loading = ref(false)
const error = ref('')

async function handleSubmit() {
  if (!titleCN.value.trim()) {
    error.value = t('form.error.titleEmpty')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.post('/subscriptions', {
      title_cn: titleCN.value,
      bangumi_id: bangumiId.value,
      subgroup_name: subgroupName.value,
      filter_json: filterJson.value,
      custom_path: customPath.value,
    })
    router.push(`/subscriptions/${data.id}`)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('form.error.createFailed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="max-w-2xl mx-auto space-y-5 pb-16">
    <!-- 顶部标题与返回 -->
    <section class="v2-panel p-4 sm:p-5 flex items-center justify-between gap-4">
      <div class="min-w-0">
        <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">{{ $t('form.title') }}</h1>
        <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('form.subtitle') }}</p>
      </div>
      <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="router.push('/')">
        <ChevronLeft :size="15" />
        <span>{{ $t('common.back') }}</span>
      </button>
    </section>

    <!-- 录入主表单 -->
    <section class="v2-panel p-5 sm:p-6">
      <div
        v-if="error"
        role="alert"
        class="mb-5 rounded-xl bg-red-500/10 border border-red-500/30 text-red-400 px-4 py-3 flex items-center gap-3 text-xs"
      >
        <AlertTriangle :size="17" class="shrink-0" />
        <div class="flex-1 min-w-0">
          <p class="font-bold">{{ $t('form.error.title') }}</p>
          <p class="opacity-85 mt-0.5">{{ error }}</p>
        </div>
        <button type="button" class="p-1 hover:bg-red-500/15 rounded" @click="error = ''">
          <X :size="14" />
        </button>
      </div>

      <form @submit.prevent="handleSubmit" class="space-y-4">
        <div class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.title') }}</label>
          <div class="relative">
            <LayoutGrid :size="15" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
            <input
              v-model="titleCN"
              type="text"
              class="v2-input pl-9"
              :placeholder="$t('form.placeholder.title')"
              required
            />
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.subgroup') }}</label>
            <input
              v-model="subgroupName"
              type="text"
              class="v2-input"
              :placeholder="$t('form.placeholder.subgroup')"
            />
          </div>
          <div class="space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.bangumiId') }}</label>
            <input
              v-model="bangumiId"
              type="text"
              class="v2-input font-mono"
              :placeholder="$t('form.placeholder.bangumiId')"
            />
          </div>
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.path') }}</label>
          <input
            v-model="customPath"
            type="text"
            class="v2-input font-mono text-xs"
            :placeholder="$t('form.placeholder.path')"
          />
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.filter') }}</label>
          <textarea
            v-model="filterJson"
            class="textarea font-mono text-xs h-28"
            placeholder='{"keywords": ["1080p"], "exclude": ["RAW"]}'
          ></textarea>
        </div>

        <div class="pt-2 flex justify-end gap-2">
          <button type="button" class="v2-btn v2-btn-ghost" @click="router.push('/')">
            {{ $t('common.cancel') }}
          </button>
          <button type="submit" class="v2-btn v2-btn-primary px-6" :disabled="loading">
            <span v-if="loading" class="loading loading-spinner loading-xs"></span>
            <template v-else>
              <Plus :size="15" />
              <span>{{ $t('form.submit') }}</span>
            </template>
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
