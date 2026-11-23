<script setup lang="ts">
import { createUUID } from './uuid'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PhArrowClockwise, PhPlay, PhStop, PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { CreditAccount, CreditEvent, ExecutionLimits, Graph, Run } from './contracts'
import { costLabel, runStateLabel, terminalRun } from './run-state'
import ResultImages from './ResultImages.vue'
import Usage from './Usage.vue'
import Batches from './Batches.vue'
import BillingSummary from './BillingSummary.vue'
import { executionQuote, isPointsShortage, pointsShortageMessage } from './execution-quote'
import RunImpact from './RunImpact.vue'
import { canRetryRun, runRecovery } from './execution-impact'
import RunRecovery from './RunRecovery.vue'

const props = defineProps<{ token: string; scope?: string; execution?: ExecutionLimits; graph?: Graph; projectId?: string; selectedNode?: string; initialRunId?: string; prepare?: () => Promise<string>; compact?: boolean }>()
const emit = defineEmits<{ expired: []; close: []; selected: [run: Run]; unavailable: [id: string]; project: [id: string] }>()
const rows = ref<Run[]>([]), total = ref(0), page = ref(1), ledgerPage = ref(1), ledgerTotal = ref(0)
const account = ref<CreditAccount>({ available: '0', held: '0', spent: '0' }), events = ref<CreditEvent[]>([])
const selected = ref<Run | null>(null), busy = ref(false), error = ref(''), tab = ref<'history' | 'credits' | 'usage' | 'batches'>('history')
const scenario = ref('success'), baseRunId = ref(''), unconfirmed = ref(false)
const managed = computed(() => props.execution?.mode === 'managed')
let intentVersion = 0
const realMode = computed(() => ['qwen', 'managed'].includes(props.execution?.mode || ''))
const supported = computed(() => props.execution?.mode === 'simulation' || realMode.value)
const paidConfirmed = ref(false)
const retryPaidConfirmed = ref(false)
watch(() => JSON.stringify([props.graph, props.selectedNode, baseRunId.value, props.execution?.mode]), () => { paidConfirmed.value = false; ++intentVersion }, { flush: 'sync' })
const eventLabels: Record<string, string> = { grant: '初始测试额度', reserve: '预占', settle: '结算', release: '释放' }
const modeLabels = { workflow: '整条流程', node: '单节点', polish: 'AI 润色', branch: '节点及上游' }
const baseRuns = computed(() => rows.value.filter(run => run.mode !== 'polish' && (run.executionMode ? run.executionMode === props.execution?.mode : run.testOnly !== realMode.value)))
let disposed = false, timer: ReturnType<typeof setTimeout> | undefined, pendingBody: Record<string, unknown> | null = null
let selectionSequence = 0, refreshSequence = 0, pollSequence = 0, wantedId = ''
const retryKeys = new Map<string, { requestId: string; quoteId?: string }>()
const retryPending = ref(false)
function retryKey(id: string) { return `pod-studio.retry:${props.scope || 'session'}:${id}` }
function readRetry(id: string) {
  if (retryKeys.has(id)) return retryKeys.get(id)
  const raw = sessionStorage.getItem(retryKey(id))
  if (!raw) return undefined
  const entry = JSON.parse(raw) as { requestId: string; quoteId?: string }
  if (!/^[0-9a-f-]{36}$/.test(entry.requestId) || entry.quoteId !== undefined && !/^[0-9a-f-]{36}$/.test(entry.quoteId)) throw new Error('恢复请求记录无法读取，请先核对运行状态。')
  retryKeys.set(id, entry); return entry
}
function storeRetry(id: string, body?: { requestId: string; quoteId?: string }) {
  if (body) { sessionStorage.setItem(retryKey(id), JSON.stringify(body)); retryKeys.set(id, body) }
  else { sessionStorage.removeItem(retryKey(id)); retryKeys.delete(id) }
  retryPending.value = !!body
}
const submissionKey = props.scope && props.projectId ? `pod-studio.pending-run:${props.scope}:${props.projectId}` : ''
function rememberSubmission() {
  if (!submissionKey) return
  try { if (pendingBody) sessionStorage.setItem(submissionKey, JSON.stringify(pendingBody)); else sessionStorage.removeItem(submissionKey) }
  catch { throw new Error('浏览器无法保留提交标识，请先恢复浏览器存储，再重试原提交。') }
}
function report(e: unknown) { error.value = e instanceof Error ? e.message : '请求失败'; if (e instanceof APIError && e.code === 401) emit('expired') }
async function reportWrite(e: unknown, required?: string) {
  if (disposed) return
  if (!isPointsShortage(e)) { report(e); return }
  error.value = '点数不足，本次未提交。'
  const message = await pointsShortageMessage(props.token, required)
  if (!disposed) error.value = message
}
async function api<T>(path: string, options: RequestInit = {}) { return request<T>(path, options, props.token) }
async function pick(id: string) {
  if (busy.value) return
  wantedId = id; const current = ++selectionSequence
  if (selected.value?.runId !== id) retryPaidConfirmed.value = false
  try { const result = await api<{ data: Run }>(`/pod/runs/${encodeURIComponent(id)}`); if (!disposed && current === selectionSequence) { if (result.data.runId !== id || (props.projectId && result.data.projectId !== props.projectId)) throw new Error('运行记录与当前项目不匹配'); selected.value = result.data; retryPending.value = !!readRetry(id); emit('selected', result.data) } }
  catch (e) { if (!disposed && current === selectionSequence) { if (e instanceof APIError && [401, 403, 404].includes(e.code)) { selected.value = null; wantedId = ''; emit('unavailable', id) }; report(e) } }
}
watch(() => props.initialRunId, id => { if (id && wantedId !== id) void pick(id) }, { immediate: true })
async function refresh() {
  const current = ++refreshSequence
  try {
    const [runs, ledger] = await Promise.all([
      api<{ rows: Run[]; total: number }>(`/pod/runs?pageNum=${page.value}&pageSize=10${props.projectId ? `&projectId=${props.projectId}` : ''}`),
      api<{ account: CreditAccount; rows: CreditEvent[]; total: number }>(`/pod/credits?pageNum=${ledgerPage.value}&pageSize=10`),
    ])
    if (disposed || current !== refreshSequence) return
    rows.value = runs.rows; total.value = runs.total; account.value = ledger.account; events.value = ledger.rows; ledgerTotal.value = ledger.total
    if (wantedId && (!selected.value || selected.value.runId !== wantedId || !terminalRun(selected.value.state))) await pick(wantedId)
  } catch (e) {
    if (!disposed && current === refreshSequence) {
      if (e instanceof APIError && [401, 403].includes(e.code)) {
        const id = wantedId; ++selectionSequence; wantedId = ''; selected.value = null; rows.value = []; events.value = []
        if (id) emit('unavailable', id)
      }
      report(e)
    }
  }
}
async function execute(mode: 'workflow' | 'node') {
  if (busy.value || !props.projectId || !props.prepare) return
  busy.value = true; ++selectionSequence; error.value = ''
  let required: string | undefined
  try {
    if (!pendingBody) {
      const version = intentVersion
      const revision = await props.prepare()
      if (disposed) return
      if (version !== intentVersion) throw new Error('执行范围已变化，本次未提交，请重新操作。')
      if (realMode.value && !managed.value && !paidConfirmed.value) throw new Error('请先确认本次执行会调用付费模型')
      const draft = { projectId: props.projectId, revision, mode, nodeId: mode === 'node' ? props.selectedNode || '' : '', baseRunId: mode === 'node' ? baseRunId.value : '', requestId: createUUID(), ...(realMode.value ? { executionMode: props.execution!.mode } : { executionMode: 'simulation', testScenario: scenario.value }) }
      const quote = managed.value ? await executionQuote(props.token, { kind: 'run', run: draft }) : null
      required = quote?.points
      if (disposed) return
      if (version !== intentVersion) throw new Error('执行范围已变化，本次未提交，请重新操作。')
      pendingBody = quote ? { ...draft, quoteId: quote.quoteId } : draft
      rememberSubmission()
    }
    rememberSubmission()
    const result = await api<{ data: Run }>('/pod/runs', { method: 'POST', body: JSON.stringify(pendingBody) })
    if (disposed) return
    selected.value = result.data; wantedId = result.data.runId; emit('selected', result.data); pendingBody = null; rememberSubmission(); unconfirmed.value = false; paidConfirmed.value = false; page.value = 1; tab.value = 'history'; await refresh()
  } catch (e) {
    if (e instanceof APIError && [400, 403, 404, 409, 429].includes(e.code)) { pendingBody = null; rememberSubmission(); unconfirmed.value = false }
    else unconfirmed.value = !!pendingBody
    await reportWrite(e, required)
  } finally { busy.value = false }
}
async function change(action: 'cancel' | 'retry') {
  if (!selected.value || busy.value) return
  const id = selected.value.runId
  let intent: { requestId: string; quoteId?: string } | undefined
  try { intent = action === 'retry' ? readRetry(id) : undefined } catch (e) { report(e); return }
  if (action === 'retry' && !intent && !canRetryRun(selected.value)) { error.value = '请按任务处理建议修改输入或核对原调用'; return }
  if (action === 'retry' && !selected.value.billing && !selected.value.testOnly && !retryPaidConfirmed.value && !intent) {
    error.value = '请先确认恢复失败步骤可能再次调用千问并计费'; return
  }
  busy.value = true; ++selectionSequence; error.value = ''
  let required: string | undefined
  try {
    if (action === 'retry' && !intent) {
      const quote = selected.value.billing ? await executionQuote(props.token, { kind: 'retry', runId: id }) : null
      required = quote?.points
      if (disposed) return
      if (selected.value?.runId !== id) throw new Error('所选运行已变化，本次未提交，请重新操作。')
      intent = { requestId: createUUID(), ...(quote ? { quoteId: quote.quoteId } : {}) }; storeRetry(id, intent)
    }
    const result = await api<{ data: Run }>(`/pod/runs/${id}/${action}`, { method: 'POST', body: JSON.stringify(intent || {}) })
    if (!disposed) { selected.value = result.data; emit('selected', result.data); if (action === 'retry') storeRetry(id); retryPaidConfirmed.value = false; await refresh() }
  } catch (e) {
    if (action === 'retry' && e instanceof APIError && [400, 403, 404, 409, 429].includes(e.code)) storeRetry(id)
    await reportWrite(e, required)
  } finally { busy.value = false }
}
function canPoll() { return document.visibilityState !== 'hidden' && navigator.onLine !== false }
async function poll() {
  const current = ++pollSequence
  clearTimeout(timer)
  if (disposed || !canPoll()) return
  await refresh()
  if (!disposed && current === pollSequence && canPoll()) timer = setTimeout(poll, 2000)
}
function resume() { clearTimeout(timer); void poll() }
onMounted(() => {
  document.addEventListener('visibilitychange', resume); window.addEventListener('online', resume)
  if (submissionKey) {
    try {
      const raw = sessionStorage.getItem(submissionKey)
      if (raw) {
        const candidate = JSON.parse(raw) as Record<string, unknown>
        if (candidate.projectId !== props.projectId || typeof candidate.requestId !== 'string' || !/^[A-Za-z0-9_-]{16,64}$/.test(candidate.requestId) || typeof candidate.revision !== 'string' || !['workflow', 'node'].includes(String(candidate.mode))) throw new Error('invalid pending request')
        pendingBody = candidate; unconfirmed.value = true
      }
    } catch { error.value = '待确认提交记录无法读取，请先检查运行历史。' }
  }
  void poll()
})
onBeforeUnmount(() => { disposed = true; ++selectionSequence; ++refreshSequence; ++pollSequence; clearTimeout(timer); document.removeEventListener('visibilitychange', resume); window.removeEventListener('online', resume) })
</script>

