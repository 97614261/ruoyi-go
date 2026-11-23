// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import GettingStarted from './GettingStarted.vue'
import App from './App.vue'
import { studioLocation } from './navigation'
import type { Bootstrap } from './contracts'

const bootstrap = { userId: '100', workspace: { workspaceId: '10', name: 'Test' }, userName: 'creator', nickName: 'Creator', nodeTypes: [], execution: { mode: 'managed' }, capabilities: { projectEditing: true, execution: true, assets: true } } as unknown as Bootstrap
let view: VueWrapper | undefined
const json = (data: unknown, code = 200) => new Response(JSON.stringify({ code, data }))
afterEach(() => { view?.unmount(); view = undefined; vi.unstubAllGlobals(); sessionStorage.clear(); history.replaceState(null, '', '/'); document.body.innerHTML = '' })

it('shows real point values and missing image capability, and only navigates without executing', async () => {
  const fetch = vi.fn(async (path: string) => path.includes('/models') ? json([{ kind: 'text', name: 'Flash' }]) : json({ account: { available: '9007199254740993' } }))
  vi.stubGlobal('fetch', fetch)
  view = mount(GettingStarted, { props: { token: 'local', bootstrap } }); await flushPromises()
  expect(view.text()).toContain('9007199254740993'); expect(view.text()).toContain('暂不能 AI 生图或改图')
  for (const [text, section] of [['去选模板', 'templates'], ['准备图片', 'assets'], ['运行记录', 'runs']]) {
    await view.findAll('button').find(b => b.text().startsWith(text!))!.trigger('click')
    expect(view.emitted('navigate')!.at(-1)).toEqual([section])
  }
  expect(fetch).toHaveBeenCalledTimes(2); expect(fetch.mock.calls.every(([path]) => !path.endsWith('/runs'))).toBe(true)
})
it('clears cached readiness after an error and allows a read-only retry', async () => {
  let available = true
  vi.stubGlobal('fetch', vi.fn(async (path: string) => !available ? json({}, 503) : path.includes('/models') ? json([{ kind: 'image' }]) : json({ account: { available: '12' } })))
  view = mount(GettingStarted, { props: { token: 'local', bootstrap } }); await flushPromises()
  expect(view.find('.creation-readiness').exists()).toBe(true)
  available = false; await view.find('[aria-label="重新检查创作条件"]').trigger('click'); await flushPromises()
  expect(view.find('.creation-readiness').exists()).toBe(false); expect(view.text()).toContain('暂时无法核对')
  available = true; await view.find('[aria-label="重新检查创作条件"]').trigger('click'); await flushPromises()
  expect(view.find('.creation-readiness').text()).toContain('12')
})
it('does not fetch a formal account in simulation and blocks editing when capability expires', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  view = mount(GettingStarted, { props: { token: 'local', bootstrap: { ...bootstrap, execution: { ...bootstrap.execution!, mode: 'simulation' }, capabilities: { ...bootstrap.capabilities, projectEditing: false, assets: false } } } })
  await flushPromises(); expect(fetch).not.toHaveBeenCalled()
  expect(view.findAll('button').filter(b => b.attributes('disabled') !== undefined)).toHaveLength(2)
})
it('restores the selected history record through reload and section navigation', async () => {
  sessionStorage.setItem('pod-studio.session', 'local')
  history.replaceState(null, '', '/#runs/55')
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path.includes('eligibility') ? json({ creator: { expired: false } }) : json(bootstrap)))
  const options = { global: { stubs: { Runs: { props: ['initialRunId'], template: '<div data-testid="runs">{{ initialRunId }}<button @click="$emit(\'selected\', {runId: \'56\'})">Pick 56</button></div>' }, GettingStarted: true, Projects: true, Assets: true } } }
  view = mount(App, options); await flushPromises()
  expect(view.find('[data-testid=runs]').text()).toContain('55')
  await view.find('[data-testid=runs] button').trigger('click'); expect(location.hash).toBe('#runs/56')
  view.unmount(); view = mount(App, options); await flushPromises()
  expect(view.find('[data-testid=runs]').text()).toContain('56')
  await view.findAll('.studio-nav button').find(b => b.text() === '素材')!.trigger('click')
  expect(location.hash).toBe('#assets')
})
it('rejects malformed routes rather than using an arbitrary private resource ID', () => {
  for (const hash of ['#runs/../secret', '#project/0', '#runs/1?token=foo', '#unknown']) expect(studioLocation(hash)).toEqual({ section: 'projects', projectId: '', runId: '' })
  expect(studioLocation('#project/4550').projectId).toBe('4550')
  expect(studioLocation('#templates').section).toBe('templates')
})
