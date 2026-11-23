<script setup lang="ts">
import { createUUID } from './uuid'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PhArrowClockwise, PhMagicWand, PhPlay, PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { ExecutionLimits, Graph, NodeType, Run } from './contracts'
import { costLabel, runStateLabel, terminalRun } from './run-state'
import ResultImages from './ResultImages.vue'
import { executionQuote, isPointsShortage, pointsShortageMessage } from './execution-quote'
import BillingSummary from './BillingSummary.vue'
import RunImpact from './RunImpact.vue'
import RunRecovery from './RunRecovery.vue'

const props = defineProps<{ token: string; scope: string; projectId: string; nodeId: string; nodeType?: NodeType; graph?: Graph; action: 'polish' | 'branch'; sourceText: string; execution?: ExecutionLimits; prepare: () => Promise<string>; autoStart?: boolean }>()
const emit = defineEmits<{ close: []; apply: [text: string, original: string]; selected: [run: Run]; unavailable: [id: string]; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), busy = ref(false), error = ref(''), paid = ref(false), run = ref<Run | null>(null), original = ref(''), unknown = ref(false), storageOK = ref(true)
const localImage = computed(() => ['imageProcess', 'productPreview', 'export'].includes(props.nodeType || ''))
const managed = computed(() => props.execution?.mode === 'managed')
let intentVersion = 0
const real = computed(() => ['qwen', 'managed'].includes(props.execution?.mode || ''))
const textAction = computed(() => props.action === 'polish' || props.nodeType === 'textGenerate')
const supported = computed(() => ['qwen', 'simulation', 'managed'].includes(props.execution?.mode || ''))
const title = computed(() => props.action === 'polish' ? 'AI 润色' : localImage.value ? '制作当前节点' : '生成当前节点')
watch(() => JSON.stringify([props.graph, props.sourceText, props.nodeId, props.action, props.execution?.mode]), () => { paid.value = false; ++intentVersion }, { flush: 'sync' })
const result = computed(() => run.value?.nodes?.find(node => node.nodeId === props.nodeId))
const changed = computed(() => props.sourceText !== original.value)
const canApply = computed(() => run.value?.state === 'succeeded' && !!result.value?.output.text && !changed.value && [...result.value.output.text].length <= 12000)
const key = `pod-studio.node-action:${props.scope}:${props.projectId}:${props.action}:${props.nodeId}`
let body: Record<string, unknown> | null = null, disposed = false, refreshSequence = 0, timer: ReturnType<typeof setTimeout> | undefined, focus: HTMLElement | null = null
function persist() {
  try {
    if (body) sessionStorage.setItem(key, JSON.stringify({ body, runId: run.value?.runId || '', original: original.value }))
    else sessionStorage.removeItem(key)
  } catch { storageOK.value = false; error.value = '浏览器无法保留提交标识，请查看运行记录确认结果后再操作。' }
}
function report(e: unknown) { error.value = e instanceof Error ? e.message : '操作失败'; if (e instanceof APIError && e.code === 401) emit('expired') }
function update(value: Run) { run.value = value; if (props.action === 'branch') emit('selected', value) }
function matches(value: Run, id: string) { return /^[1-9]\d{0,19}$/.test(id) && value.runId === id && value.projectId === props.projectId && value.nodeId === props.nodeId && value.mode === props.action }
async function refresh() {
  if (!run.value || disposed) return
  clearTimeout(timer)
  const id = run.value.runId, current = ++refreshSequence
  try {
    if (managed.value) { persist(); if (!storageOK.value) return }
    const response = await request<{ data: Run }>(`/pod/runs/${id}`, {}, props.token)
    if (disposed || current !== refreshSequence) return
    if (!matches(response.data, id)) throw new APIError('运行记录与当前节点不匹配，请从运行记录核对。', 404)
    update(response.data); error.value = ''; persist()
  } catch (e) {
    if (disposed || current !== refreshSequence) return
    if (e instanceof APIError && [401, 403, 404].includes(e.code)) {
      run.value = null; storageOK.value = false; emit('unavailable', id)
    }
    report(e)
  }
  if (!disposed && current === refreshSequence && run.value && !terminalRun(run.value.state) && document.visibilityState !== 'hidden' && navigator.onLine !== false) timer = setTimeout(refresh, 2000)
}
async function execute() {
  if (busy.value || !supported.value || !storageOK.value || (real.value && !managed.value && !paid.value && !unknown.value)) return
  busy.value = true; error.value = ''
  let required: string | undefined
  try {
    if (!body) {
      const version = intentVersion
      const revision = await props.prepare()
      if (disposed) return
      if (version !== intentVersion) throw new Error('输入已变化，本次未提交，请重新操作。')
      if (real.value && !managed.value && !paid.value) throw new Error('执行范围已变化，请重新核对并确认费用')
      original.value = props.sourceText
      const draft = { projectId: props.projectId, revision, mode: props.action, nodeId: props.nodeId, requestId: createUUID(), executionMode: props.execution?.mode || 'simulation' }
      const quote = managed.value ? await executionQuote(props.token, { kind: 'run', run: draft }) : null
      required = quote?.points
      if (disposed) return
      if (version !== intentVersion) throw new Error('输入已变化，本次未提交，请重新操作。')
      body = quote ? { ...draft, quoteId: quote.quoteId } : draft
      persist()
      if (!storageOK.value) { body = null; return }
    }
    const response = await request<{ data: Run }>('/pod/runs', { method: 'POST', body: JSON.stringify(body) }, props.token)
    if (disposed) return
    if (!matches(response.data, response.data.runId)) throw new Error('提交响应与当前节点不匹配，请核对原提交。')
    update(response.data); unknown.value = false; paid.value = false; persist(); void refresh()
  } catch (e) {
    if (e instanceof APIError && [400, 401, 403, 404, 409, 429].includes(e.code)) { body = null; unknown.value = false; persist() }
    else unknown.value = !!body
    if (!disposed && isPointsShortage(e)) {
      error.value = '点数不足，本次未提交。'
      const message = await pointsShortageMessage(props.token, required)
      if (!disposed) error.value = message
    } else if (!disposed) report(e)
  } finally { busy.value = false }
}
function discard() { body = null; run.value = null; persist(); emit('close') }
function refreshNow() { clearTimeout(timer); void refresh() }
function resume() { clearTimeout(timer); if (document.visibilityState !== 'hidden' && navigator.onLine !== false) void refresh() }
function apply() { if (canApply.value && result.value?.output.text) { emit('apply', result.value.output.text, original.value); body = null; persist(); emit('close') } }
onMounted(() => {
  document.addEventListener('visibilitychange', resume); window.addEventListener('online', resume)
  focus = document.activeElement as HTMLElement; dialog.value?.showModal()
  try {
    const cached = sessionStorage.getItem(key)
    if (cached) {
      const entry = JSON.parse(cached) as { body: Record<string, unknown>; runId: string; original: string }
      if (entry.body?.projectId !== props.projectId || entry.body.nodeId !== props.nodeId || entry.body.mode !== props.action || !/^[A-Za-z0-9_-]{16,64}$/.test(String(entry.body.requestId)) || !/^[1-9][0-9]*$/.test(String(entry.body.revision)) || typeof entry.original !== 'string' || (entry.runId && !/^[1-9][0-9]*$/.test(entry.runId))) throw new Error('invalid action cache')
      body = entry.body; original.value = entry.original
      if (entry.runId) { run.value = { runId: entry.runId } as Run; void refresh() }
      else unknown.value = true
    }
    else if (props.autoStart && managed.value) void execute()
  } catch { storageOK.value = false; error.value = '待确认的节点操作无法读取，请先查看运行记录。' }
})
onBeforeUnmount(() => { disposed = true; ++refreshSequence; clearTimeout(timer); document.removeEventListener('visibilitychange', resume); window.removeEventListener('online', resume); dialog.value?.close(); focus?.focus() })
</script>

