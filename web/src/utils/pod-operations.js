import { createUUID } from './uuid.js'
const id = value => typeof value === 'string' && /^[1-9][0-9]*$/.test(value) && BigInt(value) <= 9223372036854775807n
export const taskStates = { queued:'排队中',running:'执行中',cancelRequested:'取消中',awaitingReconciliation:'待核对',succeeded:'已成功',failed:'已失败',cancelled:'已取消',active:'派发中',stopped:'已停止',completed:'已派发' }
export function operationPayload(op, form) {
  if (!form.reason?.trim() || [...form.reason].length > 200) throw new Error('请填写 1—200 字操作原因')
  const data = { reason:form.reason }
  if (op === 'private.grant') {
    if (![form.operatorUserId,form.ownerUserId,form.resourceId].every(id) || !['run','asset'].includes(form.resourceType)) throw new Error('请指定有效的用户和单个资源编号')
    const expiry = new Date(form.expiresAt)
    if (!Number.isFinite(expiry.getTime()) || expiry <= new Date() || expiry.getTime() > Date.now()+86400000) throw new Error('授权到期时间须在未来 24 小时内')
    Object.assign(data,{operatorUserId:form.operatorUserId,ownerUserId:form.ownerUserId,resourceType:form.resourceType,resourceId:form.resourceId,expiresAt:expiry.toISOString()})
    return {path:'private-grants',data}
  }
  if (!id(form.id)) throw new Error('资源编号无效')
  if (op === 'private.revoke') {
    if (!id(form.revision)) throw new Error('授权版本无效')
    data.revision=form.revision
    return {path:`private-grants/${form.id}/revoke`,data}
  }
  if (op === 'task.cancel') return {path:`runs/${form.id}/cancel`,data}
  if (op === 'batch.stop') return {path:`batches/${form.id}/stop`,data}
  throw new Error('管理操作无效')
}
export function operationMetadata(op,form,requestId=createUUID()) {
  const value = {op,requestId,id:form.id || '',revision:form.revision || ''}
  if (op === 'private.grant') Object.assign(value, {
    operatorUserId:form.operatorUserId,ownerUserId:form.ownerUserId,
    resourceType:form.resourceType,resourceId:form.resourceId,
    expiresAt:new Date(form.expiresAt).toISOString()
  })
  return value
}
