<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { APIError, request } from './api'
import type { Batch } from './contracts'
import { pendingBatch, retryBatch, downloadBatch } from './batch-requests'
import BillingSummary from './BillingSummary.vue'
import { runStateLabel } from './run-state'
const props = defineProps<{ token: string; scope: string }>()
const emit = defineEmits<{ expired: []; run: [id: string]; project: [id: string] }>()
const rows = ref<Batch[]>([]), selected = ref<Batch | null>(null), page = ref(1), total = ref(0), busy = ref(false), error = ref(''), pending = ref(false)
const labels: Record<string, string> = { ...runStateLabel, waiting: '待派发', dispatchFailed: '派发失败', active: '派发中', dispatched: '已全部派发', stopped: '已停止待派发项' }
let sequence = 0, disposed = false, timer: ReturnType<typeof setTimeout> | undefined
function report(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
function checkPending() { try { pending.value = !!pendingBatch(props.scope) } catch (e) { pending.value = true; report(e) } }
async function pick(id: string) { const seq = ++sequence; try { const result = await request<{ data: Batch }>(`/pod/batches/${id}`, {}, props.token); if (!disposed && seq === sequence) selected.value = result.data } catch (e) { if (!disposed && seq === sequence) { if (e instanceof APIError && [401, 403, 404].includes(e.code)) selected.value = null; report(e) } } }
async function load() {
  try { const result = await request<{ rows: Batch[]; total: number }>(`/pod/batches?pageNum=${page.value}&pageSize=10`, {}, props.token); if (disposed) return; rows.value = result.rows; total.value = result.total; if (selected.value) await pick(selected.value.batchId) }
  catch (e) { if (!disposed) { if (e instanceof APIError && [401, 403].includes(e.code)) { ++sequence; selected.value = null; rows.value = [] }; report(e) } }
}
async function stop() { if (!selected.value || busy.value) return; busy.value = true; error.value = ''; ++sequence; try { selected.value = (await request<{ data: Batch }>(`/pod/batches/${selected.value.batchId}/stop`, { method: 'POST', body: '{}' }, props.token)).data; await load() } catch (e) { report(e) } finally { busy.value = false } }
async function retry() { busy.value = true; error.value = ''; try { selected.value = await retryBatch(props.scope, props.token); await load() } catch (e) { report(e) } finally { busy.value = false; checkPending() } }
async function download() { if (!selected.value || busy.value) return; busy.value = true; error.value = ''; try { const id = selected.value.batchId, blob = await downloadBatch(id, props.token); if (disposed) return; const url = URL.createObjectURL(blob), a = document.createElement('a'); a.href = url; a.download = `batch-${id}.zip`; a.click(); setTimeout(() => URL.revokeObjectURL(url), 30_000) } catch (e) { report(e) } finally { busy.value = false } }
async function poll() { if (!busy.value) await load(); if (!disposed) timer = setTimeout(poll, 3000) }
onMounted(() => { checkPending(); void poll() })
onBeforeUnmount(() => { disposed = true; ++sequence; clearTimeout(timer) })
</script>
<template>
  <section class="batch-history" aria-label="批量运行"><p class="muted">从“工作流模板 → 使用模板 → 批量制作”开始。每份生成独立项目，成功结果单独保留。</p><p v-if="pending" class="editor-alert">有一批提交结果待确认。<button class="text-button" :disabled="busy" @click="retry">重试原批次提交</button></p><p v-if="error" class="error-message" role="alert">{{ error }}<button class="text-button" :disabled="busy" @click="load">刷新</button></p>
    <p v-if="!rows.length" class="empty-runs">还没有批次。先把调好的流程保存为个人模板。</p><button v-for="row in rows" :key="row.batchId" class="run-row" :class="{ selected: selected?.batchId === row.batchId }" :disabled="busy" @click="pick(row.batchId)"><span><strong>{{ row.name }}</strong><small>#{{ row.batchId }} · 模板 v{{ row.templateVersion }} · {{ row.impact.items }} 份</small></span><span>{{ labels[row.state] || row.state }}</span><time>{{ row.createTime }}</time></button>
    <div class="run-pagination"><button class="text-button" :disabled="busy || page === 1" @click="page--; load()">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(total / 10)) }}</span><button class="text-button" :disabled="busy || page * 10 >= total" @click="page++; load()">下一页</button></div>
    <article v-if="selected" class="run-detail"><div class="batch-detail-heading"><h2>{{ selected.name }}</h2><span>{{ labels[selected.state] }}</span></div><p class="muted">{{ selected.executionMode === 'managed' ? '正式点数模式' : selected.executionMode === 'qwen' ? '千问模式' : '测试模式' }} · 固定模板版本 v{{ selected.templateVersion }}。等待不代表已经调用模型。</p>
      <BillingSummary v-if="selected.billing" :billing="selected.billing" /><div class="run-actions"><button class="secondary-button" :disabled="busy || !selected.items?.some(i => i.state === 'waiting')" @click="stop">停止待派发项</button><button class="primary-button" :disabled="busy || !selected.items?.some(i => i.runId !== '0')" @click="download">{{ busy ? '处理中…' : '下载已归档图片 ZIP' }}</button></div><p class="muted">ZIP 按每份的 print / mockup 分开，包含当前已成功归档的图片。单次最多 100 张、100 MiB；更大结果请在项目中分别下载。</p>
      <div v-for="item in selected.items" :key="item.itemNo" class="batch-result-row"><div><strong>{{ item.itemNo }}. {{ item.name }}</strong><p>{{ labels[item.state] || item.state }}<span v-if="item.errorCode === 'queueFull'"> · 正在等候可用队列位置</span><span v-else-if="item.errorCode === 'configurationChanged'"> · 配置或测试额度已变化，请核对模板后新建批次</span></p></div><div><button v-if="item.projectId !== '0'" class="text-button" :disabled="busy" @click="emit('project', item.projectId)">打开项目</button><button v-if="item.runId !== '0'" class="text-button" :disabled="busy" @click="emit('run', item.runId)">运行详情</button></div></div>
      <p class="test-mode-note">已派发项请在“运行详情”取消或恢复。仅已确认失败可以恢复；原调用结果待确认时保留预占，不重新调用模型。</p>
    </article>
  </section>
</template>
