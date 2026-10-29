import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import * as Vue from 'vue'
import { ElDialog } from 'element-plus'
import { parse } from '@vue/compiler-sfc'
import { createDialogWindow, createDialogWindows, dialogWindowsKey } from '../src/utils/dialog-windows.mjs'

function setup(t, overrides = {}, manager = createDialogWindows()) {
  const props = Vue.reactive({ modelValue: true, title: '编辑资料', fullscreen: false, showClose: true, windowControls: true, minimizable: true, maximizable: true, destroyOnClose: true, ...overrides })
  const events = [], state = createDialogWindow(props, { emit: (...args) => events.push(args), manager, id: Symbol(), nextZIndex: () => 2020 })
  t.after(state.dispose)
  return { props, state, manager, events, count: name => events.filter(e => e[0] === name).length }
}

test('minimize releases native visibility without logical close or destroying the form; restore emits no second open', async t => {
  const a = setup(t)
  await Vue.nextTick(); a.state.onOpened()
  a.state.minimize(); a.state.onClose(); a.state.onClosed()
  assert.equal(a.state.visible.value, false); assert.equal(a.state.retained.value, true)
  assert.equal(a.props.modelValue, true); assert.equal(a.manager.windows.size, 1)
  assert.equal(a.count('close'), 0); assert.equal(a.count('closed'), 0); assert.equal(a.count('update:modelValue'), 0)
  a.state.restore(); a.state.onOpen(); a.state.onOpened()
  assert.equal(a.state.visible.value, true); assert.equal(a.state.retained.value, true)
  assert.equal(a.count('open'), 1); assert.equal(a.count('opened'), 1); assert.equal(a.manager.windows.size, 0)
})

test('maximized window restores its prior mode after minimize; explicit fullscreen updates remain authoritative', async t => {
  const a = setup(t)
  a.state.toggleMaximize(); assert.equal(a.state.mode.value, 'maximized')
  a.state.minimize(); a.state.restore(); assert.equal(a.state.mode.value, 'maximized')
  a.state.toggleMaximize(); assert.equal(a.state.mode.value, 'normal')
  a.state.minimize(); a.props.fullscreen = true; await Vue.nextTick()
  a.state.restore(); assert.equal(a.state.mode.value, 'maximized')
})

test('dock close observes promise guard cancellation, admits one pending attempt, and destroys only on actual close', async t => {
  let done, attempts = 0
  const a = setup(t, { beforeClose: callback => { attempts++; done = callback; return new Promise(() => {}) } })
  a.state.minimize(); const entry = [...a.manager.windows.values()][0]
  entry.close(); entry.close(); assert.equal(attempts, 1); assert.equal(entry.closable(), false)
  done(true); done(); await Vue.nextTick()
  assert.equal(a.count('close'), 0); assert.equal(a.state.retained.value, true)
  entry.close(); done(); await Vue.nextTick(); a.state.onClosed()
  assert.equal(a.count('close'), 1); assert.equal(a.count('closed'), 1); assert.equal(a.count('update:modelValue'), 1)
  assert.equal(a.state.retained.value, false); assert.equal(a.manager.windows.size, 0)
})

test('old asynchronous close cannot close a restored or reopened window', async t => {
  let done
  const a = setup(t, { beforeClose: callback => { done = callback } })
  a.state.minimize(); [...a.manager.windows.values()][0].close()
  a.state.restore(); done(); await Vue.nextTick()
  assert.equal(a.state.visible.value, true); assert.equal(a.count('closed'), 0)
  a.state.guardClose(() => a.state.onClose()); const oldDone = done
  a.props.modelValue = false; a.state.onClosed(); a.props.modelValue = true
  oldDone(); a.state.onClosed(); await Vue.nextTick()
  assert.equal(a.state.phase.value, 'opening'); assert.equal(a.state.visible.value, true)
  assert.equal(a.count('closed'), 1)
})

test('parent closes minimized windows exactly once and next session starts with default geometry', async t => {
  const a = setup(t)
  a.state.toggleMaximize(); a.state.minimize(); a.props.modelValue = false
  await Vue.nextTick(); a.state.onClose(); a.state.onClosed()
  assert.equal(a.count('close'), 1); assert.equal(a.count('closed'), 1); assert.equal(a.state.retained.value, false)
  a.props.modelValue = true
  assert.equal(a.state.mode.value, 'normal'); assert.equal(a.state.visible.value, true)
})

