// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { Position, VueFlow } from '@vue-flow/core'
import ProjectEditor from './ProjectEditor.vue'
import AssetPicker from './AssetPicker.vue'
import Runs from './Runs.vue'
import NodeAction from './NodeAction.vue'
import ImageSettings from './ImageSettings.vue'
import MagneticHandle from './MagneticHandle.vue'
import type { Bootstrap, NodeDefinition, Project } from './contracts'
import { clone } from './graph'

const definition = (type: NodeDefinition['type'], label: string, inputs: string[], outputs: string[]): NodeDefinition => ({ type, label, version: 1, description: '测试节点', executable: ['textInput', 'imageInput', 'textGenerate'].includes(type),
  inputs: inputs.map(key => ({ key, valueType: key as 'text' | 'images' | 'files', required: true })), outputs: outputs.map(key => ({ key, valueType: key as 'text' | 'images' | 'files', required: true })) })
const bootstrap: Bootstrap = { schemaVersion: 1, userId: '10', userName: 'creator', nickName: '创作者', workspace: { workspaceId: '20', name: '测试空间' },
  capabilities: { projectEditing: true, execution: false, assets: true }, assetLimits: { maxUploadBytes: 10485760, maxPixels: 24000000, maxDimension: 12000, maxImagesPerNode: 8 }, limits: { maxNodes: 100, maxEdges: 300, maxGraphBytes: 1048576 }, nodeTypes: [
    definition('textInput', '文本输入', [], ['text']), definition('imageInput', '图片输入', [], ['images']), definition('textGenerate', 'AI 文本', ['text'], ['text']),
    definition('imageGenerate', 'AI 生图', ['text', 'images'], ['images']), definition('imageProcess', '图片处理', ['images'], ['images']), definition('productPreview', '商品预览', ['images'], ['images']), definition('export', '结果导出', ['images'], ['files']),
  ] }
const initial: Project = { projectId: '1', workspaceId: '20', name: '测试印花', revision: '1', deleted: false, createTime: '2026-11-11 12:00:00', updateTime: '2026-11-11 12:00:00', graph: { schemaVersion: 1, nodes: [], edges: [] } }
let wrapper: VueWrapper | null = null
let server: Project
let saveCode = 200
let savedBodies: { path: string; body: Record<string, unknown> }[]

beforeEach(() => {
  localStorage.clear(); sessionStorage.clear(); server = clone(initial); savedBodies = []; saveCode = 200
  vi.stubGlobal('fetch', vi.fn(async (path: string, options: RequestInit = {}) => {
    if (options.method === 'PUT') {
      const body = JSON.parse(String(options.body)) as Record<string, unknown>
      savedBodies.push({ path, body })
      if (saveCode !== 200) return new Response(JSON.stringify({ code: saveCode, msg: '测试保存失败' }))
      server = { ...server, ...body, revision: String(Number(server.revision) + 1) } as Project
    }
    return new Response(JSON.stringify({ code: 200, data: server }))
  }))
  vi.stubGlobal('confirm', vi.fn(() => true))
})
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks() })
async function open(stubs = {}) {
  wrapper = mount(ProjectEditor, { props: { token: 'test-token', bootstrap, projectId: '1' }, global: { stubs }, attachTo: document.body })
  await flushPromises()
  if (!window.matchMedia('(max-width: 900px)').matches) {
    await wrapper.find('button[aria-label="切换节点工具"]').trigger('click')
    await wrapper.find('.palette-advanced input').setValue(true)
  }
  return wrapper
}

