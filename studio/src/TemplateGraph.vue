<script setup lang="ts">
import { computed } from 'vue'
import type { Graph } from './contracts'
const props = defineProps<{ graph: Graph }>()
const labels: Record<string, string> = { textInput: '文本', imageInput: '图片', textGenerate: 'AI 文本', imageGenerate: 'AI 生图', imageProcess: '图片处理', productPreview: 'T 恤预览', export: '导出' }
const box = computed(() => {
  const xs = props.graph.nodes.map(n => n.position.x), ys = props.graph.nodes.map(n => n.position.y)
  const x = Math.min(0, ...xs), y = Math.min(0, ...ys)
  return `${x - 25} ${y - 25} ${Math.max(300, ...xs.map(v => v - x + 175))} ${Math.max(150, ...ys.map(v => v - y + 90))}`
})
const connections = computed(() => props.graph.edges.map(e => {
  const a = props.graph.nodes.find(n => n.id === e.source), b = props.graph.nodes.find(n => n.id === e.target)
  if (!a || !b) return ''
  const x = a.position.x + 145, y = a.position.y + 28, endX = b.position.x, endY = b.position.y + 28
  return `M${x},${y} C${x + 75},${y} ${endX - 75},${endY} ${endX},${endY}`
}))
</script>
<template>
  <svg class="template-graph" :viewBox="box" role="img" aria-label="模板流程结构">
    <path v-for="(path, i) in connections" :key="i" :d="path" class="template-wire" />
    <g v-for="node in graph.nodes" :key="node.id" :transform="`translate(${node.position.x},${node.position.y})`">
      <rect width="145" height="56" rx="9" />
      <circle cx="14" cy="18" r="4" :class="`template-dot-${node.type}`" />
      <text x="26" y="22">{{ String(node.config.title || labels[node.type] || node.type).slice(0, 10) }}</text>
      <text x="14" y="43" class="template-node-kind">{{ labels[node.type] }}</text>
    </g>
  </svg>
</template>
