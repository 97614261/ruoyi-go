// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import Projects from './Projects.vue'
import ProjectNameDialog from './ProjectNameDialog.vue'
import StudioNavigation from './StudioNavigation.vue'
import StudioPoints from './StudioPoints.vue'
import App from './App.vue'
import { studioLocation } from './navigation'
import type { Bootstrap, ProjectSummary } from './contracts'

const bootstrap: Bootstrap = {
  schemaVersion: 1, userId: '10', userName: 'creator', nickName: 'Creator',
  workspace: { workspaceId: '20', name: 'My workspace' }, nodeTypes: [],
  capabilities: { projectEditing: true, execution: true, assets: true },
  limits: { maxNodes: 100, maxEdges: 200, maxGraphBytes: 1048576 },
  assetLimits: { maxUploadBytes: 10485760, maxPixels: 40000000, maxDimension: 10000, maxImagesPerNode: 8 },
  execution: { mode: 'managed', globalConcurrency: 2, userConcurrency: 1, providerConcurrency: 1, maxPending: 20, maxUserPending: 5, initialCredits: '0' },
}
const project: ProjectSummary = { projectId: '5', workspaceId: '20', name: 'My print', revision: '9', createTime: '2026-11-17 12:00:00', updateTime: '2026-11-17 12:00:00', deleted: false }
const json = (data: unknown) => new Response(JSON.stringify({ code: 200, data }))
const listing = () => new Response(JSON.stringify({ code: 200, rows: [project], total: 1 }))
let view: VueWrapper | undefined
afterEach(() => { view?.unmount(); view = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks(); sessionStorage.clear(); history.replaceState(null, '', '/'); document.body.innerHTML = '' })

it('opens home by default and preserves existing private project and run addresses', () => {
  expect(studioLocation('').section).toBe('home')
  expect(studioLocation('#home').section).toBe('home')
  expect(studioLocation('#projects').section).toBe('projects')
  expect(studioLocation('#project/4550')).toEqual({ section: 'projects', projectId: '4550', runId: '' })
  expect(studioLocation('#runs/6676')).toEqual({ section: 'runs', projectId: '', runId: '6676' })
})

it('navigates through the actual product areas and announces the selected section', async () => {
  view = mount(StudioNavigation, { props: { section: 'home' } })
  expect(view.find('[aria-current="page"]').text()).toBe('首页')
  expect(view.findAll('button').map(button => button.text())).toEqual(['首页', '项目', '模板', '素材', '任务与结果'])
  await view.findAll('button').find(button => button.text() === '素材')!.trigger('click')
  expect(view.emitted('navigate')).toEqual([['assets']])
})

it('uses the real recent projects and only navigates when a home shortcut is selected', async () => {
  const fetch = vi.fn(async () => listing()); vi.stubGlobal('fetch', fetch)
  view = mount(Projects, { props: { token: 'local', bootstrap, home: true }, global: { stubs: { ProjectNameDialog: true } } }); await flushPromises()
  expect(view.find('.recent-projects').text()).toContain('My print')
  await view.find('.recent-project').trigger('click')
  expect(view.emitted('open')).toEqual([['5']])
  await view.findAll('.home-tools button')[0]!.trigger('click')
  expect(view.emitted('navigate')).toEqual([['runs']])
  expect(fetch.mock.calls).toHaveLength(1)
  expect(view.find('.workspace-home-split .home-create .launch-canvas').exists()).toBe(true)
  expect(view.find('.workspace-home-split .home-recent .recent-project').exists()).toBe(true)
  await view.setProps({ bootstrap: { ...bootstrap, capabilities: { ...bootstrap.capabilities, projectEditing: false } } })
  expect(view.find('.launch-canvas').attributes('disabled')).toBeDefined()
})