it('saves a multi-field image gesture atomically, undoes and redoes it once, and reloads it', async () => {
  server.graph.nodes = [{ id: 'crop', type: 'imageProcess', version: 1, position: { x: 100, y: 80 }, config: { operation: 'crop', rotate: 90, cropX: 0, cropY: 0, cropWidth: 1, cropHeight: 1 } }]
  const editor = await open({ ImageSettings: true })
  const flow = editor.findComponent(VueFlow)
  flow.vm.$emit('nodeClick', { node: flow.props('nodes')![0] }); await flushPromises()
  const settings = editor.findComponent(ImageSettings)
  settings.vm.$emit('gesture', true)
  settings.vm.$emit('patch', { cropX: .1, cropY: .2, cropWidth: .7, cropHeight: .6 })
  settings.vm.$emit('gesture', false); await flushPromises()
  await editor.find('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(flow.props('nodes')![0]!.data.config.cropWidth).toBe(1)
  expect(flow.props('nodes')![0]!.data.config.cropX).toBe(0)
  await editor.find('button[aria-label="重做 Ctrl+Shift+Z"]').trigger('click'); await flushPromises()
  expect(flow.props('nodes')![0]!.data.config).toMatchObject({ cropX: .1, cropY: .2, cropWidth: .7, cropHeight: .6, rotate: 90 })
  await editor.find('button[aria-label="保存"]').trigger('click'); await flushPromises()
  expect(server.graph.nodes[0]!.config).toMatchObject({ cropX: .1, cropY: .2, cropWidth: .7, cropHeight: .6 })
  editor.unmount(); wrapper = null
  const reopened = await open({ ImageSettings: true })
  expect(reopened.findComponent(VueFlow).props('nodes')![0]!.data.config.cropWidth).toBe(.7)
})

it('filters node tools by name or category without changing the graph', async () => {
  const editor = await open()
  const search = editor.find('input[aria-label="搜索节点"]')
  await search.setValue('ai')
  expect(editor.findAll('.palette-node').map(node => node.text())).toEqual([expect.stringContaining('AI 文本'), expect.stringContaining('AI 生图')])
  await search.setValue('输入')
  expect(editor.findAll('.palette-node')).toHaveLength(2)
  await search.setValue('不存在的节点')
  expect(editor.findAll('.palette-node')).toHaveLength(0)
  expect(editor.find('.palette-empty').exists()).toBe(true)
  await search.setValue('')
  expect(editor.findAll('.palette-node')).toHaveLength(7)
  expect(editor.findComponent(VueFlow).props('nodes')).toHaveLength(0)
  expect(savedBodies).toHaveLength(0)
})

it('preserves saved non-grid coordinates when opening and selecting an existing graph', async () => {
  server.graph.nodes = [{ id: 'existing', type: 'textInput', version: 1, position: { x: 101, y: 79 }, config: { text: '已有设计' } }]
  const editor = await open()
  const flow = editor.findComponent(VueFlow)
  flow.vm.$emit('nodeClick', { node: flow.props('nodes')![0] }); await flushPromises()
  await new Promise(resolve => setTimeout(resolve, 900))
  expect(flow.props('nodes')![0]!.position).toEqual({ x: 101, y: 79 })
  expect(savedBodies).toHaveLength(0)
  expect(editor.find('.save-indicator').text()).toContain('已保存')
})

it('keeps motion preferences and viewport interaction out of saved project content', async () => {
  server.graph.nodes = [{ id: 'start', type: 'textInput', version: 1, position: { x: 120, y: 90 }, config: { text: '保持画布内容' } }]
  const editor = await open()
  expect(editor.findComponent(MagneticHandle).props('following')).toBe(true)
  const motion = editor.find('button[aria-label="切换画布动效"]')
  await motion.trigger('click')
  expect(editor.classes()).toContain('canvas-motion-off')
  expect(editor.findComponent(MagneticHandle).props('following')).toBe(false)
  const flow = editor.findComponent(VueFlow)
  flow.vm.$emit('moveStart'); await flushPromises()
  expect(editor.classes()).toContain('canvas-motion-paused')
  flow.vm.$emit('moveEnd', { flowTransform: { x: 10, y: 20, zoom: .8 } }); await flushPromises()
  expect(editor.classes()).not.toContain('canvas-motion-paused')
  await new Promise(resolve => setTimeout(resolve, 900))
  expect(savedBodies).toHaveLength(0)
  expect(server.graph.nodes[0]!.position).toEqual({ x: 120, y: 90 })
  expect(server.graph.nodes[0]!.config).toEqual({ text: '保持画布内容' })
})

it('keeps parameters and execution in one shared panel without losing selection', async () => {
  const editor = await open({ Runs: true })
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.find('button[aria-label="切换参数面板"]').trigger('click')
  expect(editor.find('.editor-inspector').exists()).toBe(true)
  await editor.find('button[aria-label="打开或关闭执行面板"]').trigger('click')
  expect(editor.findComponent(Runs).exists()).toBe(true)
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.find('button[aria-label="切换参数面板"]').trigger('click')
  expect(editor.findComponent(Runs).exists()).toBe(false)
  expect(editor.find('#node-title').exists()).toBe(true)
  expect(editor.findComponent(VueFlow).props('nodes')).toHaveLength(1)
})

it('shows one floating panel at a time on small screens and responds to resizing', async () => {
  const query = new EventTarget() as MediaQueryList
  Object.assign(query, { matches: true })
  vi.stubGlobal('matchMedia', vi.fn(() => query))
  const editor = await open({ Runs: true })
  expect(editor.find('.node-palette').exists()).toBe(false)
  await editor.find('button[aria-label="切换节点工具"]').trigger('click')
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  expect(editor.find('.node-palette').exists()).toBe(false)
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.find('button[aria-label="打开或关闭执行面板"]').trigger('click')
  expect(editor.find('.editor-runs').exists()).toBe(true)
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.find('button[aria-label="切换节点工具"]').trigger('click')
  expect(editor.find('.node-palette').exists()).toBe(true)
  expect(editor.find('.editor-runs').exists()).toBe(false)
  query.dispatchEvent(new Event('change')); await flushPromises()
  expect(editor.find('.node-palette').exists()).toBe(false)
  expect(editor.find('button[aria-label="保存"]').exists()).toBe(true)
  expect(editor.find('button[aria-label="打开或关闭执行面板"]').exists()).toBe(true)
})

it('keeps mobile tools mutually exclusive and closes More after quick add or Escape', async () => {
  const query = new EventTarget() as MediaQueryList
  Object.assign(query, { matches: true })
  vi.stubGlobal('matchMedia', vi.fn(() => query))
  const editor = await open()
  const more = editor.find('button[aria-label="更多画布工具"]')
  await editor.find('button[aria-label="切换参数面板"]').trigger('click')
  await more.trigger('click')
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  expect(more.attributes('aria-expanded')).toBe('true')
  await editor.find('button[aria-label="快捷添加文本输入"]').trigger('click'); await flushPromises()
  expect(more.attributes('aria-expanded')).toBe('false')
  expect(editor.findComponent(VueFlow).props('nodes')).toHaveLength(1)
  await more.trigger('click')
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); await flushPromises()
  expect(more.attributes('aria-expanded')).toBe('false')
  await more.trigger('click')
  query.dispatchEvent(new Event('change')); await flushPromises()
  expect(more.attributes('aria-expanded')).toBe('false')
  expect(editor.findComponent(VueFlow).props('nodes')).toHaveLength(1)
})

