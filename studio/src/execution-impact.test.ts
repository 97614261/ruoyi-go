import { expect, it } from 'vitest'
import type { Graph, Run } from './contracts'
import { executionImpact, runGuidance, canRetryRun, runRecovery } from './execution-impact'
import { costLabel } from './run-state'

export const graph: Graph = { schemaVersion: 1, nodes: [
  { id: 'A', type: 'textInput', version: 1, position: { x: 0, y: 0 }, config: {} },
  { id: 'B', type: 'textGenerate', version: 1, position: { x: 0, y: 0 }, config: { title: '润色提示词' } },
  { id: 'C', type: 'imageGenerate', version: 1, position: { x: 0, y: 0 }, config: { count: 3 } },
  { id: 'D', type: 'imageProcess', version: 1, position: { x: 0, y: 0 }, config: {} },
  { id: 'Other', type: 'imageGenerate', version: 1, position: { x: 0, y: 0 }, config: { count: 4 } },
], edges: [
  { id: 'AB', source: 'A', sourcePort: 'text', target: 'B', targetPort: 'text' },
  { id: 'BC', source: 'B', sourcePort: 'text', target: 'C', targetPort: 'text' },
  { id: 'CD', source: 'C', sourcePort: 'images', target: 'D', targetPort: 'images' },
] }

it('previews exactly the branch ancestors, excluding unrelated paid nodes', () => {
  expect(executionImpact(graph, 'branch', 'D')).toEqual({ textCalls: 1, imageCalls: 1, images: 3, localNodes: 2, names: ['润色提示词', 'C'], reservationMicros: '325000' })
  expect(executionImpact(graph, 'workflow')?.images).toBe(7)
  expect(executionImpact(graph, 'node', 'D')?.reservationMicros).toBe('0')
  expect(executionImpact(graph, 'polish', 'A')?.reservationMicros).toBe('25000')
})
it('counts shared ancestors once and cannot loop forever on an invalid cyclic draft', () => {
  const changed = structuredClone(graph)
  changed.edges.push({ id: 'AC', source: 'A', sourcePort: 'text', target: 'C', targetPort: 'text' })
  expect(executionImpact(changed, 'branch', 'D')?.textCalls).toBe(1)
  changed.edges.push({ id: 'DA', source: 'D', sourcePort: 'images', target: 'A', targetPort: 'images' })
  expect(executionImpact(changed, 'branch', 'D')?.images).toBe(3)
})
it('does not invent a zero estimate for missing graph, target or invalid image counts', () => {
  expect(executionImpact(undefined, 'workflow')).toBeNull()
  expect(executionImpact(graph, 'branch', 'missing')).toBeNull()
  const changed = structuredClone(graph); changed.nodes[2]!.config.count = 20
  expect(executionImpact(changed, 'branch', 'D')).toBeNull()
})
it('keeps money exact past Number safe integers and rejects invalid money strings', () => {
  expect(costLabel('9007199254740993')).toBe('9007199254.740993 元')
  expect(costLabel('100001')).toBe('0.100001 元')
  expect(costLabel('-1')).toBe('待核对'); expect(costLabel('NaN')).toBe('待核对')
})
it('distinguishes unknown paid calls, archive download recovery, and budget rejection', () => {
  const run = { state: 'awaitingReconciliation', nodes: [] } as unknown as Run
  expect(runGuidance(run)).toContain('当前没有人工结算入口')
  run.nodes = [{ state: 'awaitingReconciliation', errorCode: 'archivePending' }] as Run['nodes']
  expect(runGuidance(run)).toContain('不重新生图')
  run.nodes = [{ errorCode: 'budgetExceeded' }] as Run['nodes']
  run.state = 'failed'
  expect(runGuidance(run)).toContain('此前已完成的调用仍计费')
})
it('unknown provider results take precedence over another failed node and cannot be retried', () => {
  const run = { state: 'failed', attempt: 1, nodes: [{ state: 'failed', errorCode: 'budgetExceeded' }, { state: 'awaitingReconciliation', errorCode: 'providerUncertain' }] } as Run
  expect(runRecovery(run)?.uncertain).toBe(true)
  expect(canRetryRun(run)).toBe(false)
  expect(runGuidance(run)).toContain('保留预占')
  run.nodes![1]!.errorCode = 'archivePending'
  expect(runGuidance(run)).toContain('不重新生图')
})
it.each(['providerInputRejected','providerModelUnavailable'])('requires new inputs or model versions for %s', code => {
  const run = { state: 'failed', attempt: 1, nodes: [{ state: 'failed', errorCode: code }] } as Run
  expect(runRecovery(run)?.newRun).toBe(true); expect(canRetryRun(run)).toBe(false)
})
it.each(['providerCredentialInvalid','providerFreeQuotaExhausted','providerRateLimited'])('explains %s while keeping retry a deliberate original-run operation', code => {
  const run = { state: 'failed', attempt: 1, nodes: [{ state: 'failed', errorCode: code }] } as Run
  expect(runGuidance(run)).not.toBe(''); expect(canRetryRun(run)).toBe(true)
})
