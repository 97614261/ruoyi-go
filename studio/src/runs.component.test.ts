// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import Runs from './Runs.vue'
import type { Run } from './contracts'

const execution = { mode: 'simulation', globalConcurrency: 2, userConcurrency: 1, providerConcurrency: 2, maxPending: 100, maxUserPending: 10, initialCredits: '1000' }
const run: Run = { runId: '10', projectId: '1', projectName: '印花流程', projectRevision: '3', mode: 'workflow', nodeId: '', state: 'succeeded', attempt: 1, testOnly: true, testScenario: 'success', reserved: '0', spent: '1', createTime: '2026-11-11 16:00:00', updateTime: '2026-11-11 16:00:00', nodes: [{ nodeId: 'B', nodeType: 'textGenerate', state: 'succeeded', attempt: 1, output: { testOnly: true, text: '[测试输出，非 AI 生成]\n<script>unsafe</script>' }, errorCode: '', errorMessage: '' }] }
const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
function server(post: (path: string, options: RequestInit) => Promise<Response> = async () => json({ code: 200, data: run })) {
  const fetch = vi.fn(async (path: string, options: RequestInit = {}) => {
    if (options.method === 'POST') return post(path, options)
    if (path.includes('/credits?')) return json({ code: 200, account: { available: '999', held: '0', spent: '1' }, rows: [{ eventId: '2', runId: '10', kind: 'settle', amount: '1', available: '999', held: '0', spent: '1', createTime: run.createTime }], total: 1 })
    if (path.includes('/runs?')) return json({ code: 200, rows: [run], total: 1 })
    return json({ code: 200, data: run })
  }); vi.stubGlobal('fetch', fetch); return fetch
}
async function open(prepare = vi.fn(async () => '3')) {
  wrapper = mount(Runs, { props: { token: 'private-test-token', execution, projectId: '1', selectedNode: 'B', prepare } }); await flushPromises(); return wrapper
}
const button = (view: VueWrapper, text: string) => view.findAll('button').find(item => item.text() === text)!

it('saves the canvas before submitting an immutable version and authenticated request', async () => {
  const fetch = server(); const prepare = vi.fn(async () => '3'); const view = await open(prepare)
  await button(view, '测试整条流程').trigger('click'); await flushPromises()
  expect(prepare).toHaveBeenCalledOnce()
  const sent = fetch.mock.calls.find(([, options]) => options?.method === 'POST')!
  expect(JSON.parse(String(sent[1]?.body))).toMatchObject({ projectId: '1', revision: '3', mode: 'workflow', testScenario: 'success' })
  expect(new Headers(sent[1]?.headers).get('Authorization')).toBe('Bearer private-test-token')
  expect(view.text()).toContain('非 AI 生成'); expect(view.find('script').exists()).toBe(false)
})
it('retains the same request identifier when submit times out', async () => {
  const bodies: Record<string, unknown>[] = []
  server(async (_path, options) => { bodies.push(JSON.parse(String(options.body))); if (bodies.length === 1) throw new Error('network interrupted'); return json({ code: 200, data: run }) })
  const view = await open(); await button(view, '测试整条流程').trigger('click'); await flushPromises()
  expect(view.text()).toContain('上次提交结果未确认')
  expect(view.find('.run-impact').exists()).toBe(false)
  await button(view, '重试原提交').trigger('click'); await flushPromises()
  expect(bodies).toHaveLength(2); expect(bodies[0]).toEqual(bodies[1])
})
it('does not create a run when the draft fails to save', async () => {
  const fetch = server(); const view = await open(vi.fn(async () => { throw new Error('画布尚未保存') }))
  await button(view, '测试整条流程').trigger('click'); await flushPromises()
  expect(view.text()).toContain('画布尚未保存'); expect(fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
})
it('uses only the selected node and explicit base run for single-node execution', async () => {
  const fetch = server(); const view = await open(); await view.findAll('select')[1]!.setValue('10')
  expect(view.text()).toContain('使用哪次生成结果'); expect(view.text()).toContain('只影响“仅执行选中节点”')
  expect(view.text()).not.toContain('单节点的上游结果'); expect(view.findAll('select')[1]!.find('option').text()).toBe('不复用之前的结果')
  await button(view, '仅测试选中节点').trigger('click'); await flushPromises()
  const sent = fetch.mock.calls.find(([, options]) => options?.method === 'POST')!
  expect(JSON.parse(String(sent[1]?.body))).toMatchObject({ mode: 'node', nodeId: 'B', baseRunId: '10' })
})

it('keeps polish visible in history but excludes it and other execution modes from upstream reuse', async () => {
  const rows = [run, { ...run, runId: '11', mode: 'polish' }, { ...run, runId: '12', testOnly: false }]
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path.includes('/credits?')
    ? json({ code: 200, account: { available: '0', held: '0', spent: '0' }, rows: [], total: 0 })
    : json({ code: 200, rows, total: rows.length })))
  const view = await open()
  expect(view.findAll('select')[1]!.findAll('option').map(option => option.attributes('value'))).toEqual(['', '10'])
  expect(view.findAll('.run-row')).toHaveLength(3)
  expect(view.text()).toContain('AI 润色')
})
it('displays ledger balances, settlement and simulation boundaries', async () => {
  server(); const view = await open(); await button(view, '测试额度').trigger('click')
  expect(view.text()).toContain('999'); expect(view.text()).toContain('结算'); expect(view.text()).toContain('无货币价值')
})
it('emits session expiration instead of displaying a successful empty history', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => json({ code: 401, msg: '请重新登录' })))
  const view = await open(); expect(view.emitted('expired')).toBeTruthy(); expect(view.text()).toContain('请重新登录')
})
it('restores an unconfirmed submission after the panel reopens without creating a new request', async () => {
  const bodies: Record<string, unknown>[] = []
  server(async (_path, options) => { bodies.push(JSON.parse(String(options.body))); if (bodies.length === 1) throw new Error('response lost'); return json({ code: 200, data: run }) })
  const props = { token: 'private-test-token', scope: 'user10:space20', execution, projectId: '1', prepare: vi.fn(async () => '3') }
  wrapper = mount(Runs, { props }); await flushPromises(); await button(wrapper, '测试整条流程').trigger('click'); await flushPromises()
  wrapper.unmount(); wrapper = mount(Runs, { props }); await flushPromises()
  expect(wrapper.text()).toContain('上次提交结果未确认')
  await button(wrapper, '重试原提交').trigger('click'); await flushPromises()
  expect(bodies[0]).toEqual(bodies[1]); expect(sessionStorage.getItem('pod-studio.pending-run:user10:space20:1')).toBeNull()
})

