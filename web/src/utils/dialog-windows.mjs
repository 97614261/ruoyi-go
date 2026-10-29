import { shallowReactive, ref, watch, nextTick } from 'vue'

export const dialogWindowsKey = Symbol('dialog-windows')

export function createDialogWindows() {
  const windows = shallowReactive(new Map())
  return { windows, add: entry => windows.set(entry.id, entry), remove: id => windows.delete(id) }
}

// Logical close and visual minimize are separate. The native dialog hides to
// release its focus trap/scroll lock, while its slot subtree remains mounted.
export function createDialogWindow(props, { emit, manager, id, nextZIndex = () => 2000, nativeVisible }) {
  const visible = ref(false), retained = ref(false), mode = ref('normal'), phase = ref('closed'), closePending = ref(false)
  let previousMode = 'normal', epoch = 0, active = true, disposed = false
  let openSent = false, openedSent = false, closeSent = false, updateSent = false
  const remove = () => manager.remove(id)
  const controlsDisabled = () => props.windowControlsDisabled || !props.showClose || closePending.value
  const title = () => props.windowTitle || props.title || '未命名弹窗'
  const notifyOpen = () => {
    if (!openSent && active && props.modelValue && phase.value === 'opening' && mode.value !== 'minimized') { openSent = true; emit('open') }
  }
  function notifyClose() { if (!closeSent) { closeSent = true; emit('close') } }
  function finishClose() {
    if (disposed || phase.value !== 'closing') return
    ++epoch; closePending.value = false; visible.value = false; remove()
    phase.value = 'closed'; mode.value = props.fullscreen ? 'maximized' : 'normal'
    if (props.destroyOnClose) retained.value = false
    emit('closed')
    if (!updateSent) { updateSent = true; emit('update:modelValue', false) }
  }
  function close(hidden = false, fromNative = false) {
    if (disposed || phase.value === 'closed' || phase.value === 'closing') return
    const ticket = ++epoch
    const wasHidden = hidden || mode.value === 'minimized' || !visible.value || (!fromNative && nativeVisible && !nativeVisible())
    phase.value = 'closing'; closePending.value = false; visible.value = false; remove(); notifyClose()
    if (wasHidden) nextTick(() => { if (ticket === epoch) finishClose() })
  }
  function open() {
    ++epoch; remove(); closePending.value = false
    openSent = openedSent = closeSent = updateSent = false
    mode.value = props.fullscreen ? 'maximized' : 'normal'; previousMode = mode.value
    retained.value = true; phase.value = 'opening'; visible.value = true
    nextTick(notifyOpen)
  }
  function guardClose(done) {
    if (disposed || !active || !props.modelValue || closePending.value || ['closed', 'closing'].includes(phase.value)) return
    const ticket = ++epoch
    let completed = false
    closePending.value = true
    const complete = cancel => {
      if (completed || ticket !== epoch || disposed || !active || !props.modelValue) return
      completed = true
      closePending.value = false
      if (!cancel) done()
    }
    try {
      if (!props.beforeClose) { complete(); return }
      const result = props.beforeClose(complete)
      if (result && typeof result.then === 'function') {
        const settled = () => { if (ticket === epoch && !completed) { completed = true; closePending.value = false } }
        Promise.resolve(result).then(settled, settled)
      } else if (!completed) {
        // Native callback-style guards may cancel simply by never calling done.
        // They provide no settlement signal: allow a fresh attempt and invalidate
        // the previous callback via epoch instead of locking the window forever.
        closePending.value = false
      }
      return result
    }
    catch (error) { completed = true; if (ticket === epoch) closePending.value = false; throw error }
  }
  function minimize() {
    if (!active || disposed || !props.modelValue || !props.windowControls || !props.minimizable || controlsDisabled() || ['closed', 'closing'].includes(phase.value) || mode.value === 'minimized') return
    ++epoch; previousMode = mode.value; mode.value = 'minimized'; visible.value = false
    manager.add({ id, title, restore, close: closeMinimized, closable: () => props.showClose && !props.windowControlsDisabled && !closePending.value, zIndex: nextZIndex() })
    emit('minimize')
  }
  function closeMinimized() { if (mode.value === 'minimized' && !controlsDisabled()) guardClose(() => close(true)) }
  function restore() {
    if (!active || disposed || !props.modelValue || mode.value !== 'minimized' || ['closed', 'closing'].includes(phase.value)) return
    ++epoch; closePending.value = false; remove(); mode.value = previousMode; visible.value = true; emit('restore')
  }
  function toggleMaximize() {
    if (!active || disposed || !props.modelValue || !props.windowControls || !props.maximizable || controlsDisabled() || mode.value === 'minimized' || ['closed', 'closing'].includes(phase.value)) return
    ++epoch; closePending.value = false
    mode.value = mode.value === 'maximized' ? 'normal' : 'maximized'
    emit(mode.value === 'maximized' ? 'maximize' : 'restore')
  }
  const stopModel = watch(() => props.modelValue, value => { if (!active || disposed) return; if (value) open(); else close() }, { immediate: true, flush: 'sync' })
  const stopFullscreen = watch(() => props.fullscreen, value => {
    const next = value ? 'maximized' : 'normal'
    if (mode.value === 'minimized') previousMode = next
    else mode.value = next
  })
  function deactivate() {
    if (!active || disposed) return
    active = false; ++epoch; closePending.value = false; remove()
    if (phase.value !== 'closed') { phase.value = 'closing'; visible.value = false; notifyClose(); finishClose() }
  }
  function dispose() { disposed = true; active = false; ++epoch; remove(); stopModel(); stopFullscreen() }
  return {
    visible, retained, mode, phase, closePending, title, controlsDisabled,
    minimize, restore, toggleMaximize, closeMinimized, guardClose, deactivate, dispose, activate: () => { active = true },
    onOpen: notifyOpen,
    onOpened: () => { if (active && props.modelValue && phase.value === 'opening' && mode.value !== 'minimized' && !openedSent) { notifyOpen(); openedSent = true; phase.value = 'open'; emit('opened') } },
    onClose: () => { if (mode.value !== 'minimized') close(false, true) },
    onClosed: finishClose,
    onOpenAutoFocus: () => { if (phase.value === 'opening') emit('openAutoFocus') },
    onCloseAutoFocus: () => { if (phase.value === 'closing' || phase.value === 'closed') emit('closeAutoFocus') }
  }
}