it('dismisses mobile panels without moving nodes, editing content or saving the graph', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  server.graph.nodes = [{ id: 'existing', type: 'textInput', version: 1, position: { x: 101, y: 79 }, config: { text: '已有设计' } }]
  const before = clone(server.graph)
  const editor = await open()
  await editor.find('button[aria-label="切换参数面板"]').trigger('click')
  await editor.find('button[aria-label="关闭工具面板"]').trigger('click')
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  expect(editor.find('.editor-panel-backdrop').exists()).toBe(false)
  await new Promise(resolve => setTimeout(resolve, 900))
  const flow = editor.findComponent(VueFlow)
  expect(flow.props('nodes')![0]!.position).toEqual(before.nodes[0]!.position)
  expect(flow.props('nodes')![0]!.data.config).toEqual(before.nodes[0]!.config)
  expect(savedBodies).toHaveLength(0)
  expect(flow.props('zoomOnPinch')).toBe(true)
  expect(flow.props('connectOnClick')).toBe(true)
})

it('connects existing nodes through the mobile form and persists the same graph coordinates', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  server.graph.nodes = [
    { id: 'source', type: 'textInput', version: 1, position: { x: 101, y: 79 }, config: { text: '设计要求' } },
    { id: 'target', type: 'textGenerate', version: 1, position: { x: 499, y: 83 }, config: {} },
  ]
  const positions = server.graph.nodes.map(node => clone(node.position))
  const editor = await open()
  await editor.find('button[aria-label="更多画布工具"]').trigger('click')
  await editor.find('button[aria-label="打开手动连线"]').trigger('click'); await flushPromises()
  expect(editor.find('.manual-connection').attributes('open')).toBeDefined()
  expect(editor.find('.canvas-more-toggle').attributes('aria-expanded')).toBe('false')
  const selects = editor.findAll('.manual-connection select')
  await selects[0]!.setValue('source')
  await selects[2]!.setValue('target')
  await editor.find('.manual-connection form').trigger('submit'); await flushPromises()
  expect(editor.findComponent(VueFlow).props('edges')).toHaveLength(1)
  await editor.find('button[aria-label="保存"]').trigger('click'); await flushPromises()
  expect(server.graph.edges[0]).toMatchObject({ source: 'source', sourcePort: 'text', target: 'target', targetPort: 'text' })
  expect(server.graph.nodes.map(node => node.position)).toEqual(positions)
})

