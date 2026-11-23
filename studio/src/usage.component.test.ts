// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import Usage from './Usage.vue'
import type { CreatorUsage } from './contracts'

let view: VueWrapper | undefined
const usage: CreatorUsage = { summary: { chargedMicros: '300051', reservedMicros: '100000', providerCalls: 4, pendingCalls: 1, generatedImages: 3, runCount: 12, failedRuns: 1, pendingRuns: 1 },
  rows: [{ runId: '10', projectId: '1', projectName: '蓝色印花', state: 'awaitingReconciliation', attempt: 2, testOnly: false, chargedMicros: '300001', reservedMicros: '100000', providerCalls: 3, pendingCalls: 1, generatedImages: 3, createTime: '2026-11-12 20:00:00' }], total: 12, budgetScope: 'instanceCumulative', budgetLimitMicros: '5000000' }
const response = (data: CreatorUsage = usage) => new Response(JSON.stringify({ code: 200, data }))
beforeEach(() => { vi.useFakeTimers(); vi.stubGlobal('fetch', vi.fn(async () => response())) })
afterEach(() => { view?.unmount(); view = undefined; vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
async function open() { view = mount(Usage, { props: { token: 'fixture-token', scope: '1:20' } }); await flushPromises(); return view }
const button = (wrapper: VueWrapper, text: string) => wrapper.findAll('button').find(item => item.text() === text)!

it('shows own totals and money, generated image semantics and instance budget without a fake remaining balance', async () => {
  const wrapper = await open()
  expect(wrapper.text()).toContain('0.300051 元'); expect(wrapper.text()).toContain('0.100000 元')
  expect(wrapper.text()).toContain('5.000000 元'); expect(wrapper.text()).toContain('不是个人余额')
  expect(wrapper.text()).toContain('包含尚未归档'); expect(wrapper.text()).toContain('不计入生图数')
  expect(wrapper.text()).not.toContain('4.699949'); expect(wrapper.text()).not.toContain('测试消耗')
  expect(wrapper.find('.usage-row').text()).toContain('确认原调用结果')
  await button(wrapper, '查看运行').trigger('click'); expect(wrapper.emitted('open')).toEqual([['10']])
  expect(vi.mocked(fetch).mock.calls.every(([, options]) => new Headers(options?.headers).get('Authorization') === 'Bearer fixture-token')).toBe(true)
  expect(vi.mocked(fetch).mock.calls.every(([, options]) => !options?.method)).toBe(true)
})
it('paginates the details, retains totals and refuses cross-project selection from a compact editor', async () => {
  const wrapper = await open(); await button(wrapper, '下一页').trigger('click'); await flushPromises()
  expect(vi.mocked(fetch).mock.calls.at(-1)?.[0]).toBe('/api/pod/usage?pageNum=2&pageSize=10')
  expect(wrapper.text()).toContain('0.300051 元')
  await wrapper.setProps({ projectId: '99' })
  expect(wrapper.text()).toContain('其他项目'); expect(wrapper.find('.usage-row button').exists()).toBe(false)
})
it('clears private amounts immediately on authorization loss and does not silently show zero', async () => {
  const wrapper = await open()
  vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ code: 403, msg: '空间访问已撤回' })))
  await button(wrapper, '刷新用量').trigger('click'); await flushPromises()
  expect(wrapper.text()).not.toContain('0.300051'); expect(wrapper.text()).toContain('空间访问已撤回')
  expect(wrapper.text()).not.toContain('还没有运行用量')
  await vi.advanceTimersByTimeAsync(30000); expect(fetch).toHaveBeenCalledTimes(2)
})
it('clears old identity data, ignores late responses and cancels polling on unmount', async () => {
  const wrapper = await open()
  let finish!: (value: Response) => void
  vi.mocked(fetch).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await button(wrapper, '刷新用量').trigger('click')
  const changed = structuredClone(usage); changed.summary.chargedMicros = '2'; changed.rows = []
  vi.mocked(fetch).mockResolvedValue(response(changed))
  await wrapper.setProps({ token: 'new-fixture-token', scope: '2:30' }); await flushPromises()
  finish(response()); await flushPromises()
  expect(wrapper.text()).toContain('0.000002 元'); expect(wrapper.text()).not.toContain('0.300051 元')
  wrapper.unmount(); view = undefined
  const calls = vi.mocked(fetch).mock.calls.length
  await vi.advanceTimersByTimeAsync(30000); expect(fetch).toHaveBeenCalledTimes(calls)
})
it('keeps network errors actionable and labels retained values as stale', async () => {
  const wrapper = await open(); vi.mocked(fetch).mockRejectedValue(new Error('offline'))
  await button(wrapper, '刷新用量').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('以下保留上次读取的数据'); expect(wrapper.text()).toContain('0.300051')
  expect(button(wrapper, '刷新用量').attributes('disabled')).toBeUndefined()
})
