export class APIError extends Error {
  constructor(message: string, public code: number) { super(message) }
}

export async function request<T>(path: string, options: RequestInit = {}, token = ''): Promise<T> {
  // Paths are application-owned constants, not remote URLs or user input.
  if (!path.startsWith('/') || path.startsWith('//')) throw new APIError('请求地址不合法', 400)
  const controller = new AbortController()
  const multipart = options.body instanceof FormData
  const timeout = setTimeout(() => controller.abort(), multipart ? 60_000 : 15_000)
  try {
    const headers = new Headers(options.headers)
    if (options.body && !multipart) headers.set('Content-Type', 'application/json')
    if (token) headers.set('Authorization', `Bearer ${token}`)
    const response = await fetch(`/api${path}`, { ...options, headers, signal: controller.signal })
    const payload = await response.json().catch(() => null) as { code?: number; msg?: string } | null
    if (!response.ok || !payload || typeof payload.code !== 'number') {
      throw new APIError('服务暂时不可用，请稍后重试', response.status || 500)
    }
    if (payload.code !== 200) throw new APIError(payload.msg || '请求失败', payload.code)
    return payload as T
  } catch (error) {
    if (error instanceof APIError) throw error
    if (controller.signal.aborted) throw new APIError('请求超时，请检查服务后重试', 408)
    throw new APIError('无法连接服务，请检查网络后重试', 503)
  } finally { clearTimeout(timeout) }
}

export async function requestImage(path: string, token: string): Promise<Blob> {
  if (!/^\/pod\/assets\/[1-9][0-9]*\/(content(?:\?variant=(?:original|thumbnail))?|download)$/.test(path)) throw new APIError('素材地址不合法', 400)
  const controller = new AbortController(), timeout = setTimeout(() => controller.abort(), 30_000)
  try {
    const response = await fetch(`/api${path}`, { headers: { Authorization: `Bearer ${token}` }, cache: 'no-store', signal: controller.signal })
    if (response.headers.get('Content-Type')?.includes('application/json')) {
      const error = await response.json() as { code?: number; msg?: string }
      throw new APIError(error.msg || '素材暂不可用', error.code || 500)
    }
    if (!response.ok || !/^image\/(png|jpeg|webp)(;|$)/.test(response.headers.get('Content-Type') || '')) throw new APIError('素材暂不可用，请重试', response.status || 500)
    return await response.blob()
  } catch (error) {
    if (error instanceof APIError) throw error
    throw new APIError(controller.signal.aborted ? '读取素材超时，请重试' : '无法读取素材，请检查连接后重试', controller.signal.aborted ? 408 : 503)
  } finally { clearTimeout(timeout) }
}