it('adds selected private asset IDs to the image node and saves them with its graph', async () => {
  const editor = await open()
  await editor.findAll('.palette-node')[1]!.trigger('click'); await flushPromises()
  await editor.find('.node-pick-image').trigger('click'); await flushPromises()
  editor.findComponent(AssetPicker).vm.$emit('confirm', ['12', '13']); await flushPromises()
  await editor.findAll('button').find(button => button.text() === '保存')!.trigger('click'); await flushPromises()
  expect(server.graph.nodes[0]!.config.assetIds).toEqual(['12', '13'])
})

it('mounts the real canvas, edits a node, validates connections and saves its graph', async () => {
  const editor = await open()
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(1)
  await editor.find('.workflow-node textarea').setValue('蓝绿色花朵')
  await editor.findAll('.palette-node')[2]!.trigger('click'); await flushPromises()
  const flow = editor.findComponent(VueFlow)
  const nodes = flow.props('nodes')!
  const [sourceCard, targetCard] = editor.findAll('.workflow-node')
  expect(sourceCard!.find('.magnetic-handle.target').exists()).toBe(false)
  expect(sourceCard!.get('.magnetic-handle.source').attributes('data-handleid')).toBe('text')
  expect(targetCard!.get('.magnetic-handle.target').attributes('data-handleid')).toBe('text')
  expect(editor.find('.workflow-port-labels').exists()).toBe(false)
  flow.vm.$emit('connect', { source: nodes[0]!.id, sourceHandle: 'text', target: nodes[1]!.id, targetHandle: 'text' })
  await flushPromises()
  expect(flow.props('edges')).toHaveLength(1)
  flow.vm.$emit('connect', { source: nodes[1]!.id, sourceHandle: 'text', target: nodes[0]!.id, targetHandle: 'text' })
  await flushPromises(); expect(flow.props('edges')).toHaveLength(1)
  await editor.find('.editor-header-actions button').trigger('click'); await flushPromises()
  expect(savedBodies).toHaveLength(1)
  const graph = savedBodies[0]!.body.graph as Project['graph']
  expect(graph.nodes[0]!.config.text).toBe('蓝绿色花朵'); expect(graph.edges).toHaveLength(1)
  expect(editor.find('.save-indicator').text()).toContain('已保存')
  expect(localStorage.getItem('pod-studio.draft:10:20:1')).toBeNull()
})

it('cancels click and drag connections, blocks a late release, and can start a new wire', async () => {
  server.graph.nodes = [
    { id: 'source', type: 'textInput', version: 1, position: { x: 25, y: 30 }, config: { text: '主题' } },
    { id: 'target', type: 'textGenerate', version: 1, position: { x: 350, y: 30 }, config: {} },
  ]
  const editor = await open()
  const flow = editor.getComponent(VueFlow)
  const handle = { nodeId: 'source', id: 'text', position: Position.Right, type: 'source' as const, x: 30, y: 30 }
  const wire = { source: 'source', sourceHandle: 'text', target: 'target', targetHandle: 'text' }
  await editor.get('.magnetic-handle.source').trigger('click'); await flushPromises()
  await editor.get('.quick-port-existing').trigger('click'); await flushPromises()
  expect(flow.vm.connectionClickStartHandle?.nodeId).toBe('source')
  await editor.get('.connection-hint button').trigger('click')
  expect(flow.vm.connectionClickStartHandle).toBeNull()
  flow.vm.startConnection(handle, { x: 30, y: 30 }); await flushPromises()
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
  expect(flow.vm.connectionStartHandle).toBeNull()
  flow.vm.$emit('connect', wire); await flushPromises()
  expect(flow.props('edges')).toHaveLength(0)
  flow.vm.$emit('connectEnd', new MouseEvent('mouseup'))
  flow.vm.$emit('connect', wire); await flushPromises()
  expect(flow.props('edges')).toHaveLength(1)
})

