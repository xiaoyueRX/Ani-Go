<script setup lang="ts">
import { ref, onMounted } from 'vue'

const username = ref('')

onMounted(async () => {
  try {
    const { default: request } = await import('../utils/request')
    const { data } = await request.get('/me')
    username.value = data.username
  } catch {
    // 401 interceptor handles redirect
  }
})
</script>

<template>
  <div class="min-h-screen bg-base-300">
    <div class="navbar bg-base-100 shadow">
      <div class="flex-1">
        <a class="btn btn-ghost text-xl">Ani-Go</a>
      </div>
      <div class="flex-none gap-2">
        <span class="text-sm opacity-70">{{ username }}</span>
      </div>
    </div>
    <div class="container mx-auto p-8">
      <div class="card bg-base-100 shadow-xl">
        <div class="card-body">
          <h2 class="card-title">仪表盘</h2>
          <p class="text-base-content/60">追番管理面板即将上线</p>
        </div>
      </div>
    </div>
  </div>
</template>
