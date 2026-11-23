<script setup lang="ts">
import { createUUID } from './uuid'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { VueFlow, useVueFlow } from '@vue-flow/core'
import type { Connection, FitViewParams } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import { PhArrowLeft, PhArrowCounterClockwise, PhArrowClockwise, PhBezierCurve, PhCopy, PhCornersOut, PhDotsThree, PhDownloadSimple, PhFloppyDisk, PhImage, PhLightbulb, PhLink, PhMagicWand, PhMagnifyingGlass, PhMapTrifold, PhMinus, PhPlay, PhPlus, PhSidebarSimple, PhSparkle, PhSquaresFour, PhTextT, PhTrash, PhWarningCircle, PhWaveform, PhX } from '@phosphor-icons/vue'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'
import { APIError, request } from './api'
import type { Bootstrap, DraftDocument, Graph, NodeDefinition, NodeType, Project, RunNode } from './contracts'
import { DraftSession } from './draft'
import type { SaveState } from './draft'
import { clone, DraftHistory, duplicateSelection, graphError, removeSelection, stable } from './graph'
import { defaultConfig, nodeFields } from './node-fields'
import type { ConfigField } from './node-fields'
import WorkflowNode from './WorkflowNode.vue'
import WorkflowEdge from './WorkflowEdge.vue'
import { useCanvasMotion } from './use-canvas-motion'
import ModelSelect from './ModelSelect.vue'
import ThemeSwitch from './ThemeSwitch.vue'
import StudioBrand from './StudioBrand.vue'
import type { CreatorModel } from './contracts'
import AssetPicker from './AssetPicker.vue'
import PrivateImage from './PrivateImage.vue'
import Runs from './Runs.vue'
import NodeAction from './NodeAction.vue'
import CreativeLibrary from './CreativeLibrary.vue'
import ImageSettings from './ImageSettings.vue'
import TemplateSaveDialog from './TemplateSaveDialog.vue'
import TemplateLibraryDialog from './TemplateLibraryDialog.vue'
import { nodeCategories, nodeIcons } from './node-ui'
import { useProjectRun } from './use-project-run'
import ProjectResults from './ProjectResults.vue'

const props = defineProps<{ token: string; bootstrap: Bootstrap; projectId: string }>()
const emit = defineEmits<{ close: []; open: [id: string]; expired: [] }>()
interface CanvasNode {
  id: string; type: string; position: { x: number; y: number }; dragHandle?: string; selected?: boolean;
  data: { definition: NodeDefinition; config: Record<string, unknown> };
}
interface CanvasEdge { id: string; source: string; target: string; sourceHandle?: string; targetHandle?: string; type?: string; selected?: boolean }
interface CachedDraft { userId: string; workspaceId: string; projectId: string; baseRevision: string; document: DraftDocument }