it('creates from the new project entry with the original request and no execution', async () => {
  const fetch = vi.fn(async (_path: string, options: RequestInit = {}) => options.method === 'POST' ? json(project) : listing()); vi.stubGlobal('fetch', fetch)
  view = mount(Projects, { props: { token: 'local', bootstrap }, global: { stubs: { ProjectNameDialog: true } } }); await flushPromises()
  const create = view.findAll('.workspace-project-index button').filter(button => button.text() === '新建项目')
  expect(create).toHaveLength(1)
  expect(view.get('.project-index-row').text()).toContain('v9')
  await create[0]!.trigger('click')
  view.findComponent(ProjectNameDialog).vm.$emit('confirm', 'New canvas'); await flushPromises()
  const writes = fetch.mock.calls.filter(([, options]) => options?.method === 'POST')
  expect(writes).toHaveLength(1)
  expect(writes[0]![0]).toContain('/pod/projects')
  expect(JSON.parse(String(writes[0]![1]?.body))).toEqual({ name: 'New canvas' })
  expect(view.emitted('open')).toEqual([['5']])
})

it('keeps revision checks when moving a project into the recycle bin', async () => {
  const fetch = vi.fn(async (_path: string, options: RequestInit = {}) => options.method === 'DELETE' ? json({}) : listing()); vi.stubGlobal('fetch', fetch)
  view = mount(Projects, { props: { token: 'local', bootstrap } }); await flushPromises()
  await view.findAll('.project-menu button').find(button => button.text() === '移入回收站')!.trigger('click'); await flushPromises()
  const deletion = fetch.mock.calls.find(([, options]) => options?.method === 'DELETE')!
  expect(deletion[0]).toContain('/pod/projects/5')
  expect(JSON.parse(String(deletion[1]?.body))).toEqual({ revision: '9' })
})

it('shows exact points and ignores an old balance response after changing sections', async () => {
  let release!: (response: Response) => void
  const fetch = vi.fn().mockImplementationOnce(() => new Promise(resolve => { release = resolve })).mockResolvedValue(json({ account: { available: '9007199254740993' } })); vi.stubGlobal('fetch', fetch)
  view = mount(StudioPoints, { props: { token: 'local', managed: true, refreshKey: 'home' } })
  await view.setProps({ refreshKey: 'projects' }); await flushPromises()
  expect(view.text()).toContain('9007199254740993')
  release(json({ account: { available: '5' } })); await flushPromises()
  expect(view.text()).toContain('9007199254740993')
  await view.find('button').trigger('click'); expect(view.emitted('open')).toHaveLength(1)
  expect(fetch.mock.calls.every(([, options]) => !options?.method)).toBe(true)
})

it('uses a modal mobile navigation that closes when a destination is chosen', async () => {
  sessionStorage.setItem('pod-studio.session', 'local')
  history.replaceState(null, '', '/#home')
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path.includes('eligibility') ? json({ creator: { expired: false } }) : json(bootstrap)))
  view = mount(App, { global: { stubs: { StudioPoints: true, Projects: true, GettingStarted: true, Assets: true } }, attachTo: document.body }); await flushPromises()
  expect(view.find('getting-started-stub').exists()).toBe(false)
  await view.find('[aria-label="打开导航"]').trigger('click')
  expect((view.find('.navigation-dialog').element as HTMLDialogElement).open).toBe(true)
  expect(view.find('.navigation-dialog .mobile-signout').text()).toBe('退出登录')
  await view.findAll('.navigation-dialog .studio-nav button').find(button => button.text() === '素材')!.trigger('click'); await flushPromises()
  expect(location.hash).toBe('#assets')
  expect((view.find('.navigation-dialog').element as HTMLDialogElement).open).toBe(false)
})

it('keeps desktop navigation in the sidebar and highlights the active page', async () => {
  sessionStorage.setItem('pod-studio.session', 'local')
  history.replaceState(null, '', '/#home')
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path.includes('eligibility') ? json({ creator: { expired: false } }) : json(bootstrap)))
  view = mount(App, { global: { stubs: { StudioPoints: true, Projects: true, Runs: true } } }); await flushPromises()
  expect(view.find('.studio-header .studio-nav').exists()).toBe(false)
  const nav = view.get('.studio-sidebar .studio-nav')
  expect(nav.get('[aria-current="page"]').text()).toBe('首页')
  await nav.findAll('button').find(button => button.text() === '任务与结果')!.trigger('click')
  expect(nav.get('[aria-current="page"]').text()).toBe('任务与结果')
  expect(view.find('runs-stub').exists()).toBe(true)
  expect(location.hash).toBe('#runs')
})

