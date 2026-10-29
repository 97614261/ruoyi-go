import test from 'node:test'
import assert from 'node:assert/strict'
import { createSSRApp, defineComponent, h, ref } from 'vue'
import { renderToString } from '@vue/server-renderer'
import { createAsyncEditor } from '../src/components/Editor/async.mjs'

test('global Editor registration and unrelated pages do not invoke the editor loader', async () => {
  let calls = 0
  const Editor = createAsyncEditor(async () => { calls++; return { render: () => h('div', {}, 'editor') } })
  const app = createSSRApp({ render: () => h('main', {}, '普通页面') })
  app.component('Editor', Editor)
  await renderToString(app)
  assert.equal(calls, 0)
})

test('async Editor forwards initial HTML, sizing, readonly and model updates; successful module is reused', async () => {
  let calls = 0, received
  const value = ref('<p>已有富文本</p>')
  const Editor = createAsyncEditor(async () => {
    calls++
    return defineComponent({
      props: { modelValue: String, height: Number, readOnly: Boolean }, emits: ['update:modelValue'],
      setup(props, { emit }) { received = { ...props }; emit('update:modelValue', '<p>修改后的富文本</p>'); return () => h('article', {}, props.modelValue) }
    })
  })
  const app = createSSRApp({ render: () => h(Editor, { modelValue: value.value, height: 384, readOnly: true, 'onUpdate:modelValue': v => { value.value = v } }) })
  const html = await renderToString(app)
  assert.deepEqual(received, { modelValue: '<p>已有富文本</p>', height: 384, readOnly: true })
  assert.equal(value.value, '<p>修改后的富文本</p>'); assert.match(html, /已有富文本/)
  await Editor.__asyncLoader(); await Editor.__asyncLoader()
  assert.equal(calls, 1)
})

test('failed editor chunk is retried once and persistent failures stay bounded', async () => {
  let calls = 0
  const recovered = createAsyncEditor(async () => { if (++calls === 1) throw new Error('temporary chunk failure'); return { render: () => h('div') } })
  await recovered.__asyncLoader(); assert.equal(calls, 2)
  calls = 0
  const failed = createAsyncEditor(async () => { calls++; throw new Error('persistent chunk failure') })
  await assert.rejects(failed.__asyncLoader(), /persistent chunk failure/)
  assert.equal(calls, 2)
})