test('multiple minimized windows are independent and route leave/unmount removes every stale entry', async t => {
  const manager = createDialogWindows(), a = setup(t, {}, manager), b = setup(t, { title: '另一个窗口' }, manager)
  a.state.minimize(); b.state.minimize(); assert.equal(manager.windows.size, 2)
  b.props.title = '已更新标题'; assert.ok([...manager.windows.values()].some(e => e.title() === '已更新标题'))
  a.state.deactivate(); assert.equal(manager.windows.size, 1); assert.equal(a.count('closed'), 1)
  a.state.activate(); assert.equal(a.state.visible.value, false)
  b.state.dispose(); assert.equal(manager.windows.size, 0)
})

test('saving locks and per-window opt-outs cannot be bypassed through exposed window actions', t => {
  for (const blocked of [{ showClose: false }, { windowControlsDisabled: true }, { windowControls: false }]) {
    const a = setup(t, blocked); a.state.minimize(); a.state.toggleMaximize()
    assert.equal(a.state.mode.value, 'normal'); assert.equal(a.manager.windows.size, 0)
  }
  const a = setup(t, { minimizable: false }); a.state.minimize(); assert.equal(a.state.mode.value, 'normal')
  const b = setup(t, { maximizable: false }); b.state.toggleMaximize(); assert.equal(b.state.mode.value, 'normal')
})

test('destroyOnClose=false retains subtree after real close; guard exceptions release the attempt lock', async t => {
  const a = setup(t, { destroyOnClose: false })
  a.props.modelValue = false; a.state.onClosed(); assert.equal(a.state.retained.value, true)
  const b = setup(t, { beforeClose() { throw new Error('guard failed') } })
  assert.throws(() => b.state.guardClose(() => {}), /guard failed/)
  assert.equal(b.state.closePending.value, false)
})

test('native callback-style cancel without done allows retry; rejected promise guards release their lock', async t => {
  const callbacks = [], a = setup(t, { beforeClose: done => { callbacks.push(done) } })
  let closed = 0
  a.state.guardClose(() => closed++); a.state.guardClose(() => closed++)
  assert.equal(callbacks.length, 2); assert.equal(a.state.closePending.value, false)
  callbacks[0](); assert.equal(closed, 0)
  callbacks[1](); assert.equal(closed, 1)
  const b = setup(t, { beforeClose: () => Promise.reject(new Error('cancel')) })
  b.state.guardClose(() => assert.fail('canceled guard closed window'))
  assert.equal(b.state.closePending.value, true)
  await Vue.nextTick(); assert.equal(b.state.closePending.value, false)
})

