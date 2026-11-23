import { createUUID } from './uuid'
import type { DraftDocument, Graph, GraphLimits, NodeDefinition } from './contracts'

export const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

// MySQL JSON can reorder object keys; key order must not create a dirty draft.
export function stable(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`
  if (value && typeof value === 'object') {
    const object = value as Record<string, unknown>
    return `{${Object.keys(object).sort().map(key => `${JSON.stringify(key)}:${stable(object[key])}`).join(',')}}`
  }
  return JSON.stringify(value)
}

export function graphError(graph: Graph, definitions: NodeDefinition[], limits: GraphLimits): string {
  if (graph.schemaVersion !== 1 || !Array.isArray(graph.nodes) || !Array.isArray(graph.edges)) return '画布版本或结构不正确'
  if (graph.nodes.length > limits.maxNodes || graph.edges.length > limits.maxEdges) return '节点或连线数量超过上限'
  if (new TextEncoder().encode(JSON.stringify(graph)).length > limits.maxGraphBytes) return '画布超过保存大小限制'
  const nodes = new Map<string, NodeDefinition>()
  for (const node of graph.nodes) {
    const definition = definitions.find(item => item.type === node.type && item.version === node.version)
    if (!definition || nodes.has(node.id) || !/^[A-Za-z0-9_-]{1,64}$/.test(node.id)) return '节点类型、版本或 ID 不正确'
    if (![node.position.x, node.position.y].every(value => Number.isFinite(value) && Math.abs(value) <= 1e6)) return '节点位置超出范围'
    if (!node.config || Array.isArray(node.config) || typeof node.config !== 'object') return '节点配置必须是对象'
    nodes.set(node.id, definition)
  }
  const ids = new Set<string>(), targets = new Set<string>()
  const next = new Map<string, string[]>(), degrees = new Map(graph.nodes.map(node => [node.id, 0]))
  for (const edge of graph.edges) {
    const source = nodes.get(edge.source), target = nodes.get(edge.target)
    if (!/^[A-Za-z0-9_-]{1,64}$/.test(edge.id) || ids.has(edge.id)) return '连线 ID 不正确或重复'
    if (!source || !target || edge.source === edge.target) return '不能连接自身或不存在的节点'
    const output = source.outputs.find(port => port.key === edge.sourcePort)
    const input = target.inputs.find(port => port.key === edge.targetPort)
    if (!output || !input || output.valueType !== input.valueType) return '端口类型不兼容，请连接同类型的端口'
    const key = `${edge.target}:${edge.targetPort}`
    if (targets.has(key)) return '这个输入端口已有上游，请先删除原连线'
    targets.add(key); ids.add(edge.id)
    next.set(edge.source, [...(next.get(edge.source) || []), edge.target])
    degrees.set(edge.target, degrees.get(edge.target)! + 1)
  }
  const queue = [...degrees].filter(([, degree]) => degree === 0).map(([id]) => id)
  for (let index = 0; index < queue.length; index++) {
    for (const id of next.get(queue[index]!) || []) {
      degrees.set(id, degrees.get(id)! - 1)
      if (degrees.get(id) === 0) queue.push(id)
    }
  }
  return queue.length === graph.nodes.length ? '' : '这条连线会形成循环，无法添加'
}

export function removeSelection(graph: Graph, nodeIds: string[], edgeIds: string[]): Graph {
  return { ...clone(graph), nodes: graph.nodes.filter(node => !nodeIds.includes(node.id)).map(clone),
    edges: graph.edges.filter(edge => !edgeIds.includes(edge.id) && !nodeIds.includes(edge.source) && !nodeIds.includes(edge.target)).map(clone) }
}

export function duplicateSelection(graph: Graph, selected: string[], makeId: () => string = () => createUUID()) {
  const ids = new Map(selected.map(id => [id, makeId()]))
  const nodes = graph.nodes.filter(node => ids.has(node.id)).map(node => ({ ...clone(node), id: ids.get(node.id)!, position: { x: node.position.x + 36, y: node.position.y + 36 } }))
  const edges = graph.edges.filter(edge => ids.has(edge.source) && ids.has(edge.target)).map(edge => ({ ...edge, id: makeId(), source: ids.get(edge.source)!, target: ids.get(edge.target)! }))
  return { graph: { ...clone(graph), nodes: [...graph.nodes.map(clone), ...nodes], edges: [...graph.edges.map(clone), ...edges] }, selected: nodes.map(node => node.id) }
}

export class DraftHistory {
  private entries: DraftDocument[]
  private index = 0
  constructor(initial: DraftDocument) { this.entries = [clone(initial)] }
  get canUndo() { return this.index > 0 }
  get canRedo() { return this.index < this.entries.length - 1 }
  record(value: DraftDocument) {
    if (stable(this.entries[this.index]) === stable(value)) return
    this.entries = this.entries.slice(0, this.index + 1)
    this.entries.push(clone(value))
    if (this.entries.length > 51) this.entries.shift()
    this.index = this.entries.length - 1
  }
  undo(): DraftDocument | null { return this.canUndo ? clone(this.entries[--this.index]!) : null }
  redo(): DraftDocument | null { return this.canRedo ? clone(this.entries[++this.index]!) : null }
}
