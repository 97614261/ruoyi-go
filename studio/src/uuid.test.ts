import { afterEach, expect, it, vi } from 'vitest'
import { webcrypto } from 'node:crypto'
import { createUUID } from './uuid'

afterEach(() => vi.unstubAllGlobals())

it('prefers the native UUID method and preserves its receiver', () => {
  const source = { randomUUID() { expect(this).toBe(source); return 'native-uuid' }, getRandomValues: vi.fn() }
  vi.stubGlobal('crypto', source)
  expect(createUUID()).toBe('native-uuid')
  expect(source.getRandomValues).not.toHaveBeenCalled()
})

it('creates UUID v4 with secure random bytes when HTTP has no randomUUID', () => {
  const source = { getRandomValues(bytes: Uint8Array) { expect(this).toBe(source); bytes.set(Array.from({ length: 16 }, (_, i) => i)); return bytes } }
  vi.stubGlobal('crypto', source)
  expect(createUUID()).toBe('00010203-0405-4607-8809-0a0b0c0d0e0f')
})

it('creates distinct valid UUIDs on the HTTP fallback path', () => {
  vi.stubGlobal('crypto', { getRandomValues: webcrypto.getRandomValues.bind(webcrypto) })
  const ids = Array.from({ length: 1000 }, () => createUUID())
  expect(new Set(ids).size).toBe(ids.length)
  expect(ids.every(id => /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id))).toBe(true)
})

it('stops instead of using weak randomness if secure randomness is unavailable', () => {
  for (const value of [undefined, {}]) {
    vi.stubGlobal('crypto', value)
    expect(() => createUUID()).toThrow('浏览器不支持安全随机数')
  }
})
