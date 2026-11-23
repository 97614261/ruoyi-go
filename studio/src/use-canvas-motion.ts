import { onBeforeUnmount, onMounted, ref } from 'vue'

const preferenceKey = 'pod-studio.canvas-motion'

export function useCanvasMotion() {
  const motionEnabled = ref(true), reducedMotion = ref(false), documentHidden = ref(false)
  let query: MediaQueryList | undefined
  const updateMotion = () => { reducedMotion.value = query?.matches || false }
  const updateVisibility = () => { documentHidden.value = document.hidden }
  function toggleMotion() {
    if (reducedMotion.value) return
    motionEnabled.value = !motionEnabled.value
    try { localStorage.setItem(preferenceKey, motionEnabled.value ? 'on' : 'off') } catch { /* Keep the choice for this canvas if storage is unavailable. */ }
  }
  onMounted(() => {
    try { motionEnabled.value = localStorage.getItem(preferenceKey) !== 'off' } catch { /* System preference still applies without storage. */ }
    query = window.matchMedia('(prefers-reduced-motion: reduce)')
    updateMotion(); updateVisibility()
    query.addEventListener('change', updateMotion)
    document.addEventListener('visibilitychange', updateVisibility)
  })
  onBeforeUnmount(() => {
    query?.removeEventListener('change', updateMotion)
    document.removeEventListener('visibilitychange', updateVisibility)
  })
  return { motionEnabled, reducedMotion, documentHidden, toggleMotion }
}
