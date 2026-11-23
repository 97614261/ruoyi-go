<script setup lang="ts">
import { createUUID } from './uuid'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PhPlus, PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { AssetLimits, Batch, BatchImpact, BatchInput, BatchRequest, TemplateVersion } from './contracts'
import { pendingBatch, retryBatch, submitBatch } from './batch-requests'
import { costLabel } from './run-state'
import { executionQuote, isPointsShortage, pointsShortageMessage } from './execution-quote'
import AssetPicker from './AssetPicker.vue'
import PrivateImage from './PrivateImage.vue'
const props = defineProps<{ name: string; snapshot: TemplateVersion; token: string; scope: string; limits: AssetLimits; mode: string }>()
const emit = defineEmits<{ close: []; created: [batch: Batch]; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), name = ref(`${props.name}批量`), items = ref<BatchInput[]>([]), busy = ref(false), error = ref(''), consent = ref(false), impact = ref<BatchImpact | null>(null), pending = ref(false)
const picker = ref<{ index: number; nodeId: string } | null>(null)
const requestId = createUUID()
const body = computed<BatchRequest>(() => ({ requestId, templateId: props.snapshot.templateId, version: props.snapshot.version, name: name.value, executionMode: props.mode, items: items.value }))
let focus: HTMLElement | null = null, disposed = false
function report(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
function refreshPending() { try { pending.value = !!pendingBatch(props.scope) } catch (e) { pending.value = true; report(e) } }
function add(from?: number) {
  const source = from === undefined ? { name: `${props.name} ${items.value.length + 1}`, bindings: (props.snapshot.slots || []).map(slot => slot.type === 'textInput' ? { nodeId: slot.nodeId, text: slot.text || '' } : { nodeId: slot.nodeId, assetIds: [...(slot.assetIds || [])] }) } : { ...items.value[from]!, name: `${props.name} ${items.value.length + 1}` }
  items.value.push(JSON.parse(JSON.stringify(source)) as BatchInput)
}
function imagesChanged(ids: string[]) { if (!picker.value) return; const b = items.value[picker.value.index]?.bindings.find(b => b.nodeId === picker.value!.nodeId); if (b) b.assetIds = ids; picker.value = null }
async function preview() {
  busy.value = true; error.value = ''; const draft = JSON.stringify(body.value)
  try { const result = await request<{ data: BatchImpact }>('/pod/batches/preview', { method: 'POST', body: draft }, props.token); if (!disposed && draft === JSON.stringify(body.value)) impact.value = result.data }
  catch (e) { if (!disposed) report(e) } finally { busy.value = false }
}
async function submit() {
  if (busy.value || (!pending.value && (!impact.value || (props.mode === 'qwen' && !consent.value)))) return
  busy.value = true; error.value = ''
  let required: string | undefined
  try {
    let batch: Batch
    if (pending.value) batch = await retryBatch(props.scope, props.token)
    else {
      const draft = JSON.parse(JSON.stringify(body.value)) as BatchRequest
      const quote = props.mode === 'managed' ? await executionQuote(props.token, { kind: 'batch', batch: draft }) : null
      required = quote?.points
      if (disposed) return
      if (JSON.stringify(draft) !== JSON.stringify(body.value)) throw new Error('批量输入已变化，本次未提交，请重新核对。')
      batch = await submitBatch(props.scope, props.token, quote ? { ...draft, quoteId: quote.quoteId } : draft)
    }
    if (!disposed) emit('created', batch)
  }
  catch (e) {
    if (!disposed && isPointsShortage(e)) {
      error.value = '点数不足，本次未提交。'
      const message = await pointsShortageMessage(props.token, required)
      if (!disposed) error.value = message
    } else if (!disposed) report(e)
  } finally { busy.value = false; refreshPending() }
}
watch(() => JSON.stringify(body.value), () => { impact.value = null; consent.value = false }, { flush: 'sync' })
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal(); add(); refreshPending() })
onBeforeUnmount(() => { disposed = true; dialog.value?.close(); focus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="batch-dialog" aria-labelledby="batch-title" @cancel.prevent="!busy && emit('close')">
    <header class="template-dialog-header"><div><h2 id="batch-title">批量制作</h2><p>{{ props.name }} · 固定版本 v{{ snapshot.version }} · 每批最多 10 份</p></div><button class="icon-button" aria-label="关闭批量制作" :disabled="busy" @click="emit('close')"><PhX :size="20" /></button></header>
    <form @submit.prevent="submit">
      <fieldset :disabled="busy || pending" class="batch-inputs"><label class="field">批次名称<input v-model="name" maxlength="100" required /></label>
        <article v-for="(item, index) in items" :key="index" class="batch-input-row"><div class="batch-input-heading"><strong>第 {{ index + 1 }} 份</strong><div><button type="button" class="text-button" :disabled="items.length >= 10" @click="add(index)">复制输入</button><button type="button" class="text-button" :disabled="items.length === 1" @click="items.splice(index, 1)">删除此份</button></div></div><label class="field">项目名称<input v-model="item.name" maxlength="100" required /></label>
          <div v-for="binding in item.bindings" :key="binding.nodeId" class="batch-slot"><label v-if="binding.text !== undefined" class="field">{{ snapshot.slots?.find(s => s.nodeId === binding.nodeId)?.label }}<textarea v-model="binding.text" rows="2" maxlength="12000" /></label><template v-else><div class="template-slot-heading"><strong>{{ snapshot.slots?.find(s => s.nodeId === binding.nodeId)?.label }}</strong><button type="button" class="secondary-button" @click="picker = { index, nodeId: binding.nodeId }">换图片（{{ binding.assetIds?.length || 0 }}）</button></div><div class="batch-thumbnails"><PrivateImage v-for="id in binding.assetIds" :key="id" :token="token" :asset-id="id" alt="本份输入图片" @expired="emit('expired')" /></div></template></div>
        </article><button type="button" class="secondary-button" :disabled="items.length >= 10" @click="add()"><PhPlus :size="17" />添加一份（{{ items.length }}/10）</button>
      </fieldset>
      <footer class="batch-confirm"><p>每份独立执行，不会自动组合输入。停止只取消待派发项；已调用部分仍可能计费。</p><button v-if="!pending" type="button" class="secondary-button" :disabled="busy" @click="preview">{{ busy ? '正在核对…' : '核对份数与费用' }}</button>
        <div v-if="impact" class="batch-impact" role="status"><strong>{{ impact.items }} 份</strong><span>文本 {{ impact.textCalls }} 次</span><span>生图 {{ impact.imageCalls }} 次 / {{ impact.images }} 张</span><span>{{ mode === 'managed' ? '开始批量后自动核价并预占点数' : mode === 'qwen' ? `累计预占估算 ${costLabel(impact.reservationMicros)}` : `测试额度 ${impact.testCredits}` }}</span><p>预览不占预算。执行时逐项检查现有并发及累计预算，预算不足可能只有部分成功；费用以供应商账单为准。</p></div>
        <label v-if="mode === 'qwen' && impact" class="paid-run-confirm"><input v-model="consent" type="checkbox" :disabled="busy" />我已核对全部输入，确认批量调用千问会产生费用</label>
        <p v-if="pending" class="error-message" role="status">上次提交结果待确认。重试只核对原批次，不使用当前新输入。</p><p v-if="error" class="error-message" role="alert">{{ error }}</p>
        <div class="dialog-actions"><button type="button" class="secondary-button" :disabled="busy" @click="emit('close')">关闭</button><button class="primary-button" :disabled="busy || (!pending && (!impact || (mode === 'qwen' && !consent)))">{{ busy ? '处理中…' : pending ? '重试原批次提交' : '开始批量' }}</button></div>
      </footer>
    </form>
    <AssetPicker v-if="picker" :key="`${picker.index}:${picker.nodeId}`" :token="token" :limits="limits" :initial="items[picker.index]?.bindings.find(b => b.nodeId === picker!.nodeId)?.assetIds || []" @confirm="imagesChanged" @close="picker = null" @expired="emit('expired')" />
  </dialog>
</template>
