// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { Position } from '@vue-flow/core'
import MagneticHandle from './MagneticHandle.vue'

let view: VueWrapper | undefined
let frames: Map<number, FrameRequestCallback>, nextFrame: number
const HandleStub = defineComponent({
  name: 'Handle', props: ['id', 'type', 'position', 'connectable'], emits: ['activate'],
  template: '<div class="vue-flow__handle" :data-handleid="id" :data-handlepos="position" @click="$emit(\'activate\')"><slot /></div>',
})
beforeEach(() => {
  frames = new Map(); nextFrame = 0
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => { frames.set(++nextFrame, callback); return nextFrame }))
  vi.stubGlobal('cancelAnimationFrame', vi.fn((id: number) => { frames.delete(id) }))
})
afterEach(() => { view?.unmount(); view = undefined; vi.restoreAllMocks(); vi.unstubAllGlobals() })
function open(zoom = 1) {
  view = mount(MagneticHandle, { props: { id: 'images', type: 'source', position: Position.Right, top: 'calc(50% + 0px)', label: '图片输入 输出 图片', valueType: 'images' }, global: { stubs: { Handle: HandleStub } } })
  const surface = view.get('.magnetic-handle')
  Object.defineProperty(surface.element, 'offsetWidth', { configurable: true, value: 1 })
  vi.spyOn(surface.element, 'getBoundingClientRect').mockReturnValue({ x: 100, y: 200, left: 100, top: 200, right: 100 + zoom, bottom: 200 + zoom, width: zoom, height: zoom, toJSON() {} })
  return surface
}
function frame() { const callbacks = [...frames.values()]; frames.clear(); for (const callback of callbacks) callback(16) }
function offset() {
  const style = (view!.element as HTMLElement).style
  return { x: parseFloat(style.getPropertyValue('--port-follow-x')) || 0, y: parseFloat(style.getPropertyValue('--port-follow-y')) || 0 }
}

it('keeps port identity and the wire anchor fixed while the plus follows within its radius', async () => {
  const port = open()
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 155, clientY: 230 })
  frame()
  expect(Math.hypot(offset().x, offset().y)).toBeCloseTo(6, 2)
  expect(offset().x).toBeGreaterThan(0); expect(offset().y).toBeGreaterThan(0)
  expect(port.attributes('data-handleid')).toBe('images')
  expect(port.attributes('data-handlepos')).toBe(Position.Right)
  expect((port.element as HTMLElement).style.top).toBe('calc(50% + 0px)')
  expect((port.element as HTMLElement).style.transform).toBe('')
  expect(port.attributes('aria-label')).toBe('图片输入 输出 图片')
  expect(port.find('.magnetic-handle-button svg').exists()).toBe(true)
})

it.each([.5, 1, 2])('accounts for canvas zoom %s rather than using raw screen pixels', async zoom => {
  const port = open(zoom)
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 100 + 8.5 * zoom, clientY: 200 + 6.5 * zoom })
  frame()
  expect(offset()).toEqual({ x: 4, y: 3 })
})

it('uses the actual anchor as the input attraction center', async () => {
  const port = open()
  await view!.setProps({ type: 'target', position: Position.Left })
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 94.5, clientY: 208.5 }); frame()
  expect(offset()).toEqual({ x: -3, y: 4 })
})

it('coalesces rapid moves and cancels pending movement when the pointer leaves', async () => {
  const port = open()
  for (let n = 0; n < 12; n++) await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 116 + n, clientY: 220 })
  expect(frames.size).toBe(1)
  await port.trigger('pointerleave'); frame()
  expect(offset()).toEqual({ x: 0, y: 0 })
  expect(port.classes()).not.toContain('is-following')
  expect(frames.size).toBe(0)
})

it('resets before a drag and does not chase a pressed mouse or touch pointer', async () => {
  const port = open()
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 130, clientY: 220 }); frame()
  expect(offset().x).not.toBe(0)
  await port.trigger('pointerdown', { pointerType: 'mouse', buttons: 1 })
  expect(offset()).toEqual({ x: 0, y: 0 })
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 1, clientX: 130, clientY: 220 }); frame()
  expect(offset()).toEqual({ x: 0, y: 0 })
  await port.trigger('pointercancel')
  await port.trigger('pointermove', { pointerType: 'touch', buttons: 0, clientX: 130, clientY: 220 }); frame()
  expect(offset()).toEqual({ x: 0, y: 0 })
})

it('clears pending animation when motion pauses or a recovery draft disables connecting', async () => {
  const port = open()
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 130, clientY: 220 })
  await view!.setProps({ following: false }); frame()
  expect(offset()).toEqual({ x: 0, y: 0 }); expect(frames.size).toBe(0)
  await view!.setProps({ following: true, disabled: true })
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 130, clientY: 220 }); frame()
  expect(offset()).toEqual({ x: 0, y: 0 })
  expect(port.attributes('tabindex')).toBe('-1')
  expect(view!.getComponent(HandleStub).props('connectable')).toBe(false)
})

it('supports keyboard click-to-connect without repeating or activating a disabled port', async () => {
  const port = open(), handle = view!.getComponent(HandleStub)
  await port.trigger('keydown', { key: 'Enter' })
  await port.trigger('keydown', { key: ' ', code: 'Space' })
  expect(handle.emitted('activate')).toHaveLength(2)
  await port.trigger('keydown', { key: 'Enter', repeat: true })
  await view!.setProps({ disabled: true })
  await port.trigger('keydown', { key: 'Enter' })
  expect(handle.emitted('activate')).toHaveLength(2)
})

it('releases the pending frame when the node is removed', async () => {
  const port = open()
  await port.trigger('pointermove', { pointerType: 'mouse', buttons: 0, clientX: 130, clientY: 220 })
  view!.unmount(); view = undefined
  expect(frames.size).toBe(0)
})
