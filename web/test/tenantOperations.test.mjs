import test from 'node:test'
import assert from 'node:assert/strict'
import { lifecycleActions, tenantStatus, quotaForm, quotaPayload, formatBytes } from '../src/utils/tenantOperations.mjs'

test('archive cannot be enabled or requested again; restore is an explicit action',()=>{
  assert.deepEqual(lifecycleActions('4'),['restore'])
  assert.deepEqual(lifecycleActions('3'),['archive','rejectCancellation'])
  assert.equal(tenantStatus('4'),'已归档')
})
test('quota conversion preserves bytes and explicit unlimited zero',()=>{
  const settings={version:4}
  const form=quotaForm({maxUsers:100,maxStorageBytes:1073741824,maxUploadBytes:10485760})
  assert.equal(form.storageMiB,1024)
  assert.equal(quotaPayload(settings,form).maxStorageBytes,1073741824)
  const zero=quotaPayload(settings,quotaForm({maxUsers:0,maxStorageBytes:0,maxUploadBytes:0}))
  assert.equal(zero.maxStorageBytes,0)
  assert.equal(zero.maxUploadBytes,0)
  assert.equal(formatBytes(0),'0 B')
})
