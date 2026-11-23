<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { PhMoon, PhSun } from '@phosphor-icons/vue'
import { initializeTheme } from './theme'

const { mode, setMode } = initializeTheme()
const isDark = computed(() => mode.value === 'dark')
const action = computed(() => `切换到${isDark.value ? '浅色' : '深色'}`)

async function toggle(event: MouseEvent) {
  const root = document.documentElement
  if (root.classList.contains('studio-theme-reveal')) return
  const next = isDark.value ? 'light' : 'dark'
  if (!document.startViewTransition || window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches) {
    setMode(next)
    return
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const x = bounds.left + bounds.width / 2, y = bounds.top + bounds.height / 2
  const radius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y))
  root.style.setProperty('--theme-reveal-origin', `${x}px ${y}px`)
  root.style.setProperty('--theme-reveal-radius', `${radius}px`)
  root.classList.add('studio-theme-reveal')
  try {
    await document.startViewTransition(async () => { setMode(next); await nextTick() }).finished
  } catch {
    if (mode.value !== next) setMode(next)
  } finally {
    root.classList.remove('studio-theme-reveal')
    root.style.removeProperty('--theme-reveal-origin')
    root.style.removeProperty('--theme-reveal-radius')
  }
}
</script>

<template>
  <button class="theme-switch theme-toggle" type="button" :aria-label="action" :title="action" @click="toggle">
    <component :is="isDark ? PhSun : PhMoon" :size="18" aria-hidden="true" />
  </button>
</template>
