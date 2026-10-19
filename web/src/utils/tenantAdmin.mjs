export function menuNodeIds(nodes) {
  const result = []
  const pending = [...(nodes || [])]
  while (pending.length) {
    const node = pending.shift()
    result.push(node.id)
    if (node.children) pending.push(...node.children)
  }
  return result
}

export function tenantMenusPayload(settings, keys, confirmTenantCode, reason) {
  return { version: settings.version, menuIds: [...keys], confirmTenantCode, reason }
}

export function emptyOwnerPasswordForm(row = {}) {
  return {
    version: row.version, ownerUserId: row.ownerUserId,
    confirmTenantCode: '', newPassword: '', confirmPassword: '', operatorPassword: '', reason: ''
  }
}