const models = ref<CreatorModel[]>([]), modelError = ref('')
onMounted(async () => {
  if (props.bootstrap.execution?.mode !== 'managed') return
  try { models.value = (await request<{ data: CreatorModel[] }>('/pod/models', {}, props.token)).data }
  catch (e) { modelError.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
})
const flowId = `pod-project-${props.projectId}`
const { screenToFlowCoordinate, fitView, setViewport, getViewport, viewport, endConnection, connectionStartHandle, connectionClickStartHandle } = useVueFlow({ id: flowId })
const canvas = ref<HTMLElement>(), nodes = ref<CanvasNode[]>([]), edges = ref<CanvasEdge[]>([])
const name = ref(''), revision = ref(''), loading = ref(true), busy = ref(false), error = ref(''), hint = ref(''), localWarning = ref('')
const selectedId = ref(''), showInspector = ref(false), showPalette = ref(false), state = ref<SaveState>('saved'), dirty = ref(false)
const paletteQuery = ref(''), showMiniMap = ref(false), narrowScreen = ref(false)
const showMoreTools = ref(false), manualConnectionOpen = ref(false)
const manualConnection = ref<HTMLDetailsElement>()
const showAdvanced = ref(false), library = ref<'templates' | 'prompts' | ''>('')
const showTemplateSave = ref(false), showTemplateLibrary = ref(false)
const nodeAction = ref<{ id: string; mode: 'polish' | 'branch' } | null>(null)
const actionNode = computed(() => nodes.value.find(node => node.id === nodeAction.value?.id))
const localTypes = ['imageProcess', 'productPreview', 'export']
const inputAssetId = computed(() => {
  const source = nodes.value.find(n => n.id === edges.value.find(e => e.target === selectedId.value && e.targetHandle === 'images')?.source)
  if (!source) return ''
  if (source.data.definition.type === 'imageInput') return String((source.data.config.assetIds as string[] | undefined)?.[0] || '')
  if (!activeRun.value?.graph) return ''
  const current = visualGraph(), previous = activeRun.value.graph
  const matches = (id: string): boolean => {
    const node = current.nodes.find(n => n.id === id), old = previous.nodes.find(n => n.id === id)
    if (!node || !old || node.type !== old.type || stable(node.config) !== stable(old.config)) return false
    const incoming = current.edges.filter(e => e.target === id)
    return stable(incoming.map(e => [e.source, e.sourcePort, e.targetPort]).sort()) === stable(previous.edges.filter(e => e.target === id).map(e => [e.source, e.sourcePort, e.targetPort]).sort()) && incoming.every(e => matches(e.source))
  }
  return matches(source.id) ? activeRun.value.nodes?.find(n => n.nodeId === source.id && n.state === 'succeeded')?.output.assetIds?.[0] || '' : ''
})
const paletteNodes = computed(() => props.bootstrap.nodeTypes.filter(node => (showAdvanced.value || (node.type !== 'textGenerate' && node.executable)) && `${node.label} ${node.description} ${nodeCategories[node.type]}`.toLocaleLowerCase().includes(paletteQuery.value.trim().toLocaleLowerCase())))
const executionLabel = computed(() => props.bootstrap.execution?.mode === 'managed' ? '模型创作' : props.bootstrap.execution?.mode === 'qwen' ? '千问模式' : props.bootstrap.execution?.mode === 'simulation' ? '模拟模式' : '仅编辑')
const canUndo = ref(false), canRedo = ref(false), recovery = ref<CachedDraft | null>(null)
const sourceId = ref(''), sourcePort = ref(''), targetId = ref(''), targetPort = ref('')
const dragging = ref(false), imageGesture = ref(false)
const panning = ref(false)
const { motionEnabled, reducedMotion, documentHidden, toggleMotion } = useCanvasMotion()
const connectionInProgress = computed(() => !!connectionStartHandle.value || !!connectionClickStartHandle.value)
const quickPort = ref<{ id: string; side: 'source' | 'target'; key: string; anchor: HTMLElement; left: number; top: number }>()
const portMenu = ref<HTMLElement>()
const quickOptions = computed(() => {
  const port = quickPort.value
  if (!port) return []
  const node = nodes.value.find(node => node.id === port.id)
  const source = port.side === 'source'
  const type = (source ? node?.data.definition.outputs : node?.data.definition.inputs)?.find(p => p.key === port.key)?.valueType
  if (!type || (!source && edges.value.some(e => e.target === port.id && e.targetHandle === port.key))) return []
  return props.bootstrap.nodeTypes.flatMap(definition => {
    const match = (source ? definition.inputs : definition.outputs).find(p => p.valueType === type)
    return match ? [{ definition, key: match.key }] : []
  })
})
const cancelledDrag = ref(false)
const pickerTarget = ref('')
const showRuns = ref(false)
const showResults = ref(false)
const { activeRun, selectedRunId, runError, selectRun, refreshRun, forgetRun: forgetSelectedRun } = useProjectRun(() => props.token, props.projectId, `${props.bootstrap.userId}:${props.bootstrap.workspace.workspaceId}`, () => showRuns.value || !!nodeAction.value, () => emit('expired'))
const displayedImages = ref(new Map<string, { runId: string; revision: string; node: RunNode }>())
watch(activeRun, run => {
  if (!run || !/^[1-9]\d{0,19}$/.test(run.runId)) return
  const next = new Map(displayedImages.value)
  for (const node of run.nodes || []) {
    const previous = next.get(node.nodeId)
    if (node.state === 'succeeded' && node.output.assetIds?.length && (!previous || BigInt(run.runId) >= BigInt(previous.runId))) next.set(node.nodeId, { runId: run.runId, revision: run.projectRevision, node })
  }
  displayedImages.value = next
}, { flush: 'sync' })
function dropRunImages(id: string) {
  displayedImages.value = new Map([...displayedImages.value].filter(([, result]) => result.runId !== id))
}
watch(selectedRunId, (id, previous) => { if (!id && previous) dropRunImages(previous) })
function forgetRun(id: string) {
  forgetSelectedRun(id)
  dropRunImages(id)
}
function imageResult(id: string, type: NodeType) {
  const result = displayedImages.value.get(id)
  return result?.node.nodeType === type ? result : undefined
}
const fitOptions = computed<FitViewParams>(() => ({
  padding: narrowScreen.value ? { top: '24px', bottom: '150px', left: '24px', right: '24px' } : { top: '110px', bottom: '120px', left: showPalette.value ? '310px' : '36px', right: showRuns.value ? '446px' : '32px' },
  maxZoom: 1, duration: 0,
}))
let revealFrame = 0
let inspectorFrame = 0, inspectorObserver: ResizeObserver | undefined
const inspectorPosition = ref<{ left: string; top: string; width: string; maxHeight: string }>()
function positionInspector() {
  cancelAnimationFrame(inspectorFrame)
  if (!showInspector.value || !selectedId.value || narrowScreen.value) { inspectorPosition.value = undefined; return }
  inspectorFrame = requestAnimationFrame(() => {
    const root = canvas.value, node = root?.querySelector<HTMLElement>(`.vue-flow__node[data-id="${CSS.escape(selectedId.value)}"] .workflow-node`)
    if (!root || !node) return
    const area = root.getBoundingClientRect(), rect = node.getBoundingClientRect()
    if (!area.width || !rect.width) return
    const paletteRight = root.parentElement?.querySelector('.node-palette')?.getBoundingClientRect().right ?? area.left
    const leftEdge = Math.min(Math.max(12, paletteRight - area.left + 12), area.width / 2)
    const width = Math.min(470, area.width - leftEdge - 12)
    const ceiling = 54, floor = area.height - 96
    const below = Math.max(ceiling, rect.bottom - area.top + 13), above = Math.min(floor, rect.top - area.top - 58)
    const flip = floor - below < 190 && above - ceiling >= 190
    const room = flip ? above - ceiling : floor - below
    const height = Math.max(150, Math.min(350, room))
    const top = flip ? above - height : Math.min(below, Math.max(ceiling, floor - height))
    const left = Math.max(leftEdge, Math.min(area.width - width - 12, (rect.left + rect.right) / 2 - area.left - width / 2))
    inspectorPosition.value = { left: `${left}px`, top: `${top}px`, width: `${width}px`, maxHeight: `${height}px` }
  })
}
watch([selectedId, showInspector, narrowScreen, showPalette, canvas], () => {
  inspectorObserver?.disconnect()
  const root = canvas.value
  const node = root?.querySelector<HTMLElement>(`.vue-flow__node[data-id="${CSS.escape(selectedId.value)}"] .workflow-node`)
  if (root && node && showInspector.value && !narrowScreen.value && typeof ResizeObserver !== 'undefined') {
    inspectorObserver = new ResizeObserver(positionInspector)
    inspectorObserver.observe(root); inspectorObserver.observe(node)
  }
  positionInspector()
}, { flush: 'post' })
watch(() => [viewport.value.x, viewport.value.y, viewport.value.zoom], positionInspector, { flush: 'post' })
watch(() => {
  const node = nodes.value.find(item => item.id === selectedId.value)
  return [node?.position.x, node?.position.y]
}, positionInspector, { flush: 'post' })
watch([panning, dragging, connectionInProgress], ([moving, dragging, connecting]) => { if (moving || dragging || connecting) closeQuickAdd() })
function revealSelectedNode() {
  if (narrowScreen.value || !selectedId.value) return
  void nextTick(() => {
    cancelAnimationFrame(revealFrame)
    revealFrame = requestAnimationFrame(() => {
      const surface = canvas.value, id = selectedId.value
      if (!surface || narrowScreen.value) return
      const element = surface.querySelector<HTMLElement>(`.vue-flow__node[data-id="${CSS.escape(id)}"]`)
      if (!element) return
      const bounds = surface.getBoundingClientRect(), node = element.getBoundingClientRect()
      if (!bounds.width || !node.width) return
      const palette = surface.parentElement?.querySelector('.node-palette')?.getBoundingClientRect()
      const panel = surface.parentElement?.querySelector('.editor-runs')?.getBoundingClientRect()
      const left = palette ? palette.right + 20 : bounds.left + 24
      const right = panel ? panel.left - 20 : bounds.right - 24
      // Pan the viewport, never the saved node position, when tools cover the selection.
      const dx = node.width > right - left ? (left + right - node.left - node.right) / 2 : Math.max(left - node.left, Math.min(0, right - node.right))
      if (Math.abs(dx) > 1) { const view = getViewport(); void setViewport({ ...view, x: view.x + dx }) }
    })
  })
}
const activeRunNodes = computed(() => activeRun.value?.projectRevision === revision.value ? activeRun.value.nodes || [] : [])
function togglePanel(panel: 'palette' | 'inspector' | 'runs') {
  showMoreTools.value = false
  const target = panel === 'palette' ? showPalette : panel === 'inspector' ? showInspector : showRuns
  const opening = !target.value
  if (opening) {
    if (panel === 'runs') showInspector.value = false
    if (panel === 'inspector') showRuns.value = false
    if (narrowScreen.value) { showPalette.value = false; showInspector.value = false; showRuns.value = false }
  }
  target.value = opening
  if (opening && panel !== 'palette') revealSelectedNode()
}
function closeTools() {
  closeQuickAdd()
  showPalette.value = false; showInspector.value = false; showRuns.value = false; showMoreTools.value = false
}
function toggleMoreTools() {
  const opening = !showMoreTools.value
  if (opening && narrowScreen.value) closeTools()
  showMoreTools.value = opening
}
async function openConnections() {
  closeTools(); showInspector.value = true; manualConnectionOpen.value = true
  await nextTick()
  manualConnection.value?.scrollIntoView({ block: 'start' })
}
function inspectNode(id: string, panel = true) {
  closeQuickAdd()
  selectedId.value = id; showInspector.value = panel; showRuns.value = false
  if (narrowScreen.value) { showPalette.value = false; showMoreTools.value = false }
  revealSelectedNode()
}
function selectCanvasNode(id: string, event?: MouseEvent | TouchEvent) {
  if (connectionInProgress.value) return
  inspectNode(id, !(event?.metaKey || event?.ctrlKey))
}
function closeQuickAdd(restoreFocus = false) {
  const anchor = quickPort.value?.anchor
  quickPort.value = undefined
  if (restoreFocus && anchor?.isConnected) anchor.focus({ preventScroll: true })
}
function openQuickAdd(id: string, side: 'source' | 'target', key: string, anchor: HTMLElement) {
  if (recovery.value || connectionInProgress.value || !canvas.value) return
  if (quickPort.value?.anchor === anchor) { closeQuickAdd(true); return }
  closeTools()
  const root = canvas.value.getBoundingClientRect(), point = anchor.getBoundingClientRect()
  const width = Math.min(246, Math.max(0, root.width - 24))
  const left = side === 'source' ? point.right - root.left + 14 : point.left - root.left - width - 14
  quickPort.value = { id, side, key, anchor, left: Math.max(12, Math.min(left, root.width - width - 12)), top: Math.max(12, Math.min(point.top - root.top - 18, root.height - 242)) }
  void nextTick(() => portMenu.value?.querySelector<HTMLElement>('button[data-primary]')?.focus({ preventScroll: true }))
}
function connectExisting() {
  const anchor = quickPort.value?.anchor
  closeQuickAdd(true)
  anchor?.dispatchEvent(new MouseEvent('click', { bubbles: true, shiftKey: true }))
}
function createLinkedNode(type: NodeType) {
  const port = quickPort.value, choice = quickOptions.value.find(item => item.definition.type === type)
  const origin = nodes.value.find(node => node.id === port?.id)
  if (!port || !choice || !origin || recovery.value) return
  const source = port.side === 'source', id = createUUID(), edgeId = createUUID(), definition = choice.definition
  const position = { x: origin.position.x + (source ? 368 : -368), y: origin.position.y }
  while (nodes.value.some(n => Math.abs(n.position.x - position.x) < 300 && Math.abs(n.position.y - position.y) < 180)) position.y += 204
  const node = { id, type: definition.type, version: definition.version, position, config: defaultConfig(definition.type) }
  const edge = { id: edgeId, source: source ? origin.id : id, sourcePort: source ? port.key : choice.key, target: source ? id : origin.id, targetPort: source ? choice.key : port.key }
  const graph = visualGraph()
  const invalid = graphError({ ...graph, nodes: [...graph.nodes, node], edges: [...graph.edges, edge] }, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; closeQuickAdd(true); return }
  nodes.value = [...nodes.value.map(node => ({ ...node, selected: false })), { id, type: 'pod', position, selected: true, dragHandle: '.workflow-node-title', data: { definition, config: node.config } }]
  edges.value = [...edges.value.map(edge => ({ ...edge, selected: false })), { id: edgeId, source: edge.source, sourceHandle: edge.sourcePort, target: edge.target, targetHandle: edge.targetPort, type: 'default' }]
  commitVisual()
  inspectNode(id, !narrowScreen.value)
  hint.value = '已添加节点并连线'
}
async function prepareRun(): Promise<string> {
  if (recovery.value || !session) throw new Error('请先处理本地草稿')
  await nextTick(); commitVisual()
  await save()
  if (session.dirty || session.state !== 'saved') throw new Error('画布尚未保存，先解决保存错误再执行')
  return session.revision
}
const selectedAssets = computed(() => Array.isArray(selected.value?.data.config.assetIds) ? selected.value.data.config.assetIds.filter((id): id is string => typeof id === 'string') : [])
const pickerInitial = computed(() => {
  const ids = nodes.value.find(node => node.id === pickerTarget.value)?.data.config.assetIds
  return Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string') : []
})
function chooseAssets(ids: string[]) { if (nodes.value.some(node => node.id === pickerTarget.value && node.data.definition.type === 'imageInput')) config(pickerTarget.value, 'assetIds', ids); pickerTarget.value = '' }
let session: DraftSession | null = null, history: DraftHistory | null = null, applying = false, viewportReady = false
const cacheKey = `pod-studio.draft:${props.bootstrap.userId}:${props.bootstrap.workspace.workspaceId}:${props.projectId}`
const selected = computed(() => nodes.value.find(node => node.id === selectedId.value))
const selectionCount = computed(() => nodes.value.filter(node => node.selected).length + edges.value.filter(edge => edge.selected).length)
const outputPorts = computed(() => nodes.value.find(node => node.id === sourceId.value)?.data.definition.outputs || [])
const inputPorts = computed(() => nodes.value.find(node => node.id === targetId.value)?.data.definition.inputs || [])
const saveLabel = computed(() => ({ saved: '已保存', dirty: '等待保存', saving: '正在保存…', error: '保存失败', conflict: '版本冲突' }[state.value]))
const selectedType = computed(() => selected.value?.data.definition.type)
const imageOptions = computed(() => {
  const modelId = selected.value?.data.config.modelId
  return models.value.find(model => model.kind === 'image' && (modelId ? model.modelId === modelId : model.default))
})
const referenceSizing = computed(() => ['Qwen/Qwen-Image-Edit', 'Qwen/Qwen-Image-Edit-2509'].includes(imageOptions.value?.officialModelId || ''))
const imageRatios = computed(() => imageOptions.value?.capabilities.ratios || ['1:1', '3:4', '4:3', '9:16', '16:9'])
const imageLimit = computed(() => imageOptions.value?.capabilities.maxImages || 4)
const selectedFields = computed(() => selectedType.value && selectedType.value !== 'imageGenerate' ? nodeFields[selectedType.value] : [])
const selectedRunNode = computed(() => activeRunNodes.value.find(node => node.nodeId === selectedId.value))
const actionBusy = computed(() => ['running', 'ready', 'waiting', 'awaitingReconciliation'].includes(selectedRunNode.value?.state || ''))
const actionLabel = computed(() => {
  if (actionBusy.value) return selectedRunNode.value?.state === 'awaitingReconciliation' ? (selectedRunNode.value.errorCode === 'archivePending' ? '图片待保存' : '结果待确认') : '处理中…'
  switch (selectedType.value) {
    case 'imageGenerate': return '立即生成'
    case 'textGenerate': return '执行 AI 文本'
    case 'export': return '生成导出文件'
    default: return '开始制作'
  }
})
const canPolish = computed(() => ['qwen', 'simulation', 'managed'].includes(props.bootstrap.execution?.mode || '') && !!String(selected.value?.data.config.text || '').trim())
const selectedTitle = computed(() => selected.value ? String(selected.value.data.config.title || selected.value.data.definition.label) : '')

function visualGraph(): Graph {
  return { schemaVersion: 1, nodes: nodes.value.map(node => ({ id: node.id, type: node.data.definition.type, version: node.data.definition.version, position: { x: node.position.x, y: node.position.y }, config: clone(node.data.config) })),
    edges: edges.value.map(edge => ({ id: edge.id, source: edge.source, sourcePort: edge.sourceHandle || '', target: edge.target, targetPort: edge.targetHandle || '' })) }
}
function historyFlags() { canUndo.value = history?.canUndo || false; canRedo.value = history?.canRedo || false }
function persistDraft() {
  if (!session) return
  state.value = session.state; revision.value = session.revision; dirty.value = session.dirty; error.value = session.error
  try {
    if (session.dirty) localStorage.setItem(cacheKey, JSON.stringify({ userId: props.bootstrap.userId, workspaceId: props.bootstrap.workspace.workspaceId, projectId: props.projectId, baseRevision: session.revision, document: session.document } satisfies CachedDraft))
    else if (!recovery.value) localStorage.removeItem(cacheKey)
    localWarning.value = ''
  } catch { localWarning.value = '浏览器无法保存本地草稿，请保持页面打开或下载草稿。' }
}
function report(e: unknown) { error.value = e instanceof Error ? e.message : '请求失败'; if (e instanceof APIError && e.code === 401) emit('expired') }
async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  try { return await request<T>(path, options, props.token) }
  catch (e) { if (e instanceof APIError && e.code === 401) emit('expired'); throw e }
}
async function showDocument(document: DraftDocument) {
  closeQuickAdd()
  applying = true; name.value = document.name
  nodes.value = document.graph.nodes.map(node => ({ id: node.id, type: 'pod', position: clone(node.position), dragHandle: '.workflow-node-title', data: { definition: props.bootstrap.nodeTypes.find(item => item.type === node.type)!, config: clone(node.config) } }))
  edges.value = document.graph.edges.map(edge => ({ id: edge.id, source: edge.source, sourceHandle: edge.sourcePort, target: edge.target, targetHandle: edge.targetPort, type: 'default' }))
  selectedId.value = ''; await nextTick(); applying = false
}
function beginSession(project: Project) {
  session?.dispose()
  const document = { name: project.name, graph: project.graph }
  history = new DraftHistory(document); historyFlags()
  session = new DraftSession(document, project.revision, async (submitted, version) => {
    const result = await api<{ data: Project }>(`/pod/projects/${props.projectId}`, { method: 'PUT', body: JSON.stringify({ ...submitted, revision: version }) })
    return { revision: result.data.revision }
  }, persistDraft)
  state.value = 'saved'; revision.value = project.revision; dirty.value = false; error.value = ''; viewportReady = false
}
async function load(checkCache = true) {
  loading.value = true; error.value = ''
  try {
    const result = await api<{ data: Project }>(`/pod/projects/${props.projectId}`)
    const invalid = graphError(result.data.graph, props.bootstrap.nodeTypes, props.bootstrap.limits)
    if (invalid) throw new APIError(`此项目无法编辑：${invalid}`, 400)
    beginSession(result.data); await showDocument({ name: result.data.name, graph: result.data.graph })
    if (checkCache) {
      try {
        const raw = localStorage.getItem(cacheKey)
        if (raw) {
          const cached = JSON.parse(raw) as CachedDraft
          if (cached.userId !== props.bootstrap.userId || cached.workspaceId !== props.bootstrap.workspace.workspaceId || cached.projectId !== props.projectId || typeof cached.baseRevision !== 'string' || !cached.document || typeof cached.document.name !== 'string' || graphError(cached.document.graph, props.bootstrap.nodeTypes, props.bootstrap.limits)) throw new Error('Invalid cache')
          if (stable(cached.document) !== stable(session!.document)) recovery.value = cached
          else localStorage.removeItem(cacheKey)
        }
      } catch { localWarning.value = '本地草稿读取失败，未自动覆盖服务端项目。' }
    }
  } catch (e) { report(e) } finally { loading.value = false }
}
function commitVisual(record = true) {
  if (!session || applying || recovery.value) return
  const graph = visualGraph(), invalid = graphError(graph, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; return }
  const document = { name: name.value, graph }
  session.edit(document)
  if (record) { history?.record(document); historyFlags() }
}
watch([nodes, edges], () => commitVisual(!dragging.value && !imageGesture.value), { deep: true, flush: 'post' })

