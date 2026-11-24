<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { PageHeader, SurfaceCard } from '../components/ui'
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
  <div class="space-y-6 pb-16 max-w-2xl mx-auto">
    <PageHeader :title="$t('form.title')" :subtitle="$t('form.subtitle')">
      <template #actions>
        <button
          type="button"
          class="btn btn-ghost btn-sm border border-base-content/12 rounded-lg gap-1.5"
          @click="router.push('/')"
        >
          <ChevronLeft :size="15" />
          <span>{{ $t('common.back') }}</span>
        </button>
      </template>
    </PageHeader>

    <SurfaceCard padding="lg">
      <div
        v-if="error"
        role="alert"
        class="bg-error/10 border border-error/25 text-error rounded-lg px-4 py-3 mb-5 flex items-center gap-3 text-sm"
      >
        <AlertTriangle :size="18" class="shrink-0" />
        <div class="flex-1 min-w-0">
          <p class="font-semibold text-xs">{{ $t('form.error.title') }}</p>
          <p class="text-xs opacity-85 mt-0.5">{{ error }}</p>
        </div>
        <button type="button" class="btn btn-ghost btn-circle btn-xs" @click="error = ''">
          <X :size="14" />
        </button>
      </div>

      <form @submit.prevent="handleSubmit" class="space-y-5">
        <div>
          <label class="app-field-label">{{ $t('form.label.title') }}</label>
          <div class="relative group">
            <div class="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-base-content/35 group-focus-within:text-primary transition-colors">
              <LayoutGrid :size="16" />
            </div>
            <input
              v-model="titleCN"
              type="text"
              class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg pl-10 pr-3.5 text-sm font-medium"
              :placeholder="$t('form.placeholder.title')"
              required
            />
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="app-field-label">{{ $t('form.label.subgroup') }}</label>
            <input
              v-model="subgroupName"
              type="text"
              class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg px-3.5 text-sm"
              :placeholder="$t('form.placeholder.subgroup')"
            />
          </div>
          <div>
            <label class="app-field-label">{{ $t('form.label.bangumiId') }}</label>
            <input
              v-model="bangumiId"
              type="text"
              class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg px-3.5 text-sm font-mono"
              :placeholder="$t('form.placeholder.bangumiId')"
            />
          </div>
        </div>

        <div>
          <label class="app-field-label">{{ $t('form.label.path') }}</label>
          <input
            v-model="customPath"
            type="text"
            class="input w-full h-10 bg-base-200/70 border border-base-content/12 rounded-lg px-3.5 text-sm font-mono"
            :placeholder="$t('form.placeholder.path')"
          />
        </div>

        <div>
          <label class="app-field-label">{{ $t('form.label.filter') }}</label>
          <textarea
            v-model="filterJson"
            class="textarea w-full bg-base-200/70 border border-base-content/12 rounded-lg px-3.5 py-2.5 text-xs font-mono h-28"
            placeholder='{"keywords": ["1080p"], "exclude": ["RAW"]}'
          ></textarea>
        </div>

        <div class="pt-2 flex justify-end">
          <button
            type="submit"
            class="btn btn-primary w-full sm:w-auto px-6 h-10 rounded-lg font-semibold text-sm gap-2"
            :disabled="loading"
          >
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            <template v-else>
              <Plus :size="16" />
              <span>{{ $t('form.submit') }}</span>
            </template>
          </button>
        </div>
      </form>
    </SurfaceCard>
  </div>
</template>