it('creates and connects a compatible downstream node in a single undoable edit', async () => {
  server.graph.nodes = [{ id: 'source', type: 'textInput', version: 1, position: { x: 25, y: 30 }, config: { text: '主题' } }]
  const editor = await open()
  const flow = editor.getComponent(VueFlow)
  await editor.get('.magnetic-handle.source').trigger('click'); await flushPromises()
  const menu = editor.get('.quick-port-menu')
  expect(menu.text()).toContain('添加下游节点')
  expect(menu.find('[data-node-type="imageInput"]').exists()).toBe(false)
  expect(flow.vm.connectionClickStartHandle).toBeNull()
  await menu.get('[data-node-type="imageGenerate"]').trigger('click'); await flushPromises()
  const created = flow.props('nodes')![1]!
  expect(created.position.x).toBeGreaterThan(25)
  expect(flow.props('edges')).toEqual([expect.objectContaining({ source: 'source', sourceHandle: 'text', target: created.id, targetHandle: 'text' })])
  expect(vi.mocked(fetch).mock.calls.some(([_url, options]) => options?.method === 'POST')).toBe(false)
  await editor.get('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(flow.props('edges')).toHaveLength(0)
  expect(flow.props('nodes')).toHaveLength(1)
})

it('creates a compatible upstream node on the left and preserves its occupied input', async () => {
  server.graph.nodes = [{ id: 'target', type: 'imageGenerate', version: 1, position: { x: 400, y: 30 }, config: {} }]
  const editor = await open()
  const flow = editor.getComponent(VueFlow)
  const input = () => editor.get('.magnetic-handle.target[data-handleid="images"]')
  await input().trigger('keydown', { key: 'Enter' }); await flushPromises()
  expect(editor.get('.quick-port-menu').text()).toContain('添加上游节点')
  await editor.get('.quick-port-menu [data-node-type="imageInput"]').trigger('click'); await flushPromises()
  const created = flow.props('nodes')![1]!
  expect(created.position.x).toBeLessThan(400)
  expect(flow.props('edges')).toEqual([expect.objectContaining({ source: created.id, sourceHandle: 'images', target: 'target', targetHandle: 'images' })])
  await input().trigger('click'); await flushPromises()
  expect(editor.get('.quick-port-menu').findAll('[data-node-type]')).toHaveLength(0)
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })); await flushPromises()
  expect(editor.find('.quick-port-menu').exists()).toBe(false)
  expect(document.activeElement).toBe(input().element)
})

it('disconnects only the selected wire and restores it with undo', async () => {
  server.graph.nodes = [
    { id: 'source', type: 'textInput', version: 1, position: { x: 25, y: 30 }, config: { text: '主题' } },
    { id: 'target', type: 'textGenerate', version: 1, position: { x: 350, y: 30 }, config: {} },
  ]
  server.graph.edges = [{ id: 'wire', source: 'source', sourcePort: 'text', target: 'target', targetPort: 'text' }]
  const editor = await open()
  const flow = editor.getComponent(VueFlow)
  await editor.get('.vue-flow__edge').trigger('click'); await flushPromises()
  const button = document.body.querySelector<HTMLButtonElement>('.edge-disconnect')
  expect(button?.getAttribute('aria-label')).toBe('断开这条连线')
  button!.click(); await flushPromises()
  expect(flow.props('edges')).toHaveLength(0)
  expect(flow.props('nodes')).toHaveLength(2)
  await editor.get('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(flow.props('edges')).toEqual([expect.objectContaining({ id: 'wire' })])
})

it('undoes and redoes changes and supports save from a focused text field', async () => {
  const editor = await open()
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  await editor.find('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(0)
  await editor.find('button[aria-label="重做 Ctrl+Shift+Z"]').trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(1)
  const text = editor.find('.workflow-node textarea')
  await text.setValue('快捷键保存')
  const event = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, bubbles: true, cancelable: true })
  text.element.dispatchEvent(event); await flushPromises()
  expect(event.defaultPrevented).toBe(true); expect(savedBodies).toHaveLength(1)
})

