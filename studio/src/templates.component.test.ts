// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type { Bootstrap, PersonalTemplate, TemplateVersion } from './contracts'
import Templates from './Templates.vue'
import TemplateSaveDialog from './TemplateSaveDialog.vue'
import TemplateUseDialog from './TemplateUseDialog.vue'
import TemplateLibraryDialog from './TemplateLibraryDialog.vue'
import AssetPicker from './AssetPicker.vue'
import { pendingTemplateRequest, retryTemplateRequest, submitTemplateRequest } from './template-requests'

const bootstrap = { userId: '10', workspace: { workspaceId: '20' }, assetLimits: { maxUploadBytes: 100000, maxPixels: 1000000, maxDimension: 8192, maxImagesPerNode: 8 } } as Bootstrap
const row: PersonalTemplate = { templateId: '7', name: '透明印花', description: '替换输入就能用', currentVersion: '2', revision: '3', deleted: false, createTime: '', updateTime: '2026-11-12 18:00:00' }
const snapshot = (version: string): TemplateVersion => ({ templateId: '7', version, createTime: '', graph: { schemaVersion: 1, nodes: [{ id: 'A', type: 'textInput', version: 1, position: { x: 0, y: 0 }, config: { text: `version ${version}` } }], edges: [] }, slots: [{ nodeId: 'A', type: 'textInput', label: '设计要求', text: `version ${version}` }, { nodeId: 'B', type: 'imageInput', label: '印花图片', assetIds: ['9'] }], outputs: [{ nodeId: 'E', type: 'export', label: '成品' }] })
const scope = '10:20'
let wrapper: VueWrapper | null = null
let calls: { path: string; method: string; body: Record<string, unknown> }[]
let failure = false
const respond = (data: unknown) => new Response(JSON.stringify(data), { headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  sessionStorage.clear(); calls = []; failure = false
  vi.stubGlobal('fetch', vi.fn(async (path: string, options: RequestInit = {}) => {
    const method = options.method || 'GET', body = options.body ? JSON.parse(String(options.body)) as Record<string, unknown> : {}
    calls.push({ path, method, body })
    if (method === 'POST' && failure) throw new Error('lost response')
    if (method === 'POST' && path.endsWith('/use')) return respond({ code: 200, data: { projectId: '88' } })
    if (path.includes('/versions/') && method === 'GET') return respond({ code: 200, data: snapshot(path.split('/').at(-1)!) })
    if (path.includes('/versions?')) return respond({ code: 200, rows: ['2', '1'].map(version => ({ version, templateId: '7', createTime: '' })), total: 2 })
    if (method === 'GET' && path.includes('/pod/templates?')) return respond({ code: 200, rows: path.includes('deleted=true') ? [{ ...row, deleted: true }] : [row], total: 1 })
    return respond({ code: 200, data: { ...row, snapshot: snapshot('3') } })
  }))
})
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('lists personal templates separately from presets and trash; delete and restore carry revision', async () => {
  wrapper = mount(Templates, { props: { token: 'test-token', bootstrap }, attachTo: document.body })
  await flushPromises()
  expect(wrapper.text()).toContain('透明印花')
  await wrapper.find('.personal-template-actions .text-button').trigger('click'); await flushPromises()
  expect(calls.find(c => c.method === 'DELETE')?.body).toEqual({ revision: '3' })
  await wrapper.findAll('.project-tabs button').find(b => b.text() === '回收站')!.trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('恢复模板')
  await wrapper.find('.personal-template-actions .text-button').trigger('click'); await flushPromises()
  expect(calls.some(c => c.path.endsWith('/restore') && c.body.revision === '3')).toBe(true)
  await wrapper.findAll('.project-tabs button').find(b => b.text() === '内置预设')!.trigger('click'); await flushPromises()
  expect(wrapper.findAll('.template-preset-grid article')).toHaveLength(10)
})

it('loads a pinned version and submits replaced text/images without execution', async () => {
  wrapper = mount(TemplateUseDialog, { props: { row, token: 'test-token', scope, limits: bootstrap.assetLimits }, global: { stubs: { PrivateImage: true, AssetPicker: true } }, attachTo: document.body })
  await flushPromises()
  await wrapper.find('.template-version-bar select').setValue('1'); await flushPromises()
  expect(wrapper.find('textarea').element.value).toBe('version 1')
  await wrapper.find('textarea').setValue('一朵蓝花')
  await wrapper.find('.template-slot-heading button').trigger('click'); await flushPromises()
  wrapper.findComponent(AssetPicker).vm.$emit('confirm', ['12', '13']); await flushPromises()
  await wrapper.find('form').trigger('submit'); await flushPromises()
  const call = calls.find(c => c.path.endsWith('/use'))!
  expect(call.body.version).toBe('1')
  expect(call.body.bindings).toEqual([{ nodeId: 'A', text: '一朵蓝花' }, { nodeId: 'B', assetIds: ['12', '13'] }])
  expect(wrapper.emitted('open')).toEqual([['88']])
  expect(calls.some(c => c.path.includes('/pod/runs'))).toBe(false)
})

