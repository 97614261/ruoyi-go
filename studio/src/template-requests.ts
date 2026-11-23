import { createUUID } from './uuid'
import { APIError, request } from './api'

export interface TemplateIntent { path: string; body: Record<string, unknown>; requestId: string }
const storageKey = (scope: string) => `pod-studio.template-request:${scope}`
const allowedPath = (path: string) => /^\/pod\/templates(?:\/[1-9][0-9]*\/(?:versions|copy|use))?$/.test(path)

export function pendingTemplateRequest(scope: string): TemplateIntent | null {
  const raw = sessionStorage.getItem(storageKey(scope))
  if (!raw) return null
  const value = JSON.parse(raw) as TemplateIntent
  if (!allowedPath(value.path) || !value.body || Array.isArray(value.body) || typeof value.body !== 'object' || !/^[0-9a-f-]{36}$/.test(value.requestId)) throw new Error('模板提交记录无法读取，请先在项目或模板列表核对上次操作。')
  return value
}

export async function retryTemplateRequest<T>(scope: string, token: string): Promise<{ data: T }> {
  const pending = pendingTemplateRequest(scope)
  if (!pending) throw new Error('没有待确认的模板操作')
  try {
    const result = await request<{ data: T }>(pending.path, { method: 'POST', body: JSON.stringify({ ...pending.body, requestId: pending.requestId }) }, token)
    sessionStorage.removeItem(storageKey(scope))
    return result
  } catch (error) {
    if (error instanceof APIError && error.code >= 400 && error.code < 500 && error.code !== 408) sessionStorage.removeItem(storageKey(scope))
    throw error
  }
}

export async function submitTemplateRequest<T>(scope: string, path: string, body: Record<string, unknown>, token: string): Promise<{ data: T }> {
  if (!allowedPath(path)) throw new Error('模板操作地址不正确')
  if (pendingTemplateRequest(scope)) throw new Error('上一项模板操作结果待确认，请先重试原提交；也可在模板页面处理。')
  const intent: TemplateIntent = { path, body, requestId: createUUID() }
  // Save the intent before sending; a lost response must reuse the same request ID.
  sessionStorage.setItem(storageKey(scope), JSON.stringify(intent))
  return retryTemplateRequest<T>(scope, token)
}