<template>
  <section class="runs-workbench" :class="{ 'runs-compact': compact }" :aria-label="compact ? '执行记录' : '任务与结果'">
    <div class="runs-heading"><div><h1>{{ compact ? '执行记录' : '任务与结果' }}</h1><p v-if="!managed">{{ realMode ? '模型调用会产生费用；本地处理不计模型费。' : '测试模式，不调用付费模型。' }}</p></div><button v-if="compact" class="icon-button" aria-label="关闭运行记录" @click="emit('close')"><PhX :size="18" /></button></div>
    <div class="test-mode-note">{{ managed ? '每个 AI 节点可选择已授权的模型；执行前由服务端报价。' : realMode ? `文字：${execution?.textModel}；图片：${execution?.imageModel}。先核对输入与图片数量。` : '可执行图片输入、本地图片处理、T 恤预览与导出；AI 文本为模拟结果。' }}</div>
    <div v-if="projectId && supported" class="run-submit">
      <label v-if="!realMode">测试场景<select v-model="scenario" :disabled="busy || unconfirmed"><option value="success">正常完成</option><option value="delay">延迟响应</option><option value="failOnce">首次失败，恢复成功</option><option value="fail">持续失败</option><option value="uncertain">响应丢失后确认</option></select></label>
      <label v-else-if="!managed" class="paid-run-confirm"><input v-model="paidConfirmed" type="checkbox" :disabled="busy || unconfirmed" />我已核对输入，确认本次执行会产生千问费用</label>
      <div class="run-actions"><button class="primary-button" :disabled="busy || (realMode && !managed && !paidConfirmed && !unconfirmed)" @click="execute('workflow')"><PhPlay :size="16" />{{ unconfirmed ? '重试原提交' : realMode ? '执行整条流程' : '测试整条流程' }}</button><button class="secondary-button" :disabled="busy || !selectedNode || unconfirmed || (realMode && !managed && !paidConfirmed)" @click="execute('node')">{{ realMode ? '仅执行选中节点' : '仅测试选中节点' }}</button></div>
      <label>使用哪次生成结果<select v-model="baseRunId" :disabled="busy || unconfirmed" aria-describedby="reuse-result-help"><option value="">不复用之前的结果</option><option v-for="run in baseRuns" :key="run.runId" :value="run.runId">#{{ run.runId }} · 画布版本 {{ run.projectRevision }} · {{ runStateLabel[run.state] }}</option></select></label>
      <p id="reuse-result-help" class="muted">只影响“仅执行选中节点”。选择之前同画布版本的成功结果，可以接着处理，不必重新生成。</p>
      <p class="muted">{{ realMode ? '先保存再执行。单节点只复用同版本、同模式的成功上游；取消只停止后续步骤，已调用部分仍可能计费。' : '先保存画布再执行。单节点只复用同版本的成功上游；测试文本成功每节点消耗 1 单位。' }}</p>
      <RunImpact v-if="!unconfirmed" :graph="graph" mode="workflow" :real="realMode" :managed="managed" />
      <RunImpact v-if="selectedNode && !unconfirmed" :graph="graph" mode="node" :target="selectedNode" :real="realMode" :managed="managed" />
      <p v-if="unconfirmed" role="status">上次提交结果未确认。重试会使用原请求标识和原版本，避免重复创建；不使用当前画布的新修改。</p>
    </div>
    <p v-if="error" class="error-message" role="alert">{{ error }}<button class="text-button" @click="refresh">重新检查</button></p>
    <div class="runs-tabs" aria-label="运行分类"><button :aria-pressed="tab === 'history'" @click="tab = 'history'">运行历史</button><button v-if="!projectId && scope" :aria-pressed="tab === 'batches'" @click="tab = 'batches'">批量</button><button v-if="!managed && !compact" :aria-pressed="tab === 'usage'" @click="tab = 'usage'">我的用量</button><button v-if="!realMode" :aria-pressed="tab === 'credits'" @click="tab = 'credits'">测试额度</button><button class="icon-button" aria-label="刷新运行记录" @click="refresh"><PhArrowClockwise :size="17" /></button></div>
    <Batches v-if="tab === 'batches' && scope" :token="token" :scope="scope" @expired="emit('expired')" @run="id => { tab = 'history'; void pick(id) }" @project="id => emit('project', id)" />
    <Usage v-if="tab === 'usage'" :token="token" :scope="scope" :project-id="projectId" @expired="emit('expired')" @open="id => { tab = 'history'; void pick(id) }" />
    <div v-if="tab === 'history'" class="run-history">
      <div class="run-history-list" :class="{ 'has-selection': selected }" role="group" aria-label="运行记录">
      <p v-if="!rows.length" class="empty-runs">还没有运行记录。在项目画布中打开执行面板开始。</p>
      <button v-for="run in rows" :key="run.runId" class="run-row" :class="{ selected: selected?.runId === run.runId }" :aria-current="selected?.runId === run.runId ? 'true' : undefined" @click="pick(run.runId)"><span><strong>{{ run.projectName }}</strong><small>#{{ run.runId }} · {{ modeLabels[run.mode] }} · 版本 {{ run.projectRevision }} · 第 {{ run.attempt }} 次</small></span><span class="run-state" :data-state="run.state">{{ runStateLabel[run.state] }}</span><time>{{ run.createTime }}</time></button>
      <div v-if="compact || total > 10" class="run-pagination"><button class="text-button" :disabled="page <= 1" @click="page--; refresh()">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(total / 10)) }}</span><button class="text-button" :disabled="page * 10 >= total" @click="page++; refresh()">下一页</button></div>
      </div>
      <article v-if="selected" class="run-detail" aria-label="运行详情">
        <div class="run-detail-heading"><strong>运行 #{{ selected.runId }}</strong><span class="run-state" :data-state="selected.state">{{ runStateLabel[selected.state] }}</span></div>
        <BillingSummary v-if="selected.billing" :billing="selected.billing" /><p class="muted">画布版本 {{ selected.projectRevision }} · {{ selected.billing ? '正式点数模式，明细见权益与点数页' : selected.testOnly ? `测试消耗 ${selected.spent} · 预占 ${selected.reserved}` : '千问真实调用；下列费用为估算，以供应商账单为准' }}</p>
        <label v-if="!selected.testOnly && !selected.billing && (canRetryRun(selected) || retryPending)" class="paid-run-confirm"><input v-model="retryPaidConfirmed" type="checkbox" :disabled="busy" />我确认恢复失败步骤可能再次调用千问并计费</label>
        <div class="run-actions"><button v-if="!terminalRun(selected.state) && selected.state !== 'cancelRequested'" class="secondary-button" :disabled="busy" @click="change('cancel')"><PhStop :size="16" />取消后续步骤</button><button v-if="canRetryRun(selected) || retryPending" class="secondary-button" :disabled="busy || (!selected.testOnly && !selected.billing && !retryPaidConfirmed && !retryPending)" @click="change('retry')"><PhArrowClockwise :size="16" />{{ retryPending ? '重试原恢复提交' : '恢复失败步骤' }}</button></div>
        <RunRecovery :run="selected" @refresh="pick(selected!.runId)" /><button v-if="runRecovery(selected)?.newRun" class="secondary-button" @click="compact ? emit('close') : emit('project',selected.projectId)">返回画布修改</button>
        <p v-if="!selected.testOnly && !selected.billing" class="muted">节点下方只显示当前结果的费用；包含历史恢复尝试的累计费用请看“我的用量”。</p>
        <div v-for="node in selected.nodes" :key="node.nodeId" class="run-node-result">
          <div><strong>{{ selected.graph?.nodes.find(n => n.id === node.nodeId)?.config.title || node.nodeId }}</strong><span class="run-state" :data-state="node.state">{{ runStateLabel[node.state] }}</span></div>
          <p v-if="node.errorMessage" class="run-node-error">{{ node.errorMessage }}</p>
          <p v-if="node.output.usage && !selected.billing" class="muted">{{ node.output.usage.model }} · {{ costLabel(node.output.usage.estimatedCostMicros) }} · Token {{ node.output.usage.inputTokens }}/{{ node.output.usage.outputTokens }} · {{ node.output.usage.imageCount }} 张</p>
          <pre v-if="node.output.text">{{ node.output.text }}</pre>
          <ResultImages :compare="node.nodeType === 'imageProcess'" v-if="node.output.assetIds?.length" :ids="node.output.assetIds" :info="node.output.images" :token="token" @expired="emit('expired')" />
        </div>
      </article>
    </div>
    <div v-if="tab === 'credits'" class="run-credits"><div class="credit-balances"><span>可用<strong>{{ account.available }}</strong></span><span>预占<strong>{{ account.held }}</strong></span><span>累计消耗<strong>{{ account.spent }}</strong></span></div><p class="muted">仅测试额度，无货币价值。首次提交运行时一次性发放 {{ execution?.initialCredits || '1000' }} 单位；失败和取消按已确认结果结算并释放剩余额度。</p><div v-for="event in events" :key="event.eventId" class="credit-event"><span>{{ eventLabels[event.kind] || event.kind }}<small>{{ event.createTime }} · {{ event.runId === '0' ? '账户' : `运行 #${event.runId}` }}</small></span><strong>{{ event.amount }}</strong></div><div class="run-pagination"><button class="text-button" :disabled="ledgerPage <= 1" @click="ledgerPage--; refresh()">上一页</button><span>{{ ledgerPage }} / {{ Math.max(1, Math.ceil(ledgerTotal / 10)) }}</span><button class="text-button" :disabled="ledgerPage * 10 >= ledgerTotal" @click="ledgerPage++; refresh()">下一页</button></div></div>
  </section>
</template>
