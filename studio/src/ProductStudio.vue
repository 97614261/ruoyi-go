<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { PhArrowLeft, PhArrowRight, PhArrowsClockwise, PhBezierCurve, PhDownloadSimple, PhImage, PhLayout, PhPackage } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import AssetPicker from './AssetPicker.vue'
import Assets from './Assets.vue'
import PrivateImage from './PrivateImage.vue'
import ResultImages from './ResultImages.vue'
import type { Bootstrap, CreatorModel, ExecutionQuote, Run } from './contracts'
import { executionQuote, isPointsShortage, pointsShortageMessage } from './execution-quote'
import { pageDefaults } from './product-page'
import { runStateLabel, terminalRun } from './run-state'
import { createUUID } from './uuid'

type Fields = { productName: string; sku: string; sellingPoints: string; protectedDetails: string; referenceAssetId: string }
type Pack = Fields & { packId: string }
type WorkKind = 'product' | 'print' | 'mockup'
type Create = Fields & { modelId: string; ratio: string; requestId: string; workKind?: WorkKind; outputs?: string[] }
type Recent = { projectId: string; name: string; workKind?: WorkKind }
type ProductRun = { runId: string; projectRevision: string; runState: Run['state']; state: string; testOnly?: boolean; errorCode?: string; errorMessage?: string; output: NonNullable<Run['nodes']>[number]['output'] }
type ProductFinal = { runId: string; assetId: string; projectRevision: string; testOnly?: boolean }
type Slot = { nodeId: string; latest?: ProductRun; result?: ProductRun; final?: ProductFinal }
type Product = { project: Recent & { revision: string }; pack: Pack; quickRun: boolean; slots: Slot[]; workKind?: WorkKind }
type RunDraft = { projectId: string; revision: string; mode: 'workflow' | 'branch'; nodeId?: string; executionMode: 'managed'; requestId: string }
type RunSubmission = RunDraft & { quoteId: string }
type Offer = { body: RunSubmission; quote: ExecutionQuote }
type Page<T> = { rows: T[]; total: number }

const props = defineProps<{ projectId: string; token: string; bootstrap: Bootstrap }>()
const emit = defineEmits<{ open: [id: string]; canvas: [id: string]; expired: [] }>()
const ProductPageEditor = defineAsyncComponent(() => import('./ProductPageEditor.vue'))
const labels: Record<string, string> = { 'product-main': '白底主图', 'product-scene': '场景图', 'product-angle': '45° 展示图', 'product-side': '侧面图', 'product-detail': '细节特写' }
const workKinds: Record<WorkKind, string> = { product: '商品图', print: '整理印花', mockup: 'T 恤样机' }
const workKind = ref<WorkKind>('product'), outputs = ref(['product-main', 'product-scene'])
const pageSize = 12
const scope = 'pod-studio.product:' + props.bootstrap.userId + ':' + props.bootstrap.workspace.workspaceId
const createKey = scope + ':create'
const runKey = scope + ':run:' + props.projectId
const form = reactive<Fields>({ productName: '', sku: '', sellingPoints: '', protectedDetails: '', referenceAssetId: '' })
const selectedPack = ref(''), modelId = ref(''), ratio = ref(''), picker = ref(false)
const modelList = ref<CreatorModel[]>([]), packs = ref<Pack[]>([]), jobs = ref<Recent[]>([])
const packPage = ref(1), packTotal = ref(0), jobPage = ref(1), jobTotal = ref(0)
const view = ref<Product | null>(null), offer = ref<Offer | null>(null), balance = ref<string>()
const showPageEditor = ref(false)
const showPageAssets = ref(false), pageAssetsHeading = ref<HTMLElement>()
const pageAssetsRevision = ref(0)
const pendingCreate = ref<Create | null>(null), pendingRun = ref<RunSubmission | null>(null)
const historyNode = ref(''), history = ref<ProductRun[]>([]), historyPage = ref(1), historyTotal = ref(0), historyBusy = ref(false), historyError = ref('')
let historySequence = 0
const error = ref(''), statusError = ref(''), catalogError = ref(''), libraryError = ref(''), busy = ref(''), loading = ref(true), storageOK = ref(true)
const canEdit = computed(() => props.bootstrap.capabilities.projectEditing)
const managed = computed(() => props.bootstrap.execution?.mode === 'managed')
const creatingDisabled = computed(() => !canEdit.value || (workKind.value === 'product' && !managed.value))
const supportsRun = computed(() => props.bootstrap.capabilities.execution && canEdit.value && managed.value)
const model = computed(() => modelList.value.find(item => item.modelId === modelId.value))
const slots = computed<Slot[]>(() => view.value?.slots || [])
const resultIsProduct = computed(() => (view.value?.workKind || 'product') === 'product')
const polling = computed(() => slots.value.some(slot => active(slot.latest)))
const canDownload = computed(() => {
  const expected = slots.value[0]?.result
  return !!expected && slots.value.every(({ result }) => !!result?.output.assetIds?.length && result.projectRevision === expected.projectRevision && !!result.testOnly === !!expected.testOnly)
})
const hasFinals = computed(() => {
  const expected = slots.value[0]?.final
  return !!expected && slots.value.every(({ final }) => !!final && !!final.testOnly === !!expected.testOnly)
})
const shortage = computed(() => offer.value && balance.value !== undefined && BigInt(balance.value) < BigInt(offer.value.quote.points))
let alive = true, refreshSequence = 0, timer: ReturnType<typeof setTimeout> | undefined
let packsSequence = 0, jobsSequence = 0, focusOffer = false
const offerPanel = ref<HTMLElement>()