it('shows a public home without requesting private data or captcha until a feature is clicked', async () => {
  const fetch = vi.fn(async (_path: string) => new Response(JSON.stringify({ code: 200, captchaEnabled: true, uuid: 'captcha-1', img: 'fixture' })))
  vi.stubGlobal('fetch', fetch)
  history.replaceState(null, '', '/#project/4550')
  view = mount(App, { attachTo: document.body }); await flushPromises()
  const dialog = view.get('.signin-dialog')
  expect(view.get('.workspace-home').text()).toContain('新建画布创作')
  expect(view.find('.account-profile').exists()).toBe(false)
  expect((dialog.element as HTMLDialogElement).open).toBe(false)
  expect(fetch).not.toHaveBeenCalled()
  await view.get('.launch-template').trigger('click'); await flushPromises()
  expect((dialog.element as HTMLDialogElement).open).toBe(true)
  expect(dialog.find('#captcha').exists()).toBe(true)
  expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/captchaImage'])
  await dialog.get('[aria-label="关闭登录"]').trigger('click')
  expect((dialog.element as HTMLDialogElement).open).toBe(false)
  expect(location.hash).toBe('#project/4550')
  expect(view.find('.workspace-home').exists()).toBe(true)
})

it('opens the requested section after login without making an unauthenticated section request', async () => {
  const fetch = vi.fn(async (path: string) => {
    if (path === '/api/captchaImage') return new Response(JSON.stringify({ code: 200, captchaEnabled: false }))
    if (path === '/api/login') return new Response(JSON.stringify({ code: 200, token: 'session' }))
    if (path === '/api/pod/eligibility') return json({ creator: { expired: false } })
    return json(bootstrap)
  })
  vi.stubGlobal('fetch', fetch)
  view = mount(App, { global: { stubs: { StudioPoints: true, Templates: true } }, attachTo: document.body })
  await view.get('.launch-template').trigger('click'); await flushPromises()
  const dialog = view.get('.signin-dialog')
  await dialog.get('#username').setValue('creator')
  await dialog.get('#password').setValue('fixture-password')
  expect(dialog.find('#captcha').exists()).toBe(false)
  await dialog.get('form').trigger('submit'); await flushPromises()
  expect(location.hash).toBe('#templates')
  expect(view.find('templates-stub').exists()).toBe(true)
  expect((dialog.element as HTMLDialogElement).open).toBe(false)
  expect((dialog.get('#password').element as HTMLInputElement).value).toBe('')
  expect(fetch.mock.calls.map(([url]) => url)).toEqual(['/api/captchaImage', '/api/login', '/api/pod/eligibility', '/api/pod/bootstrap'])
})

it('resumes new-project naming after login without creating a project automatically', async () => {
  const fetch = vi.fn(async (path: string, options: RequestInit = {}) => {
    if (path === '/api/captchaImage') return new Response(JSON.stringify({ code: 200, captchaEnabled: false }))
    if (path === '/api/login') return new Response(JSON.stringify({ code: 200, token: 'session' }))
    if (path === '/api/pod/eligibility') return json({ creator: { expired: false } })
    if (path.startsWith('/api/pod/projects?') && options.method !== 'POST') return listing()
    return json(bootstrap)
  })
  vi.stubGlobal('fetch', fetch)
  view = mount(App, { global: { stubs: { StudioPoints: true } }, attachTo: document.body })
  await view.get('.launch-canvas').trigger('click'); await flushPromises()
  await view.get('.signin-dialog #username').setValue('creator')
  await view.get('.signin-dialog #password').setValue('fixture-password')
  await view.get('.signin-dialog form').trigger('submit'); await flushPromises()
  expect(view.getComponent(ProjectNameDialog).props('title')).toBe('新建项目')
  expect((view.get('.signin-dialog').element as HTMLDialogElement).open).toBe(false)
  expect(fetch.mock.calls.filter(([, options]) => options?.method === 'POST').map(([path]) => path)).toEqual(['/api/login'])
})
