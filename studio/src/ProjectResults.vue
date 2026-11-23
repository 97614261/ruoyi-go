<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowClockwise, PhImages, PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { Run } from './contracts'
import { runStateLabel, terminalRun } from './run-state'
import ResultImages from './ResultImages.vue'

const props = defineProps<{ token: string; projectId: string; revision: string; initialRun?: Run | null }>()
const emit = defineEmits<{ close: []; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), rows = ref<Run[]>([]), selected = ref<Run | null>(null)
const page = ref(1), total = ref(0), loading = ref(false), error = ref('')
const pageSize = 8
let sequence = 0, timer: ReturnType<typeof setTimeout> | undefined, focus: HTMLElement | null = null
const imageNodes = (run: Run | null) => (run?.nodes || []).filter(node => node.state === 'succeeded' && node.output.assetIds?.length)
const labels: Record<string, string> = { imageGenerate: 'AI 生图', imageProcess: '图片处理', productPreview: '商品预览', export: '导出文件' }
async function detail(id: string) {
  const { data } = await request<{ data: Run }>(`/pod/runs/${encodeURIComponent(id)}`, {}, props.token)
  if (data.projectId !== props.projectId || data.runId !== id || data.mode === 'polish') throw new APIError('这条记录不属于当前项目结果。', 404)
  return data
}
function report(e: unknown) {
  error.value = e instanceof Error ? e.message : '无法读取项目结果。'
  if (e instanceof APIError && e.code === 401) emit('expired')
}
function schedule() {
  if (selected.value && !terminalRun(selected.value.state) && document.visibilityState !== 'hidden') {
    const id = selected.value.runId; timer = setTimeout(() => void pick(id, false), 2500)
  }
}
async function pick(id: string, manual = true) {
  clearTimeout(timer)
  const current = ++sequence
  if (manual) { loading.value = true; error.value = ''; if (selected.value?.runId !== id) selected.value = null }
  try { const run = await detail(id); if (current === sequence) { selected.value = run; error.value = '' } }
  catch (e) { if (current === sequence) { if (e instanceof APIError && [401, 403, 404].includes(e.code)) selected.value = null; report(e) } }
  finally { if (current === sequence) { loading.value = false; schedule() } }
}
async function load(target = 1, preferred?: Run | null) {
  clearTimeout(timer)
  const current = ++sequence
  loading.value = true; error.value = ''; page.value = target; selected.value = null; rows.value = []
  try {
    const list = await request<{ rows: Run[]; total: number }>(`/pod/runs?pageNum=${target}&pageSize=${pageSize}&projectId=${encodeURIComponent(props.projectId)}`, {}, props.token)
    if (current !== sequence) return
    rows.value = list.rows.filter(run => run.projectId === props.projectId && run.mode !== 'polish'); total.value = list.total
    const candidates = rows.value.filter(run => terminalRun(run.state) || run.state === 'awaitingReconciliation')
    if (preferred && imageNodes(preferred).length && rows.value.some(run => run.runId === preferred.runId) && !candidates.some(run => run.runId === preferred.runId)) candidates.unshift(preferred)
    for (const candidate of candidates) {
      try {
        const run = await detail(candidate.runId)
        if (current !== sequence) return
        if (imageNodes(run).length) { selected.value = run; break }
      } catch (e) { if (current !== sequence) return; if (!(e instanceof APIError && [403, 404].includes(e.code))) throw e }
    }
    if (current === sequence && !selected.value) {
      const active = rows.value.find(run => !terminalRun(run.state) && run.state !== 'awaitingReconciliation')
      if (active) { await pick(active.runId); return }
    }
    if (current === sequence) schedule()
  } catch (e) { if (current === sequence) report(e) }
  finally { if (current === sequence) loading.value = false }
}
function resume() { if (selected.value && !terminalRun(selected.value.state) && document.visibilityState !== 'hidden') void pick(selected.value.runId, false) }
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal(); document.addEventListener('visibilitychange', resume); void load(1, props.initialRun) })
onBeforeUnmount(() => { ++sequence; clearTimeout(timer); document.removeEventListener('visibilitychange', resume); dialog.value?.close(); focus?.focus() })
</script>

<template>
  <dialog ref="dialog" class="project-results-dialog" aria-labelledby="project-results-title" @cancel.prevent="emit('close')">
    <header class="project-results-heading">
      <div><h2 id="project-results-title"><PhImages :size="21" />项目结果</h2><p>查看和下载已有图片，不会重新运行或扣点。</p></div>
      <button type="button" class="icon-button" aria-label="关闭项目结果" @click="emit('close')"><PhX :size="19" /></button>
    </header>
    <div class="project-results-body" :aria-busy="loading">
      <div class="project-results-tools">
        <label>运行记录<select :value="selected?.runId || ''" :disabled="!rows.length" @change="pick(($event.target as HTMLSelectElement).value)"><option value="" disabled>选择一条记录</option><option v-for="run in rows" :key="run.runId" :value="run.runId">#{{ run.runId }} · {{ run.createTime }} · {{ runStateLabel[run.state] || run.state }}</option></select></label>
        <button type="button" class="icon-button" aria-label="刷新项目结果" :disabled="loading" @click="load(page)"><PhArrowClockwise :size="18" /></button>
      </div>
      <p v-if="error" class="error-message" role="alert">{{ error }}</p>
      <p v-else-if="loading" class="project-results-empty" role="status">正在读取结果…</p>
      <template v-else-if="selected">
        <p class="project-results-version">画布 v{{ selected.projectRevision }}<span v-if="selected.projectRevision !== revision"> · 历史结果，不代表当前参数</span></p>
        <section v-for="node in imageNodes(selected)" :key="node.nodeId" class="project-results-group"><h3>{{ selected.graph?.nodes.find(item => item.id === node.nodeId)?.config.title || labels[node.nodeType] || '图片结果' }}</h3><ResultImages :ids="node.output.assetIds || []" :info="node.output.images" :token="token" :compare="node.nodeType === 'imageProcess'" @expired="emit('expired')" /></section>
        <p v-if="!imageNodes(selected).length" class="project-results-empty">{{ terminalRun(selected.state) ? '这次运行没有可查看的图片。' : '还没产出图片，完成后会自动更新。' }}</p>
      </template>
      <p v-else class="project-results-empty">{{ total ? '本页还没有已生成的图片，可以查看其他运行记录。' : '生成或处理图片后，结果会保留在这里。' }}</p>
      <nav v-if="total > pageSize" class="run-pagination" aria-label="结果记录分页"><button type="button" class="text-button" :disabled="loading || page === 1" @click="load(page - 1)">上一页</button><span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span><button type="button" class="text-button" :disabled="loading || page * pageSize >= total" @click="load(page + 1)">下一页</button></nav>
    </div>
  </dialog>
</template>
