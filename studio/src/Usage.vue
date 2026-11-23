<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { APIError, request } from './api'
import type { CreatorUsage } from './contracts'
import { costLabel, runStateLabel } from './run-state'

const props = defineProps<{ token: string; scope?: string; projectId?: string }>()
const emit = defineEmits<{ expired: []; open: [id: string] }>()
const data = ref<CreatorUsage | null>(null), page = ref(1), loading = ref(false), error = ref('')
let sequence = 0, disposed = false, timer: ReturnType<typeof setTimeout> | undefined
async function load() {
  clearTimeout(timer)
  const current = ++sequence
  loading.value = true; error.value = ''
  try {
    const result = await request<{ data: CreatorUsage }>(`/pod/usage?pageNum=${page.value}&pageSize=10`, { cache: 'no-store' }, props.token)
    if (disposed || current !== sequence) return
    data.value = result.data
  } catch (e) {
    if (disposed || current !== sequence) return
    error.value = e instanceof Error ? e.message : '读取用量失败'
    if (e instanceof APIError && [401, 403].includes(e.code)) {
      data.value = null
      if (e.code === 401) emit('expired')
    }
  } finally {
    if (!disposed && current === sequence) {
      loading.value = false
      if (!error.value) timer = setTimeout(load, 10000)
    }
  }
}
watch([() => props.token, () => props.scope], () => {
  ++sequence; clearTimeout(timer); data.value = null
  if (page.value !== 1) { page.value = 1; return }
  void load()
}, { immediate: true })
watch(page, () => { void load() })
onBeforeUnmount(() => { disposed = true; ++sequence; clearTimeout(timer) })
</script>

<template>
  <section class="creator-usage" aria-label="我的用量">
    <div class="usage-heading"><h2>我的用量</h2><button class="text-button" :disabled="loading" @click="load">{{ loading ? '正在读取…' : '刷新用量' }}</button></div>
    <p v-if="error" class="error-message" role="alert">{{ error }}{{ data ? '；以下保留上次读取的数据。' : '' }}</p>
    <p v-if="loading && !data" class="muted" role="status">正在读取本人的调用记录…</p>
    <template v-if="data">
      <div class="usage-receipt">
        <div class="usage-money"><span>本人累计估算费用</span><strong>{{ costLabel(data.summary.chargedMicros) }}</strong><small>所有运行及恢复尝试 · 供应商账单为准</small></div>
        <div class="usage-hold"><span>本人预占金额</span><strong>{{ costLabel(data.summary.reservedMicros) }}</strong><small>{{ data.summary.pendingCalls }} 次结果未确认（含执行中），预占不等于已扣费</small></div>
      </div>
      <dl class="usage-facts"><div><dt>模型调用</dt><dd>{{ data.summary.providerCalls }} 次</dd></div><div><dt>已确认生成图片</dt><dd>{{ data.summary.generatedImages }} 张</dd></div><div><dt>当前失败运行</dt><dd>{{ data.summary.failedRuns }} 条</dd></div><div><dt>待确认运行</dt><dd>{{ data.summary.pendingRuns }} 条</dd></div></dl>
      <p class="test-mode-note">预算是整个实例的累计上限{{ data.budgetLimitMicros !== '0' ? `（当前 ${costLabel(data.budgetLimitMicros)}）` : '' }}，不是个人余额。本页只显示本人用量，不能用上限减去本页费用判断还能用多少。</p>
      <p class="usage-explanation">图片数来自已确认的模型调用，包含尚未归档的图片；本地裁剪、抠图、放大、T 恤处理和导出不计入生图数。模拟结果与测试额度均不计人民币费用。</p>
      <div class="usage-list" aria-label="每次运行用量" :aria-busy="loading">
        <p v-if="!data.rows.length" class="empty-runs">还没有运行用量。在画布确认执行后，这里会保留费用与状态。</p>
        <article v-for="row in data.rows" :key="row.runId" class="usage-row">
          <div class="usage-run-title"><strong>{{ row.projectName }}</strong><small>#{{ row.runId }} · 第 {{ row.attempt }} 次 · {{ row.testOnly ? '测试模式' : '真实模式' }} · {{ row.createTime }}</small><span class="run-state" :data-state="row.state">{{ runStateLabel[row.state] }}</span></div>
          <div class="usage-run-amount"><strong>{{ costLabel(row.chargedMicros) }}</strong><small>预占 {{ costLabel(row.reservedMicros) }} · 模型 {{ row.providerCalls }} 次 · 生图 {{ row.generatedImages }} 张</small><small v-if="row.pendingCalls">{{ row.pendingCalls }} 次调用结果未确认，费用尚未结清</small><small v-else-if="!row.providerCalls">未记录收费模型调用；可能是本地处理、测试或尚未提交</small></div>
          <button v-if="!projectId || projectId === row.projectId" class="text-button" :aria-label="`查看运行 ${row.runId}`" @click="emit('open', row.runId)">查看运行</button><small v-else class="muted">其他项目，请在运行主页面查看</small>
        </article>
      </div>
      <div class="run-pagination"><button class="text-button" :disabled="loading || page <= 1" @click="page--">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(data.total / 10)) }}</span><button class="text-button" :disabled="loading || page * 10 >= data.total" @click="page++">下一页</button></div>
      <details class="usage-help"><summary>预算不足或结果待确认，怎么办？</summary><p>预算不足：该失败步骤未调用模型，此前已完成的调用仍计入费用。先查看预占和运行详情，请部署者核对实例上限；反复点击、删除记录都不能释放费用。</p><p>图片归档待确认：保留运行编号，稍后刷新。系统只恢复原结果下载，不重新生图。</p><p>调用结果未知：保留预占，请部署者凭本人运行编号核对供应商账单。当前没有人工结算按钮，不能盲目重发。提交网络中断时使用“重试原提交”，沿用原请求标识。</p></details>
    </template>
  </section>
</template>
