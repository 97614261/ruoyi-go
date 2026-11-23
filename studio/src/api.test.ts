import { afterEach, describe, expect, it, vi } from 'vitest'
import { request, requestImage } from './api'

afterEach(() => vi.unstubAllGlobals())
describe('RuoYi response contract', () => {
  it('rejects business 401 even when HTTP is 200', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 401, msg: '认证失败' }))))
    await expect(request('/pod/bootstrap')).rejects.toMatchObject({ code: 401 })
  })
  it('keeps login token at top level and sends Authorization', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 200, token: 'test-token' })))
    vi.stubGlobal('fetch', fetch)
    const result = await request<{ token: string }>('/login', { method: 'POST', body: '{}' }, 'test-session')
    expect(result.token).toBe('test-token')
    expect(fetch.mock.calls[0]?.[1].headers.get('Authorization')).toBe('Bearer test-session')
  })
  it('does not display an HTML proxy failure or raw network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>internal debug</html>', { status: 502 })))
    await expect(request('/pod/bootstrap')).rejects.toMatchObject({ message: '服务暂时不可用，请稍后重试' })
  })
  it('lets fetch set the multipart boundary and keeps the token in headers', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 200, data: { assetId: '1' } })))
    vi.stubGlobal('fetch', fetch)
    const body = new FormData(); body.append('file', new Blob(['image']), 'print.png')
    await request('/pod/assets', { method: 'POST', body }, 'private-session')
    expect(fetch.mock.calls[0]?.[1].headers.has('Content-Type')).toBe(false)
    expect(fetch.mock.calls[0]?.[1].headers.get('Authorization')).toBe('Bearer private-session')
    expect(fetch.mock.calls[0]?.[0]).toBe('/api/pod/assets')
  })
  it('reads authenticated images without exposing tokens in URLs and rejects business errors', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(new Blob(['image'], { type: 'image/png' }), { headers: { 'Content-Type': 'image/png' } }))
    vi.stubGlobal('fetch', fetch)
    expect((await requestImage('/pod/assets/1/download', 'private-session')).type).toBe('image/png')
    expect(fetch.mock.calls[0]?.[0]).toBe('/api/pod/assets/1/download')
    expect(fetch.mock.calls[0]?.[1].headers.Authorization).toBe('Bearer private-session')
    expect(fetch.mock.calls[0]?.[1].cache).toBe('no-store')
    fetch.mockResolvedValueOnce(new Response(JSON.stringify({ code: 401, msg: '登录失效' }), { headers: { 'Content-Type': 'application/json' } }))
    await expect(requestImage('/pod/assets/1/content', 'private-session')).rejects.toMatchObject({ code: 401 })
    await expect(requestImage('https://example.com/private', 'private-session')).rejects.toMatchObject({ code: 400 })
  })
})
