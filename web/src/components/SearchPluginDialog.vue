<script lang="ts">
export interface SearchPluginTarget {
  id: string
  name: string
  action: string
  label: string
  url: string
  official: boolean
}
</script>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAxiosError } from 'axios'
import request from '../utils/request'
import { bindDialogSession } from '../utils/dialogSession'

const props = defineProps<{ item: { title: string; magnet: string }; targets: SearchPluginTarget[] }>()
const emit = defineEmits<{ close: [] }>()
const { t, te } = useI18n()
const dialog = ref<HTMLDialogElement | null>(null)
const session = bindDialogSession(localStorage, () => dialog.value?.close())
const selected = ref(0)
const target = computed(() => props.targets[selected.value])
const busy = ref(false)
const message = ref('')
const error = ref('')

function label(provider: SearchPluginTarget, text: string) {
  return provider.official && te(text) ? t(text) : text
}

async function send() {
  const provider = target.value
  if (!session.active() || busy.value || message.value || !provider) return
  busy.value = true
  error.value = ''
  try {
    const { data } = await request.post<{ message?: string }>(
      '/plugins/' + encodeURIComponent(provider.id) + '/panel/actions/' + encodeURIComponent(provider.action),
      { input: props.item.magnet, placement: 'search_result', expected_url: provider.url }, session.options,
    )
    if (!session.active()) return
    if (typeof data?.message === 'string' && data.message.trim()) message.value = label(provider, data.message)
    else error.value = 'settings.pluginPanel.submitFailed'
  } catch (cause) {
    if (!session.active()) return
    const status = isAxiosError(cause) ? cause.response?.status : undefined
    if (status === 400) error.value = 'settings.pluginPanel.invalidMagnet'
    else if (status === 404 || status === 409) error.value = 'search.plugin.unavailable'
    else if (status === 422) error.value = 'settings.pluginPanel.authorizationFailed'
    else if (status === 429) error.value = 'settings.pluginPanel.rateLimited'
    else error.value = 'settings.pluginPanel.submitFailed'
  } finally {
    busy.value = false
  }
}

onMounted(() => dialog.value?.showModal())
onUnmounted(session.abort)
</script>

<template>
  <dialog ref="dialog" class="modal" aria-labelledby="search-plugin-heading" @cancel="busy && $event.preventDefault()" @close="emit('close')">
    <form class="modal-box space-y-4" :aria-busy="busy" @submit.prevent="send">
      <h2 id="search-plugin-heading" class="text-lg font-bold">{{ t('search.plugin.open') }}</h2>
      <p class="break-words text-sm">{{ item.title }}</p>
      <label v-if="targets.length > 1" class="block">
        <span class="mb-1 block text-sm font-semibold">{{ t('search.plugin.destination') }}</span>
        <select v-model="selected" class="select w-full" :disabled="busy || !!message" @change="error = ''">
          <option v-for="(provider, index) in targets" :key="index" :value="index">
            {{ provider.name }} · {{ label(provider, provider.label) }}
          </option>
        </select>
      </label>
      <div v-if="target" class="space-y-1 rounded-xl bg-base-200 p-3 text-sm">
        <p v-if="targets.length === 1" class="font-semibold">{{ target.name }} · {{ label(target, target.label) }}</p>
        <p class="text-xs">{{ t(target.official ? 'settings.plugins.official' : 'settings.plugins.community') }}</p>
        <p v-if="target.url" dir="ltr" class="break-all text-xs">{{ target.url }}</p>
      </div>
      <p v-if="message" role="status" class="break-words text-sm text-success">{{ message }}</p>
      <div v-if="error" role="alert" class="space-y-1 text-sm">
        <p class="text-error">{{ t(error) }}</p>
        <RouterLink v-if="error === 'search.plugin.unavailable' || error === 'settings.pluginPanel.authorizationFailed'"
          class="link" to="/settings?tab=plugins">{{ t('search.plugin.settings') }}</RouterLink>
      </div>
      <div class="modal-action flex-wrap">
        <button type="button" class="btn btn-ghost" :disabled="busy" @click="dialog?.close()">{{ t('common.close') }}</button>
        <button v-if="!message" type="submit" class="btn btn-primary" :disabled="busy || !target">
          <span v-if="busy" class="loading loading-spinner loading-xs"></span>{{ t('search.plugin.send') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
