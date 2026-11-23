import test from 'node:test'
import assert from 'node:assert/strict'
import { webcrypto } from 'node:crypto'
import { createUUID } from '../src/utils/uuid.js'
import { creatorPending } from '../src/utils/pod-creator.js'
import { pendingMetadata } from '../src/utils/pod-catalog.js'
import { operationMetadata } from '../src/utils/pod-operations.js'

function withCrypto(source, run) {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: source })
  try { run() } finally {
    if (descriptor) Object.defineProperty(globalThis, 'crypto', descriptor)
    else delete globalThis.crypto
  }
}

test('UUID uses native method with its receiver', () => {
  const source = { randomUUID() { assert.equal(this, source); return 'native-uuid' } }
  withCrypto(source, () => assert.equal(createUUID(), 'native-uuid'))
})

test('HTTP fallback sets UUID v4 version and variant without randomUUID', () => {
  const source = { getRandomValues(bytes) { assert.equal(this, source); bytes.set(Array.from({ length: 16 }, (_, i) => i)); return bytes } }
  withCrypto(source, () => assert.equal(createUUID(), '00010203-0405-4607-8809-0a0b0c0d0e0f'))
})

test('HTTP fallback stays unique and supports admin pending operation IDs', () => {
  withCrypto({ getRandomValues: webcrypto.getRandomValues.bind(webcrypto) }, () => {
    const ids = Array.from({ length: 1000 }, createUUID)
    ids.push(creatorPending('creator.models', {}).requestId)
    ids.push(pendingMetadata('model.status', { path: 'models/1/status', method: 'POST' }, {}, '启用').requestId)
    ids.push(operationMetadata('run.cancel', {}).requestId)
    assert.equal(new Set(ids).size, ids.length)
    for (const id of ids) assert.match(id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    assert.equal(creatorPending('creator.models', {}, ids[0]).requestId, ids[0])
  })
})

test('missing secure randomness stops the operation with an actionable error', () => {
  for (const source of [undefined, {}]) withCrypto(source, () => assert.throws(createUUID, /浏览器不支持安全随机数/))
})
