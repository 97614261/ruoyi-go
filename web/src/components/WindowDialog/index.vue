<script>
import { defineComponent, h, ref, inject, computed, onActivated, onDeactivated, onBeforeUnmount } from 'vue'
import { ElDialog as NativeDialog, ElButton, useZIndex } from 'element-plus'
import { Minus, FullScreen, CopyDocument, Close } from '@element-plus/icons-vue'
import { createDialogWindow, createDialogWindows, dialogWindowsKey } from '@/utils/dialog-windows.mjs'

export default defineComponent({
  name: 'WindowDialog',
  inheritAttrs: false,
  props: {
    ...NativeDialog.props,
    appendToBody: { type: Boolean, default: true },
    // Use native viewport-bounded dragging; callers may explicitly opt out.
    draggable: { type: Boolean, default: true },
    windowControls: { type: Boolean, default: true },
    minimizable: { type: Boolean, default: true },
    maximizable: { type: Boolean, default: true },
    windowControlsDisabled: Boolean,
    windowTitle: String
  },
  emits: { ...NativeDialog.emits, minimize: () => true, maximize: () => true, restore: () => true },
  setup(props, { attrs, slots, emit, expose }) {
    const dialog = ref()
    const manager = inject(dialogWindowsKey, null) || createDialogWindows()
    const { nextZIndex } = useZIndex()
    const state = createDialogWindow(props, { emit, manager, id: Symbol('dialog'), nextZIndex, nativeVisible: () => !!dialog.value?.visible })
    const fullscreen = computed(() => state.mode.value === 'maximized')
    function handleClose() {
      if (state.mode.value === 'minimized') state.closeMinimized()
      else dialog.value?.handleClose()
    }
    const button = (label, icon, onClick, disabled = false) => h(ElButton, {
      text: true, circle: true, icon, nativeType: 'button', title: label, 'aria-label': label,
      disabled, onClick, onMousedown: event => event.stopPropagation()
    })
    expose({
      get visible() { return dialog.value?.visible },
      get dialogContentRef() { return dialog.value?.dialogContentRef },
      handleClose,
      resetPosition: () => dialog.value?.resetPosition(),
      minimize: state.minimize, restore: state.restore, toggleMaximize: state.toggleMaximize,
      windowState: state.mode
    })
    onActivated(state.activate)
    onDeactivated(state.deactivate)
    onBeforeUnmount(state.dispose)
    return () => {
      const nativeProps = Object.fromEntries(Object.keys(NativeDialog.props).map(key => [key, props[key]]))
      return h(NativeDialog, {
        ...nativeProps, ...attrs, ref: dialog, class: ['app-window-dialog', attrs.class],
        modelValue: state.visible.value, fullscreen: fullscreen.value,
        // Native hiding releases focus/scroll, but never destroys a minimized form.
        destroyOnClose: false, showClose: false, beforeClose: state.guardClose,
        onOpen: state.onOpen, onOpened: () => {
          // A fresh logical session starts centered; minimize/restore retains its position.
          if (state.phase.value === 'opening') dialog.value?.resetPosition()
          state.onOpened()
        }, onClose: state.onClose, onClosed: state.onClosed,
        'onUpdate:modelValue': () => {},
        onOpenAutoFocus: state.onOpenAutoFocus, onCloseAutoFocus: state.onCloseAutoFocus
      }, {
        header: scope => h('div', { class: 'app-window-dialog__header' }, [
          h('div', { class: 'app-window-dialog__title' }, slots.header ? slots.header(scope) : slots.title ? slots.title() : h('span', { id: scope.titleId, class: scope.titleClass, role: 'heading', 'aria-level': props.headerAriaLevel }, props.title)),
          h('div', { class: 'app-window-dialog__actions', onMousedown: event => event.stopPropagation() }, [
            props.windowControls && props.minimizable ? button('最小化', Minus, state.minimize, state.controlsDisabled()) : null,
            props.windowControls && props.maximizable ? button(fullscreen.value ? '还原' : '最大化', fullscreen.value ? CopyDocument : FullScreen, state.toggleMaximize, state.controlsDisabled()) : null,
            props.showClose ? button('关闭弹窗', props.closeIcon || Close, handleClose, state.closePending.value) : null
          ])
        ]),
        default: () => state.retained.value ? slots.default?.() : null,
        ...(slots.footer ? { footer: () => state.retained.value ? slots.footer() : null } : {})
      })
    }
  }
})
</script>
