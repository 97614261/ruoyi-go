<script setup lang="ts">
import { computed } from 'vue'
import type { Graph, Run } from './contracts'
import { executionImpact } from './execution-impact'
import { costLabel } from './run-state'
const props = defineProps<{ graph?: Graph; mode: Run['mode']; target?: string; real: boolean; managed?: boolean }>()
const impact = computed(() => executionImpact(props.graph, props.mode, props.target))
</script>
<template>
  <div class="run-impact" aria-label="执行范围预览">
    <template v-if="impact">
      <strong>{{ mode === 'branch' ? '当前节点及所需上游' : mode === 'polish' ? '仅本次文字润色' : mode === 'node' ? '仅选中节点，复用历史上游' : '整条流程' }}</strong>
      <p>{{ real ? '预计' : '模拟' }}文本调用 {{ impact.textCalls }} 次 · 生图调用 {{ impact.imageCalls }} 次 / {{ impact.images }} 张 · 本地步骤 {{ impact.localNodes }} 个</p>
      <p v-if="real && impact.names.length">涉及 AI：{{ impact.names.join('、') }}。上游也会重新执行（单节点模式除外）。</p>
      <p v-if="real && !managed && (impact.textCalls || impact.imageCalls)">逐项预占合计 {{ costLabel(impact.reservationMicros) }}，不是最终费用；文字按 Token 结算，图片暂估 0.10 元/张。以实际开始的步骤和供应商账单为准。</p>
      <p v-else-if="managed">系统自动核价并预占点数，本地处理不扣正式点数。</p><p v-else-if="real">本次范围没有收费模型节点，本地处理不产生模型费用。</p>
    </template>
    <p v-else>暂不能计算执行范围，请先检查并保存画布；上游含 AI 时仍可能产生费用。最终以服务端校验和实际调用为准。</p>
    <p v-if="mode === 'node'">需要同版本、同模式且已成功的历史上游；缺失时会拒绝，不自动重跑上游。</p>
  </div>
</template>