async function openPageAssets() {
  showPageEditor.value = false
  showPageAssets.value = true
  await nextTick()
  pageAssetsHeading.value?.focus()
}

function report(value: unknown) {
  if (value instanceof APIError && value.code === 401 && alive) emit('expired')
  return value instanceof Error ? value.message : '请求失败，请稍后重试'
}
function knownRejection(value: unknown) {
  return value instanceof APIError && [400, 401, 403, 404, 409, 429].includes(value.code)
}
function remember(key: string, value: Create | RunSubmission | null) {
  if (value) sessionStorage.setItem(key, JSON.stringify(value))
  else sessionStorage.removeItem(key)
}
function validRequest(value?: string) { return typeof value === 'string' && /^[A-Za-z0-9_-]{16,64}$/.test(value) }
function active(value?: ProductRun) {
  return Boolean(value && !terminalRun(value.runState) && !['succeeded', 'failed', 'blocked', 'cancelled'].includes(value.state))
}
function stateLabel(slot: Slot) {
  const latest = slot.latest
  if (!latest) return '待生成'
  if (latest.state === 'succeeded') return slot.result ? '已完成' : '没有可下载的图片'
  if (latest.errorCode === 'archivePending') return '正在保存图片'
  if (latest.runState === 'awaitingReconciliation' && active(latest)) return '正在核对原调用'
  if (latest.runState === 'queued') return '排队中'
  if (latest.runState === 'cancelRequested' && active(latest)) return '正在取消'
  return runStateLabel[latest.state] || runStateLabel[latest.runState] || '处理中'
}
function selectModel(id: string) {
  modelId.value = id
  const ratios = model.value?.capabilities.ratios || []
  if (!ratios.includes(ratio.value)) ratio.value = ratios.includes('1:1') ? '1:1' : ratios[0] || ''
}
function selectPack(id: string) {
  selectedPack.value = id
  const pack = packs.value.find(item => item.packId === id)
  if (!pack) return
  const { productName, sku, sellingPoints, protectedDetails, referenceAssetId } = pack
  Object.assign(form, { productName, sku, sellingPoints, protectedDetails, referenceAssetId })
}
async function loadPacks(target = 1) {
  const sequence = ++packsSequence
  try {
    const { rows, total } = await request<Page<Pack>>('/pod/product-packs?pageNum=' + target + '&pageSize=' + pageSize, {}, props.token)
    if (!alive || sequence !== packsSequence) return
    packs.value = rows; packTotal.value = total; packPage.value = target
    if (!rows.some(row => row.packId === selectedPack.value)) selectedPack.value = ''
  } catch (e) { if (alive && sequence === packsSequence) libraryError.value = report(e) }
}
async function loadJobs(target = 1) {
  const sequence = ++jobsSequence
  try {
    const { rows, total } = await request<Page<Recent>>('/pod/product-jobs?pageNum=' + target + '&pageSize=' + pageSize, {}, props.token)
    if (!alive || sequence !== jobsSequence) return
    jobs.value = rows; jobTotal.value = total; jobPage.value = target
  } catch (e) { if (alive && sequence === jobsSequence) libraryError.value = report(e) }
}
async function loadModels() {
  if (!canEdit.value || !managed.value) return
  catalogError.value = ''
  try {
    const { data } = await request<{ data: CreatorModel[] }>('/pod/models?kind=image', {}, props.token)
    if (!alive) return
    modelList.value = data.filter(item => item.kind === 'image' && (item.capabilities.maxReferenceImages || 0) >= 1 && item.capabilities.ratios?.length)
    selectModel(modelList.value.find(item => item.default)?.modelId || modelList.value[0]?.modelId || '')
  } catch (e) { if (alive) catalogError.value = report(e) }
}
async function refresh() {
  clearTimeout(timer)
  const current = ++refreshSequence
  try {
    const { data } = await request<{ data: Product }>('/pod/product-jobs/' + props.projectId, {}, props.token)
    if (!alive || current !== refreshSequence) return
    if (data.project.projectId !== props.projectId) throw new Error('这条记录不属于当前商品图项目')
    if (offer.value) {
      const { revision, nodeId } = offer.value.body
      const changed = nodeId ? data.slots.some(slot => slot.nodeId === nodeId && active(slot.latest)) : data.slots.some(slot => slot.latest)
      if (revision !== data.project.revision || changed) offer.value = null
    }
    view.value = data; statusError.value = ''
  } catch (e) {
    if (!alive || current !== refreshSequence) return
    statusError.value = report(e)
    if (e instanceof APIError && [403, 404].includes(e.code)) { view.value = null; offer.value = null }
  } finally {
    if (alive && current === refreshSequence) {
      loading.value = false
      if (polling.value && document.visibilityState !== 'hidden') timer = setTimeout(refresh, 2400)
    }
  }
}
function resume() { if (document.visibilityState !== 'hidden') void refresh() }
async function create() {
  if (busy.value || !storageOK.value || (!pendingCreate.value && (creatingDisabled.value || (workKind.value === 'product' && (!model.value || !ratio.value || !outputs.value.length))))) return
  error.value = ''
  if (!pendingCreate.value && (!form.productName.trim() || !form.referenceAssetId)) {
    error.value = '请填写商品名并选择一张参考图。'
    return
  }
  busy.value = 'create'
  try {
    const body = pendingCreate.value || {
      ...form, workKind: workKind.value, requestId: createUUID(),
      ...(workKind.value === 'product' ? { modelId: modelId.value, ratio: ratio.value, outputs: Object.keys(labels).filter(id => outputs.value.includes(id)) } : { modelId: '', ratio: '' }),
    }
    remember(createKey, body)
    pendingCreate.value = body
    const { data } = await request<{ data: Product }>('/pod/product-jobs', { method: 'POST', body: JSON.stringify(body) }, props.token)
    if (!/^[1-9][0-9]*$/.test(data.project.projectId)) throw new Error('项目创建结果未确认')
    remember(createKey, null); pendingCreate.value = null
    if (alive) {
      if ((data.workKind || body.workKind || 'product') === 'product') {
        try { sessionStorage.setItem(scope + ':show-quote', data.project.projectId) } catch { /* The project can be quoted from its result page. */ }
        emit('open', data.project.projectId)
      } else emit('canvas', data.project.projectId)
    }
  } catch (e) {
    if (knownRejection(e)) {
      try { remember(createKey, null); pendingCreate.value = null } catch { storageOK.value = false }
    }
    if (alive) error.value = report(e)
  } finally { if (alive) busy.value = '' }
}
function eligible(nodeId?: string) {
  if (!view.value?.quickRun || !slots.value.length || !supportsRun.value || !storageOK.value || statusError.value || pendingRun.value) return false
  if (nodeId) return slots.value.some(slot => slot.nodeId === nodeId && !active(slot.latest))
  return slots.value.every(slot => !slot.latest)
}
async function prepare(nodeId?: string) {
  if (busy.value || !view.value || !eligible(nodeId)) return
  busy.value = 'quote'; offer.value = null; balance.value = undefined; error.value = ''
  try {
    const draft: RunDraft = { projectId: props.projectId, revision: view.value.project.revision, mode: nodeId ? 'branch' : 'workflow', ...(nodeId ? { nodeId } : {}), executionMode: 'managed', requestId: createUUID() }
    const quote = await executionQuote(props.token, { kind: 'run', run: draft })
    const expected = nodeId ? [nodeId] : slots.value.map(slot => slot.nodeId)
    if (!Array.isArray(quote.lines) || quote.lines.length !== expected.length || !expected.every(id => quote.lines.some(line => line.nodeId === id && line.kind === 'image' && line.images === 1))) {
      throw new Error('报价范围和商品图不一致，请到画布核对节点。')
    }
    if (!alive) return
    offer.value = { quote, body: { ...draft, quoteId: quote.quoteId } }; focusOffer = true
    try {
      const { data } = await request<{ data: { account?: { available?: string } } }>('/pod/points?pageNum=1&pageSize=1', {}, props.token)
      if (alive && typeof data?.account?.available === 'string' && /^(0|[1-9]\d*)$/.test(data.account.available)) balance.value = data.account.available
    } catch (e) { if (alive && e instanceof APIError && e.code === 401) emit('expired') }
  } catch (e) { if (alive) error.value = report(e) }
  finally {
    if (alive) {
      busy.value = ''
      if (focusOffer) { queueMicrotask(() => offerPanel.value?.focus()); focusOffer = false }
    }
  }
}
async function submit() {
  if (busy.value || !storageOK.value) return
  const current = offer.value
  const body = pendingRun.value || current?.body
  if (!body || (!pendingRun.value && (!current || shortage.value || !eligible(body.nodeId)))) return
  if (!pendingRun.value && current && Date.now() >= Date.parse(current.quote.expiresAt)) {
    offer.value = null; error.value = '报价已过期，请重新查看点数。'; return
  }
  busy.value = 'run'; error.value = ''
  try {
    remember(runKey, body); pendingRun.value = body
    const { data } = await request<{ data: Run }>('/pod/runs', { method: 'POST', body: JSON.stringify(body) }, props.token)
    if (data.projectId !== props.projectId || !/^[1-9][0-9]*$/.test(data.runId) || data.mode !== body.mode || (data.nodeId || '') !== (body.nodeId || '')) throw new Error('执行结果未确认')
    remember(runKey, null); pendingRun.value = null; offer.value = null
    if (alive) await refresh()
  } catch (e) {
    if (knownRejection(e)) {
      try { remember(runKey, null); pendingRun.value = null; offer.value = null } catch { storageOK.value = false }
    }
    if (alive) {
      error.value = isPointsShortage(e) ? await pointsShortageMessage(props.token, current?.quote.points) : report(e)
      if (knownRejection(e)) await refresh()
    }
  } finally { if (alive) busy.value = '' }
}
async function showHistory(nodeId: string, target = 1) {
  const sequence = ++historySequence
  if (historyNode.value !== nodeId || target === 1) history.value = []
  historyNode.value = nodeId; historyBusy.value = true; historyError.value = ''
  try {
    const { rows, total } = await request<Page<ProductRun>>('/pod/product-jobs/' + props.projectId + '/history/' + nodeId + '?pageNum=' + target + '&pageSize=4', {}, props.token)
    if (!alive || historySequence !== sequence) return
    history.value = rows; historyTotal.value = total; historyPage.value = target
  } catch (e) { if (alive && historySequence === sequence) historyError.value = report(e) }
  finally { if (alive && historySequence === sequence) historyBusy.value = false }
}
function toggleHistory(nodeId: string) {
  if (historyNode.value === nodeId) { historySequence++; historyNode.value = ''; historyBusy.value = false }
  else void showHistory(nodeId)
}
async function setFinal(slot: Slot, runId: string, assetId: string) {
  if (busy.value || !props.bootstrap.capabilities.projectEditing) return
  busy.value = 'final'; error.value = ''
  try {
    const body = { runId, assetId, previousRunId: slot.final?.runId || '', previousAssetId: slot.final?.assetId || '' }
    await request('/pod/product-jobs/' + props.projectId + '/finals/' + slot.nodeId, { method: 'PUT', body: JSON.stringify(body) }, props.token)
    if (alive) await refresh()
  } catch (e) { if (alive) { error.value = report(e); await refresh() } }
  finally { if (alive) busy.value = '' }
}
async function download(finalOnly = false) {
  if (busy.value || (finalOnly ? !hasFinals.value : !canDownload.value)) return
  busy.value = 'download'; error.value = ''
  const controller = new AbortController(), timeout = setTimeout(() => controller.abort(), 60000)
  try {
    const response = await fetch('/api/pod/product-jobs/' + props.projectId + '/download' + (finalOnly ? '?selection=final' : ''), { headers: { Authorization: 'Bearer ' + props.token }, cache: 'no-store', signal: controller.signal })
    const type = response.headers.get('Content-Type') || ''
    if (!response.ok || !type.startsWith('application/zip')) {
      const problem = type.includes('application/json') ? await response.json().catch(() => null) : null
      throw new APIError(problem?.msg || '整套图片暂时无法下载，请刷新后重试。', problem?.code || response.status)
    }
    const blob = await response.blob()
    if (!alive) return
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a'); link.href = url
    link.download = 'product-' + props.projectId + '.zip'
    const encodedName = /filename\*=UTF-8''([^;]+)/i.exec(response.headers.get('Content-Disposition') || '')?.[1]
    if (encodedName) {
      try {
        const filename = decodeURIComponent(encodedName)
        if (/^[\p{L}\p{N}_-]+\.zip$/u.test(filename)) link.download = filename
      } catch { /* Use the project number when the filename header is malformed. */ }
    }
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) { if (alive) error.value = report(e) }
  finally { clearTimeout(timeout); if (alive) busy.value = '' }
}
onMounted(async () => {
  let quoteOnEntry = false
  try {
    if (props.projectId) {
      const cached = sessionStorage.getItem(runKey)
      if (cached) {
        const body = JSON.parse(cached) as RunSubmission
        if (body.projectId !== props.projectId || body.executionMode !== 'managed' || !validRequest(body.requestId) || !validRequest(body.quoteId) || !/^[1-9][0-9]*$/.test(body.revision) || (body.mode === 'workflow' ? Boolean(body.nodeId) : body.mode !== 'branch' || !body.nodeId || !Object.hasOwn(labels, body.nodeId))) throw new Error('invalid run cache')
        pendingRun.value = body
      }
      quoteOnEntry = sessionStorage.getItem(scope + ':show-quote') === props.projectId
      if (quoteOnEntry) sessionStorage.removeItem(scope + ':show-quote')
    } else {
      const cached = sessionStorage.getItem(createKey)
      if (cached) {
        const body = JSON.parse(cached) as Create
        if (!validRequest(body.requestId) || !/^[1-9][0-9]*$/.test(body.referenceAssetId)) throw new Error('invalid product cache')
        pendingCreate.value = body
      }
    }
  } catch { storageOK.value = false; error.value = '浏览器无法读取待确认的提交，请先从商品图项目或运行记录核对。' }
  if (props.projectId) {
    document.addEventListener('visibilitychange', resume); window.addEventListener('online', resume)
    await refresh()
    if (alive && quoteOnEntry) await prepare()
  } else {
    await Promise.all([loadPacks(), loadJobs(), loadModels()])
    if (alive) loading.value = false
  }
})
onBeforeUnmount(() => {
  alive = false; clearTimeout(timer)
  document.removeEventListener('visibilitychange', resume); window.removeEventListener('online', resume)
})
</script>

