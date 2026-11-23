// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import BatchDialog from './BatchDialog.vue'
import Batches from './Batches.vue'
import { pendingBatch, retryBatch, submitBatch } from './batch-requests'
import type { BatchRequest, TemplateVersion } from './contracts'
const snapshot: TemplateVersion = { templateId: '5', version: '2', createTime: '', slots: [{ nodeId: 'T', type: 'textInput', label: '创意', text: 'original' }], outputs: [] }
const limits = { maxUploadBytes: 100000, maxPixels: 1000000, maxDimension: 8192, maxImagesPerNode: 8 }
const intent = (): BatchRequest => ({ requestId: crypto.randomUUID(), templateId: '5', version: '2', name: 'batch', executionMode: 'qwen', items: [{ name: 'one', bindings: [] }] })
let wrapper: VueWrapper | null = null, lost = false
let calls: { path: string; body: Record<string, unknown> }[]
const button = (text: string) => wrapper!.findAll('button').find(b => b.text() === text)!
beforeEach(() => {
  calls = []; lost = false; sessionStorage.clear()
  vi.stubGlobal('fetch', vi.fn(async (path: string, options: RequestInit = {}) => {
    const body = options.body ? JSON.parse(String(options.body)) as Record<string, unknown> : {}; calls.push({ path, body })
    if (lost && path === '/api/pod/batches') throw new Error('response lost')
    const data = path.endsWith('/preview') ? { items: (body.items as unknown[]).length, textCalls: 2, imageCalls: 2, images: 4, reservationMicros: '450000', testCredits: 0 } : { batchId: '8', name: 'batch', items: [] }
    return new Response(JSON.stringify(path.includes('?') ? { code: 200, rows: [], total: 0 } : { code: 200, data }), { headers: { 'Content-Type': 'application/json' } })
  }))
})
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks() })
it('requires server preview and explicit consent, with one row per input and no Cartesian expansion', async () => {
  wrapper = mount(BatchDialog, { props: { snapshot, name: 'Flower', scope: '1:2', token: 'fixture', limits, mode: 'qwen' }, attachTo: document.body })
  await flushPromises(); await wrapper.find('textarea').setValue('first'); await button('复制输入').trigger('click'); await flushPromises()
  expect(wrapper.findAll('textarea')).toHaveLength(2)
  await wrapper.findAll('textarea')[1]!.setValue('second')
  expect(button('开始批量').attributes('disabled')).toBeDefined()
  await button('核对份数与费用').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('0.450000 元'); expect(button('开始批量').attributes('disabled')).toBeDefined()
  await wrapper.find('input[type="checkbox"]').setValue(true); await wrapper.find('form').trigger('submit'); await flushPromises()
  const body = calls.find(c => c.path === '/api/pod/batches')!.body
  expect((body.items as { bindings: { text: string }[] }[]).map(i => i.bindings[0]!.text)).toEqual(['first', 'second'])
  expect(body.version).toBe('2'); expect(wrapper.emitted('created')).toHaveLength(1)
})
it('invalidates the preview and paid consent on any draft change', async () => {
  wrapper = mount(BatchDialog, { props: { snapshot, name: 'Flower', scope: '1:2', token: 'fixture', limits, mode: 'qwen' }, attachTo: document.body })
  await flushPromises(); await button('核对份数与费用').trigger('click'); await flushPromises(); await wrapper.find('input[type="checkbox"]').setValue(true)
  await wrapper.find('textarea').setValue('changed')
  expect(wrapper.find('.batch-impact').exists()).toBe(false); expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
  await wrapper.find('form').trigger('submit'); expect(calls.some(c => c.path === '/api/pod/batches')).toBe(false)
})
it('persists uncertain submissions, separates accounts and retries exact original inputs', async () => {
  lost = true; const original = intent()
  await expect(submitBatch('1:2', 'fixture', original)).rejects.toThrow()
  expect(pendingBatch('1:2')).toEqual(original); expect(pendingBatch('3:4')).toBeNull()
  await expect(submitBatch('1:2', 'fixture', intent())).rejects.toThrow('上一批')
  lost = false; await retryBatch('1:2', 'fixture')
  expect(calls.filter(c => c.path === '/api/pod/batches').map(c => c.body)).toEqual([original, original]); expect(pendingBatch('1:2')).toBeNull()
})
it('can recover the original submission from the batch history after the dialog is closed', async () => {
  lost = true; await expect(submitBatch('1:2', 'fixture', intent())).rejects.toThrow(); lost = false
  wrapper = mount(Batches, { props: { token: 'fixture', scope: '1:2' }, attachTo: document.body }); await flushPromises()
  await button('重试原批次提交').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('batch'); expect(pendingBatch('1:2')).toBeNull()
})
