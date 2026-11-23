// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import NodeAction from './NodeAction.vue'
import CreativeLibrary from './CreativeLibrary.vue'
import PrivateImage from './PrivateImage.vue'
import type { Run } from './contracts'

let wrapper: VueWrapper | null
const run: Run = { runId: '55', projectId: '1', projectRevision: '2', projectName: 'test', mode: 'polish', nodeId: 'A', state: 'succeeded', attempt: 1, testOnly: false, testScenario: '', reserved: '0', spent: '0', createTime: '', updateTime: '', nodes: [{ nodeId: 'A', nodeType: 'textGenerate', state: 'succeeded', attempt: 1, output: { testOnly: false, text: '润色后的蓝色花朵' }, errorCode: '', errorMessage: '' }] }
const execution = { mode: 'qwen', textModel: 'qwen-flash', imageModel: 'qwen-image-2.1-turbo', globalConcurrency: 1, userConcurrency: 1, providerConcurrency: 1, maxPending: 10, maxUserPending: 10, initialCredits: '100' }
const prepare = vi.fn(async () => '2')
let submitted: Record<string, unknown>[], fail: number
let responseRun: Run
beforeEach(() => {
  sessionStorage.clear(); submitted = []; fail = 0; responseRun = run; prepare.mockClear()
  vi.stubGlobal('fetch', vi.fn(async (_path: string, options: RequestInit = {}) => {
    if (options.method === 'POST') { submitted.push(JSON.parse(String(options.body))); if (fail) return new Response(JSON.stringify({ code: fail, msg: '提交失败' })) }
    return new Response(JSON.stringify({ code: 200, data: responseRun }))
  }))
})
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.unstubAllGlobals() })
async function open(action: 'polish' | 'branch' = 'polish') {
  wrapper = mount(NodeAction, { props: { token: 'test', scope: '10:20', projectId: '1', nodeId: 'A', action, sourceText: '蓝色花朵', execution, prepare }, global: { stubs: { PrivateImage: true } }, attachTo: document.body })
  await flushPromises(); return wrapper
}

it('confirms paid polish and previews before applying without adding canvas nodes', async () => {
  const action = await open()
  expect(action.find('.primary-button').attributes('disabled')).toBeDefined()
  expect(submitted).toHaveLength(0)
  await action.find('input[type="checkbox"]').setValue(true)
  await action.find('.primary-button').trigger('click'); await flushPromises()
  expect(submitted).toHaveLength(1)
  expect(submitted[0]).toMatchObject({ mode: 'polish', nodeId: 'A', executionMode: 'qwen', revision: '2', projectId: '1' })
  expect(prepare).toHaveBeenCalledOnce()
  expect(action.emitted('apply')).toBeUndefined()
  expect(action.find('.polish-result').text()).toContain('润色后的蓝色花朵')
  await action.find('.primary-button').trigger('click')
  expect(action.emitted('apply')).toEqual([['润色后的蓝色花朵', '蓝色花朵']])
  expect(sessionStorage.length).toBe(0)
})

it('does not overwrite text changed while the polish result is pending', async () => {
  const action = await open()
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  await action.setProps({ sourceText: '已经修改过的内容' })
  expect(action.find('.primary-button').attributes('disabled')).toBeDefined()
  expect(action.text()).toContain('原文已修改')
  expect(action.emitted('apply')).toBeUndefined()
})

it('reuses the exact request after an ambiguous submission and on reopening', async () => {
  fail = 503
  let action = await open()
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  const originalBody = submitted[0]
  expect(action.find('.run-impact').exists()).toBe(false)
  action.unmount(); wrapper = null
  action = await open()
  expect(action.text()).toContain('重试原提交')
  fail = 0
  await action.find('.primary-button').trigger('click'); await flushPromises()
  expect(submitted[1]).toEqual(originalBody)
  expect(prepare).toHaveBeenCalledOnce()
})

it('keeps input untouched when the user discards a successful result', async () => {
  const action = await open()
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  await action.find('.text-button').trigger('click')
  expect(action.emitted('apply')).toBeUndefined()
  expect(action.emitted('close')).toHaveLength(1)
  expect(sessionStorage.length).toBe(0)
})

it('never submits if the canvas cannot be saved', async () => {
  const action = await open()
  await action.setProps({ prepare: async () => { throw new Error('保存冲突') } })
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  expect(submitted).toHaveLength(0)
  expect(action.text()).toContain('保存冲突')
})

it('rejects a damaged pending record instead of creating another paid request', async () => {
  sessionStorage.setItem('pod-studio.node-action:10:20:1:polish:A', '{broken')
  const action = await open()
  await action.find('input').setValue(true)
  expect(action.find('.primary-button').attributes('disabled')).toBeDefined()
  expect(action.text()).toContain('待确认的节点操作无法读取')
  expect(submitted).toHaveLength(0)
})

it('submits a branch action with its own upstream execution warning', async () => {
  responseRun = { ...run, mode: 'branch' }
  const action = await open()
  await action.setProps({ action: 'branch', nodeType: 'textGenerate' })
  expect(action.text()).toContain('上游含 AI 时仍可能产生费用')
  expect(action.find('.action-model').text()).toBe('qwen-flash')
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  expect(submitted[0]).toMatchObject({ mode: 'branch', nodeId: 'A' })
  expect(action.emitted('selected')).toBeDefined()
  expect(action.text()).toContain('文本结果')
  expect(action.text()).not.toContain('替换原文')
})