it('requires an explicit paid choice and sends the real mode without a test scenario', async () => {
  const fetch = server()
  wrapper = mount(Runs, { props: { token: 'local-test-token', execution: { ...execution, mode: 'qwen', textModel: 'qwen-flash', imageModel: 'qwen-image-2.1-turbo' }, projectId: '1', selectedNode: 'B', prepare: vi.fn(async () => '3') } })
  await flushPromises()
  const submit = button(wrapper, '执行整条流程')
  expect(submit.attributes('disabled')).toBeDefined()
  expect(wrapper.text()).toContain('qwen-image-2.1-turbo')
  expect(wrapper.text()).not.toContain('测试场景')
  await wrapper.find('input[type="checkbox"]').setValue(true)
  await submit.trigger('click'); await flushPromises()
  const sent = fetch.mock.calls.find(([, options]) => options?.method === 'POST')!
  const body = JSON.parse(String(sent[1]?.body))
  expect(body.executionMode).toBe('qwen'); expect(body.testScenario).toBeUndefined()
  expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
})

it('shows real token usage and estimated cost without treating test credits as money', async () => {
  const realRun: Run = { ...run, testOnly: false, testScenario: '', nodes: [{ ...run.nodes![0]!, output: { testOnly: false, text: 'real text fixture', usage: { model: 'qwen-flash', requestId: 'local-request', inputTokens: 1000, outputTokens: 1000, imageCount: 0, estimatedCostMicros: '1650' } } }] }
  server(async () => json({ code: 200, data: realRun }))
  wrapper = mount(Runs, { props: { token: 'local-test-token', execution: { ...execution, mode: 'qwen' }, projectId: '1', prepare: vi.fn(async () => '3') } })
  await flushPromises(); await wrapper.find('input[type="checkbox"]').setValue(true)
  await button(wrapper, '执行整条流程').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('0.001650 元'); expect(wrapper.text()).toContain('Token 1000/1000')
  expect(wrapper.text()).toContain('以供应商账单为准'); expect(wrapper.text()).not.toContain('测试消耗')
})

it('requires separate paid consent before recovering a confirmed failed real run', async () => {
  const failed: Run = { ...run, state: 'failed', testOnly: false, testScenario: '' }
  const fetch = vi.fn(async (path: string, options: RequestInit = {}) => {
    if (path.includes('/credits?')) return json({ code: 200, account: { available: '0', held: '0', spent: '0' }, rows: [], total: 0 })
    if (path.includes('/runs?')) return json({ code: 200, rows: [failed], total: 1 })
    if (options.method === 'POST') return json({ code: 200, data: { ...failed, state: 'queued', attempt: 2 } })
    return json({ code: 200, data: failed })
  })
  vi.stubGlobal('fetch', fetch)
  wrapper = mount(Runs, { props: { token: 'local-test-token', execution: { ...execution, mode: 'qwen' } } })
  await flushPromises(); await wrapper.find('.run-row').trigger('click'); await flushPromises()
  const retry = button(wrapper, '恢复失败步骤')
  expect(retry.attributes('disabled')).toBeDefined()
  expect(fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  await wrapper.find('input[type="checkbox"]').setValue(true)
  await retry.trigger('click'); await flushPromises()
  expect(fetch.mock.calls.filter(([path, options]) => path.endsWith('/pod/runs/10/retry') && options?.method === 'POST')).toHaveLength(1)
})