it('keeps a failed draft locally and renders a retry action', async () => {
  const editor = await open()
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  await editor.find('.workflow-node textarea').setValue('离线时保留')
  saveCode = 503
  await editor.find('.editor-header-actions button').trigger('click'); await flushPromises()
  expect(editor.text()).toContain('保存失败')
  expect(localStorage.getItem('pod-studio.draft:10:20:1')).toContain('离线时保留')
  saveCode = 200
  const retry = editor.findAll('button').find(button => button.text() === '重试保存')!
  await retry.trigger('click'); await flushPromises()
  expect(editor.find('.save-indicator').text()).toContain('已保存')
})

it('offers a stale cached draft without automatically overwriting the newer server', async () => {
  server.revision = '2'
  localStorage.setItem('pod-studio.draft:10:20:1', JSON.stringify({ userId: '10', workspaceId: '20', projectId: '1', baseRevision: '1', document: { name: '旧版未保存', graph: initial.graph } }))
  const editor = await open()
  expect(editor.text()).toContain('发现此浏览器的未保存草稿')
  await editor.findAll('button').find(button => button.text() === '恢复草稿')!.trigger('click'); await flushPromises()
  expect(editor.find('.save-indicator').text()).toContain('版本冲突')
  expect(savedBodies).toHaveLength(0)
  expect(editor.find('button').exists()).toBe(true)
})