it('rejects a restored response for another project, node or action without resubmitting', async () => {
  const key = 'pod-studio.node-action:10:20:1:polish:A'
  for (const mismatch of [{ projectId: '9' }, { nodeId: 'B' }, { mode: 'branch' as const }, { runId: '99' }]) {
    sessionStorage.setItem(key, JSON.stringify({ body: { projectId: '1', nodeId: 'A', mode: 'polish', revision: '2', requestId: crypto.randomUUID() }, runId: '55', original: '蓝色花朵' }))
    responseRun = { ...run, ...mismatch }
    const action = await open()
    expect(action.text()).toContain('不匹配'); expect(action.find('.polish-result').exists()).toBe(false)
    expect(action.find('.primary-button').attributes('disabled')).toBeDefined()
    expect(action.emitted('unavailable')).toEqual([['55']]); expect(submitted).toHaveLength(0)
    action.unmount(); wrapper = null
  }
})

it('removes a visible private result when a subsequent refresh denies access', async () => {
  const key = 'pod-studio.node-action:10:20:1:polish:A'
  sessionStorage.setItem(key, JSON.stringify({ body: { projectId: '1', nodeId: 'A', mode: 'polish', revision: '2', requestId: crypto.randomUUID() }, runId: '55', original: '蓝色花朵' }))
  const action = await open()
  expect(action.find('.polish-result').exists()).toBe(true)
  vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ code: 403, msg: '访问已撤回' })))
  window.dispatchEvent(new Event('online')); await flushPromises()
  expect(action.find('.polish-result').exists()).toBe(false)
  expect(action.text()).toContain('访问已撤回'); expect(submitted).toHaveLength(0)
})

it('reveals paid ancestors of a local action and resets consent when the graph changes', async () => {
  const action = await open('branch')
  const graph = { schemaVersion: 1 as const, nodes: [
    { id: 'B', type: 'imageGenerate' as const, version: 1, position: { x: 0, y: 0 }, config: { count: 3 } },
    { id: 'A', type: 'imageProcess' as const, version: 1, position: { x: 0, y: 0 }, config: {} },
  ], edges: [{ id: 'BA', source: 'B', sourcePort: 'images', target: 'A', targetPort: 'images' }] }
  await action.setProps({ graph, nodeType: 'imageProcess' })
  expect(action.find('.run-impact').text()).toContain('生图调用 1 次 / 3 张')
  expect(action.find('.run-impact').text()).toContain('0.300000 元')
  await action.find('input').setValue(true)
  graph.nodes[0]!.config.count = 4
  await action.setProps({ graph: structuredClone(graph) })
  expect(action.find('.primary-button').attributes('disabled')).toBeDefined()
  expect(action.find('.run-impact').text()).toContain('0.400000 元')
  expect(submitted).toHaveLength(0)
})

it('does not submit if closed while the initial save is still pending', async () => {
  let finishSave!: (revision: string) => void
  const action = await open()
  await action.setProps({ prepare: () => new Promise<string>(resolve => { finishSave = resolve }) })
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click')
  action.unmount(); wrapper = null
  finishSave('2'); await flushPromises()
  expect(submitted).toHaveLength(0)
  expect(sessionStorage.length).toBe(0)
})

it('restores an in-flight polish after remounting and reads its result without another submission', async () => {
  responseRun = { ...run, state: 'running', nodes: [{ ...run.nodes![0]!, state: 'running', output: { testOnly: false } }] }
  let action = await open()
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  expect(action.text()).toContain('运行 #55')
  action.unmount(); wrapper = null
  responseRun = run
  action = await open()
  expect(submitted).toHaveLength(1)
  expect(prepare).toHaveBeenCalledOnce()
  expect(action.find('.polish-result').text()).toContain('润色后的蓝色花朵')
  expect(action.find('.primary-button').attributes('disabled')).toBeUndefined()
})

it('keeps completed polish recoverable until the user chooses whether to apply it', async () => {
  let action = await open()
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  action.unmount(); wrapper = null
  action = await open()
  expect(action.emitted('apply')).toBeUndefined()
  expect(submitted).toHaveLength(1)
  await action.find('.primary-button').trigger('click')
  expect(action.emitted('apply')).toEqual([['润色后的蓝色花朵', '蓝色花朵']])
})

it('restores every generated image of a branch after remounting without generating again', async () => {
  responseRun = { ...run, mode: 'branch', nodes: [{ ...run.nodes![0]!, nodeType: 'imageGenerate', output: { testOnly: false, assetIds: ['71', '72'] } }] }
  let action = await open('branch')
  await action.find('input').setValue(true); await action.find('.primary-button').trigger('click'); await flushPromises()
  action.unmount(); wrapper = null
  action = await open('branch')
  expect(action.findAllComponents(PrivateImage).map(image => image.props('assetId'))).toEqual(['71', '72'])
  expect(submitted).toHaveLength(1)
  expect(action.emitted('selected')?.at(-1)).toEqual([responseRun])
})

it('filters original prompts and emits a choice without calling a model', async () => {
  wrapper = mount(CreativeLibrary, { props: { initialTab: 'prompts' }, attachTo: document.body }); await flushPromises()
  expect(wrapper.findAll('.inspiration-index button')).toHaveLength(24)
  await wrapper.findAll('.shelf-filters button').find(button => button.text() === '植物花卉')!.trigger('click')
  expect(wrapper.findAll('.inspiration-index button')).toHaveLength(4)
  await wrapper.find('input').setValue('野花')
  expect(wrapper.findAll('.inspiration-index button')).toHaveLength(1)
  await wrapper.get('.inspiration-actions .primary-button').trigger('click')
  expect(wrapper.emitted('prompt')?.[0]?.[0]).toContain('蓝色野花')
  expect(submitted).toHaveLength(0)
})
