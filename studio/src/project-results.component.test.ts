// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import ProjectResults from './ProjectResults.vue'
import WorkflowNode from './WorkflowNode.vue'
import ResultImages from './ResultImages.vue'
import { APIError } from './api'
import { isPointsShortage, pointsShortageMessage } from './execution-quote'
import type { NodeDefinition, Run, RunNode } from './contracts'

const ok = (payload: object) => new Response(JSON.stringify({ code: 200, ...payload }))
const output: RunNode = { nodeId: 'render', nodeType: 'imageGenerate', state: 'succeeded', attempt: 1, output: { testOnly: false, assetIds: ['91'] }, errorCode: '', errorMessage: '' }
const fileRun = (runId: string, state: Run['state'], nodes: RunNode[] = []): Run => ({ runId, projectId: '7', projectRevision: '2', projectName: 'Prints', mode: 'branch', nodeId: 'render', state, attempt: 1, testOnly: false, testScenario: '', reserved: '0', spent: '0', createTime: '2026-11-18 12:15:00', updateTime: '', nodes })
afterEach(() => vi.unstubAllGlobals())

it('finds archived images after reopening, including results produced before a downstream failure', async () => {
  const newer = fileRun('61', 'failed'), older = fileRun('60', 'failed', [output])
  const fetchMock = vi.fn(async (path: string, _options?: RequestInit) => path.includes('?') ? ok({ rows: [newer, older], total: 2 }) : ok({ data: path.endsWith('/61') ? newer : older }))
  vi.stubGlobal('fetch', fetchMock)
  const view = mount(ProjectResults, { props: { token: 'local', projectId: '7', revision: '3' }, global: { stubs: { ResultImages: true } } })
  try {
    await flushPromises()
    expect(view.find('select').element.value).toBe('60')
    expect(view.text()).toContain('历史结果，不代表当前参数')
    expect(view.findComponent(ResultImages).props('ids')).toEqual(['91'])
    expect(fetchMock.mock.calls.every(([, options]) => !options?.method)).toBe(true)
    await view.find('select').setValue('61'); await flushPromises()
    expect(view.text()).toContain('这次运行没有可查看的图片')
    expect(view.findComponent(ResultImages).exists()).toBe(false)
  } finally { view.unmount() }
})

it('keeps an earlier image visible on the node when settings changed or a newer run has no output', async () => {
  const definition = { type: 'imageGenerate', label: 'AI image', description: '', executable: true, inputs: [], outputs: [], version: 1 } as NodeDefinition
  const view = mount(WorkflowNode, { props: { token: 'local', id: 'render', data: { definition, config: {} }, currentRevision: '3', imageResult: { revision: '2', node: output }, runNode: { ...output, state: 'failed', output: { testOnly: false } }, runRevision: '3' }, global: { stubs: { ResultImages: true, MagneticHandle: true } } })
  try {
    expect(view.findComponent(ResultImages).props('ids')).toEqual(['91'])
    expect(view.text()).toContain('历史图片 · v2')
  } finally { view.unmount() }
})

it('shows quoted points and refreshed balance precisely without conflating provider budget failures', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => ok({ data: { account: { available: '12' } } })))
  expect(isPointsShortage(new APIError('正式点数不足，请联系管理员发放点数', 409))).toBe(true)
  expect(isPointsShortage(new APIError('模型预算不足', 409))).toBe(false)
  expect(await pointsShortageMessage('local', '9007199254740993')).toContain('本次需 9007199254740993 点，当前可用 12 点')
  vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('offline') }))
  expect(await pointsShortageMessage('local', '25')).toContain('本次需 25 点')
})
