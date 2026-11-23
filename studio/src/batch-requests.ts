import { APIError, request } from './api'
import type { Batch, BatchRequest } from './contracts'
const key = (scope: string) => `pod-studio.batch-request:${scope}`
export function pendingBatch(scope: string): BatchRequest | null {
  const raw = sessionStorage.getItem(key(scope))
  if (!raw) return null
  const body = JSON.parse(raw) as BatchRequest
  if (!/^[0-9a-f-]{36}$/.test(body.requestId) || !/^[1-9][0-9]*$/.test(body.templateId) || !Array.isArray(body.items) || body.items.length < 1 || body.items.length > 10) throw new Error('批量提交记录无法读取，请先核对批次列表。')
  return body
}
export async function retryBatch(scope: string, token: string): Promise<Batch> {
  const body = pendingBatch(scope)
  if (!body) throw new Error('没有待确认的批量提交')
  try {
    const result = await request<{ data: Batch }>('/pod/batches', { method: 'POST', body: JSON.stringify(body) }, token)
    sessionStorage.removeItem(key(scope)); return result.data
  } catch (e) {
    if (e instanceof APIError && [400, 403, 404, 409, 429].includes(e.code)) sessionStorage.removeItem(key(scope))
    throw e
  }
}
export async function submitBatch(scope: string, token: string, body: BatchRequest) {
  if (pendingBatch(scope)) throw new Error('上一批提交结果待确认，请先重试原提交。')
  sessionStorage.setItem(key(scope), JSON.stringify(body))
  return retryBatch(scope, token)
}
export async function downloadBatch(id: string, token: string) {
  if (!/^[1-9][0-9]*$/.test(id)) throw new Error('批次编号不合法')
  const controller = new AbortController(), timeout = setTimeout(() => controller.abort(), 60_000)
  try {
    const response = await fetch(`/api/pod/batches/${id}/download`, { headers: { Authorization: `Bearer ${token}` }, cache: 'no-store', signal: controller.signal })
    if (response.headers.get('Content-Type')?.includes('application/json')) { const data = await response.json(); throw new APIError(data.msg || '无法下载', data.code || 500) }
    if (!response.ok || response.headers.get('Content-Type') !== 'application/zip') throw new Error('下载失败，请重试')
    return await response.blob()
  } finally { clearTimeout(timeout) }
}
