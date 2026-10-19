import test from 'node:test'
import assert from 'node:assert/strict'
import { createRepeatSubmitGuard } from '../src/utils/repeatSubmit.mjs'

test('repeat guard preserves URL/body/time semantics without persistent storage', () => {
  const duplicate = createRepeatSubmitGuard()
  const request = { url: '/user/password', data: 'test-only-sensitive-body', time: 1000 }
  assert.equal(duplicate(request), false)
  assert.equal(duplicate({ ...request, time: 1500 }), true)
  assert.equal(duplicate({ ...request, time: 2000 }), false)
  assert.equal(duplicate({ ...request, url: '/other', time: 2001 }), false)
  assert.equal(duplicate({ ...request, url: '/other', data: 'different', time: 2002 }), false)
})

test('guard is page-local, honors interval and handles clock rollback', () => {
  const duplicate = createRepeatSubmitGuard()
  const request = { url: '/user', data: '{}', time: 1000 }
  assert.equal(duplicate(request, 200), false)
  assert.equal(duplicate({ ...request, time: 1199 }, 200), true)
  assert.equal(duplicate({ ...request, time: 1200 }, 200), false)
  assert.equal(duplicate({ ...request, time: 500 }, 200), false)
  assert.equal(createRepeatSubmitGuard()(request), false)
})
