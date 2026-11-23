import { afterEach, expect, it, vi } from 'vitest'
import { APIError } from './api'
import { DraftSession } from './draft'
import type { DraftDocument } from './contracts'

afterEach(() => vi.useRealTimers())
const document = (name = '原稿'): DraftDocument => ({ name, graph: { schemaVersion: 1, nodes: [], edges: [] } })

it('debounces edits and serializes changes made during an inflight save', async () => {
  vi.useFakeTimers()
  let complete!: (result: { revision: string }) => void
  const save = vi.fn().mockImplementationOnce(() => new Promise(resolve => { complete = resolve })).mockResolvedValueOnce({ revision: '3' })
  const session = new DraftSession(document(), '1', save, () => {})
  session.edit(document('编辑 A')); session.edit(document('编辑 B'))
  await vi.advanceTimersByTimeAsync(800)
  expect(save).toHaveBeenCalledTimes(1); expect(save.mock.calls[0]).toEqual([document('编辑 B'), '1'])
  session.edit(document('保存期间继续编辑'))
  complete({ revision: '2' }); await Promise.resolve(); await Promise.resolve()
  await vi.advanceTimersByTimeAsync(800)
  expect(save.mock.calls[1]).toEqual([document('保存期间继续编辑'), '2'])
  expect(session.state).toBe('saved'); expect(session.dirty).toBe(false)
  session.dispose()
})

it('retains changes after conflict and never retries with the new revision automatically', async () => {
  vi.useFakeTimers()
  const save = vi.fn().mockRejectedValue(new APIError('版本冲突', 409))
  const session = new DraftSession(document(), '1', save, () => {})
  session.edit(document('本地修改')); await session.flush()
  session.edit(document('继续修改')); await vi.advanceTimersByTimeAsync(5000); await session.flush()
  expect(save).toHaveBeenCalledTimes(1); expect(session.state).toBe('conflict'); expect(session.document.name).toBe('继续修改')
  expect(session.dirty).toBe(true); expect(session.revision).toBe('1'); session.dispose()
})

it('retains a failed draft and permits an explicit retry', async () => {
  const save = vi.fn().mockRejectedValueOnce(new APIError('离线', 503)).mockResolvedValueOnce({ revision: '2' })
  const session = new DraftSession(document(), '1', save, () => {})
  session.edit(document('未保存')); await session.flush()
  expect(session.state).toBe('error'); expect(session.dirty).toBe(true)
  await session.flush(); expect(session.state).toBe('saved'); expect(session.revision).toBe('2'); session.dispose()
})

it('restores stale cached work as a blocked conflict', async () => {
  const save = vi.fn()
  const session = new DraftSession(document(), '5', save, () => {})
  session.recover(document('恢复旧草稿'), true); await session.flush()
  expect(save).not.toHaveBeenCalled(); expect(session.state).toBe('conflict'); session.dispose()
})
