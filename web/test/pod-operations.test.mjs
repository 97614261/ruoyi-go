import test from 'node:test'
import assert from 'node:assert/strict'
import { operationPayload, operationMetadata } from '../src/utils/pod-operations.js'

test('grant retry retains exact resource, operator and expiry without private content', () => {
  const form={operatorUserId:'1',ownerUserId:'921',resourceId:'15',resourceType:'run',expiresAt:new Date(Date.now()+3600000),reason:'investigate fixture',body:'private content'}
  const original=operationPayload('private.grant',form)
  const pending=operationMetadata('private.grant',form,'3f2777ef-7648-4fbe-aeb5-8e68244b6ed7')
  assert.equal(pending.reason,undefined)
  assert.equal(pending.body,undefined)
  assert.equal(pending.expiresAt,form.expiresAt.toISOString())
  assert.deepEqual(operationPayload(pending.op,{...JSON.parse(JSON.stringify(pending)),reason:form.reason}),original)
})

test('management actions reject imprecise ids and invalid lifetimes', () => {
  assert.throws(()=>operationPayload('task.cancel',{id:7,reason:'cancel'}))
  assert.throws(()=>operationPayload('task.cancel',{id:'9223372036854775808',reason:'cancel'}))
  assert.throws(()=>operationPayload('private.grant',{operatorUserId:'1',ownerUserId:'2',resourceId:'3',resourceType:'run',expiresAt:new Date(Date.now()-1000),reason:'test'}))
  assert.deepEqual(operationPayload('batch.stop',{id:'9',reason:'stop waiting only'}),{path:'batches/9/stop',data:{reason:'stop waiting only'}})
})
