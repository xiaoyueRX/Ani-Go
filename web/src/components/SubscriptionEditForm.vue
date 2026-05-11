<script setup lang="ts">
import { ref } from 'vue'
import { Check } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  sub: Record<string, any>
}>()

const emit = defineEmits<{ save: [updated: Record<string, any>]; cancel: [] }>()

const titleCN = ref(props.sub.title_cn)
const titleEN = ref(props.sub.title_en || '')
const titleJP = ref(props.sub.title_jp || '')
const year = ref(props.sub.year)
const season = ref(props.sub.season)
const bangumiId = ref(props.sub.bangumi_id || '')
const subgroupName = ref(props.sub.subgroup_name || '')
const animeType = ref(props.sub.anime_type || '')
const totalEpisodes = ref(props.sub.total_episodes)
const completed = ref(props.sub.completed)
const customPath = ref(props.sub.custom_path || '')
const filterJson = ref(props.sub.filter_json || '')

function buildUpdate(): Record<string, any> {
  const u: Record<string, any> = {}
  if (titleCN.value !== props.sub.title_cn) u.title_cn = titleCN.value
  if (titleEN.value !== (props.sub.title_en || '')) u.title_en = titleEN.value || null
  if (titleJP.value !== (props.sub.title_jp || '')) u.title_jp = titleJP.value || null
  if (year.value !== props.sub.year) u.year = year.value || null
  if (season.value !== props.sub.season) u.season = season.value
  if (bangumiId.value !== (props.sub.bangumi_id || '')) u.bangumi_id = bangumiId.value || null
  if (subgroupName.value !== (props.sub.subgroup_name || '')) u.subgroup_name = subgroupName.value || null
  if (animeType.value !== (props.sub.anime_type || '')) u.anime_type = animeType.value || null
  if (totalEpisodes.value !== props.sub.total_episodes) u.total_episodes = totalEpisodes.value
  if (completed.value !== props.sub.completed) u.completed = completed.value
  if (customPath.value !== (props.sub.custom_path || '')) u.custom_path = customPath.value || null
  if (filterJson.value !== (props.sub.filter_json || '')) u.filter_json = filterJson.value || null
  return u
}

function handleSubmit() {
  const u = buildUpdate()
  if (Object.keys(u).length === 0) {
    emit('cancel')
    return
  }
  emit('save', u)
}
</script>

<template>
  <form @submit.prevent="handleSubmit" class="space-y-6">
    <div class="space-y-2">
      <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.primary') }}</label>
      <input v-model="titleCN" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" required />
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.aliasEn') }}</label>
        <input v-model="titleEN" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
      </div>
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.aliasJp') }}</label>
        <input v-model="titleJP" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
      </div>
    </div>

    <div class="grid grid-cols-3 gap-6">
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.year') }}</label>
        <input v-model.number="year" type="number" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
      </div>
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.season') }}</label>
        <input v-model.number="season" type="number" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
      </div>
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.classification') }}</label>
        <select v-model="animeType" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold appearance-none">
          <option value="">{{ t('edit.select') }}</option>
          <option value="TV">{{ t('edit.types.tv') }}</option>
          <option value="剧场版">{{ t('edit.types.movie') }}</option>
          <option value="OVA">{{ t('edit.types.ova') }}</option>
          <option value="特别篇">{{ t('edit.types.special') }}</option>
        </select>
      </div>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.releaseGroup') }}</label>
        <input v-model="subgroupName" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
      </div>
      <div class="space-y-2">
        <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.bangumiRef') }}</label>
        <input v-model="bangumiId" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" placeholder="ID: 123456" />
      </div>
    </div>

    <div class="space-y-2">
      <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.totalEpisodes') }}</label>
      <input v-model.number="totalEpisodes" type="number" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" />
    </div>

    <div class="space-y-2">
      <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.pathOverride') }}</label>
      <input v-model="customPath" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" placeholder="{title_cn} ({year})/Season {season:02}" />
    </div>

    <div class="space-y-2">
      <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ t('edit.label.filterRules') }}</label>
      <textarea v-model="filterJson" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-[1.5rem] px-6 py-4 transition-all outline-none font-bold h-32" placeholder='{"keywords": ["1080p"], "exclude": ["RAW"]}'></textarea>
    </div>

    <div class="p-6 bg-base-200/50 rounded-2xl border border-base-300 flex items-center justify-between">
       <div class="flex items-center gap-4">
         <div class="w-10 h-10 rounded-full bg-success/20 flex items-center justify-center text-success">
            <Check :size="20" />
         </div>
         <div>
            <p class="text-sm font-black tracking-tight">{{ t('edit.label.finalized') }}</p>
            <p class="text-[10px] font-bold opacity-40 uppercase tracking-widest">{{ t('edit.label.finalizedDesc') }}</p>
         </div>
      </div>
       <input v-model="completed" type="checkbox" class="toggle toggle-success" />
    </div>

    <div class="flex gap-4 justify-end pt-6">
      <button type="button" class="btn btn-ghost rounded-2xl px-8 h-14 min-h-0 uppercase font-black tracking-widest text-[10px]" @click="emit('cancel')">{{ t('edit.abandon') }}</button>
      <button type="submit" class="btn btn-primary rounded-2xl px-12 h-14 min-h-0 uppercase font-black tracking-widest text-[10px] shadow-xl shadow-lg">{{ t('edit.commit') }}</button>
    </div>
  </form>
</template>
