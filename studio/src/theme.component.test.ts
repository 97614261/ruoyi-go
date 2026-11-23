// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import * as themes from './theme'
import ThemeSwitch from './ThemeSwitch.vue'

type Controller = ReturnType<typeof themes.createThemeController>
const controllers: Controller[] = []
let view: VueWrapper | undefined
const originalTransition = Object.getOwnPropertyDescriptor(document, 'startViewTransition')
function controller() { const value = themes.createThemeController(); controllers.push(value); return value }
afterEach(() => {
  view?.unmount(); view = undefined
  for (const value of controllers.splice(0)) value.dispose()
  vi.restoreAllMocks(); vi.unstubAllGlobals(); localStorage.clear()
  if (originalTransition) Object.defineProperty(document, 'startViewTransition', originalTransition)
  else Reflect.deleteProperty(document, 'startViewTransition')
  delete document.documentElement.dataset.theme
  delete document.documentElement.dataset.themeMode
  document.documentElement.style.colorScheme = ''
})

it('defaults to dark even on a light system and applies the native color scheme', () => {
  const media = vi.fn(() => ({ matches: false }))
  vi.stubGlobal('matchMedia', media)
  expect(controller().mode.value).toBe('dark')
  expect(document.documentElement.style.colorScheme).toBe('dark')
  expect(media).not.toHaveBeenCalled()
})

it('restores a saved light choice and resets to dark when the preference is cleared', () => {
  const value = controller(); value.setMode('light')
  expect(localStorage.getItem(themes.themeStorageKey)).toBe('light')
  const restored = controller()
  expect(restored.mode.value).toBe('light')
  window.dispatchEvent(new StorageEvent('storage', { key: null, newValue: null, storageArea: localStorage }))
  expect(restored.mode.value).toBe('dark')
})

it('synchronizes appearance between tabs and stops listening on disposal', () => {
  const value = controller()
  window.dispatchEvent(new StorageEvent('storage', { key: themes.themeStorageKey, newValue: 'light', storageArea: localStorage }))
  expect(value.mode.value).toBe('light')
  value.dispose()
  window.dispatchEvent(new StorageEvent('storage', { key: themes.themeStorageKey, newValue: 'dark', storageArea: localStorage }))
  expect(value.mode.value).toBe('light')
})

it('allows switching when storage is blocked', () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked') })
  const value = controller()
  expect(value.mode.value).toBe('dark')
  expect(() => value.setMode('light')).not.toThrow()
  expect(document.documentElement.dataset.theme).toBe('light')
})

it.each(['system', 'light', 'dark', 'invalid'] as const)('applies %s before app mount consistently with the controller', mode => {
  localStorage.setItem(themes.themeStorageKey, mode)
  const startup = readFileSync('public/theme-init.js', 'utf8')
  new Function('localStorage', 'document', startup)(localStorage, document)
  const initial = document.documentElement.dataset.theme
  expect(initial).toBe(mode === 'light' ? 'light' : 'dark')
  expect(controller().mode.value).toBe(initial)
})

it('switches directly in both directions when the browser does not provide view transitions', async () => {
  const value = controller(); vi.spyOn(themes, 'initializeTheme').mockReturnValue(value)
  Object.defineProperty(document, 'startViewTransition', { configurable: true, value: undefined })
  view = mount(ThemeSwitch, { attachTo: document.body })
  const button = view.get('button')
  expect(button.attributes('aria-label')).toBe('切换到浅色')
  await button.trigger('click')
  expect(value.mode.value).toBe('light')
  expect(button.attributes('aria-label')).toBe('切换到深色')
  await button.trigger('click')
  expect(value.mode.value).toBe('dark')
})

it('starts the theme transition at the button center and removes the effect when it finishes', async () => {
  const value = controller(); vi.spyOn(themes, 'initializeTheme').mockReturnValue(value)
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))
  const root = document.documentElement
  const transition = vi.fn((update: () => Promise<void>) => {
    expect(root.style.getPropertyValue('--theme-reveal-origin')).toBe('40px 60px')
    expect(root.classList.contains('studio-theme-reveal')).toBe(true)
    return { finished: update() }
  })
  Object.defineProperty(document, 'startViewTransition', { configurable: true, value: transition })
  view = mount(ThemeSwitch, { attachTo: document.body })
  const button = view.get('button')
  vi.spyOn(button.element, 'getBoundingClientRect').mockReturnValue({ left: 18, top: 38, width: 44, height: 44 } as DOMRect)
  await button.trigger('click'); await flushPromises()
  expect(transition).toHaveBeenCalledOnce()
  expect(value.mode.value).toBe('light')
  expect(root.classList.contains('studio-theme-reveal')).toBe(false)
})
