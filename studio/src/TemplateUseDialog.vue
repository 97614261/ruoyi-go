<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhCopy, PhImage, PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { AssetLimits, PersonalTemplate, Project, TemplateVersion } from './contracts'
import AssetPicker from './AssetPicker.vue'
import PrivateImage from './PrivateImage.vue'
import TemplateGraph from './TemplateGraph.vue'
import BatchDialog from './BatchDialog.vue'
import { pendingTemplateRequest, retryTemplateRequest, submitTemplateRequest } from './template-requests'
const props = defineProps<{ row: PersonalTemplate; token: string; scope: string; limits: AssetLimits; executionMode?: string }>()
const emit = defineEmits<{ close: []; open: [id: string]; copy: [version: string]; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), version = ref(props.row.currentVersion), versions = ref<TemplateVersion[]>([]), page = ref(1), total = ref(0)
const snapshot = ref<TemplateVersion | null>(null), name = ref(props.row.name), loading = ref(true), busy = ref(false), error = ref(''), pending = ref(false), picker = ref('')
const texts = ref<Record<string, string>>({}), images = ref<Record<string, string[]>>({})
const batchOpen = ref(false), batchCreated = ref('')
const canSubmit = computed(() => !busy.value && (pending.value || (!loading.value && !!snapshot.value && !!name.value.trim())))
const options = computed(() => versions.value.some(v => v.version === version.value) ? versions.value : [{ version: version.value, templateId: props.row.templateId, createTime: '' }, ...versions.value])
let focus: HTMLElement | null = null, sequence = 0
function showError(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
function refreshPending() { try { pending.value = !!pendingTemplateRequest(props.scope) } catch (e) { pending.value = true; showError(e) } }
async function loadVersions() {
  try { const result = await request<{ rows: TemplateVersion[]; total: number }>(`/pod/templates/${props.row.templateId}/versions?pageNum=${page.value}&pageSize=20`, {}, props.token); versions.value = result.rows; total.value = result.total }
  catch (e) { showError(e) }
}
async function loadVersion() {
  const id = ++sequence; loading.value = true; error.value = ''; snapshot.value = null
  try {
    const result = await request<{ data: TemplateVersion }>(`/pod/templates/${props.row.templateId}/versions/${version.value}`, {}, props.token)
    if (id !== sequence) return
    snapshot.value = result.data; texts.value = {}; images.value = {}
    for (const slot of result.data.slots || []) { if (slot.type === 'textInput') texts.value[slot.nodeId] = slot.text || ''; else images.value[slot.nodeId] = [...(slot.assetIds || [])] }
  } catch (e) { if (id === sequence) showError(e) } finally { if (id === sequence) loading.value = false }
}
async function submit() {
  if (!canSubmit.value) return
  busy.value = true; error.value = ''
  try {
    let result: { data: Project }
    if (pending.value) {
      if (!pendingTemplateRequest(props.scope)?.path.endsWith('/use')) throw new Error('请先在模板页面处理上一项待确认操作。')
      result = await retryTemplateRequest<Project>(props.scope, props.token)
    } else {
      const bindings = (snapshot.value?.slots || []).map(slot => slot.type === 'textInput' ? { nodeId: slot.nodeId, text: texts.value[slot.nodeId] || '' } : { nodeId: slot.nodeId, assetIds: images.value[slot.nodeId] || [] })
      result = await submitTemplateRequest<Project>(props.scope, `/pod/templates/${props.row.templateId}/use`, { version: version.value, name: name.value, bindings }, props.token)
    }
    emit('open', result.data.projectId)
  } catch (e) { showError(e) } finally { busy.value = false; refreshPending() }
}
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal(); refreshPending(); void loadVersions(); void loadVersion() })
onBeforeUnmount(() => { ++sequence; dialog.value?.close(); focus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="template-use-dialog" aria-labelledby="template-use-title" @cancel.prevent="!busy && emit('close')">
    <header class="template-dialog-header"><div><h2 id="template-use-title">{{ row.name }}</h2></div><button class="icon-button" aria-label="关闭模板详情" :disabled="busy" @click="emit('close')"><PhX :size="20" /></button></header>
    <form @submit.prevent="submit">
      <div class="template-use-layout"><section class="template-preview"><div class="template-version-bar"><label>固定版本<select v-model="version" :disabled="busy || pending" @change="loadVersion"><option v-for="v in options" :key="v.version" :value="v.version">v{{ v.version }}{{ v.version === row.currentVersion ? ' · 当前' : '' }}</option></select></label><button type="button" class="text-button" :disabled="busy || loading || pending" @click="emit('copy', version)"><PhCopy :size="16" />复制此版本</button></div>
          <div v-if="total > 20" class="template-pagination"><button type="button" class="text-button" :disabled="busy || pending || page === 1" @click="page--; loadVersions()">较新版本</button><span>第 {{ page }} 页</span><button type="button" class="text-button" :disabled="busy || pending || page * 20 >= total" @click="page++; loadVersions()">较早版本</button></div>
          <p v-if="loading" class="muted" role="status">正在读取版本…</p>
          <details v-if="snapshot?.graph" class="template-disclosure"><summary>流程预览 · {{ snapshot.graph.nodes.length }} 个节点</summary><TemplateGraph :graph="snapshot.graph" /><div class="template-output-list"><h3>输出位置</h3><span v-for="output in snapshot.outputs" :key="output.nodeId">{{ output.label }}</span></div></details>
          <details v-if="row.description" class="template-disclosure"><summary>模板说明</summary><p>{{ row.description }}</p></details>
        </section>
        <section class="template-inputs"><fieldset :disabled="busy || pending || loading"><label class="field">新项目名称<input v-model="name" maxlength="100" required autocomplete="off" /></label>
          <p v-if="snapshot && !snapshot.slots?.length" class="muted">创建后在画布中调整参数。</p>
          <div v-for="slot in snapshot?.slots" :key="slot.nodeId" class="template-slot"><label v-if="slot.type === 'textInput'" class="field">{{ slot.label }}<textarea v-model="texts[slot.nodeId]" rows="4" maxlength="12000" /></label><template v-else><div class="template-slot-heading"><strong>{{ slot.label }}</strong><button type="button" class="secondary-button" @click="picker = slot.nodeId"><PhImage :size="16" />换图片</button></div><div class="template-slot-images"><PrivateImage v-for="id in images[slot.nodeId]" :key="id" :asset-id="id" :token="token" alt="模板输入图片" @expired="emit('expired')" /></div><p class="template-note">已选 {{ images[slot.nodeId]?.length || 0 }} 张，最多 {{ limits.maxImagesPerNode }} 张。</p></template></div>
        </fieldset></section>
      </div>
      <footer class="template-use-footer"><p>创建项目不执行模型，不扣生成点数。</p><p v-if="batchCreated" class="test-mode-note" role="status">批次 #{{ batchCreated }} 已创建，到“运行与用量 → 批量”查看进度。</p><p v-if="pending" class="error-message" role="status">上次创建结果待确认，重试只核对原提交。</p><p v-if="error" class="error-message" role="alert">{{ error }}</p><div class="dialog-actions"><button v-if="executionMode && ['qwen', 'simulation', 'managed'].includes(executionMode)" type="button" class="secondary-button" :disabled="busy || pending || loading || !snapshot" @click="batchOpen = true">批量制作</button><button class="primary-button" :disabled="!canSubmit">{{ busy ? '正在创建…' : pending ? '重试原创建' : '创建项目并打开' }}</button></div></footer>
    </form>
    <AssetPicker v-if="picker" :key="picker" :token="token" :limits="limits" :initial="images[picker] || []" @close="picker = ''" @confirm="ids => { images[picker] = ids; picker = '' }" @expired="emit('expired')" />
    <BatchDialog v-if="batchOpen && snapshot" :snapshot="snapshot" :name="row.name" :token="token" :scope="scope" :limits="limits" :mode="executionMode!" @close="batchOpen = false" @created="batch => { batchCreated = batch.batchId; batchOpen = false }" @expired="emit('expired')" />
  </dialog>
</template>