it('keeps one library heading and a close control while switching tabs in the canvas dialog', async () => {
  wrapper = mount(TemplateLibraryDialog, { props: { token: 'test-token', bootstrap }, attachTo: document.body })
  await flushPromises()
  expect(wrapper.findAll('h1')).toHaveLength(0)
  expect(wrapper.findAll('#template-library-title')).toHaveLength(1)
  expect(wrapper.get('dialog').attributes('aria-labelledby')).toBe('template-library-title')
  await wrapper.findAll('.template-tabs button').find(b => b.text() === '内置预设')!.trigger('click')
  expect(wrapper.findAll('.template-preset-grid article')).toHaveLength(10)
  expect(calls.every(call => call.method === 'GET')).toBe(true)
  await wrapper.get('button[aria-label="关闭模板库"]').trigger('click')
  expect(wrapper.emitted('close')).toHaveLength(1)
})

it('saves only after the project has been flushed and appends with template revision', async () => {
  const prepare = vi.fn(async () => '15')
  wrapper = mount(TemplateSaveDialog, { props: { token: 'test-token', scope, projectId: '42', projectName: '测试项目', prepare }, attachTo: document.body })
  await flushPromises()
  await wrapper.find('select').setValue('version'); await flushPromises()
  await wrapper.findAll('select')[1]!.setValue('7'); await flushPromises()
  await wrapper.find('form').trigger('submit'); await flushPromises()
  expect(prepare).toHaveBeenCalledOnce()
  const call = calls.find(c => c.path.endsWith('/versions') && c.method === 'POST')!
  expect(call.body).toMatchObject({ projectId: '42', projectRevision: '15', revision: '3', name: '透明印花' })
  expect(call.body).not.toHaveProperty('graph')
  expect(wrapper.emitted('saved')).toHaveLength(1)
})

it('does not submit a template when source project saving failed', async () => {
  wrapper = mount(TemplateSaveDialog, { props: { token: 'test-token', scope, projectId: '42', projectName: '测试项目', prepare: async () => { throw new Error('版本冲突') } }, attachTo: document.body })
  await wrapper.find('form').trigger('submit'); await flushPromises()
  expect(wrapper.text()).toContain('版本冲突')
  expect(calls.some(c => c.method === 'POST')).toBe(false)
})

it('keeps an uncertain submit ID across remount and retries the original inputs', async () => {
  failure = true
  wrapper = mount(TemplateUseDialog, { props: { row, token: 'test-token', scope, limits: bootstrap.assetLimits }, global: { stubs: { PrivateImage: true } }, attachTo: document.body })
  await flushPromises(); await wrapper.find('textarea').setValue('keep this input'); await wrapper.find('form').trigger('submit'); await flushPromises()
  const first = calls.find(c => c.method === 'POST')!
  expect(wrapper.text()).toContain('重试原创建')
  expect(pendingTemplateRequest(scope)?.requestId).toBe(first.body.requestId)
  wrapper.unmount(); wrapper = null; failure = false
  wrapper = mount(Templates, { props: { token: 'test-token', bootstrap }, attachTo: document.body }); await flushPromises()
  await wrapper.findAll('button').find(b => b.text() === '重试原操作')!.trigger('click'); await flushPromises()
  const posts = calls.filter(c => c.method === 'POST')
  expect(posts).toHaveLength(2); expect(posts[1]!.body).toEqual(first.body)
  expect(wrapper.emitted('open')).toEqual([['88']]); expect(pendingTemplateRequest(scope)).toBeNull()
})

it('separates pending intents by account and refuses to replace an unresolved request', async () => {
  failure = true
  await expect(submitTemplateRequest(scope, '/pod/templates', { name: 'one' }, 'test-token')).rejects.toThrow()
  const original = pendingTemplateRequest(scope)
  expect(pendingTemplateRequest('11:21')).toBeNull()
  await expect(submitTemplateRequest(scope, '/pod/templates', { name: 'two' }, 'test-token')).rejects.toThrow('上一项')
  failure = false
  await retryTemplateRequest(scope, 'test-token')
  expect(calls.filter(c => c.method === 'POST').map(c => c.body.requestId)).toEqual([original!.requestId, original!.requestId])
})
