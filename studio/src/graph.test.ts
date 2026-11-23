import { describe, expect, it } from 'vitest'
import { DraftHistory, duplicateSelection, graphError, removeSelection, stable } from './graph'
import type { DraftDocument, Graph, NodeDefinition } from './contracts'

const definitions: NodeDefinition[] = [{ type: 'textGenerate', version: 1, label: '文本', description: '', executable: false,
  inputs: [{ key: 'text', valueType: 'text', required: true }], outputs: [{ key: 'text', valueType: 'text', required: true }] }]
const limits = { maxNodes: 100, maxEdges: 300, maxGraphBytes: 1048576 }
const graph = (): Graph => ({ schemaVersion: 1, nodes: ['a', 'b'].map(id => ({ id, type: 'textGenerate', version: 1, position: { x: 10, y: 20 }, config: {} })),
  edges: [{ id: 'ab', source: 'a', sourcePort: 'text', target: 'b', targetPort: 'text' }] })
describe('graph editing invariants', () => {
  it('rejects cycles, duplicate inputs, mismatched ports and limits', () => {
    expect(graphError(graph(), definitions, limits)).toBe('')
    const cycle = graph(); cycle.edges.push({ id: 'ba', source: 'b', sourcePort: 'text', target: 'a', targetPort: 'text' })
    expect(graphError(cycle, definitions, limits)).toContain('循环')
    const occupied = graph(); occupied.edges.push({ ...occupied.edges[0]!, id: 'duplicate' })
    expect(graphError(occupied, definitions, limits)).toContain('已有上游')
    const wrong = graph(); wrong.edges[0]!.targetPort = 'images'
    expect(graphError(wrong, definitions, limits)).toContain('不兼容')
    expect(graphError(graph(), definitions, { ...limits, maxNodes: 1 })).toContain('超过')
  })
  it('duplicates only internal edges with new IDs and independent config', () => {
    let counter = 0
    const original = graph(), result = duplicateSelection(original, ['a', 'b'], () => `copy_${counter++}`)
    expect(result.graph.nodes).toHaveLength(4); expect(result.graph.edges).toHaveLength(2)
    expect(result.graph.edges[1]).toMatchObject({ source: result.selected[0], target: result.selected[1] })
    result.graph.nodes[2]!.config.title = 'changed'
    expect(original.nodes[0]!.config.title).toBeUndefined()
    expect(duplicateSelection(original, ['a']).graph.edges).toHaveLength(1)
  })
  it('removes incident edges with a node and restores via undo/redo', () => {
    const initial: DraftDocument = { name: '印花', graph: graph() }, history = new DraftHistory(initial)
    const removed = { ...initial, graph: removeSelection(initial.graph, ['a'], []) }
    history.record(removed)
    expect(removed.graph.edges).toHaveLength(0)
    expect(history.undo()).toEqual(initial); expect(history.redo()).toEqual(removed)
    history.undo(); history.record({ ...initial, name: '新分支' }); expect(history.canRedo).toBe(false)
  })
  it('compares JSON semantically despite MySQL object key ordering', () => {
    expect(stable({ title: 'a', count: 2 })).toBe(stable({ count: 2, title: 'a' }))
  })
})
