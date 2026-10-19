import test from 'node:test'
import assert from 'node:assert/strict'
import { identitySummary } from '../src/utils/sessionIdentity.mjs'

test('shows verified tenant, username and complete role names', () => {
  const result = identitySummary({ tenantName: '示例企业 A', tenantCode: 'demo', userName: 'alice', roleNames: ['采购', '财务', '采购', ''] })
  assert.equal(result.tenantName, '示例企业 A')
  assert.equal(result.tenantCode, 'demo')
  assert.equal(result.userName, 'alice')
  assert.equal(result.roleLabel, '采购、财务')
  assert.equal(result.platformAdmin, false)
})

test('platform access cannot be guessed from a role name, ID or truthy string', () => {
  assert.equal(identitySummary({ userId: 1, roleNames: ['平台超级管理员'], platformAdmin: 'true' }).platformAdmin, false)
  assert.equal(identitySummary({ tenantCode: 'platform', roleNames: ['admin'] }).platformAdmin, false)
  assert.equal(identitySummary({ platformAdmin: true }).platformAdmin, true)
})

test('loading, empty, malformed and logged-out identity cannot retain old labels', () => {
  assert.equal(identitySummary(null).tenantName, '企业信息未加载')
  assert.equal(identitySummary(null).userName, '未登录')
  assert.equal(identitySummary({ roleNames: [null, 42, ' '] }).roleLabel, '未分配角色')
  assert.equal(identitySummary({}, 'bob').userName, 'bob')
  assert.deepEqual(identitySummary(null).roleNames, [])
})
