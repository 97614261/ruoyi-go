import { createUUID } from './uuid.js'
const positive = s => typeof s === 'string' && /^[1-9][0-9]*$/.test(s) && BigInt(s) <= 9223372036854775807n
const operations = new Set(['creator.create', 'creator.policy', 'creator.models', 'points.adjust'])

export function creatorPayload(op, f, limits) {
  if (!operations.has(op)) throw new Error('管理操作无效')
  const create = op === 'creator.create'
  if (!create && (!positive(f.id) || !positive(f.revision))) throw new Error('用户编号或版本无效，请刷新')
  if (!f.reason?.trim() || [...f.reason].length > 200) throw new Error('请填写 1—200 字操作原因')
  let method = create || op === 'points.adjust' ? 'POST' : 'PUT'
  let path = create ? 'creators' : `creators/${f.id}/${op === 'creator.policy' ? 'policy' : op === 'creator.models' ? 'models' : 'point-adjustments'}`
  const data = { reason: f.reason }
  if (!create) data[op === 'points.adjust' ? 'accountRevision' : 'revision'] = f.revision
  if (create || op === 'creator.policy') {
    if (!limits || !['active', 'suspended'].includes(f.status) || !Number.isInteger(f.maxConcurrentNodes) || f.maxConcurrentNodes < 1 || f.maxConcurrentNodes > limits.maxConcurrentNodes || !Number.isInteger(f.maxPendingRuns) || f.maxPendingRuns < 1 || f.maxPendingRuns > limits.maxPendingRuns || !Number.isInteger(f.maxDailySubmissions) || f.maxDailySubmissions < 1 || f.maxDailySubmissions > limits.maxDailySubmissions || !positive(f.maxStorageBytes) || BigInt(f.maxStorageBytes) > BigInt(limits.maxStorageBytes)) throw new Error('资源限制须为正整数，且不能超过页面提示的实例上限')
    if (f.validUntil && !/^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d$/.test(f.validUntil)) throw new Error('有效期格式无效')
    data.policy = { status: f.status, validUntil: f.validUntil || null, maxConcurrentNodes: f.maxConcurrentNodes, maxPendingRuns: f.maxPendingRuns, maxDailySubmissions: f.maxDailySubmissions, maxStorageBytes: f.maxStorageBytes }
  }
  if (create) {
    if (!/^[a-zA-Z][a-zA-Z0-9_-]{2,19}$/.test(f.userName) || !f.nickName?.trim() || [...f.nickName].length > 30 || [...(f.initialPassword || '')].length < 8 || [...f.initialPassword].length > 20 || f.reason.includes(f.initialPassword)) throw new Error('账号为 3—20 位字母开头的字母数字或 _-，昵称 1—30 字，密码 8—20 字；原因里不能填密码')
    Object.assign(data, { userName: f.userName, nickName: f.nickName, initialPassword: f.initialPassword })
  }
  if (op === 'creator.models') {
    if (!Array.isArray(f.modelIds) || f.modelIds.length > 100 || !f.modelIds.every(positive) || new Set(f.modelIds).size !== f.modelIds.length) throw new Error('模型范围无效，最多 100 项')
    data.modelIds = [...f.modelIds]
  }
  if (op === 'points.adjust') {
    if (!['grant', 'deduct'].includes(f.kind) || !positive(f.amount) || BigInt(f.amount) > 1000000000n) throw new Error('点数请输入 1—1000000000 的整数')
    Object.assign(data, { kind: f.kind, amount: f.amount })
  }
  return { method, path, data }
}

// Persist only an allowlisted identity/version, never a password or request body.
export function creatorPending(op, f, requestId = createUUID()) {
  return { op, requestId, id: op === 'creator.create' ? '' : f.id, revision: op === 'creator.create' ? '' : f.revision }
}
export function readCreatorPending(raw) {
  try {
    const p = JSON.parse(raw)
    if (!p || !operations.has(p.op) || typeof p.requestId !== 'string' || !/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(p.requestId) || p.requestId === '00000000-0000-0000-0000-000000000000') return null
    if (p.op === 'creator.create' ? p.id !== '' || p.revision !== '' : !positive(p.id) || !positive(p.revision)) return null
    return creatorPending(p.op, { id: p.id, revision: p.revision }, p.requestId)
  } catch { return null }
}

export function pointEventLabel(kind) {
  return ({ grant: '发放', deduct: '扣减', hold: '预占', settle: '结算', release: '释放', transfer: '份额转移' })[kind] || '未知操作'
}
