import type { Graph, Run } from './contracts'

export function executionImpact(graph: Graph | undefined, mode: Run['mode'], target = '') {
  if (!graph || (mode !== 'workflow' && !graph.nodes.some(node => node.id === target))) return null
  if (mode === 'polish') return { textCalls: 1, imageCalls: 0, images: 0, localNodes: 1, names: ['AI 润色'], reservationMicros: '25000' }
  const ids = new Set(mode === 'workflow' ? graph.nodes.map(node => node.id) : [target])
  if (mode === 'branch') {
    for (let changed = true; changed;) {
      changed = false
      for (const edge of graph.edges) if (ids.has(edge.target) && !ids.has(edge.source)) { ids.add(edge.source); changed = true }
    }
  }
  const nodes = graph.nodes.filter(node => ids.has(node.id))
  const textCalls = nodes.filter(node => node.type === 'textGenerate').length
  const imageNodes = nodes.filter(node => node.type === 'imageGenerate')
  let images = 0
  for (const node of imageNodes) {
    const count = node.config.count ?? 1
    if (typeof count !== 'number' || !Number.isInteger(count) || count < 1 || count > 4) return null
    images += count
  }
  return { textCalls, imageCalls: imageNodes.length, images, localNodes: nodes.length - textCalls - imageNodes.length,
    names: nodes.filter(node => ['textGenerate', 'imageGenerate'].includes(node.type)).map(node => String(node.config.title || node.id)),
    reservationMicros: (BigInt(textCalls) * 25000n + BigInt(images) * 100000n).toString() }
}

export function runRecovery(run: Run) {
  const pending = run.nodes?.filter(n => n.state === 'awaitingReconciliation') || []
  if (run.state === 'awaitingReconciliation' || pending.length) {
    if (pending.length && pending.every(n => n.errorCode === 'archivePending')) return { title: '图片已生成，正在恢复归档', detail: '系统只核对或恢复原结果下载，不重新生图；稍后刷新即可，已确认模型费用仍保留。不要重新生成相同图片。', refresh: true, newRun: false, uncertain: true }
    return { title: '请核对原调用，暂不重做', detail: '原调用结果待确认，保留预占，不自动重发。请保留运行编号，由部署者核对供应商账单；当前没有人工结算入口。不要重新创建同样的任务。', refresh: true, newRun: false, uncertain: true }
  }
  if (run.state === 'cancelRequested') return { title: '正在确认取消结果', detail: '取消只停止后续步骤，已调用部分仍可能计费；不要新建相同运行绕过确认。', refresh: true, newRun: false, uncertain: true }
  const advice: Record<string, [string, string, boolean]> = {
    providerFreeQuotaExhausted: ['先处理模型额度', '请管理员在对应厂商控制台核对免费额度和仅免费调用限制。提高本站预算不能解决供应商额度；处理完成后再对原失败步骤重新报价。', false],
    providerCredentialInvalid: ['请管理员检查 API Key', '核对对应厂商、空间和 Key，必要时轮换凭据。处理后再对原失败步骤重新报价，不要反复点击生成。', false],
    credentialUnavailable: ['供应商凭据尚未就绪', '请管理员补齐或轮换对应厂商的凭据，检查主密钥配置；本次尚未调用模型。', false],
    providerAccessDenied: ['请管理员核对模型权限', '检查供应商账号的模型开通、额度和地域。处理后再对原失败步骤重新报价。', false],
    providerModelUnavailable: ['核对型号后重新制作', '请管理员核对官方模型 ID，并保存正确的新版本。当前运行保留原版本，请返回画布选择可用模型后重新报价。', true],
    providerInputRejected: ['修改输入后重新制作', '请返回画布检查提示词、参考图片和参数，保存修改后重新报价；恢复旧步骤会沿用原输入。', true],
    providerRateLimited: ['等待限流解除', '稍后查看原任务，确认已经失败后，可以手动恢复失败步骤；点击恢复后系统自动核价并提交，不会自行重试。', false],
    budgetExceeded: ['请管理员核对预算', '该失败步骤预算不足，尚未调用模型；此前已完成的调用仍计费。请查看“我的用量”和预占，请管理员核对实例累计上限，再决定是否恢复。', false],
    providerRejected: ['核对模型配置后再恢复', '这是历史的供应商拒绝记录。请管理员检查 Key、模型权限、参数和额度，再决定是否恢复；不会自动重试。', false],
  }
  const code = run.nodes?.find(n => n.errorCode && advice[n.errorCode])?.errorCode
  if (code) { const [title, detail, newRun] = advice[code]!; return { title, detail, newRun, refresh: false, uncertain: false } }
  return null
}
export function canRetryRun(run: Run) { const advice = runRecovery(run); return run.state === 'failed' && run.attempt < 5 && !advice?.newRun && !advice?.uncertain }
export function runGuidance(run: Run) { return runRecovery(run)?.detail || '' }
