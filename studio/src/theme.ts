import { readonly, ref } from 'vue'

export type ThemeMode = 'light' | 'dark'
export const themeStorageKey = 'pod-studio.theme'
const validMode = (value: unknown): ThemeMode => value === 'light' ? 'light' : 'dark'

export function createThemeController(host: Window = window, root: HTMLElement = document.documentElement) {
  let saved: string | null = null
  try { saved = host.localStorage.getItem(themeStorageKey) } catch { /* Storage can be unavailable in private browsing. */ }
  const mode = ref<ThemeMode>(validMode(saved))
  function apply() {
    root.dataset.theme = mode.value
    root.dataset.themeMode = mode.value
    root.style.colorScheme = mode.value
  }
  function setMode(value: ThemeMode) {
    mode.value = validMode(value)
    apply()
    try { host.localStorage.setItem(themeStorageKey, mode.value) } catch { /* Keep the current choice in memory. */ }
  }
  function storageChanged(event: StorageEvent) {
    if (event.key !== themeStorageKey && event.key !== null) return
    if (event.storageArea) {
      try { if (event.storageArea !== host.localStorage) return } catch { return }
    }
    mode.value = validMode(event.newValue); apply()
  }
  host.addEventListener('storage', storageChanged)
  apply()
  return {
    mode: readonly(mode), setMode,
    dispose() { host.removeEventListener('storage', storageChanged) },
  }
}

let activeTheme: ReturnType<typeof createThemeController> | undefined
export function initializeTheme() { return activeTheme ||= createThemeController() }
if (import.meta.hot) import.meta.hot.dispose(() => { activeTheme?.dispose(); activeTheme = undefined })
