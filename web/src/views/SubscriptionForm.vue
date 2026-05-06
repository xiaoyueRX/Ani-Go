<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

const router = useRouter()
const titleCN = ref('')
const bangumiId = ref('')
const subgroupName = ref('')
const filterJson = ref('')
const customPath = ref('')
const loading = ref(false)
const error = ref('')

async function handleSubmit() {
  if (!titleCN.value.trim()) {
    error.value = '番剧标题不能为空'
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
    error.value = e.response?.data?.error || '创建失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <button class="btn btn-ghost btn-sm mb-4 gap-1" @click="router.push('/')">
      <IconSax name="chevron-left" :size="16" />
      返回订阅列表
    </button>

    <div class="card bg-base-100 shadow-sm border border-base-200 max-w-xl mx-auto">
      <div class="card-body">
        <div class="flex items-center gap-2 mb-4">
          <IconSax name="add" :size="22" class="text-primary" />
          <h1 class="card-title text-2xl">添加订阅</h1>
        </div>

        <div v-if="error" class="alert alert-error mb-4">
          <IconSax name="warning" class="shrink-0" />
          <span>{{ error }}</span>
          <button class="btn btn-ghost btn-sm" @click="error = ''">
            <IconSax name="close" :size="16" />
          </button>
        </div>

        <form @submit.prevent="handleSubmit" class="flex flex-col gap-4">
          <label class="form-control">
            <span class="label-text">番剧标题 *</span>
            <input v-model="titleCN" type="text" class="input input-bordered" placeholder="例如：鬼灭之刃" required />
          </label>
          <label class="form-control">
            <span class="label-text">字幕组</span>
            <input v-model="subgroupName" type="text" class="input input-bordered" placeholder="例如：桜都字幕组" />
          </label>
          <label class="form-control">
            <span class="label-text">Bangumi ID</span>
            <input v-model="bangumiId" type="text" class="input input-bordered" placeholder="例如 123456" />
          </label>
          <label class="form-control">
            <span class="label-text">自定义路径模板</span>
            <input v-model="customPath" type="text" class="input input-bordered" placeholder="留空使用默认路径" />
          </label>
          <label class="form-control">
            <span class="label-text">过滤规则 JSON</span>
            <textarea v-model="filterJson" class="textarea textarea-bordered h-20" placeholder='{"keywords": ["1080p"], "exclude": ["内嵌"]}'></textarea>
          </label>
          <button type="submit" class="btn btn-primary gap-1" :disabled="loading">
            <span v-if="loading" class="loading loading-spinner"></span>
            <IconSax v-else name="add" :size="16" />
            创建订阅
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
