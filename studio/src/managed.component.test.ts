// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { executionQuote } from './execution-quote'
import ModelSelect from './ModelSelect.vue'
import NodeAction from './NodeAction.vue'
import Runs from './Runs.vue'
import BatchDialog from './BatchDialog.vue'
import type { Run, QuoteSource, ExecutionQuote, CreatorModel, TemplateVersion } from './contracts'

let view: VueWrapper | undefined
const quote: ExecutionQuote = { quoteId: '11111111-1111-4111-8111-111111111111', kind: 'run', state: 'ready', points: '9007199254740993', expiresAt: new Date(Date.now() + 600000).toISOString(), lines: [{ itemNo: 0, nodeId: 'B', modelId: '8', modelVersionId: '22', name: '文本创作', points: '9007199254740993' }] }
const source: QuoteSource = { kind: 'run', run: { projectId: '1', revision: '3', executionMode: 'managed', mode: 'workflow' } }
const execution = { mode: 'managed', globalConcurrency: 2, userConcurrency: 1, providerConcurrency: 2, maxPending: 100, maxUserPending: 10, initialCredits: '1000' }
const run: Run = { runId: '10', projectId: '1', projectName: 'managed fixture', projectRevision: '3', mode: 'workflow', nodeId: '', state: 'succeeded', attempt: 1, testOnly: false, testScenario: '', reserved: '0', spent: '0', createTime: '', updateTime: '', billing: { scheme: 'managedPointsV1', held: '0', spent: '20', released: '876' }, nodes: [] }
const json = (data: unknown, code = 200) => new Response(JSON.stringify({ code, data, ...(code === 200 ? {} : { msg: 'fixture denied' }) }), { headers: { 'Content-Type': 'application/json' } })
const button = (text: string) => view!.findAll('button').find(b => b.text() === text)!
function server(post?: (path: string, body: Record<string, unknown>) => Response | Promise<Response>, rows: Run[] = []) {
  let detail = run
  const fetch = vi.fn(async (path: string, options: RequestInit = {}) => {
    if (path.endsWith('/execution-quotes')) return json({ ...quote, kind: JSON.parse(String(options.body)).kind })
    if (options.method === 'POST') {
      const body = JSON.parse(String(options.body)) as Record<string, unknown>
      if (post) return post(path, body)
      detail = { ...run, mode: body.mode as Run['mode'], nodeId: String(body.nodeId || '') }
      return json(detail)
    }
    if (path.includes('/credits?')) return new Response(JSON.stringify({ code: 200, account: { available: '0', held: '0', spent: '0' }, rows: [], total: 0 }))
    if (path.includes('/runs?')) return new Response(JSON.stringify({ code: 200, rows, total: rows.length }))
    return json(detail)
  }); vi.stubGlobal('fetch', fetch); return fetch
}
afterEach(() => { view?.unmount(); view = undefined; sessionStorage.clear(); document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.useRealTimers() })

