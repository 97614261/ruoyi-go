import test from 'node:test'
import assert from 'node:assert/strict'
import { formatMicros, formatBytes, localDate } from '../src/utils/pod-admin.js'

test('money retains micros beyond Number precision', () => {
  assert.equal(formatMicros('9007199254740993'), '9007199254.740993')
  assert.equal(formatMicros('0'), '0.000000')
  assert.equal(formatMicros('1000001'), '1.000001')
})
test('storage includes subunit fractions without floating point rounding', () => {
  assert.equal(formatBytes('1023'), '1023 B')
  assert.equal(formatBytes('1536'), '1.5 KiB')
  assert.equal(formatBytes('1073741824'), '1.0 GiB')
})
test('business dates match Shanghai across UTC midnight boundary', () => {
  assert.equal(localDate(new Date('2026-11-12T16:01:00Z')), '2026-11-13')
  assert.equal(localDate(new Date('2026-11-12T15:59:00Z')), '2026-11-12')
})
test('management session ignores inherited cookie and creator session', async () => {
  const values = new Map([['pod-studio.session', 'creator-session']])
  globalThis.sessionStorage = { getItem: key => values.get(key) || null, setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key) }
  const auth = await import('../src/utils/auth.js')
  assert.equal(auth.getToken(), null)
  auth.setToken('admin-session')
  assert.equal(auth.getToken(), 'admin-session')
  auth.removeToken()
  assert.equal(auth.getToken(), null)
  assert.equal(values.get('pod-studio.session'), 'creator-session')
  delete globalThis.sessionStorage
})
