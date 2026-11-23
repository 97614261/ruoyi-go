// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type { Run } from './contracts'
import { useProjectRun } from './use-project-run'
import Runs from './Runs.vue'

const key = 'pod-studio.selected-run:10:20:1'
const run = (runId = '11', state = 'running'): Run => ({ runId, projectId: '1', projectRevision: '3', projectName: '图片测试', mode: 'branch', nodeId: 'crop', state, attempt: 1, testOnly: true, testScenario: 'success', reserved: '0', spent: '0', createTime: '', updateTime: '', nodes: [] } as Run)
const response = (value: Run, code = 200) => new Response(JSON.stringify({ code, data: value, msg: `测试错误 ${code}` }))
let wrapper: VueWrapper | undefined, state: ReturnType<typeof useProjectRun>
const paused = ref(false), expired = vi.fn()
function open() { wrapper = mount(defineComponent({ setup() { state = useProjectRun(() => 'test-token', '1', '10:20', () => paused.value, expired); return () => h('div') } })); return wrapper }
beforeEach(() => { sessionStorage.clear(); paused.value = false; expired.mockClear(); vi.useFakeTimers(); vi.stubGlobal('fetch', vi.fn(async () => response(run()))) })
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('restores the selected run with GET, polls until terminal, and never resubmits a task', async () => {
  sessionStorage.setItem(key, '11'); open(); await flushPromises()
  expect(state.activeRun.value?.runId).toBe('11')
  vi.mocked(fetch).mockResolvedValue(response(run('11', 'succeeded')))
  await vi.advanceTimersByTimeAsync(2000); await flushPromises()
  expect(state.activeRun.value?.state).toBe('succeeded')
  await vi.advanceTimersByTimeAsync(10000)
  expect(fetch).toHaveBeenCalledTimes(2)
  for (const [path, init] of vi.mocked(fetch).mock.calls) { expect(path).toBe('/api/pod/runs/11'); expect(init?.method).toBeUndefined(); expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer test-token') }
})
it('rechecks the original run on network return, but never reposts or polls a hidden tab', async () => {
  const visibility = vi.spyOn(document, 'visibilityState', 'get')
  sessionStorage.setItem(key, '11'); open(); await flushPromises()
  visibility.mockReturnValue('hidden'); document.dispatchEvent(new Event('visibilitychange'))
  await vi.advanceTimersByTimeAsync(8000); expect(fetch).toHaveBeenCalledTimes(1)
  visibility.mockReturnValue('visible'); vi.mocked(fetch).mockResolvedValue(response(run('11', 'succeeded')))
  window.dispatchEvent(new Event('online')); await flushPromises()
  expect(state.activeRun.value?.state).toBe('succeeded'); expect(fetch).toHaveBeenCalledTimes(2)
  expect(vi.mocked(fetch).mock.calls.every(([, init]) => !init?.method || init.method === 'GET')).toBe(true)
})
it('rejects cross-project or unauthorized restored records and clears the private selection', async () => {
  sessionStorage.setItem(key, '11'); vi.mocked(fetch).mockResolvedValue(response({ ...run(), projectId: '2' })); open(); await flushPromises()
  expect(state.activeRun.value).toBeNull(); expect(sessionStorage.getItem(key)).toBeNull()
  state.selectRun({ ...run('12'), projectId: '2' }); expect(state.activeRun.value).toBeNull()
  state.selectRun(run('12')); vi.mocked(fetch).mockResolvedValue(response(run('12'), 401)); await state.refreshRun()
  expect(state.activeRun.value).toBeNull(); expect(expired).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(10000); expect(fetch).toHaveBeenCalledTimes(2)
})
it('ignores other accounts and malformed IDs without issuing requests', async () => {
  sessionStorage.setItem('pod-studio.selected-run:99:20:1', '11'); sessionStorage.setItem(key, '../elsewhere'); open(); await flushPromises()
  expect(fetch).not.toHaveBeenCalled(); expect(sessionStorage.getItem(key)).toBeNull()
  state.selectRun({ ...run(), mode: 'polish' }); expect(sessionStorage.getItem(key)).toBeNull()
})
it('prevents an older restore response from replacing a newly selected run', async () => {
  let release!: (r: Response) => void
  sessionStorage.setItem(key, '11'); vi.mocked(fetch).mockImplementationOnce(() => new Promise(resolve => { release = resolve })); open()
  state.selectRun(run('12', 'succeeded')); release(response(run('11'))); await flushPromises()
  expect(state.activeRun.value?.runId).toBe('12'); expect(sessionStorage.getItem(key)).toBe('12')
})
it('pauses duplicate polling while the execution panel is open and stops on unmount', async () => {
  open(); state.selectRun(run()); paused.value = true; await flushPromises()
  await vi.advanceTimersByTimeAsync(5000); expect(fetch).not.toHaveBeenCalled()
  paused.value = false; await flushPromises(); await vi.advanceTimersByTimeAsync(2000)
  expect(fetch).toHaveBeenCalledTimes(1)
  wrapper!.unmount(); wrapper = undefined; await vi.advanceTimersByTimeAsync(10000); expect(fetch).toHaveBeenCalledTimes(1)
})
it('keeps transient failure retryable, and removes a missing run without endless polling', async () => {
  sessionStorage.setItem(key, '11'); vi.mocked(fetch).mockResolvedValue(response(run(), 503)); open(); await flushPromises()
  expect(state.runError.value).toContain('503'); expect(sessionStorage.getItem(key)).toBe('11')
  vi.mocked(fetch).mockResolvedValue(response(run('11', 'failed'))); await state.refreshRun()
  expect(state.runError.value).toBe(''); expect(state.activeRun.value?.state).toBe('failed')
  vi.mocked(fetch).mockResolvedValue(response(run(), 404)); await state.refreshRun()
  expect(state.activeRun.value).toBeNull(); expect(sessionStorage.getItem(key)).toBeNull()
})
it('clears a run reported unavailable by the execution panel without forgetting a newer selection', async () => {
  open(); state.selectRun(run('11')); state.selectRun(run('12'))
  state.forgetRun('11'); expect(state.activeRun.value?.runId).toBe('12')
  state.forgetRun('12'); expect(state.activeRun.value).toBeNull(); expect(sessionStorage.getItem(key)).toBeNull()
  expect(state.runError.value).toContain('不可访问')
})
it('removes stale private result details when a restored history record becomes unavailable', async () => {
  vi.mocked(fetch).mockImplementation(async (path) => String(path).includes('/runs/') ? response(run(), 404) : new Response(JSON.stringify({ code: 200, rows: [], total: 0, account: { available: '0', held: '0', spent: '0' } })))
  wrapper = mount(Runs, { props: { projectId: '1', token: 'test', initialRunId: '11' } }); await flushPromises()
  expect(wrapper.emitted('unavailable')).toEqual([['11']])
  expect(wrapper.find('.run-detail').exists()).toBe(false)
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(fetch).mock.calls.filter(([path]) => String(path).includes('/runs/'))).toHaveLength(1)
})
it('clears a selected private result if list refresh detects revoked access before detail polling', async () => {
  let allowed = true
  vi.mocked(fetch).mockImplementation(async (path) => {
    if (!allowed) return response(run(), 403)
    if (String(path).includes('/runs/')) return response(run('11', 'succeeded'))
    return new Response(JSON.stringify({ code: 200, rows: [], total: 0, account: { available: '0', held: '0', spent: '0' } }))
  })
  wrapper = mount(Runs, { props: { projectId: '1', token: 'test', initialRunId: '11' } }); await flushPromises()
  expect(wrapper.find('.run-detail').exists()).toBe(true)
  allowed = false; await wrapper.find('button[aria-label="刷新运行记录"]').trigger('click'); await flushPromises()
  expect(wrapper.emitted('unavailable')).toEqual([['11']]); expect(wrapper.find('.run-detail').exists()).toBe(false)
})
it('keeps the latest clicked run when history responses arrive out of order', async () => {
  const releases = new Map<string, (r: Response) => void>()
  vi.mocked(fetch).mockImplementation(async (path) => {
    const url = String(path)
    if (/\/runs\/\d+$/.test(url)) return new Promise<Response>(resolve => { releases.set(url.split('/').pop()!, resolve) })
    return new Response(JSON.stringify({ code: 200, rows: [run('11'), run('12')], total: 2, account: { available: '100', held: '0', spent: '0' } }))
  })
  wrapper = mount(Runs, { props: { projectId: '1', token: 'test' } }); await flushPromises()
  const buttons = wrapper.findAll('.run-row')
  await buttons[0]!.trigger('click'); await buttons[1]!.trigger('click')
  releases.get('12')!(response(run('12', 'succeeded'))); await flushPromises()
  releases.get('11')!(response(run('11', 'failed'))); await flushPromises()
  expect(wrapper.find('.run-detail-heading').text()).toContain('#12')
  expect(wrapper.emitted('selected')).toHaveLength(1)
})