function rename() {
  const trimmed = name.value.trim()
  if (!trimmed || [...trimmed].length > 100 || /[\u0000-\u001f\u007f]/.test(trimmed)) { hint.value = '项目名称须为 1 到 100 个字符'; name.value = session?.document.name || ''; return }
  name.value = trimmed; commitVisual()
}
function addNode(type: NodeType, at?: { x: number; y: number }) {
  if (recovery.value) { hint.value = '请先处理本地草稿'; return }
  if (nodes.value.length >= props.bootstrap.limits.maxNodes) { hint.value = '节点数量已到上限'; return }
  const definition = props.bootstrap.nodeTypes.find(node => node.type === type)
  if (!definition) return
  const rect = canvas.value?.getBoundingClientRect()
  const point = at || screenToFlowCoordinate({ x: rect ? rect.left + rect.width / 2 : 400, y: rect ? rect.top + rect.height / 2 : 250 })
  const id = createUUID()
  nodes.value = [...nodes.value.map(node => ({ ...node, selected: false })), { id, type: 'pod', position: point, selected: true, dragHandle: '.workflow-node-title', data: { definition, config: defaultConfig(type) } }]
  inspectNode(id, false); hint.value = ''
  return id
}
function drop(event: DragEvent) {
  const kind = event.dataTransfer?.getData('application/pod-node') as NodeType
  if (kind) addNode(kind, screenToFlowCoordinate({ x: event.clientX, y: event.clientY }))
}
function connect(connection: Connection) {
  if (recovery.value) return
  const edge = { id: createUUID(), source: connection.source, sourcePort: connection.sourceHandle || '', target: connection.target, targetPort: connection.targetHandle || '' }
  const invalid = graphError({ ...visualGraph(), edges: [...visualGraph().edges, edge] }, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; return }
  edges.value = [...edges.value, { id: edge.id, source: edge.source, sourceHandle: edge.sourcePort, target: edge.target, targetHandle: edge.targetPort, type: 'default' }]; hint.value = ''
}
function onCanvasConnect(connection: Connection) {
  if (!cancelledDrag.value) connect(connection)
}
function cancelConnection() {
  if (connectionStartHandle.value) {
    // Vue Flow may still emit connect when the pointer is released after Escape.
    cancelledDrag.value = true
    endConnection()
  }
  if (connectionClickStartHandle.value) endConnection(undefined, true)
}
function paneClicked() {
  cancelConnection()
  closeQuickAdd()
  selectedId.value = ''
  showInspector.value = false
}
function disconnect(id: string) {
  if (recovery.value) return
  edges.value = edges.value.filter(edge => edge.id !== id)
}
function config(id: string, key: string, value: unknown) { patchConfig(id, { [key]: value }) }
function patchConfig(id: string, values: Record<string, unknown>) {
  if (recovery.value) return
  nodes.value = nodes.value.map(node => node.id === id ? { ...node, data: { ...node.data, config: { ...node.data.config, ...values } } } : node)
}
function updateField(field: ConfigField, event: Event) {
  const input = event.target as HTMLInputElement
  if (!selected.value) return
  if (field.type === 'number' && (!input.value || !input.checkValidity())) { hint.value = `${field.label} 的数值超出允许范围`; return }
  const value = field.type === 'number' || field.key === 'scale' ? Number(input.value) : input.value
  config(selected.value.id, field.key, value); hint.value = ''
}
async function replaceGraph(graph: Graph, selection: string[] = []) {
  if (!session) return
  const document = { name: name.value, graph }
  await showDocument(document)
  nodes.value = nodes.value.map(node => ({ ...node, selected: selection.includes(node.id) }))
  selectedId.value = selection.at(-1) || ''
  session.edit(document); history?.record(document); historyFlags()
}
function duplicate() {
  const result = duplicateSelection(visualGraph(), nodes.value.filter(node => node.selected).map(node => node.id))
  const invalid = graphError(result.graph, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; return }; void replaceGraph(result.graph, result.selected)
}
function duplicateNode(id: string) {
  if (recovery.value) return
  const result = duplicateSelection(visualGraph(), [id])
  const invalid = graphError(result.graph, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; return }
  void replaceGraph(result.graph, result.selected)
}
function removeNode(id: string) { if (!recovery.value) void replaceGraph(removeSelection(visualGraph(), [id], [])) }
function startNodeAction(id: string, mode: 'polish' | 'branch') {
  if (recovery.value || state.value === 'conflict') { hint.value = '请先解决草稿或保存冲突'; return }
  const node = nodes.value.find(node => node.id === id)
  if (!node) return
  if (mode === 'polish' && (node.data.definition.type !== 'textInput' || !String(node.data.config.text || '').trim())) { hint.value = '请先填写需要润色的文字'; return }
  selectedId.value = id; nodeAction.value = { id, mode }
}
function applyPolish(text: string, original: string) {
  const node = actionNode.value
  if (!node || node.data.definition.type !== 'textInput' || recovery.value || String(node.data.config.text || '') !== original) { hint.value = '节点或原文已变化，未覆盖当前内容'; return }
  config(node.id, 'text', text); hint.value = '已替换为润色结果，可用撤销恢复原文'
}
function insertPrompt(text: string) {
  if (recovery.value) return
  if (selected.value?.data.definition.type === 'textInput') {
    if (String(selected.value.data.config.text || '').trim() && !window.confirm('使用这条提示词替换当前文本？可通过撤销恢复原文。')) return
    config(selected.value.id, 'text', text)
  } else { const id = addNode('textInput'); if (!id) return; config(id, 'text', text) }
  library.value = ''
}
async function insertTemplate(graph: Graph) {
  if (recovery.value) return
  const current = visualGraph(), ids = new Map(graph.nodes.map(node => [node.id, createUUID()]))
  const offset = current.nodes.length ? Math.max(...current.nodes.map(node => node.position.y)) + 300 : 100
  const additions = graph.nodes.map(node => ({ ...clone(node), id: ids.get(node.id)!, position: { x: node.position.x + 140, y: node.position.y + offset } }))
  const next = { schemaVersion: 1 as const, nodes: [...current.nodes, ...additions], edges: [...current.edges, ...graph.edges.map(edge => ({ ...edge, id: createUUID(), source: ids.get(edge.source)!, target: ids.get(edge.target)! }))] }
  const invalid = graphError(next, props.bootstrap.nodeTypes, props.bootstrap.limits)
  if (invalid) { hint.value = invalid; return }
  await replaceGraph(next, additions.map(node => node.id)); library.value = ''; void nextTick(() => fitView(fitOptions.value))
}
function remove() { void replaceGraph(removeSelection(visualGraph(), nodes.value.filter(node => node.selected).map(node => node.id), edges.value.filter(edge => edge.selected).map(edge => edge.id))) }
async function travel(redo = false) {
  const document = redo ? history?.redo() : history?.undo()
  if (document && session) { await showDocument(document); session.edit(document); historyFlags() }
}
async function save() { rename(); await session?.flush() }
async function openTemplateLibrary() {
  try { await prepareRun(); showTemplateLibrary.value = true }
  catch (e) { hint.value = (e as Error).message }
}
async function close() {
  if (!recovery.value) await save()
  if ((session?.dirty || recovery.value) && !window.confirm(localWarning.value ? '还有未保存修改，浏览器也无法保存草稿。建议先下载草稿。仍然离开？' : '还有未保存修改。它们保留在此浏览器，重新打开项目可恢复。现在离开？')) return
  emit('close')
}
async function recoverLocal() {
  const cached = recovery.value
  if (!cached || !session) return
  recovery.value = null; await showDocument(cached.document)
  session.recover(cached.document, cached.baseRevision !== session.revision); history?.record(cached.document); historyFlags()
}
async function reload() {
  if ((session?.dirty || recovery.value) && !window.confirm('重新加载会放弃此浏览器保存的本地修改。需要保留时，请先下载草稿或另存为项目。确定放弃并加载服务端版本？')) return
  try {
    const result = await api<{ data: Project }>(`/pod/projects/${props.projectId}`)
    recovery.value = null; beginSession(result.data); await showDocument({ name: result.data.name, graph: result.data.graph }); persistDraft()
  } catch (e) { report(e) }
}
function download() {
  const document = recovery.value?.document || session?.document
  if (!document) return
  const url = URL.createObjectURL(new Blob([JSON.stringify(document, null, 2)], { type: 'application/json' }))
  const link = window.document.createElement('a'); link.href = url; link.download = `${document.name.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_') || '工作流'}-草稿.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function saveAs() {
  const document = recovery.value?.document || session?.document
  if (!document || busy.value) return
  busy.value = true
  try {
    const copyName = [...document.name].slice(0, 95).join('') + ' 副本'
    const created = await api<{ data: Project }>('/pod/projects', { method: 'POST', body: JSON.stringify({ name: copyName }) })
    const saved = await api<{ data: Project }>(`/pod/projects/${created.data.projectId}`, { method: 'PUT', body: JSON.stringify({ name: copyName, graph: document.graph, revision: created.data.revision }) })
    try { localStorage.removeItem(cacheKey) } catch { /* The original draft can remain if storage is unavailable. */ }
    session?.dispose(); emit('open', saved.data.projectId)
  } catch (e) { report(e) } finally { busy.value = false }
}
function beforeUnload(event: BeforeUnloadEvent) { if (session?.dirty || recovery.value) { event.preventDefault(); event.returnValue = '' } }
function keyboard(event: KeyboardEvent) {
  if (loading.value || recovery.value || document.querySelector('dialog[open]')) return
  if (event.key === 'Escape') {
    if (connectionInProgress.value) { event.preventDefault(); cancelConnection() }
    else if (quickPort.value) { event.preventDefault(); closeQuickAdd(true) }
    else closeTools()
    return
  }
  const modifier = event.ctrlKey || event.metaKey
  if (modifier && event.key.toLowerCase() === 's') { event.preventDefault(); void save() }
  else if (event.target instanceof HTMLElement && event.target.closest('input,textarea,select,[contenteditable="true"]')) return
  else if (modifier && event.key.toLowerCase() === 'z') { event.preventDefault(); void travel(event.shiftKey) }
  else if (modifier && event.key.toLowerCase() === 'y') { event.preventDefault(); void travel(true) }
  else if (modifier && event.key.toLowerCase() === 'd') { event.preventDefault(); duplicate() }
  else if (modifier && event.key.toLowerCase() === 'a') { event.preventDefault(); nodes.value = nodes.value.map(node => ({ ...node, selected: true })) }
  else if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); remove() }
}
function online() { if (session?.state === 'error') void session.flush() }
function initializeViewport() {
  if (viewportReady) return
  viewportReady = true
  try { const stored = JSON.parse(localStorage.getItem(cacheKey + ':viewport') || 'null') as { x: number; y: number; zoom: number } | null
    if (stored && [stored.x, stored.y, stored.zoom].every(Number.isFinite) && Math.abs(stored.x) <= 1e6 && Math.abs(stored.y) <= 1e6 && stored.zoom >= 0.2 && stored.zoom <= 2) { void setViewport(stored); return }
  } catch { /* Missing viewport does not affect the saved graph. */ }
  void fitView(fitOptions.value)
}
function rememberViewport(viewport: { x: number; y: number; zoom: number }) { try { localStorage.setItem(cacheKey + ':viewport', JSON.stringify(viewport)) } catch { /* Viewport is optional. */ } }
let screenQuery: MediaQueryList | undefined
function screenChanged() {
  closeQuickAdd()
  narrowScreen.value = screenQuery?.matches || false
  showMoreTools.value = false
  if (narrowScreen.value) { showPalette.value = false; showInspector.value = false; showRuns.value = false }
}
onMounted(() => { screenQuery = window.matchMedia('(max-width: 900px), (max-height: 500px) and (max-width: 1000px)'); screenChanged(); screenQuery.addEventListener('change', screenChanged); void load(); window.addEventListener('keydown', keyboard); window.addEventListener('beforeunload', beforeUnload); window.addEventListener('online', online) })
onBeforeUnmount(() => { session?.dispose(); cancelAnimationFrame(revealFrame); cancelAnimationFrame(inspectorFrame); inspectorObserver?.disconnect(); screenQuery?.removeEventListener('change', screenChanged); window.removeEventListener('keydown', keyboard); window.removeEventListener('beforeunload', beforeUnload); window.removeEventListener('online', online) })
</script>

<template>
  <div class="project-editor" :class="{ 'canvas-motion-off': !motionEnabled || reducedMotion, 'canvas-motion-paused': dragging || panning || documentHidden, 'canvas-motion-dense': edges.length > 80 }">
    <header class="editor-header">
      <div class="editor-workspace-bar">
      <button class="icon-button editor-back" aria-label="返回项目列表" :disabled="busy" @click="close"><PhArrowLeft :size="20" /></button>
      <StudioBrand class="canvas-brand" />
      <div class="editor-project-name"><label for="editor-name" class="sr-only">项目名称</label><input id="editor-name" v-model="name" maxlength="100" :disabled="loading || !!recovery" @change="rename" /></div>
      <span class="save-indicator" :class="`save-${state}`" role="status">{{ saveLabel }}<small v-if="revision">v{{ revision }}</small></span>
      </div>
      <div class="editor-actions-bar">
      <ThemeSwitch />
      <span class="editor-mode"><i />{{ executionLabel }}</span>
      <div class="editor-header-actions">
        <button class="secondary-button" aria-label="保存" :disabled="loading || busy || state === 'saving' || !!recovery || state === 'conflict'" @click="save"><PhFloppyDisk :size="16" /><span>保存</span></button>
        <button class="secondary-button" :disabled="loading || busy || !!recovery || state === 'conflict'" @click="showTemplateSave = true"><PhSquaresFour :size="16" /><span>保存为模板</span></button>
        <button v-if="bootstrap.execution?.mode && bootstrap.execution.mode !== 'disabled'" class="secondary-button editor-results" type="button" :disabled="loading || !session" aria-label="查看项目结果" title="查看项目结果" @click="showResults = true"><PhImage :size="16" /><span>结果</span></button>
        <button class="primary-button editor-execute" :disabled="loading || !!recovery" :aria-pressed="showRuns" aria-label="打开或关闭执行面板" @click="togglePanel('runs')"><PhPlay :size="16" weight="fill" /><span>{{ ['qwen', 'managed'].includes(bootstrap.execution?.mode || '') ? '执行流程' : '测试执行' }}</span></button>
      </div>
      </div>
    </header>
    <div class="editor-notices">
    <div v-if="recovery" class="editor-alert"><PhWarningCircle :size="18" /><span>发现此浏览器的未保存草稿。{{ recovery.baseRevision === revision ? '可以继续编辑。' : '服务端已有更新，恢复后不会自动覆盖。' }}</span><button class="text-button" @click="recoverLocal">恢复草稿</button><button class="text-button" @click="reload">使用服务端版本</button><button class="text-button" @click="download">下载草稿</button></div>
    <div v-if="error || localWarning" class="editor-alert error" role="alert"><PhWarningCircle :size="18" /><span>{{ error || localWarning }}</span><button v-if="state === 'error'" class="text-button" @click="save">重试保存</button><button v-if="session" class="text-button" @click="reload">重新加载</button><button v-if="session" class="text-button" @click="download">下载草稿</button><button v-if="state === 'conflict'" class="text-button" :disabled="busy" @click="saveAs">另存为新项目</button><button v-if="!session" class="text-button" @click="load()">重试读取</button></div>
    <div v-if="runError" class="editor-alert error" role="alert"><span>运行状态：{{ runError }}</span><button class="text-button" @click="refreshRun">重新读取运行</button></div>
    </div>
    <div v-if="loading" class="editor-loading" role="status">正在打开画布…</div>
    <div v-else-if="session" class="editor-body">
      <button v-if="narrowScreen && (showPalette || showInspector || showRuns)" class="editor-panel-backdrop" aria-label="关闭工具面板" @click="closeTools" />
      <aside v-if="showPalette" class="node-palette" aria-label="添加节点">
        <div class="panel-heading"><h2><PhPlus :size="17" />节点工具</h2><button class="icon-button palette-close" aria-label="收起节点工具" @click="showPalette = false"><PhX :size="17" /></button></div>
        <label class="palette-search"><PhMagnifyingGlass :size="15" /><input v-model="paletteQuery" aria-label="搜索节点" placeholder="搜索节点" type="search" /></label>
        <button v-for="definition in paletteNodes" :key="definition.type" class="palette-node" :class="`kind-${definition.type}`" draggable="true" :disabled="!!recovery" :title="definition.description" @click="addNode(definition.type)" @dragstart="event => event.dataTransfer?.setData('application/pod-node', definition.type)">
          <span class="node-icon"><component :is="nodeIcons[definition.type]" :size="19" weight="duotone" /></span><span><strong>{{ definition.label }}</strong><small>{{ nodeCategories[definition.type] }}{{ definition.executable ? '' : ' · 仅配置' }}</small></span><PhPlus class="palette-add" :size="15" />
        </button>
        <p v-if="!paletteNodes.length" class="palette-empty">没有匹配的节点，试试其他名称或展开进阶节点。</p>
        <label class="palette-advanced"><input v-model="showAdvanced" type="checkbox" />显示进阶及仅配置节点</label>
        <div class="palette-note"><span>点击添加，也可拖入画布</span><span><kbd>Shift</kbd> 框选多个节点</span></div>
      </aside>
      <main ref="canvas" class="flow-canvas" :class="{ 'has-inspector': showInspector || showRuns }" aria-label="工作流画布" @dragover.prevent @drop.prevent="drop">
        <div class="canvas-toolbar canvas-dock" role="toolbar" aria-label="画布工具">
          <button class="icon-button" :aria-pressed="showPalette" aria-label="切换节点工具" title="节点工具" @click="togglePanel('palette')"><PhPlus :size="20" /><span class="mobile-tool-label">节点</span></button><span />
          <button class="icon-button" aria-label="打开个人模板库" title="我的模板" :disabled="loading || !!recovery" @click="openTemplateLibrary"><PhSquaresFour :size="20" /><span class="mobile-tool-label">模板</span></button>
          <button class="icon-button" :disabled="!canUndo || !!recovery" aria-label="撤销 Ctrl+Z" title="撤销 Ctrl+Z" @click="travel()"><PhArrowCounterClockwise :size="19" /><span class="mobile-tool-label">撤销</span></button>
          <button class="icon-button" :aria-pressed="showInspector" aria-label="切换参数面板" title="节点参数" @click="togglePanel('inspector')"><PhSidebarSimple :size="19" /><span class="mobile-tool-label">参数</span></button>
          <div :id="`${flowId}-tools`" class="canvas-tool-overflow" :class="{ 'is-open': showMoreTools }" role="group" aria-label="辅助画布工具" @click="showMoreTools = false">
            <button class="icon-button" aria-label="打开工作流预设" title="工作流预设" data-mobile-label="预设" @click="library = 'templates'"><PhCopy :size="20" /></button><button class="icon-button" aria-label="打开提示词灵感库" title="提示词灵感库" data-mobile-label="灵感" @click="library = 'prompts'"><PhLightbulb :size="20" /></button><span />
            <button class="icon-button" aria-label="快捷添加文本输入" title="文本输入" data-mobile-label="文本" :disabled="!!recovery" @click="addNode('textInput')"><PhTextT :size="20" /></button><button class="icon-button" aria-label="快捷添加图片输入" title="图片输入" data-mobile-label="图片" :disabled="!!recovery" @click="addNode('imageInput')"><PhImage :size="20" /></button><button class="icon-button" aria-label="快捷添加生图节点" title="AI 生图" data-mobile-label="生图" :disabled="!!recovery" @click="addNode('imageGenerate')"><PhSparkle :size="20" /></button><span />
            <button class="icon-button" :disabled="!canRedo || !!recovery" aria-label="重做 Ctrl+Shift+Z" title="重做 Ctrl+Shift+Z" data-mobile-label="重做" @click="travel(true)"><PhArrowClockwise :size="19" /></button>
            <button class="icon-button" :disabled="!selectionCount || !!recovery" aria-label="复制选中节点 Ctrl+D" title="复制选中节点 Ctrl+D" data-mobile-label="复制" @click="duplicate"><PhCopy :size="19" /></button><button class="icon-button" :disabled="!selectionCount || !!recovery" aria-label="删除选中节点或连线 Delete" title="删除选中项 Delete" data-mobile-label="删除" @click="remove"><PhTrash :size="19" /></button><span />
            <button class="icon-button" aria-label="打开手动连线" title="手动连线" data-mobile-label="连线" :disabled="!!recovery" @click="openConnections"><PhLink :size="19" /></button>
            <button class="icon-button" :aria-pressed="showMiniMap" aria-label="切换画布缩略图" title="画布缩略图" data-mobile-label="缩略图" @click="showMiniMap = !showMiniMap"><PhMapTrifold :size="19" /></button>
            <button class="icon-button" :aria-pressed="motionEnabled && !reducedMotion" aria-label="切换画布动效" :title="reducedMotion ? '跟随系统减少动态效果' : motionEnabled ? '暂停画布动效' : '开启动效'" data-mobile-label="动效" :disabled="reducedMotion" @click="toggleMotion"><PhWaveform :size="19" /></button>
          </div>
          <button class="icon-button canvas-more-toggle" :aria-expanded="showMoreTools" :aria-controls="`${flowId}-tools`" aria-label="更多画布工具" @click="toggleMoreTools"><PhDotsThree :size="22" /><span class="mobile-tool-label">更多</span></button>
        </div>
        <p class="canvas-touch-guide">拖动空白处平移 · 双指缩放 · 点节点打开参数</p>
        <!-- Snap only during a drag: mounting must preserve saved non-grid coordinates. -->
        <VueFlow :id="flowId" v-model:nodes="nodes" v-model:edges="edges" :class="{ 'canvas-connecting': connectionInProgress }" :min-zoom="0.2" :max-zoom="2" :delete-key-code="null" :multi-selection-key-code="['Meta', 'Control']" :nodes-draggable="!recovery" :nodes-connectable="!recovery" :elements-selectable="!recovery" :zoom-on-pinch="true" :connect-on-click="true" :auto-pan-on-connect="!cancelledDrag" :connection-radius="28" :snap-to-grid="dragging" :snap-grid="[12, 12]" @connect="onCanvasConnect" @connect-start="cancelledDrag = false" @connect-end="cancelledDrag = false" @node-click="event => selectCanvasNode(event.node.id, event.event)" @pane-click="paneClicked" @node-drag-start="dragging = true" @selection-drag-start="dragging = true" @node-drag-stop="dragging = false; commitVisual()" @selection-drag-stop="dragging = false; commitVisual()" @nodes-initialized="initializeViewport" @move-start="panning = true" @move-end="event => { panning = false; rememberViewport(event.flowTransform) }">
          <template #edge-default="edge"><WorkflowEdge v-bind="edge" :zoom="viewport.zoom" :removable="!recovery" :highlighted="edge.source === selectedId || edge.target === selectedId" @remove="disconnect" /></template>
          <Background :gap="24" :size="1" color="var(--canvas-dot)" />
          <Controls :show-interactive="false" :fit-view-params="fitOptions">
            <template #icon-zoom-in><PhPlus /><span class="sr-only">放大画布</span></template>
            <template #icon-zoom-out><PhMinus /><span class="sr-only">缩小画布</span></template>
            <template #icon-fit-view><PhCornersOut /><span class="sr-only">适应画布</span></template>
          </Controls>
          <MiniMap v-if="showMiniMap" class="canvas-minimap" :pannable="true" :zoomable="true" node-color="#8199e2" />
          <template #node-pod="node"><WorkflowNode :handle-motion="motionEnabled && !reducedMotion && !documentHidden && !dragging && !panning && !connectionInProgress" :id="node.id" :data="node.data" :token="token" :selected="node.selected || node.id === selectedId" :disabled="!!recovery" :existing-connection="!!connectionClickStartHandle" :quick-port="quickPort?.id === node.id ? `${quickPort.side}:${quickPort.key}` : undefined" :run-node="activeRunNodes.find(item => item.nodeId === node.id)" :run-revision="activeRun?.projectRevision" :image-result="imageResult(node.id, node.data.definition.type)" :current-revision="revision" :draft-changed="dirty" @text="value => config(node.id, 'text', value)" @choose="!recovery && (pickerTarget = node.id)" @copy="duplicateNode(node.id)" @remove="removeNode(node.id)" @inspect="inspectNode(node.id)" @quick-add="(side, key, anchor) => openQuickAdd(node.id, side, key, anchor)" @expired="emit('expired')" /></template>
          <div v-if="!nodes.length" class="blank-canvas nodrag nopan"><div class="blank-node-chain" aria-hidden="true"><span><PhTextT :size="24" /></span><i /><span><PhSparkle :size="27" weight="duotone" /></span><i /><span><PhImage :size="24" /></span></div><h1>从一条想法开始</h1><p>把文字、参考图和生成节点连起来，<br>让每一次灵感都有自己的创作路径。</p><button class="primary-button" :disabled="!!recovery" @click="addNode('textInput')"><PhPlus :size="17" />添加文本节点</button></div>
        </VueFlow>
        <section v-if="quickPort" ref="portMenu" class="quick-port-menu" :style="{ left: `${quickPort.left}px`, top: `${quickPort.top}px`, maxHeight: `calc(100% - ${quickPort.top + 12}px)` }" aria-label="快捷添加并连线">
          <header><strong>{{ quickPort.side === 'source' ? '添加下游节点' : '添加上游节点' }}</strong><button class="icon-button" type="button" aria-label="关闭快捷添加" @click="closeQuickAdd(true)"><PhX :size="16" /></button></header>
          <div class="quick-port-options">
            <button v-for="choice in quickOptions" :key="choice.definition.type" type="button" data-primary :data-node-type="choice.definition.type" @click="createLinkedNode(choice.definition.type)"><component :is="nodeIcons[choice.definition.type]" :size="17" /><span>{{ choice.definition.label }}</span><PhPlus :size="15" /></button>
            <p v-if="!quickOptions.length">没有可添加的节点，或此输入已连接。</p>
          </div>
          <button class="text-button quick-port-existing" type="button" :data-primary="!quickOptions.length ? '' : undefined" @click="connectExisting"><PhLink :size="15" />连接已有节点</button>
        </section>
        <div v-if="connectionInProgress" class="canvas-hint connection-hint"><span role="status">选择另一个端口完成连线</span><button class="text-button" type="button" @click="cancelConnection">取消连线</button></div>
        <p v-else-if="hint" class="canvas-hint" role="status">{{ hint }}<button class="icon-button" aria-label="关闭画布提示" @click="hint = ''"><PhX :size="15" /></button></p>
        <div class="canvas-status"><PhBezierCurve :size="16" /><span>{{ nodes.length }} / {{ bootstrap.limits.maxNodes }} 节点</span><i /><span>{{ edges.length }} 连线</span></div>
      </main>
      <aside v-if="showInspector && !showRuns" class="editor-inspector compact-inspector" :class="{ 'node-anchored-inspector': inspectorPosition }" :style="inspectorPosition" aria-label="节点参数"><div class="panel-heading"><div><small>{{ selected ? '节点参数' : '画布' }}</small><h2><component :is="selected ? nodeIcons[selected.data.definition.type] : PhSidebarSimple" :size="17" />{{ selected ? selectedTitle : '流程设置' }}</h2></div><button class="icon-button" aria-label="收起参数面板" @click="showInspector = false"><PhX :size="17" /></button></div>
        <div class="inspector-scroll" tabindex="0" aria-label="参数内容">
        <div v-if="selected" class="inspector-fields"><div class="field inspector-wide"><label for="node-title">节点名称</label><input id="node-title" :value="String(selected.data.config.title || '')" :placeholder="selected.data.definition.label" maxlength="60" :disabled="!!recovery" @input="config(selected.id, 'title', ($event.target as HTMLInputElement).value)" /></div>
          <p v-if="modelError" class="error-message" role="alert">{{ modelError }}</p><ModelSelect v-if="bootstrap.execution?.mode === 'managed' && ['textInput', 'textGenerate', 'imageGenerate'].includes(selected.data.definition.type)" :models="models" :kind="selected.data.definition.type === 'imageGenerate' ? 'image' : 'text'" :value="selected.data.config[selected.data.definition.type === 'textInput' ? 'polishModelId' : 'modelId']" :disabled="!!recovery" :label="selected.data.definition.type === 'textInput' ? '润色模型' : '生成模型'" @change="id => config(selected!.id, selected!.data.definition.type === 'textInput' ? 'polishModelId' : 'modelId', id)" /><ImageSettings v-if="localTypes.includes(selected.data.definition.type)" :key="selected.id" :run-node="activeRun?.nodes?.find(n => n.nodeId === selected!.id)" :run-revision="activeRun?.projectRevision" :run-method="String(activeRun?.graph?.nodes.find(n => n.id === selected!.id)?.config.method || 'basic')" :type="selected.data.definition.type" :config="selected.data.config" :token="token" :input-id="inputAssetId" :disabled="!!recovery" :ai-enabled="bootstrap.execution?.imageAIEnabled" @config="(key, value) => config(selected!.id, key, value)" @patch="values => patchConfig(selected!.id, values)" @gesture="active => { imageGesture = active; if (!active) commitVisual() }" @expired="emit('expired')" /><div v-for="field in localTypes.includes(selected.data.definition.type) ? [] : selectedFields" :key="`${selected.id}-${field.key}`" class="field"><label :for="`field-${field.key}`">{{ field.label }}</label><textarea v-if="field.type === 'textarea'" :id="`field-${field.key}`" :value="String(selected.data.config[field.key] || '')" :maxlength="field.max" :disabled="!!recovery" rows="5" @input="updateField(field, $event)" /><select v-else-if="field.type === 'select'" :id="`field-${field.key}`" :value="selected.data.config[field.key] ?? field.fallback" :disabled="!!recovery" @change="updateField(field, $event)"><option v-for="option in field.options" :key="option.value" :value="option.value">{{ option.label }}</option></select><input v-else :id="`field-${field.key}`" :type="field.type" :value="selected.data.config[field.key] ?? field.fallback ?? ''" :min="field.min" :max="field.max" :step="field.step" :maxlength="field.max" :disabled="!!recovery" @change="updateField(field, $event)" /></div>
          <template v-if="selectedType === 'imageGenerate'">
            <p v-if="referenceSizing" class="inspector-future inspector-wide">需要参考图，输出沿用参考图尺寸。</p>
            <div v-else class="field"><label for="node-image-ratio">画面比例</label><select id="node-image-ratio" :value="selected.data.config.ratio || '1:1'" :disabled="!!recovery" @change="config(selected.id, 'ratio', ($event.target as HTMLSelectElement).value)"><option v-if="selected.data.config.ratio && !imageRatios.includes(String(selected.data.config.ratio))" :value="String(selected.data.config.ratio)" disabled>{{ selected.data.config.ratio }}（模型不支持）</option><option v-for="ratio in imageRatios" :key="ratio">{{ ratio }}</option></select></div>
            <div class="field"><label for="node-image-count">生成数量</label><select id="node-image-count" :value="selected.data.config.count || 1" :disabled="!!recovery" @change="config(selected.id, 'count', Number(($event.target as HTMLSelectElement).value))"><option v-if="Number(selected.data.config.count) > imageLimit" :value="Number(selected.data.config.count)" disabled>{{ selected.data.config.count }} 张（模型不支持）</option><option v-for="count in imageLimit" :key="count" :value="count">{{ count }} 张</option></select></div>
            <div class="field inspector-wide"><label for="node-image-prompt">补充提示词（选填）</label><textarea id="node-image-prompt" :value="String(selected.data.config.prompt || '')" :disabled="!!recovery" maxlength="1200" rows="3" @input="config(selected.id, 'prompt', ($event.target as HTMLTextAreaElement).value)" /></div>
          </template>
          <div v-if="selectedType === 'imageInput'" class="inspector-assets"><button class="secondary-button" :disabled="!!recovery" @click="pickerTarget = selected.id"><PhPlus :size="17" />选择或上传图片</button><div v-for="(id, index) in selectedAssets" :key="id" class="inspector-asset"><PrivateImage :asset-id="id" :token="token" :alt="`已选图片 ${index + 1}`" @expired="emit('expired')" /><span>图片 {{ index + 1 }}</span><button class="icon-button" :aria-label="`移除图片 ${index + 1}`" :disabled="!!recovery" @click="config(selected.id, 'assetIds', selectedAssets.filter(value => value !== id))"><PhX :size="16" /></button></div><p class="inspector-future">仅引用本人素材，最多 {{ bootstrap.assetLimits.maxImagesPerNode }} 张。原图不会随节点删除。</p></div>
          <p v-else-if="selected.data.definition.type === 'textGenerate' && bootstrap.execution?.mode !== 'managed'" class="inspector-future">{{ bootstrap.execution?.mode === 'qwen' ? `使用 ${bootstrap.execution.textModel}，当前仅支持文字输入。` : '可用测试执行器验证流程。输出带测试标记，不是真实 AI 结果。' }}</p>
          <p v-else-if="selected.data.definition.type === 'imageGenerate' && bootstrap.execution?.mode === 'qwen'" class="inspector-future">使用 {{ bootstrap.execution.imageModel }}；参考图最多 3 张，输出最多 4 张。组合后的提示词请控制在 1200 字内。生成图片保存在素材库。</p>
        </div>
        <p v-else class="inspector-description">选中画布上的节点即可编辑参数。</p>
        <details ref="manualConnection" class="manual-connection" :open="manualConnectionOpen" @toggle="manualConnectionOpen = ($event.target as HTMLDetailsElement).open"><summary><PhLink :size="16" />连接节点</summary><p>也可以使用表单添加连线。</p><form @submit.prevent="connect({ source: sourceId, sourceHandle: sourcePort, target: targetId, targetHandle: targetPort })"><label>来源节点<select v-model="sourceId" required @change="sourcePort = outputPorts[0]?.key || ''"><option value="" disabled>请选择</option><option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.data.config.title || node.data.definition.label }} ({{ node.id.slice(0, 4) }})</option></select></label><label>输出端口<select v-model="sourcePort" required><option v-for="port in outputPorts" :key="port.key" :value="port.key">{{ port.key }}</option></select></label><label>目标节点<select v-model="targetId" required @change="targetPort = inputPorts[0]?.key || ''"><option value="" disabled>请选择</option><option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.data.config.title || node.data.definition.label }} ({{ node.id.slice(0, 4) }})</option></select></label><label>输入端口<select v-model="targetPort" required><option v-for="port in inputPorts" :key="port.key" :value="port.key">{{ port.key }}</option></select></label><button class="secondary-button" :disabled="!!recovery">添加连线</button></form></details>
        <div v-if="!selected" class="editor-local-tools"><button class="text-button" @click="download"><PhDownloadSimple :size="16" />下载流程草稿</button><button class="text-button" :disabled="busy" @click="saveAs"><PhCopy :size="16" />另存为新项目</button></div>
        </div>
        <div v-if="selected && selectedType !== 'imageInput'" class="inspector-node-actions">
          <button v-if="selectedType === 'textInput'" class="primary-button" :disabled="!!recovery || !canPolish" @click="startNodeAction(selected.id, 'polish')"><PhMagicWand :size="16" />AI 润色</button>
          <button v-else class="primary-button" :disabled="!!recovery || !selected.data.definition.executable || actionBusy" @click="startNodeAction(selected.id, 'branch')"><PhPlay :size="16" weight="fill" />{{ actionLabel }}</button>
        </div>
      </aside>
      <aside v-if="showRuns" class="editor-runs" aria-label="执行面板"><Runs :graph="visualGraph()" :scope="`${bootstrap.userId}:${bootstrap.workspace.workspaceId}`" :token="token" :execution="bootstrap.execution" :project-id="projectId" :selected-node="selectedId" :initial-run-id="selectedRunId" :prepare="prepareRun" compact @close="showRuns = false" @selected="selectRun" @unavailable="forgetRun" @expired="emit('expired')" /></aside>
    </div>
    <AssetPicker v-if="pickerTarget" :key="pickerTarget" :token="token" :limits="bootstrap.assetLimits" :initial="pickerInitial" @close="pickerTarget = ''" @confirm="chooseAssets" @expired="emit('expired')" />
    <NodeAction :graph="visualGraph()" v-if="nodeAction && actionNode" auto-start :key="`${nodeAction.mode}:${nodeAction.id}`" :token="token" :scope="`${bootstrap.userId}:${bootstrap.workspace.workspaceId}`" :project-id="projectId" :node-id="nodeAction.id" :node-type="actionNode.data.definition.type" :action="nodeAction.mode" :source-text="String(actionNode.data.config.text || '')" :execution="bootstrap.execution" :prepare="prepareRun" @close="nodeAction = null" @apply="applyPolish" @selected="selectRun" @unavailable="forgetRun" @expired="emit('expired')" />
    <ProjectResults v-if="showResults" :token="token" :project-id="projectId" :revision="revision" :initial-run="activeRun" @close="showResults = false" @expired="emit('expired')" />
    <CreativeLibrary v-if="library" :initial-tab="library" :disabled="!!recovery" :scope="bootstrap.userId + ':' + bootstrap.workspace.workspaceId" @close="library = ''" @prompt="insertPrompt" @template="insertTemplate" />
    <TemplateSaveDialog v-if="showTemplateSave" :token="token" :scope="`${bootstrap.userId}:${bootstrap.workspace.workspaceId}`" :project-id="projectId" :project-name="name" :prepare="prepareRun" @close="showTemplateSave = false" @saved="row => { showTemplateSave = false; hint = `已保存模板「${row.name}」v${row.snapshot?.version || row.currentVersion}` }" @expired="emit('expired')" />
    <TemplateLibraryDialog v-if="showTemplateLibrary" :token="token" :bootstrap="bootstrap" @close="showTemplateLibrary = false" @open="id => emit('open', id)" @expired="emit('expired')" />
  </div>
</template>
