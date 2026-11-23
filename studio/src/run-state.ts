export const runStateLabel: Record<string, string> = {
  queued: '排队中', waiting: '等待上游', ready: '等待执行', running: '执行中', succeeded: '已完成',
  failed: '失败', blocked: '上游阻断', cancelRequested: '正在取消', cancelled: '已取消', awaitingReconciliation: '确认原调用结果',
}
export const terminalRun = (state: string) => ['succeeded', 'failed', 'cancelled'].includes(state)

export function costLabel(micros: string) {
  if (!/^\d{1,18}$/.test(micros)) return '待核对'
  const amount = BigInt(micros)
  return `${amount / 1000000n}.${(amount % 1000000n).toString().padStart(6, '0')} 元`
}