<template>
  <dialog ref="dialog" class="node-action-dialog" aria-labelledby="node-action-title" @cancel.prevent="emit('close')">
    <header><div><h2 id="node-action-title"><PhMagicWand v-if="action === 'polish'" :size="20" /><PhPlay v-else :size="20" />{{ title }}</h2><p>{{ action === 'polish' ? '保留你的主题，完善构图、风格和细节。' : '执行当前节点和所需上游，不执行其他分支。' }}</p></div><button class="icon-button" aria-label="关闭节点操作" @click="emit('close')"><PhX :size="20" /></button></header>
    <div class="node-action-body">
      <template v-if="!run">
        <template v-if="action === 'polish'"><label>原文</label><p class="action-source">{{ sourceText }}</p></template>
        <p class="action-model">{{ localImage ? '图片在本机处理，结果保存到私有素材库' : managed ? '按所选模型执行，系统自动核对点数' : real ? (textAction ? execution?.textModel : execution?.imageModel) : '模拟执行，不调用真实模型' }}</p>
        <RunImpact v-if="!unknown" :graph="graph" :mode="action" :target="nodeId" :real="real" :managed="managed" />
        <p v-if="real && !managed && !localImage" class="muted">{{ textAction ? '按 Token 用量计费，每个文本调用先预占 0.025 元，完成后按用量估算结算。' : '千问 Turbo 当前估算 0.10 元/张，上游文字费用另计。' }}</p>
        <label v-if="real && !managed" class="paid-run-confirm"><input v-model="paid" type="checkbox" :disabled="busy || unknown" />{{ localImage ? '已核对上游，确认执行（若上游含 AI 生成仍可能计费）' : '确认本次操作调用千问并可能产生费用' }}</label>
      </template>
      <template v-else>
        <BillingSummary v-if="run.billing" :billing="run.billing" /><p class="action-state" role="status">{{ runStateLabel[run.state] || '正在读取运行状态' }} <small>运行 #{{ run.runId }}</small></p>
        <p v-if="result?.output.usage && !run.billing" class="action-model">{{ result.output.usage.model }} · 估算 {{ costLabel(result.output.usage.estimatedCostMicros) }}</p>
        <div v-if="result?.output.text" class="polish-result"><label>{{ run.testOnly ? '模拟结果' : action === 'polish' ? '润色结果' : '文本结果' }}</label><p>{{ result.output.text }}</p></div>
        <ResultImages :compare="result?.nodeType === 'imageProcess'" v-if="result?.output.assetIds?.length" :ids="result.output.assetIds" :info="result.output.images" :token="token" @expired="emit('expired')" />
        <p v-if="result?.errorMessage" class="error-message" role="alert">{{ result.errorMessage }}</p>
        <RunRecovery :run="run" @refresh="refreshNow" />
        <p v-if="!run.testOnly && !run.billing" class="muted">此处为当前节点结果；包含上游和恢复尝试的累计费用请看运行中的“我的用量”。</p>
        <p v-if="action === 'polish' && result?.output.text && changed" class="error-message" role="alert">原文已修改，为避免覆盖新内容，请保留当前文字。润色结果仍在运行记录中。</p>
        <p v-if="action === 'polish' && result?.output.text && [...result.output.text].length > 12000" class="error-message">结果超过文本节点上限，请从运行记录复制需要的部分。</p>
      </template>
      <p v-if="unknown" class="muted" role="status">上次提交结果未确认。只能重试原提交，不会创建新请求，也不使用当前画布的新修改。</p>
      <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    </div>
    <footer>
      <button v-if="run && terminalRun(run.state)" class="text-button" @click="discard">{{ action === 'polish' ? '保留原文' : '完成' }}</button>
      <button v-else class="secondary-button" @click="emit('close')">{{ run || unknown ? '稍后查看' : '取消' }}</button>
      <button v-if="!run" class="primary-button" :disabled="busy || !supported || !storageOK || (real && !managed && !paid && !unknown)" @click="execute">{{ busy ? '正在提交…' : unknown ? '重试原提交' : action === 'polish' ? '开始润色' : '开始生成' }}</button>
      <button v-else-if="action === 'polish' && run.state === 'succeeded'" class="primary-button" :disabled="!canApply" @click="apply">替换原文</button>
      <button v-else-if="error" class="secondary-button" @click="refreshNow"><PhArrowClockwise :size="16" />刷新状态</button>
    </footer>
  </dialog>
</template>
