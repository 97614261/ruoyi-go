<script setup lang="ts">
import { PhClockCounterClockwise, PhFolderSimple, PhHouse, PhImages, PhSquaresFour } from '@phosphor-icons/vue'
import type { Section } from './navigation'

defineProps<{ section: Section }>()
const emit = defineEmits<{ navigate: [section: Section] }>()
const items = [
  { section: 'home', label: '首页', icon: PhHouse }, { section: 'projects', label: '项目', icon: PhFolderSimple },
  { section: 'templates', label: '模板', icon: PhSquaresFour }, { section: 'assets', label: '素材', icon: PhImages },
  { section: 'runs', label: '任务与结果', icon: PhClockCounterClockwise },
] as const
</script>

<template>
  <nav class="studio-nav" aria-label="创作导航">
    <div class="navigation-group">
      <button v-for="item in items" :key="item.section" :class="{ active: section === item.section }" :aria-current="section === item.section ? 'page' : undefined" @click="emit('navigate', item.section)"><component :is="item.icon" :size="18" :weight="section === item.section ? 'fill' : 'regular'" aria-hidden="true" /><span>{{ item.label }}</span></button>
    </div>
  </nav>
</template>
