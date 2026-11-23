import { APIError, request } from './api'
import type { ExecutionQuote, FormalPoints, QuoteSource } from './contracts'

export function isPointsShortage(error: unknown): error is APIError {
  return error instanceof APIError && error.code === 409 && error.message.startsWith('正式点数不足')
}

export async function pointsShortageMessage(token: string, required?: string): Promise<string> {
  let available: string | undefined
  try {
    const { data } = await request<{ data: FormalPoints }>('/pod/points?pageNum=1&pageSize=1', {}, token)
    const value = data?.account?.available
    if (typeof value === 'string' && /^(0|[1-9]\d*)$/.test(value)) available = value
  } catch { /* Keep the original rejection useful even if the balance can no longer be fetched. */ }
  const counts: string[] = []
  if (required && /^(0|[1-9]\d*)$/.test(required)) counts.push(`本次需 ${required} 点`)
  if (available !== undefined) counts.push(`当前可用 ${available} 点`)
  return `点数不足，本次未提交。${counts.length ? counts.join('，') + '。' : ''}可以减少生成张数、换低价模型，或联系管理员补充点数。已有图片仍可查看和下载。`
}

// A fresh server quote remains mandatory even when execution has no confirmation dialog.
export async function executionQuote(token: string, source: QuoteSource): Promise<ExecutionQuote> {
  const body = JSON.stringify(source), kind = source.kind
  const { data } = await request<{ data: ExecutionQuote }>('/pod/execution-quotes', { method: 'POST', body }, token)
  if (!data || typeof data.quoteId !== 'string' || !/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(data.quoteId) || data.kind !== kind || data.state !== 'ready' || typeof data.points !== 'string' || !/^(0|[1-9]\d*)$/.test(data.points)) {
    throw new Error('点数报价无效，本次未提交，请重新操作。')
  }
  if (typeof data.expiresAt !== 'string' || !Number.isFinite(Date.parse(data.expiresAt)) || Date.now() >= Date.parse(data.expiresAt)) {
    throw new Error('点数报价已过期，本次未提交，请重新操作。')
  }
  return data
}