// Mount the actual wrapper with a native-dialog protocol stub. No browser DOM
// or focus claim: the dev fixture exercises the real Element Plus dialog.
test('actual wrapper forwards native props/slots/ref and keeps a mounted editor across minimize/fullscreen', async () => {
  let native, mounted = 0, unmounted = 0, field, lastProps, resets = 0
  const NativeStub = Vue.defineComponent({
    props: ElDialog.props, emits: ElDialog.emits,
    setup(props, { slots, emit, expose }) {
      const seen = Vue.ref(false)
      const handleClose = () => props.beforeClose(() => { emit('close'); emit('closed'); emit('update:modelValue', false) })
      native = { handleClose }; expose({ handleClose, visible: Vue.computed(() => props.modelValue), resetPosition() { resets++ } })
      Vue.watch(() => props.modelValue, value => { if (value) { seen.value = true; emit('open'); emit('opened') } else { emit('close'); emit('closed'); emit('update:modelValue', false) } })
      return () => { lastProps = props; return Vue.h('section', {}, seen.value ? [slots.header?.({ titleId: 't', titleClass: 'title', close: handleClose }), slots.default?.(), slots.footer?.()] : []) }
    }
  })
  const source = parse(await readFile(new URL('../src/components/WindowDialog/index.vue', import.meta.url), 'utf8')).descriptor.script.content
  const context = { ...Vue, NativeDialog: NativeStub, ElButton: 'button', useZIndex: () => ({ nextZIndex: () => 2020 }), Minus: {}, FullScreen: {}, CopyDocument: {}, Close: {}, createDialogWindow, createDialogWindows, dialogWindowsKey }
  const keys = ['defineComponent', 'h', 'ref', 'inject', 'computed', 'onActivated', 'onDeactivated', 'onBeforeUnmount', 'NativeDialog', 'ElButton', 'useZIndex', 'Minus', 'FullScreen', 'CopyDocument', 'Close', 'createDialogWindow', 'createDialogWindows', 'dialogWindowsKey']
  const component = new Function(...keys, source.replace(/^import[^\n]*\n/gm, '').replace('export default', 'return'))(...keys.map(key => context[key]))
  const host = () => ({ children: [], parent: null })
  const renderer = Vue.createRenderer({
    createElement: host, createText: text => ({ ...host(), text }), createComment: host,
    setText: (n, text) => { n.text = text }, setElementText: (n, text) => { n.text = text }, patchProp() {},
    insert: (n, parent) => { if (n.parent) n.parent.children = n.parent.children.filter(c => c !== n); n.parent = parent; parent.children.push(n) },
    remove: n => { if (n.parent) n.parent.children = n.parent.children.filter(c => c !== n) },
    parentNode: n => n.parent, nextSibling: () => null
  })
  const Editor = Vue.defineComponent({ setup() { const value = Vue.ref('initial'); field = value; Vue.onMounted(() => mounted++); Vue.onBeforeUnmount(() => unmounted++); return () => Vue.h('input', { value: value.value }) } })
  const dragging = Vue.ref(undefined), visible = Vue.ref(false), dialog = Vue.ref(), manager = createDialogWindows(), events = []
  const app = renderer.createApp({ render: () => Vue.h(component, {
    ref: dialog, modelValue: visible.value, title: '编辑', width: '760px', destroyOnClose: true, draggable: dragging.value,
    'onUpdate:modelValue': value => { visible.value = value }, onClose: () => events.push('close'), onClosed: () => events.push('closed')
  }, { header: scope => Vue.h('strong', { id: scope.titleId }, '自定义标题'), default: () => Vue.h(Editor), footer: () => Vue.h('button', {}, '保存') }) })
  app.provide(dialogWindowsKey, manager); app.mount(host())
  visible.value = true; await Vue.nextTick(); await Vue.nextTick()
  assert.equal(mounted, 1); field.value = 'unsaved'
  assert.equal(lastProps.draggable, true); assert.equal(resets, 1)
  dragging.value = false; await Vue.nextTick(); assert.equal(lastProps.draggable, false)
  dragging.value = true; await Vue.nextTick()
  assert.equal(lastProps.width, '760px'); assert.equal(lastProps.destroyOnClose, false)
  dialog.value.toggleMaximize(); await Vue.nextTick(); assert.equal(lastProps.fullscreen, true)
  dialog.value.minimize(); await Vue.nextTick(); await Vue.nextTick()
  assert.equal(events.length, 0); assert.equal(unmounted, 0); assert.equal(manager.windows.size, 1)
  dialog.value.restore(); await Vue.nextTick(); await Vue.nextTick()
  assert.equal(resets, 1); // Visual restore must not reset a dragged window.
  assert.equal(field.value, 'unsaved'); assert.equal(mounted, 1); assert.equal(lastProps.fullscreen, true)
  native.handleClose(); await Vue.nextTick(); await Vue.nextTick()
  assert.deepEqual(events, ['close', 'closed']); assert.equal(visible.value, false); assert.equal(unmounted, 1)
  visible.value = true; await Vue.nextTick(); await Vue.nextTick()
  assert.equal(resets, 2); // A genuinely reopened form resets its geometry.
  dialog.value.minimize(); await Vue.nextTick(); await Vue.nextTick()
  // Restore and parent-close in the same flush: the native dialog never sees
  // the intermediate true value and therefore cannot emit a leave transition.
  dialog.value.restore(); visible.value = false
  await Vue.nextTick(); await Vue.nextTick(); await Vue.nextTick()
  assert.equal(unmounted, 2); assert.deepEqual(events, ['close', 'closed', 'close', 'closed'])
  app.unmount(); assert.equal(manager.windows.size, 0)
})