<template>
  <section class="product-studio" aria-labelledby="product-title">
    <header class="product-heading">
      <div>
        <button v-if="projectId" type="button" class="text-button product-back" @click="emit('open', '')"><PhArrowLeft :size="16" />商品创作工作台</button>
        <h1 id="product-title">{{ view?.project.name || '制作商品图' }}</h1>
      </div>
      <div v-if="view" class="product-actions">
        <button v-if="resultIsProduct" class="secondary-button" type="button" @click="showPageEditor = true"><PhLayout :size="17" />卖点图 / 详情页</button>
        <button v-if="resultIsProduct && bootstrap.capabilities.assets" class="secondary-button" type="button" :aria-expanded="showPageAssets" @click="showPageAssets ? showPageAssets = false : openPageAssets()">排版成品</button>
        <button class="secondary-button" type="button" @click="emit('canvas', projectId)"><PhBezierCurve :size="17" />打开画布</button>
        <button class="icon-button" type="button" aria-label="刷新图片状态" @click="refresh"><PhArrowsClockwise :size="19" /></button>
      </div>
    </header>
    <template v-if="!projectId">
      <form class="product-form product-panel" :aria-busy="loading || !!busy" @submit.prevent="create">
        <fieldset class="product-kind" aria-label="开工类型" :disabled="!!pendingCreate || !!busy || !canEdit">
          <label v-for="(title, kind) in workKinds" :key="kind"><input v-model="workKind" type="radio" name="product-work-kind" :value="kind" />{{ title }}</label>
        </fieldset>
        <fieldset class="product-form-layout" :disabled="!!pendingCreate || !!busy || !canEdit">
          <div class="product-reference">
            <strong>{{ workKind === 'product' ? '商品参考图' : '印花原图' }}</strong>
            <button type="button" class="product-reference-button" :aria-label="form.referenceAssetId ? '更换商品参考图' : '上传或选择商品参考图'" @click="picker = true">
              <PrivateImage v-if="form.referenceAssetId" :asset-id="form.referenceAssetId" :token="token" alt="商品参考图" :retry="false" @expired="emit('expired')" />
              <span v-else><PhImage :size="34" weight="light" />上传或选择图片</span>
            </button>
            <label v-if="packs.length">复用商品素材包
              <select :value="selectedPack" @change="selectPack(($event.target as HTMLSelectElement).value)">
                <option value="">选择已保存的商品</option>
                <option v-for="pack in packs" :key="pack.packId" :value="pack.packId">{{ pack.productName }}{{ pack.sku ? ' · ' + pack.sku : '' }}</option>
              </select>
            </label>
            <div v-if="packTotal > pageSize" class="product-pagination">
              <button type="button" class="text-button" :disabled="packPage === 1" @click="loadPacks(packPage - 1)">上一页</button>
              <span>{{ packPage }} / {{ Math.ceil(packTotal / pageSize) }}</span>
              <button type="button" class="text-button" :disabled="packPage * pageSize >= packTotal" @click="loadPacks(packPage + 1)">下一页</button>
            </div>
          </div>
          <div class="product-fields">
            <label>商品名 <input v-model="form.productName" required maxlength="80" placeholder="例如 陶瓷马克杯" /></label>
            <label>SKU <input v-model="form.sku" maxlength="60" placeholder="选填" /></label>
            <label>真实卖点 <textarea v-model="form.sellingPoints" maxlength="400" rows="2" placeholder="例如 哑光釉面，容量 350 mL" /></label>
            <label>禁改项 <textarea v-model="form.protectedDetails" maxlength="400" rows="2" placeholder="例如 保留杯柄形状和正面标志" /></label>
            <template v-if="workKind === 'product'">
              <label>生图模型 <select :value="modelId" @change="selectModel(($event.target as HTMLSelectElement).value)"><option v-if="!modelList.length" value="">暂无可用模型</option><option v-for="item in modelList" :key="item.modelId" :value="item.modelId">{{ item.name }}</option></select></label>
              <label>画面比例 <select v-model="ratio"><option v-for="size in model?.capabilities.ratios || []" :key="size" :value="size">{{ size }}</option></select></label>
              <fieldset class="product-output-choices"><legend>要生成的图片</legend>
                <label v-for="(title, nodeId) in labels" :key="nodeId"><input v-model="outputs" type="checkbox" :value="nodeId" />{{ title }}</label>
              </fieldset>
            </template>
            <p v-else class="product-local-note muted">请选择包含印花图案的原图。创建画布后可自行调整{{ workKind === 'mockup' ? '衣服颜色和图案位置' : '尺寸和印刷规格' }}。</p>
          </div>
        </fieldset>
        <p v-if="creatingDisabled" class="muted">制作商品图需要创作资格及正式生图模型。</p>
        <p v-else-if="workKind === 'product' && catalogError" class="error-message" role="alert">{{ catalogError }} <button type="button" class="text-button" @click="loadModels">重新读取模型</button></p>
        <p v-else-if="workKind === 'product' && !loading && !modelList.length" class="muted">当前没有可用的参考图模型，请联系管理员开通。</p>
        <p v-if="pendingCreate" class="muted" role="status">开工结果待确认。重试会继续核对同一项目。</p>
        <p v-if="error" class="error-message" role="alert">{{ error }}</p>
        <footer class="product-form-footer"><span><PhPackage :size="18" />{{ workKind === 'product' ? '已选 ' + outputs.length + ' 张' : workKinds[workKind] }}</span><button type="submit" class="primary-button" :disabled="!!busy || !storageOK || (!pendingCreate && (creatingDisabled || (workKind === 'product' && (!model || !ratio || !outputs.length))))">{{ busy === 'create' ? '正在创建…' : pendingCreate ? '核对原开工请求' : workKind === 'product' ? '开始制作 · 查看报价' : '创建并打开画布' }}<PhArrowRight :size="17" /></button></footer>
      </form>
      <section class="product-panel product-recent" aria-labelledby="product-recent-title">
        <h2 id="product-recent-title">商品创作项目</h2>
        <div v-if="jobs.length" class="product-jobs"><button v-for="job in jobs" :key="job.projectId" class="text-button" type="button" @click="(job.workKind || 'product') === 'product' ? emit('open', job.projectId) : emit('canvas', job.projectId)">{{ job.name }}<PhArrowRight :size="15" /></button></div>
        <p v-else-if="!loading && !libraryError" class="muted">创建后的项目和结果会保存在这里。</p>
        <p v-if="libraryError" class="error-message" role="alert">{{ libraryError }} <button type="button" class="text-button" @click="libraryError = ''; loadPacks(packPage); loadJobs(jobPage)">重新读取</button></p>
        <div v-if="jobTotal > pageSize" class="product-pagination"><button type="button" class="text-button" :disabled="jobPage === 1" @click="loadJobs(jobPage - 1)">上一页</button><span>{{ jobPage }} / {{ Math.ceil(jobTotal / pageSize) }}</span><button type="button" class="text-button" :disabled="jobPage * pageSize >= jobTotal" @click="loadJobs(jobPage + 1)">下一页</button></div>
      </section>
    </template>
    <template v-else>
      <p v-if="loading" role="status" class="muted">正在读取商品图…</p>
      <p v-if="statusError" class="error-message" role="alert">{{ statusError }} <button class="text-button" type="button" @click="refresh">重新读取</button></p>
      <p v-if="error" class="error-message" role="alert">{{ error }}</p>
      <div v-if="pendingRun" class="product-quote product-panel" role="status">
        <p>上次提交未确认。继续核对会沿用原请求标识。</p>
        <button class="primary-button" :disabled="!!busy || !storageOK" type="button" @click="submit">{{ busy === 'run' ? '正在核对…' : '核对原提交' }}</button>
      </div>
      <p v-if="view && !resultIsProduct" class="muted">从画布运行{{ workKinds[view.workKind || 'product'] }}并查看结果。</p>
      <template v-else-if="view">
        <section v-if="showPageAssets" class="product-panel product-page-library" aria-labelledby="product-page-library-title">
          <h2 id="product-page-library-title" ref="pageAssetsHeading" tabindex="-1">排版成品</h2>
          <Assets :key="pageAssetsRevision" :token="token" :limits="bootstrap.assetLimits" :product-project-id="projectId" @expired="emit('expired')" />
        </section>
        <div class="product-results">
          <aside class="product-panel product-source">
            <h2>参考图</h2>
            <PrivateImage :asset-id="view.pack.referenceAssetId" :token="token" alt="商品参考图" @expired="emit('expired')" />
            <strong>{{ view.pack.productName }}</strong>
            <small v-if="view.pack.sku">SKU {{ view.pack.sku }}</small>
          </aside>
          <div class="product-output-grid">
          <article v-for="slot in slots" :key="slot.nodeId" class="product-panel product-output">
            <header><h2>{{ labels[slot.nodeId] }}</h2><small class="product-status" role="status">{{ stateLabel(slot) }}</small></header>
            <ResultImages v-if="slot.final || slot.result" :ids="slot.final ? [slot.final.assetId] : slot.result?.output.assetIds?.slice(0, 1) || []" :info="slot.result?.output.images" :token="token" @expired="emit('expired')" />
            <div v-else class="product-placeholder"><PhImage :size="33" weight="light" /></div>
            <p v-if="slot.final" class="product-final-label">定稿<span v-if="slot.result && (slot.result.runId !== slot.final.runId || slot.result.output.assetIds?.[0] !== slot.final.assetId)"> · 已有其他候选图</span></p>
            <p v-else-if="slot.result && (slot.result.runId !== slot.latest?.runId || slot.result.projectRevision !== view.project.revision)" class="muted">目前显示上次可下载的图片。</p>
            <p v-if="slot.latest?.errorMessage && ['failed', 'blocked'].includes(slot.latest.state)" class="error-message">{{ slot.latest.errorMessage }}</p>
            <button v-if="(slot.latest || slots.some(item => item.latest)) && !active(slot.latest) && view.quickRun && supportsRun" class="secondary-button" type="button" :disabled="!!busy || !eligible(slot.nodeId)" @click="prepare(slot.nodeId)">单张补做</button>
            <div v-if="slot.result" class="product-actions">
              <button v-if="!slot.final && bootstrap.capabilities.projectEditing && slot.result.output.assetIds?.[0]" type="button" class="text-button" :disabled="!!busy" @click="setFinal(slot, slot.result.runId, slot.result.output.assetIds[0])">设为定稿</button>
              <button type="button" class="text-button" :aria-expanded="historyNode === slot.nodeId" @click="toggleHistory(slot.nodeId)">{{ historyNode === slot.nodeId ? '收起候选图' : '挑选候选图' }}</button>
            </div>
            <section v-if="historyNode === slot.nodeId" class="product-candidates" :aria-label="labels[slot.nodeId] + '候选图'" :aria-busy="historyBusy">
              <p v-if="historyError" class="error-message" role="alert">{{ historyError }} <button type="button" class="text-button" @click="showHistory(slot.nodeId, historyPage)">重试</button></p>
              <p v-else-if="historyBusy" class="muted" role="status">正在读取候选图…</p>
              <p v-else-if="!history.length" class="muted">暂无已保存的候选图。</p>
              <div v-else class="product-candidate-grid">
                <template v-for="run in history" :key="run.runId">
                  <div v-for="(id, index) in run.output.assetIds" :key="id" class="product-candidate">
                    <PrivateImage :asset-id="id" :token="token" :alt="labels[slot.nodeId] + '候选图 ' + (index + 1)" @expired="emit('expired')" />
                    <small>任务 #{{ run.runId }} · 版本 {{ run.projectRevision }}</small>
                    <span v-if="slot.final?.runId === run.runId && slot.final.assetId === id" class="product-final-label">当前定稿</span>
                    <button v-else-if="bootstrap.capabilities.projectEditing" type="button" class="text-button" :disabled="!!busy" @click="setFinal(slot, run.runId, id)">{{ slot.final ? '换成这张' : '设为定稿' }}</button>
                  </div>
                </template>
              </div>
              <div v-if="historyTotal > 4" class="product-pagination">
                <button type="button" class="text-button" :disabled="historyBusy || historyPage <= 1" @click="showHistory(slot.nodeId, historyPage - 1)">上一页</button>
                <span>{{ historyPage }} / {{ Math.ceil(historyTotal / 4) }}</span>
                <button type="button" class="text-button" :disabled="historyBusy || historyPage * 4 >= historyTotal" @click="showHistory(slot.nodeId, historyPage + 1)">下一页</button>
              </div>
            </section>
          </article>
          </div>
        </div>
        <footer class="product-actions product-result-actions">
          <button v-if="hasFinals" class="primary-button" type="button" :disabled="!!busy" @click="download(true)"><PhDownloadSimple :size="18" />{{ busy === 'download' ? '正在打包…' : '下载定稿 ZIP' }}</button>
          <button v-else-if="canDownload" class="secondary-button" type="button" :disabled="!!busy" @click="download()"><PhDownloadSimple :size="18" />{{ busy === 'download' ? '正在打包…' : '下载当前结果 ZIP' }}</button>
          <button v-if="eligible() && !offer" class="primary-button" type="button" :disabled="!!busy" @click="prepare()">{{ busy === 'quote' ? '正在计算点数…' : slots.length === 2 ? '查看两张图的报价' : '查看 ' + slots.length + ' 张图的报价' }}</button>
          <span v-if="!view.quickRun" class="muted">项目已有新版本，可打开画布继续运行。</span>
          <span v-else-if="!supportsRun" class="muted">当前可查看和下载已有图片。</span>
        </footer>
        <section v-if="offer" ref="offerPanel" class="product-panel product-quote" tabindex="-1" aria-label="点数报价">
          <div><h2>{{ offer.quote.lines.map(line => labels[line.nodeId]).join(' + ') }} · {{ offer.quote.points }} 点</h2>
            <ul><li v-for="line in offer.quote.lines" :key="line.nodeId">{{ labels[line.nodeId] }} · {{ line.name }} · {{ line.points }} 点</li></ul>
            <p v-if="shortage" class="error-message" role="alert">点数不足。当前可用 {{ balance }} 点，请联系管理员补充。</p>
          </div>
          <div class="product-actions"><button class="text-button" type="button" :disabled="!!busy" @click="offer = null">取消</button><button class="primary-button" type="button" :disabled="!!busy || !!shortage || !eligible(offer.body.nodeId)" @click="submit">{{ busy === 'run' ? '正在提交…' : '确认并生成' }}</button></div>
        </section>
      </template>
    </template>
  </section>
  <ProductPageEditor v-if="showPageEditor && view" :project-id="projectId" :token="token" :limits="bootstrap.assetLimits" :asset-access="bootstrap.capabilities.assets" :storage-key="scope + ':layout:v1:' + projectId" :sku="view.pack.sku" :initial="pageDefaults(view.pack, view.slots)" @close="showPageEditor = false" @expired="emit('expired')" @saved="pageAssetsRevision++" @view-saved="openPageAssets" />
  <AssetPicker v-if="picker" single :token="token" :limits="bootstrap.assetLimits" :initial="form.referenceAssetId ? [form.referenceAssetId] : []" @close="picker = false" @confirm="form.referenceAssetId = $event[0] || ''; picker = false" @expired="emit('expired')" />
</template>