it('preserves server integer points and the exact execution intent without another confirmation', async () => {
  const fetch = server(); const result = await executionQuote('local-token', source)
  expect(result.points).toBe('9007199254740993'); expect(result.lines[0]?.modelVersionId).toBe('22')
  expect(fetch).toHaveBeenCalledOnce(); expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body))).toEqual(source)
  expect(result.quoteId).toBe(quote.quoteId)
})
it('rejects expired quotes without silently requesting a replacement', async () => {
  const fetch = vi.fn(async () => json({ ...quote, expiresAt: new Date(Date.now() - 1).toISOString() })); vi.stubGlobal('fetch', fetch)
  await expect(executionQuote('local', source)).rejects.toThrow('已过期')
  expect(fetch).toHaveBeenCalledOnce()
})
it('fails closed on quote errors without creating a run', async () => {
  const fetch = vi.fn(async () => json(null, 409)); vi.stubGlobal('fetch', fetch)
  view = mount(NodeAction, { props: { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action: 'branch', sourceText: '', execution, prepare: async () => '3', autoStart: true }, attachTo: document.body }); await flushPromises()
  expect(view.text()).toContain('fixture denied'); expect(fetch).toHaveBeenCalledOnce()
  expect(fetch.mock.calls[0]).toBeDefined(); expect(view.emitted('selected')).toBeUndefined()
})
it.each([{ kind: 'retry' }, { state: 'consumed' }, { points: 25 }, { points: '-1' }, { quoteId: '' }])('rejects malformed or mismatched quotes: %j', async patch => {
  vi.stubGlobal('fetch', vi.fn(async () => json({ ...quote, ...patch })))
  await expect(executionQuote('local', source)).rejects.toThrow('报价无效')
})
it('preserves unavailable saved selections instead of silently changing the model', async () => {
  const models: CreatorModel[] = [{ modelId: '8', modelVersionId: '22', name: '文本创作', kind: 'text', officialModelId: 'qwen-flash', default: true, capabilities: {}, pointRule: { inputPerMillion: '1', outputPerMillion: '2', perImage: '0' } }]
  view = mount(ModelSelect, { props: { models, kind: 'text', value: '99' } })
  expect(view.find('select').element.value).toBe('99'); expect(view.text()).toContain('原模型 #99 已不可用'); expect(view.emitted('change')).toBeUndefined()
  await view.find('select').setValue('8'); expect(view.emitted('change')).toEqual([['8']])
})
it.each(['polish', 'branch'] as const)('quotes %s then submits from the original click without an extra dialog', async action => {
  const fetch = server(); view = mount(NodeAction, { props: { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action, sourceText: 'cat', execution, prepare: async () => '3' }, attachTo: document.body }); await flushPromises()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))).toHaveLength(0)
  await button(action === 'polish' ? '开始润色' : '开始生成').trigger('click'); await flushPromises()
  expect(view.text()).not.toContain('0.10 元'); expect(view.find('.execution-quote-dialog').exists()).toBe(false)
  const submits = fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))
  expect(submits).toHaveLength(1); expect(JSON.parse(String(submits[0]![1]?.body))).toMatchObject({ quoteId: quote.quoteId, mode: action, revision: '3', executionMode: 'managed' })
  expect(view.text()).toContain('已完成'); expect(view.find('[role="alert"]').exists()).toBe(false)
})
it('starts once from the canvas click and only reads the cached result when reopened', async () => {
  const fetch = server(), prepare = vi.fn(async () => '3')
  const props = { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action: 'branch' as const, sourceText: 'cat', execution, prepare, autoStart: true }
  view = mount(NodeAction, { props, attachTo: document.body }); await flushPromises()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))).toHaveLength(1)
  expect(view.emitted('selected')?.[0]?.[0]).toMatchObject({ runId: '10', nodeId: 'A', mode: 'branch' })
  view.unmount(); view = mount(NodeAction, { props, attachTo: document.body }); await flushPromises()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))).toHaveLength(1)
  expect(prepare).toHaveBeenCalledOnce(); expect(view.text()).toContain('已完成')
})
it('shows an actionable point shortage and does not retain a rejected submission', async () => {
  const fetchMock = vi.fn(async (path: string, _options?: RequestInit) => {
    if (path.endsWith('/execution-quotes')) return json({ ...quote, points: '25' })
    if (path.includes('/pod/points?')) return json({ account: { available: '12' } })
    return new Response(JSON.stringify({ code: 409, msg: '正式点数不足，请联系管理员发放点数' }))
  }); vi.stubGlobal('fetch', fetchMock)
  view = mount(NodeAction, { props: { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action: 'branch', sourceText: 'cat', execution, prepare: async () => '3', autoStart: true }, attachTo: document.body })
  await flushPromises()
  expect(view.find('[role="alert"]').text()).toContain('本次需 25 点，当前可用 12 点')
  expect(view.text()).toContain('已有图片仍可查看和下载')
  expect(view.emitted('selected')).toBeUndefined()
  expect(sessionStorage.length).toBe(0)
  expect(fetchMock.mock.calls.filter(([path, options]) => path.endsWith('/runs') && options?.method === 'POST')).toHaveLength(1)
})
it.each(['double-click', 'input-change', 'close'] as const)('handles %s while automatic pricing is pending without duplicate or stale execution', async scenario => {
  let resolveQuote!: (value: Response) => void
  const pendingQuote = new Promise<Response>(resolve => { resolveQuote = resolve })
  const fetch = vi.fn(async (path: string) => path.endsWith('/execution-quotes') ? pendingQuote : json({ ...run, mode: 'branch', nodeId: 'A' })); vi.stubGlobal('fetch', fetch)
  const prepare = vi.fn(async () => '3')
  view = mount(NodeAction, { props: { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action: 'branch', sourceText: 'cat', execution, prepare, autoStart: true }, attachTo: document.body }); await flushPromises()
  expect(fetch).toHaveBeenCalledOnce()
  if (scenario === 'double-click') await view.find('.primary-button').trigger('click')
  else if (scenario === 'input-change') await view.setProps({ sourceText: 'dog' })
  else { view.unmount(); view = undefined }
  resolveQuote(json(quote)); await flushPromises()
  const submits = fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))
  expect(submits).toHaveLength(scenario === 'double-click' ? 1 : 0)
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
  expect(prepare).toHaveBeenCalledOnce()
  if (scenario === 'input-change') expect(view!.text()).toContain('输入已变化，本次未提交')
})
it('reopens an unknown canvas submission without auto submitting, and explicitly retries the exact request', async () => {
  const bodies: Record<string, unknown>[] = []
  const fetch = server((_path, body) => { bodies.push(body); throw new Error('lost response') })
  const prepare = vi.fn(async () => '3')
  const props = { token: 'local', scope: '1:2', projectId: '1', nodeId: 'A', action: 'branch' as const, sourceText: 'cat', execution, prepare, autoStart: true }
  view = mount(NodeAction, { props, attachTo: document.body }); await flushPromises(); expect(bodies).toHaveLength(1)
  view.unmount(); view = mount(NodeAction, { props: { ...props, sourceText: 'dog' }, attachTo: document.body }); await flushPromises()
  expect(bodies).toHaveLength(1); expect(view.text()).toContain('上次提交结果未确认')
  await button('重试原提交').trigger('click'); await flushPromises()
  expect(bodies).toHaveLength(2); expect(bodies[1]).toEqual(bodies[0]); expect(prepare).toHaveBeenCalledOnce()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
})
it.each(['执行整条流程', '仅执行选中节点'])('quotes %s and retains the exact unknown submission across reopening', async text => {
  const bodies: Record<string, unknown>[] = []
  const fetch = server((_path, body) => { bodies.push(body); if (bodies.length === 1) throw new Error('lost response'); return json(run) })
  const props = { token: 'local', scope: '1:2', projectId: '1', selectedNode: 'B', execution, prepare: vi.fn(async () => '3') }
  view = mount(Runs, { props, attachTo: document.body }); await flushPromises(); await button(text).trigger('click'); await flushPromises()
  expect(bodies).toHaveLength(1); expect(bodies[0]).toMatchObject({ quoteId: quote.quoteId, mode: text === '执行整条流程' ? 'workflow' : 'node' })
  view.unmount(); view = mount(Runs, { props, attachTo: document.body }); await flushPromises()
  await button('重试原提交').trigger('click'); await flushPromises(); expect(bodies[1]).toEqual(bodies[0]); expect(props.prepare).toHaveBeenCalledOnce()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
})
it.each(['selection-change', 'close'] as const)('does not execute a stale run after %s during pricing', async scenario => {
  const fetch = server(); let resolveQuote!: (value: Response) => void
  const pendingQuote = new Promise<Response>(resolve => { resolveQuote = resolve }), original = fetch.getMockImplementation()!
  fetch.mockImplementation((path: string, options: RequestInit = {}) => path.endsWith('/execution-quotes') ? pendingQuote : original(path, options))
  view = mount(Runs, { props: { token: 'local', scope: '1:2', projectId: '1', selectedNode: 'B', execution, prepare: async () => '3' }, attachTo: document.body }); await flushPromises()
  await button('仅执行选中节点').trigger('click'); await flushPromises()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
  if (scenario === 'selection-change') await view.setProps({ selectedNode: 'C' })
  else { view.unmount(); view = undefined }
  resolveQuote(json(quote)); await flushPromises()
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))).toHaveLength(0)
  if (view) expect(view.text()).toContain('执行范围已变化，本次未提交')
})
it('quotes retry separately and persists its original request and quote after a lost response', async () => {
  const failed = { ...run, state: 'failed' } as Run, bodies: Record<string, unknown>[] = []
  const fetch = server((_path, body) => { bodies.push(body); throw new Error('lost retry response') }, [failed])
  // A failed detail remains failed until the exact retry response has been confirmed.
  fetch.mockImplementation(async (path: string, options: RequestInit = {}) => {
    if (path.endsWith('/execution-quotes')) return json({ ...quote, kind: 'retry' })
    if (options.method === 'POST') { bodies.push(JSON.parse(String(options.body))); throw new Error('lost retry response') }
    if (path.includes('/credits?')) return new Response(JSON.stringify({ code: 200, account: { available: '0', held: '0', spent: '0' }, rows: [], total: 0 }))
    if (path.includes('/runs?')) return new Response(JSON.stringify({ code: 200, rows: [failed], total: 1 }))
    return json(failed)
  })
  const props = { token: 'local', scope: '1:2', execution }
  view = mount(Runs, { props, attachTo: document.body }); await flushPromises(); await view.find('.run-row').trigger('click'); await flushPromises(); await button('恢复失败步骤').trigger('click'); await flushPromises()
  expect(bodies).toHaveLength(1); view.unmount(); view = mount(Runs, { props, attachTo: document.body }); await flushPromises(); await view.find('.run-row').trigger('click'); await flushPromises()
  await button('重试原恢复提交').trigger('click'); await flushPromises(); expect(bodies[1]).toEqual(bodies[0]); expect(bodies[0]?.quoteId).toBe(quote.quoteId)
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/execution-quotes'))).toHaveLength(1)
})
it('quotes the full immutable batch and submits after the existing start button', async () => {
  const bodies: Record<string, unknown>[] = []
  server((path, body) => {
    if (path.endsWith('/preview')) return json({ items: 1, textCalls: 1, imageCalls: 0, images: 0, reservationMicros: '0', testCredits: 0 })
    bodies.push(body); return json({ batchId: '9' })
  })
  const snapshot: TemplateVersion = { templateId: '3', version: '2', createTime: '', slots: [{ nodeId: 'A', type: 'textInput', label: '主题', text: 'cat' }] }
  view = mount(BatchDialog, { props: { token: 'local', scope: '1:2', mode: 'managed', name: '模板', snapshot, limits: { maxUploadBytes: 1000, maxPixels: 1000, maxDimension: 1000, maxImagesPerNode: 3 } }, attachTo: document.body }); await flushPromises()
  await button('核对份数与费用').trigger('click'); await flushPromises(); expect(bodies).toHaveLength(0)
  await view.find('form').trigger('submit'); await flushPromises()
  expect(bodies[0]).toMatchObject({ quoteId: quote.quoteId, executionMode: 'managed', templateId: '3', version: '2' }); expect(view.emitted('created')).toHaveLength(1)
})
it('does not submit a batch after its input changes while pricing', async () => {
  let resolveQuote!: (value: Response) => void
  const pendingQuote = new Promise<Response>(resolve => { resolveQuote = resolve })
  const fetch = vi.fn(async (path: string) => path.endsWith('/execution-quotes') ? pendingQuote : json({ items: 1, textCalls: 1, imageCalls: 0, images: 0, reservationMicros: '0', testCredits: 0 })); vi.stubGlobal('fetch', fetch)
  const snapshot: TemplateVersion = { templateId: '3', version: '2', createTime: '', slots: [{ nodeId: 'A', type: 'textInput', label: '主题', text: 'cat' }] }
  view = mount(BatchDialog, { props: { token: 'local', scope: '1:2', mode: 'managed', name: '模板', snapshot, limits: { maxUploadBytes: 1000, maxPixels: 1000, maxDimension: 1000, maxImagesPerNode: 3 } }, attachTo: document.body }); await flushPromises()
  await button('核对份数与费用').trigger('click'); await flushPromises(); await view.find('form').trigger('submit'); await flushPromises()
  await view.setProps({ snapshot: { ...snapshot, version: '3' } }); resolveQuote(json({ ...quote, kind: 'batch' })); await flushPromises()
  expect(view.text()).toContain('批量输入已变化，本次未提交'); expect(view.emitted('created')).toBeUndefined()
  expect(fetch.mock.calls.some(([path]) => path.endsWith('/batches'))).toBe(false)
})
