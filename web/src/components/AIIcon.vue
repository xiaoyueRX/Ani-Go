<script setup lang="ts">
import { 
  Sparkles, 
  Image as ImageIcon,
  Palette,
  Wand2
} from 'lucide-vue-next'

const props = defineProps<{
  type: 'text-to-image' | 'image-to-image'
  size?: number
}>()
</script>

<template>
  <div class="relative flex items-center justify-center" :style="{ width: size + 'px', height: size + 'px' }">
    <template v-if="type === 'text-to-image'">
      <!-- 文生图图标：画板 + 闪烁 -->
      <Palette :size="size" class="text-primary" />
      <Sparkles 
        :size="size ? size * 0.5 : 10" 
        class="absolute -top-1 -right-1 text-secondary animate-pulse" 
      />
    </template>
    
    <template v-else>
      <!-- 图生图图标：图片 + 魔法棒 -->
      <ImageIcon :size="size" class="text-primary" />
      <Wand2 
        :size="size ? size * 0.5 : 10" 
        class="absolute -top-1 -right-1 text-accent" 
      />
    </template>
  </div>
</template>
