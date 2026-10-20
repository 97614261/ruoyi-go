import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { randomUUID } from 'node:crypto'
import { ref, reactive, computed } from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as erp from '../src/utils/erp.mjs'

const deferred = () => {
  let resolve, reject
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail })
  return { promise, resolve, reject }
}

// Execute the real SFC setup logic with injected API promises. This is a
// component-state test, NOT a browser, DOM, rendering or click acceptance test.
async function component(name, overrides = {}, props = {}) {
  const file = new URL(`../src/views/erp/components/${name}.vue`, import.meta.url)
  const { descriptor } = parse(await readFile(file, 'utf8'))
  const compiled = compileScript(descriptor, { id: `test-${name}` }).content
    .replace(/^import\s[^\n]*\n/gm, '').replace('export default', 'const component =')
  const messages = [], prompts = []
  const stubs = {
    ref, reactive, computed, onMounted() {}, ReferenceSelect: {},
    crypto: { randomUUID }, ...erp,
    useUserStore: () => ({ id: 1, identity: { tenantAdmin: true } }),
    ElMessage: { warning: text => messages.push(['warning', text]), success: text => messages.push(['success', text]) },
    ElMessageBox: { confirm: async text => { prompts.push(text) } },
    listDocuments: async () => ({ rows: [], total: 0 }),
    listErpWarehouse: async () => ({ rows: [], total: 0 }),
    listUser: async () => ({ rows: [] }),
    getWarehouseAccess: async () => ({ data: { warehouseIds: [], version: 1 } }),
    setWarehouseAccess: async () => {}, createDocument: async () => {},
    editDocument: async () => {}, actOnDocument: async () => {}, getDocument: async () => ({ data: {} }),
    ...overrides
  }
  const definition = vm.runInNewContext(`${compiled}\ncomponent`, stubs, { filename: file.pathname })
  const state = definition.setup(props, { expose() {} })
  return { state, messages, prompts }
}

test('document double submit sends one request and preserves draft on timeout/offline rejection', async () => {
  for (const message of ['Network Error', 'timeout of 10000ms exceeded', 'warehouse access revoked']) {
    const pending = deferred(); let writes = 0
    const { state, messages } = await component('DocumentWorkspace', {
      createDocument: () => { writes++; return pending.promise }
    }, { type: 'receipt' })
    state.start()
    Object.assign(state.form.value, { warehouseId: 1, lines: [{ skuId: 2, quantity: 3 }], remark: 'keep my draft' })
    const requestKey = state.form.value.requestKey
    const first = state.save(); await state.save()
    assert.equal(writes, 1); assert.equal(state.saving.value, true)
    pending.reject(new Error(message)); await assert.rejects(first, new RegExp(message.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
    assert.equal(state.saving.value, false); assert.equal(state.open.value, true)
    assert.equal(state.form.value.remark, 'keep my draft'); assert.equal(state.form.value.requestKey, requestKey)
    assert.equal(messages.filter(([type]) => type === 'success').length, 0)
  }
})

test('successful draft retry reuses the original creation idempotency key', async () => {
  let attempts = 0; const payloads = []
  const { state, messages } = await component('DocumentWorkspace', {
    createDocument: async (_, payload) => { payloads.push(payload); if (++attempts === 1) throw new Error('Network Error') }
  }, { type: 'receipt' })
  state.start(); Object.assign(state.form.value, { warehouseId: 1, lines: [{ skuId: 2, quantity: 1 }] })
  await assert.rejects(state.save(), /Network Error/); await state.save()
  assert.equal(payloads[0].requestKey, payloads[1].requestKey)
  assert.equal(state.open.value, false); assert.equal(messages.filter(([type]) => type === 'success').length, 1)
})

test('document confirmation cancellation and failure release busy state without a success message', async () => {
  let writes = 0
  const { state, messages } = await component('DocumentWorkspace', {
    ElMessageBox: { confirm: async () => { throw new Error('cancel') } },
    actOnDocument: async () => { writes++ }
  }, { type: 'sales' })
  await state.act({ documentId: 1 }, 'confirm')
  assert.equal(writes, 0); assert.equal(state.working.value, false); assert.equal(messages.length, 0)
  const failure = await component('DocumentWorkspace', { actOnDocument: async () => { throw new Error('scope denied') } }, { type: 'sales' })
  await assert.rejects(failure.state.act({ documentId: 1 }, 'ship'), /scope denied/)
  assert.equal(failure.state.working.value, false); assert.equal(failure.messages.length, 0)
})

test('document repeated action while confirmation is pending opens one prompt only', async () => {
  const pending = deferred(); let prompts = 0, writes = 0
  const { state } = await component('DocumentWorkspace', {
    ElMessageBox: { confirm: () => { prompts++; return pending.promise } },
    actOnDocument: async () => { writes++ }
  }, { type: 'sales' })
  const first = state.act({ documentId: 1 }, 'ship'); await state.act({ documentId: 1 }, 'ship')
  assert.equal(prompts, 1); pending.resolve(); await first; assert.equal(writes, 1)
})

test('warehouse authorization blocks repeated save before the confirmation resolves', async () => {
  const pending = deferred(); let prompts = 0, writes = 0
  const { state } = await component('WarehouseAccess', {
    ElMessageBox: { confirm: () => { prompts++; return pending.promise } },
    setWarehouseAccess: async () => { writes++ }
  })
  state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
  const first = state.save(); const second = state.save()
  assert.equal(prompts, 1)
  pending.resolve(); await Promise.all([first, second]); assert.equal(writes, 1); assert.equal(state.saving.value, false)
})

test('warehouse canceled/rejected authorization never reports success or closes the form', async () => {
  for (const cancelPrompt of [true, false]) {
    let writes = 0
    const { state, messages } = await component('WarehouseAccess', {
      ElMessageBox: { confirm: async () => { if (cancelPrompt) throw new Error('cancel') } },
      setWarehouseAccess: async () => { writes++; throw new Error('stale version') }
    })
    state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
    if (cancelPrompt) await state.save(); else await assert.rejects(state.save(), /stale version/)
    assert.equal(writes, cancelPrompt ? 0 : 1); assert.equal(state.open.value, true)
    assert.equal(state.saving.value, false); assert.equal(messages.length, 0)
  }
})

test('closed/reopened warehouse form ignores the old employee scope response', async () => {
  const pending = deferred()
  const { state } = await component('WarehouseAccess', { getWarehouseAccess: () => pending.promise })
  state.open.value = true; state.userId.value = 2
  const old = state.chooseUser(2); state.open.value = false; state.start()
  pending.resolve({ data: { version: 9, warehouseIds: [999] } }); await old
  assert.equal(state.userId.value, undefined); assert.equal(state.version.value, undefined)
  assert.equal(state.selected.value.length, 0)
})
