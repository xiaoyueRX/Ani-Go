<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.post('/login', {
      username: username.value,
      password: password.value,
    })
    localStorage.setItem('token', data.token)
    router.push('/')
  } catch (e: any) {
    error.value = e.response?.data?.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-base-300 to-base-200">
    <div class="card w-96 bg-base-100 shadow-xl">
      <div class="card-body">
        <div class="text-center mb-6">
          <div class="flex justify-center mb-3">
            <div class="w-14 h-14 rounded-2xl bg-primary/10 flex items-center justify-center">
              <IconSax name="antenna" :size="32" class="text-primary" />
            </div>
          </div>
          <h2 class="card-title text-2xl justify-center">Ani-Go</h2>
          <p class="text-sm text-base-content/40 mt-1">番剧追番下载管理系统</p>
        </div>

        <div v-if="error" class="alert alert-error text-sm mb-4">
          <IconSax name="warning" class="shrink-0" :size="16" />
          <span>{{ error }}</span>
        </div>

        <form @submit.prevent="handleLogin" class="flex flex-col gap-4">
          <label class="input input-bordered flex items-center gap-2">
            <IconSax name="user" :size="16" class="opacity-50 shrink-0" />
            <input v-model="username" type="text" class="grow" placeholder="用户名" />
          </label>
          <label class="input input-bordered flex items-center gap-2">
            <IconSax name="lock" :size="16" class="opacity-50 shrink-0" />
            <input v-model="password" type="password" class="grow" placeholder="密码" />
          </label>
          <button type="submit" class="btn btn-primary gap-1" :disabled="loading">
            <span v-if="loading" class="loading loading-spinner"></span>
            <IconSax v-else name="login" :size="16" />
            登录
          </button>
        </form>
      <p class="text-center text-xs text-base-content/20 mt-5">Ani-Go · 番剧追番下载管理系统</p>
      </div>
    </div>
  </div>
</template>
