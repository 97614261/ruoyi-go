// Display-only normalization. Never infer platform privileges from role names,
// the login form, cookies, or user/role IDs.
export function identitySummary(identity, userName = '') {
  const value = identity && typeof identity === 'object' ? identity : {}
  const text = input => typeof input === 'string' ? input.trim() : ''
  const roleNames = [...new Set((Array.isArray(value.roleNames) ? value.roleNames : []).map(text).filter(Boolean))]
  return {
    tenantName: text(value.tenantName) || '企业信息未加载',
    tenantCode: text(value.tenantCode),
    userName: text(value.userName) || text(userName) || '未登录',
    roleNames,
    roleLabel: roleNames.join('、') || '未分配角色',
    platformAdmin: value.platformAdmin === true,
    tenantAdmin: value.tenantAdmin === true
  }
}
