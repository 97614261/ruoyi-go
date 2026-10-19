const MiB = 1024 * 1024
export function formatBytes(bytes = 0) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(2)} GiB`
  if (bytes >= MiB) return `${(bytes / MiB).toFixed(2)} MiB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(2)} KiB`
  return `${bytes} B`
}
export function tenantStatus(status) {
  return ({ '0': '正常', '1': '停用', '2': '初始化', '3': '待注销', '4': '已归档' })[status] || '未知状态'
}
export function lifecycleActions(status) {
  if (status === '0' || status === '1') return ['requestCancellation']
  if (status === '3') return ['archive', 'rejectCancellation']
  if (status === '4') return ['restore']
  return []
}
export const lifecycleLabels = { requestCancellation: '申请注销', archive: '确认归档', rejectCancellation: '驳回注销', restore: '恢复为停用' }
export function quotaForm(policy) {
  return { maxUsers: policy.maxUsers, storageMiB: policy.maxStorageBytes / MiB, uploadMiB: policy.maxUploadBytes / MiB, confirmTenantCode: '', reason: '' }
}
export function quotaPayload(settings, form) {
  return { version: settings.version, confirmTenantCode: form.confirmTenantCode, reason: form.reason,
    maxUsers: form.maxUsers, maxStorageBytes: Math.round(form.storageMiB * MiB), maxUploadBytes: Math.round(form.uploadMiB * MiB) }
}
