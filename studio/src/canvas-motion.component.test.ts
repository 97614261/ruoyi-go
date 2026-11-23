// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { getBezierPath, Position } from '@vue-flow/core'
import WorkflowEdge from './WorkflowEdge.vue'
import { useCanvasMotion } from './use-canvas-motion'

let view: VueWrapper | undefined
afterEach(() => { view?.unmount(); view = undefined; localStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('keeps stream geometry in sync with the selectable Bezier path and preserves markers', async () => {
  const props = { id: 'connection-1', sourceX: 20, sourceY: 40, targetX: 320, targetY: 240, sourcePosition: Position.Right, targetPosition: Position.Left, markerStart: 'url(#start)', markerEnd: 'url(#end)' }
  view = mount(WorkflowEdge, { props })
  expect(view.find('.vue-flow__edge-path').attributes('marker-end')).toBe('url(#end)')
  expect(view.find('.vue-flow__edge-interaction').attributes('stroke-width')).toBe('24')
  await view.setProps({ targetX: 520, highlighted: true })
  const expectedPath = getBezierPath({ ...props, targetX: 520 })[0]
  for (const path of view.findAll('path')) expect(path.attributes('d')).toBe(expectedPath)
  for (const path of view.findAll('.workflow-edge-stream, .workflow-edge-halo')) {
    expect(path.attributes('aria-hidden')).toBe('true')
    expect(path.attributes('pointer-events')).toBe('none')
  }
  expect(view.get('.workflow-edge').classes()).toContain('is-related')
})

it('restores the pause preference, follows live system motion changes and releases listeners', async () => {
  localStorage.setItem('pod-studio.canvas-motion', 'off')
  let changed: (() => void) | undefined
  const query = { matches: false, addEventListener: vi.fn((_event, listener) => { changed = listener }), removeEventListener: vi.fn() }
  vi.stubGlobal('matchMedia', vi.fn(() => query))
  const remove = vi.spyOn(document, 'removeEventListener')
  let motion!: ReturnType<typeof useCanvasMotion>
  view = mount(defineComponent({ setup() { motion = useCanvasMotion(); return () => null } }))
  expect(motion.motionEnabled.value).toBe(false)
  motion.toggleMotion(); expect(localStorage.getItem('pod-studio.canvas-motion')).toBe('on')
  query.matches = true; changed!(); await nextTick()
  expect(motion.reducedMotion.value).toBe(true)
  motion.toggleMotion(); expect(motion.motionEnabled.value).toBe(true)
  query.matches = false; changed!(); expect(motion.reducedMotion.value).toBe(false)
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
  document.dispatchEvent(new Event('visibilitychange')); expect(motion.documentHidden.value).toBe(true)
  view.unmount(); view = undefined
  expect(query.removeEventListener).toHaveBeenCalledWith('change', changed)
  expect(remove).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
})

it('retains local motion control if browser storage is unavailable', () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked') })
  let motion!: ReturnType<typeof useCanvasMotion>
  view = mount(defineComponent({ setup() { motion = useCanvasMotion(); return () => null } }))
  expect(motion.motionEnabled.value).toBe(true)
  expect(() => motion.toggleMotion()).not.toThrow()
  expect(motion.motionEnabled.value).toBe(false)
})
