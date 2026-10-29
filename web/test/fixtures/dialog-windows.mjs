// Local dev fixture only; never imported by the production entry or router.
import { createApp, defineComponent, h, ref, resolveComponent, onMounted, onBeforeUnmount } from 'vue'
import ElementPlus, { ElButton, ElInput, ElConfigProvider } from 'element-plus'
import 'element-plus/dist/index.css'
import '@/assets/styles/index.scss'
import installDialogWindows from '@/plugins/dialog-windows.js'

let mounted = ref(0), unmounted = ref(0)
const Editor = defineComponent({ setup() {
  const value = ref('未保存的表单内容')
  onMounted(() => mounted.value++); onBeforeUnmount(() => unmounted.value++)
  return () => h('div', {}, [h('p', {}, '最小化/最大化后应保留输入；关闭后重新打开应恢复初始值。'), h(ElInput, { modelValue: value.value, 'onUpdate:modelValue': v => { value.value = v }, 'aria-label': '弹窗输入' }), h('div', { style: 'height:700px;padding-top:16px' }, '长内容：底部保存按钮应始终可达。')])
} })
const Root = defineComponent({ setup() {
  const a = ref(false), b = ref(false), busy = ref(false), blocked = ref(false), size = ref('default'), page = ref(''), events = ref([])
  const record = event => events.value.push(event)
  return () => h(ElConfigProvider, { size: size.value }, { default: () => {
    const Dialog = resolveComponent('el-dialog'), Dock = resolveComponent('DialogWindowDock')
    return h('main', { style: 'padding:20px' }, [
      h('h2', {}, '全局弹窗本地验收（无业务接口）'),
      h('div', { style: 'display:flex;flex-wrap:wrap;gap:8px;margin-bottom:12px' }, [
        h(ElButton, { onClick: () => { a.value = true } }, () => '打开表单'),
        h(ElButton, { onClick: () => { b.value = true } }, () => '打开第二个弹窗'),
        h(ElButton, { onClick: () => { busy.value = !busy.value } }, () => busy.value ? '解除保存锁' : '模拟保存锁'),
        h(ElButton, { onClick: () => { blocked.value = !blocked.value } }, () => blocked.value ? '允许关闭' : '阻止关闭'),
        ...['small', 'default', 'large'].map(v => h(ElButton, { onClick: () => { size.value = v } }, () => v))
      ]),
      h(ElInput, { modelValue: page.value, 'onUpdate:modelValue': v => { page.value = v }, 'aria-label': '页面输入', placeholder: '最小化后应能输入和操作页面' }),
      h('pre', { id: 'result' }, JSON.stringify({ a: a.value, b: b.value, mounted: mounted.value, unmounted: unmounted.value, events: events.value }, null, 2)),
      h(Dialog, { modelValue: a.value, 'onUpdate:modelValue': v => { a.value = v }, title: '测试表单', width: 'min(760px, calc(100vw - 24px))', class: 'erp-dialog', destroyOnClose: true, showClose: !busy.value,
        beforeClose: done => { if (!blocked.value) done(); else done(true) }, onOpen: () => record('open'), onOpened: () => record('opened'), onClose: () => record('close'), onClosed: () => record('closed')
      }, { default: () => h(Editor), footer: () => h(ElButton, { type: 'primary', onClick: () => { a.value = false } }, () => '保存并关闭') }),
      h(Dialog, { modelValue: b.value, 'onUpdate:modelValue': v => { b.value = v }, title: '第二个弹窗', width: '520px' }, { default: () => h('p', {}, '这个窗口的最小化与恢复不应影响测试表单。') }),
      h(Dock)
    ])
  } })
} })
const app = createApp(Root)
app.use(ElementPlus); installDialogWindows(app); app.mount('#app')