it('records dragged positions and supports copy, delete and undo', async () => {
  const editor = await open()
  await editor.findAll('.palette-node')[0]!.trigger('click'); await flushPromises()
  const flow = editor.findComponent(VueFlow)
  const original = clone(flow.props('nodes')![0]!.position)
  flow.vm.$emit('nodeDragStart', {})
  flow.props('nodes')![0]!.position.x = original.x + 120
  flow.props('nodes')![0]!.position.y = original.y + 60
  await flushPromises(); flow.vm.$emit('nodeDragStop', {}); await flushPromises()
  await editor.find('.editor-header-actions button').trigger('click'); await flushPromises()
  const graph = savedBodies.at(-1)!.body.graph as Project['graph']
  expect(graph.nodes[0]!.position).toEqual({ x: original.x + 120, y: original.y + 60 })
  await editor.find('button[aria-label="复制选中节点 Ctrl+D"]').trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(2)
  await editor.find('button[aria-label="删除选中节点或连线 Delete"]').trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(1)
  await editor.find('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(editor.findAll('.workflow-node')).toHaveLength(2)
})

it('opens polish from the selected node parameters and applies it to the same input node', async () => {
  server.graph.nodes = [{ id: 'A', type: 'textInput', version: 1, position: { x: 100, y: 100 }, config: { text: '原来的花朵' } }]
  const editor = await open({ NodeAction: true })
  await editor.setProps({ bootstrap: { ...bootstrap, execution: { mode: 'simulation', globalConcurrency: 1, userConcurrency: 1, providerConcurrency: 1, maxPending: 10, maxUserPending: 10, initialCredits: '100' } } })
  expect(editor.find('.inspector-node-actions').exists()).toBe(false)
  await editor.get('.workflow-node-content textarea').trigger('click')
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.get('button[aria-label="设置节点 A"]').trigger('click')
  await editor.get('.editor-inspector .inspector-node-actions button').trigger('click'); await flushPromises()
  const action = editor.findComponent(NodeAction)
  expect(action.props('action')).toBe('polish')
  action.vm.$emit('apply', '润色后的花朵', '原来的花朵'); action.vm.$emit('close'); await flushPromises()
  const flow = editor.findComponent(VueFlow)
  expect(flow.props('nodes')).toHaveLength(1)
  expect(flow.props('nodes')![0]!.data.config.text).toBe('润色后的花朵')
  await editor.find('button[aria-label="撤销 Ctrl+Z"]').trigger('click'); await flushPromises()
  expect(flow.props('nodes')![0]!.data.config.text).toBe('原来的花朵')
})

it('copies and removes the named node even when another node is selected', async () => {
  const editor = await open()
  await editor.find('button[aria-label="快捷添加文本输入"]').trigger('click'); await flushPromises()
  const id = editor.findComponent(VueFlow).props('nodes')![0]!.id
  await editor.find('button[aria-label="快捷添加图片输入"]').trigger('click'); await flushPromises()
  await editor.find(`button[aria-label="复制节点 ${id}"]`).trigger('click'); await flushPromises()
  let nodes = editor.findComponent(VueFlow).props('nodes')!
  expect(nodes).toHaveLength(3)
  expect(nodes.filter(node => node.data.definition.type === 'textInput')).toHaveLength(2)
  await editor.find(`button[aria-label="删除节点 ${id}"]`).trigger('click'); await flushPromises()
  nodes = editor.findComponent(VueFlow).props('nodes')!
  expect(nodes).toHaveLength(2)
  expect(nodes.some(node => node.id === id)).toBe(false)
  expect(nodes.filter(node => node.data.definition.type === 'imageInput')).toHaveLength(1)
})

it('opens parameters on node click, switches node types, and saves image parameters as before', async () => {
  const editor = await open()
  await editor.find('button[aria-label="快捷添加生图节点"]').trigger('click'); await flushPromises()
  const flow = editor.findComponent(VueFlow)
  expect(editor.find('#node-image-ratio').exists()).toBe(false)
  flow.vm.$emit('nodeClick', { node: flow.props('nodes')![0] }); await flushPromises()
  expect(editor.find('.workflow-node select').exists()).toBe(false)
  expect(editor.get('.editor-inspector .inspector-node-actions').text()).toContain('立即生成')
  await editor.get('#node-image-ratio').setValue('3:4')
  await editor.get('#node-image-count').setValue('2')
  await editor.get('#node-image-prompt').setValue('柔和水彩')
  await editor.get('button[aria-label="快捷添加文本输入"]').trigger('click'); await flushPromises()
  flow.vm.$emit('nodeClick', { node: flow.props('nodes')![1] }); await flushPromises()
  expect(editor.find('#node-image-ratio').exists()).toBe(false)
  expect(editor.get('.editor-inspector .inspector-node-actions').text()).toContain('AI 润色')
  flow.vm.$emit('paneClick', {}); await flushPromises()
  expect(editor.find('.editor-inspector').exists()).toBe(false)
  await editor.find('button[aria-label="保存"]').trigger('click'); await flushPromises()
  expect(server.graph.nodes[0]!.config).toMatchObject({ ratio: '3:4', count: 2, prompt: '柔和水彩' })
})

it('inserts a preset alongside existing nodes without executing a model', async () => {
  server.graph.nodes = [{ id: 'keep', type: 'textInput', version: 1, position: { x: 100, y: 100 }, config: { text: '保留已有内容' } }]
  const editor = await open()
  await editor.find('button[aria-label="打开工作流预设"]').trigger('click'); await flushPromises()
  await editor.get('button[aria-label="添加到画布：一条想法生成印花"]').trigger('click'); await flushPromises()
  const nodes = editor.findComponent(VueFlow).props('nodes')!
  expect(nodes).toHaveLength(3)
  expect(nodes.find(node => node.id === 'keep')?.data.config.text).toBe('保留已有内容')
  expect(new Set(nodes.map(node => node.id)).size).toBe(3)
  expect(vi.mocked(fetch).mock.calls.some(([_url, options]) => options?.method === 'POST')).toBe(false)
})

it('does not let canvas shortcuts alter nodes behind an open library dialog', async () => {
  const editor = await open()
  await editor.find('button[aria-label="快捷添加文本输入"]').trigger('click'); await flushPromises()
  await editor.find('button[aria-label="打开工作流预设"]').trigger('click'); await flushPromises()
  const dialog = editor.find('dialog')
  expect(dialog.attributes('open')).toBeDefined()
  dialog.element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }))
  await flushPromises()
  expect(editor.findComponent(VueFlow).props('nodes')).toHaveLength(1)
})
