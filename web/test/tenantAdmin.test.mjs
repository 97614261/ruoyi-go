import test from 'node:test'
import assert from 'node:assert/strict'
import { menuNodeIds, tenantMenusPayload, emptyOwnerPasswordForm } from '../src/utils/tenantAdmin.mjs'

test('tree selection includes buttons; clear selection is an explicit empty array', () => {
  const nodes = [{ id: 1, children: [{ id: 2, children: [{ id: 3 }] }] }]
  assert.deepEqual(menuNodeIds(nodes), [1, 2, 3])
  assert.deepEqual(tenantMenusPayload({ version: 7, tenantId: 999 }, [], 'demo', '撤回'), { version: 7, menuIds: [], confirmTenantCode: 'demo', reason: '撤回' })
})

test('password dialog starts clean and resets all credentials without storing them', () => {
  const row = { version: 3, ownerUserId: 42, password: 'never-copy-me', tenantCode: 'demo' }
  const form = emptyOwnerPasswordForm(row)
  assert.equal(form.version, 3)
  assert.equal(form.ownerUserId, 42)
  assert.equal(form.confirmTenantCode, '')
  assert.equal(form.newPassword, '')
  assert.equal(form.operatorPassword, '')
  assert.equal(JSON.stringify(form).includes('never-copy-me'), false)
  form.newPassword = 'temporary'
  assert.equal(emptyOwnerPasswordForm(row).newPassword, '')
})
