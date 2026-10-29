import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile, readdir } from 'node:fs/promises'
import vm from 'node:vm'
import * as Vue from 'vue'
import * as Icons from '@element-plus/icons-vue'
import ElementPlus, { ElConfigProvider, ElFormItem, ElInput, ElButton, ID_INJECTION_KEY } from 'element-plus'
import { renderToString } from '@vue/server-renderer'
import { parse, compileScript } from '@vue/compiler-sfc'
import { parse as parseDOM } from '@vue/compiler-dom'
import { resetERPQuery } from '../src/utils/erp-query.mjs'

const root = new URL('../src/views/erp/', import.meta.url)
async function vueFiles(dir) {
  const out = []
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const url = new URL(entry.name + (entry.isDirectory() ? '/' : ''), dir)
    if (entry.isDirectory()) out.push(...await vueFiles(url))
    else if (entry.name.endsWith('.vue')) out.push(url)
  }
  return out
}
function walk(node, fn) { fn(node); for (const c of node.children || []) walk(c, fn) }
const queryFile = new URL('components/ERPQueryBar.vue', root)
const querySource = await readFile(queryFile, 'utf8')
const queryDescriptor = parse(querySource).descriptor

function queryBar(inlineTemplate) {
  let code = compileScript(queryDescriptor, { id: 'query-render', inlineTemplate }).content
  code = code.replace(/import\s*\{([^}]+)\}\s*from\s*['"](vue|@element-plus\/icons-vue)['"];?/g,
    (_, members, from) => `const {${members.replace(/\bas\b/g, ':')}} = ${from === 'vue' ? 'Vue' : 'Icons'};`)
  return new Function('Vue', 'Icons', code.replace('export default', 'return'))(Vue, Icons)
}

test('query bar uses actual Element Plus global size for fields and both actions', async () => {
  const component = queryBar(true)
  for (const size of ['small', 'default', 'large']) {
    const app = Vue.createSSRApp({
      render: () => Vue.h(ElConfigProvider, { size }, {
        default: () => Vue.h(component, { model: {} }, {
          default: () => Vue.h(ElFormItem, { label: '商品' }, {
            default: () => Vue.h(ElInput, { modelValue: 'SKU-1' })
          }),
          toolbar: () => Vue.h(ElButton, {}, () => '新增商品')
        })
      })
    })
    app.use(ElementPlus)
    app.provide(ID_INJECTION_KEY, { prefix: 1, current: 0 })
    const html = await renderToString(app)
    assert.match(html, /role="search"/)
    assert.match(html, /type="submit"/)
    assert.match(html, /查询/)
    assert.match(html, /重置/)
    // Business actions must be outside the native form to avoid accidental submission.
    assert.ok(html.indexOf('</form>') < html.indexOf('新增商品'))
    if (size === 'default') assert.doesNotMatch(html, /el-(?:input|button)--(?:small|large)/)
    else {
      assert.match(html, new RegExp(`el-input--${size}`))
      assert.equal((html.match(new RegExp(`el-button--${size}`, 'g')) || []).length, 3)
    }
  }
})

test('loading and business locks suppress both query actions', () => {
  const component = queryBar(false), emitted = []
  const props = Vue.reactive({ loading: false, disabled: false, showSearch: true })
  const state = component.setup(props, { expose() {}, emit: (...event) => emitted.push(event) })
  state.search(); state.reset()
  assert.deepEqual(emitted, [['search'], ['reset']])
  for (const flag of ['loading', 'disabled']) {
    props[flag] = true; state.search(); state.reset(); props[flag] = false
  }
  assert.equal(emitted.length, 2)
})

test('reset clears route and visible filters while retaining page length and list defaults', () => {
  for (const defaults of [{ status: 'READY_TO_SHIP' }, { stockState: 'ALL' }, { awaitingReceipt: false }]) {
    const query = Vue.reactive({ pageNum: 8, pageSize: 50, orderId: 91, warehouseId: 3, code: 'old', ...defaults })
    resetERPQuery(query, defaults)
    assert.deepEqual({ ...query }, { ...defaults, pageNum: 1, pageSize: 50 })
  }
})

test('list queries share one submit handler; controls cannot retain fixed small size', async () => {
  for (const file of await vueFiles(root)) {
    const source = await readFile(file, 'utf8'), template = parse(source).descriptor.template
    if (!template) continue
    walk(parseDOM(template.content), node => {
      if (node.type !== 1) return
      assert.ok(!node.props.some(p => p.type === 6 && p.name === 'size' && p.value?.content === 'small'), file.pathname)
      if (node.tag !== 'ERPQueryBar') return
      walk(node, child => {
        if (child.type !== 1) return
        assert.ok(!child.props.some(p => p.type === 7 && p.name === 'on' && p.arg?.content === 'keyup' && p.modifiers.some(m => (m.content || m) === 'enter')), file.pathname)
      })
    })
  }
})

// Exercise actual page reset handlers, including date/ID refs outside query.
test('list reset handlers clear auxiliary filters without erasing a pending command', async () => {
  const cases = [
    ['PurchaseReceiptWorkspace', 'purchaseFilter'], ['PurchaseReturnWorkspace', 'numberFilter'],
    ['SalesReturnWorkspace', 'numberFilter'], ['ShipmentWorkspace', 'dates'],
    ['StockWorkspace', 'times'], ['PackageWorkspace', 'orderText'], ['../importJob/index', 'shopText']
  ]
  for (const [name, auxiliary] of cases) {
    const source = await readFile(new URL(`components/${name}.vue`, root), 'utf8')
    const compiled = compileScript(parse(source).descriptor, { id: name })
    const code = compiled.content
      .replace(/^import\s[^\n]*\n/gm, '').replace('export default', 'const component =')
    // Resolve imported dependencies, then suppress only network initialization.
    const imported = Object.keys(compiled.imports)
    const noAPI = async () => ({ rows: [], total: 0, data: {} })
    const stubs = Object.fromEntries(imported.map(key => [key, noAPI]))
    Object.assign(stubs, Vue, {
      ERPQueryBar: {}, resetERPQuery, ReferenceSelect: {}, PurchasingReferenceSelect: {},
      checkPermi: () => false, useRoute: () => ({ query: {} }), useRouter: () => ({ push() {} }),
      getCurrentInstance: () => ({ proxy: { download() {} } }),
      onMounted() {}, onBeforeUnmount() {}, onDeactivated() {}, onActivated() {}, watch() {},
      defineProps() {}, ElMessage: { warning() {} }, safeID: v => /^\d+$/.test(v) ? Number(v) : undefined
    })
    const component = vm.runInNewContext(code + '\ncomponent', stubs)
    const props = name === 'StockWorkspace' ? { ledger: false } : {}
    const state = component.setup(props, { expose() {} })
    state.query.pageNum = 8; state.query.pageSize = 50; state.query.warehouseId = 9
    state[auxiliary].value = ['dates', 'times'].includes(auxiliary) ? ['2026-01-01', '2026-02-01'] : '91'
    const command = state.pending ? { requestKey: 'must-stay' } : null
    if (command) state.pending.value = command
    await state.reset()
    assert.equal(state.query.pageNum, 1, name)
    assert.equal(state.query.pageSize, 50, name)
    assert.equal(state.query.warehouseId, undefined, name)
    assert.equal(Array.isArray(state[auxiliary].value) ? state[auxiliary].value.length : state[auxiliary].value, Array.isArray(state[auxiliary].value) ? 0 : '', name)
    if (command) assert.equal(state.pending.value.requestKey, 'must-stay', name)
  }
})
