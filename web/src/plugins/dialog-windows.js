import WindowDialog from '@/components/WindowDialog/index.vue'
import DialogWindowDock from '@/components/WindowDialog/Dock.vue'
import { createDialogWindows, dialogWindowsKey } from '@/utils/dialog-windows.mjs'

export default function installDialogWindows(app) {
  app.provide(dialogWindowsKey, createDialogWindows())
  // The existing templates resolve this exact kebab-case name before ElDialog.
  // Keep the native import available for specialized integrations.
  app.component('el-dialog', WindowDialog)
  app.component('WindowDialog', WindowDialog)
  app.component('DialogWindowDock', DialogWindowDock)
}
