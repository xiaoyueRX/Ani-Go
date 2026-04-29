<script setup lang="ts">
import { ref } from 'vue'

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
  <form @submit.prevent="handleSubmit" class="flex flex-col gap-3">
    <label class="form-control">
      <span class="label-text">中文标题 *</span>
      <input v-model="titleCN" type="text" class="input input-bordered" required />
    </label>
    <div class="grid grid-cols-2 gap-3">
      <label class="form-control">
        <span class="label-text">英文标题</span>
        <input v-model="titleEN" type="text" class="input input-bordered" />
      </label>
      <label class="form-control">
        <span class="label-text">日文标题</span>
        <input v-model="titleJP" type="text" class="input input-bordered" />
      </label>
    </div>
    <div class="grid grid-cols-3 gap-3">
      <label class="form-control">
        <span class="label-text">年份</span>
        <input v-model.number="year" type="number" class="input input-bordered" />
      </label>
      <label class="form-control">
        <span class="label-text">季</span>
        <input v-model.number="season" type="number" class="input input-bordered" />
      </label>
      <label class="form-control">
        <span class="label-text">类型</span>
        <select v-model="animeType" class="select select-bordered">
          <option value="">-</option>
          <option value="TV">TV</option>
          <option value="剧场版">剧场版</option>
          <option value="OVA">OVA</option>
          <option value="特别篇">特别篇</option>
        </select>
      </label>
    </div>
    <label class="form-control">
      <span class="label-text">字幕组</span>
      <input v-model="subgroupName" type="text" class="input input-bordered" />
    </label>
    <label class="form-control">
      <span class="label-text">Bangumi ID</span>
      <input v-model="bangumiId" type="text" class="input input-bordered" placeholder="例如 123456" />
    </label>
    <label class="form-control">
      <span class="label-text">总集数</span>
      <input v-model.number="totalEpisodes" type="number" class="input input-bordered" />
    </label>
    <label class="form-control">
      <span class="label-text">自定义路径模板</span>
      <input v-model="customPath" type="text" class="input input-bordered" placeholder="例如 {title_cn} ({year})/Season {season:02}" />
    </label>
    <label class="form-control">
      <span class="label-text">过滤规则 JSON</span>
      <textarea v-model="filterJson" class="textarea textarea-bordered h-20" placeholder='{"keywords": ["1080p"], "exclude": ["内嵌"]}'></textarea>
    </label>
    <label class="cursor-pointer flex items-center gap-2">
      <input v-model="completed" type="checkbox" class="checkbox" />
      <span class="label-text">已完结</span>
    </label>
    <div class="flex gap-2 justify-end mt-2">
      <button type="button" class="btn btn-ghost" @click="emit('cancel')">取消</button>
      <button type="submit" class="btn btn-primary">保存</button>
    </div>
  </form>
</template>
