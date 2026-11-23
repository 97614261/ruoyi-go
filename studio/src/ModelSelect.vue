<script setup lang="ts">
import { computed } from 'vue'
import type { CreatorModel } from './contracts'
const props = defineProps<{ models: CreatorModel[]; kind: 'text' | 'image'; value?: unknown; disabled?: boolean; label?: string; compact?: boolean }>()
const emit = defineEmits<{ change: [id: string] }>()
const options = computed(() => props.models.filter(m => m.kind === props.kind))
const current = computed(() => String(props.value || ''))
const fallback = computed(() => options.value.find(m => m.default))
const selected = computed(() => current.value ? options.value.find(m => m.modelId === current.value) : fallback.value)
const rate = computed(() => {
  const rule = selected.value?.pointRule
  if (!rule) return ''
  return props.kind === 'image' ? `每张 ${rule.perImage} 点` : `每百万 Token：输入 ${rule.inputPerMillion} 点 / 输出 ${rule.outputPerMillion} 点`
})
</script>
<template>
  <label class="model-select">{{ label || '生成模型' }}
    <select :value="current" :disabled="disabled" @change="emit('change', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ fallback ? (compact ? `${fallback.name}（默认）` : `平台默认 · ${fallback.name}`) : '平台默认（尚无可用默认模型）' }}</option>
      <option v-if="current && !options.some(m => m.modelId === current)" :value="current" disabled>原模型 #{{ current }} 已不可用，请重新选择</option>
      <option v-for="m in options" :key="m.modelId" :value="m.modelId">{{ m.name }}</option>
    </select>
    <small v-if="!options.length">暂无已授权的可用模型，请联系管理员配置。</small>
    <small v-else-if="selected && compact" class="model-rate" :title="`${selected.officialModelId} · 版本 #${selected.modelVersionId}；最终预占以执行前报价为准。`">{{ rate }}</small>
    <small v-else-if="selected" class="model-rate">{{ selected.officialModelId }} · 版本 #{{ selected.modelVersionId }}<br />{{ rate }}；最终预占以执行前报价为准。</small>
  </label>
</template>
<style scoped>
.model-select{display:grid;gap:7px;font-size:12px;color:var(--muted)}.model-select select{width:100%;min-width:0;min-height:34px;padding:6px 8px;border:1px solid var(--line);border-radius:6px;background:var(--paper);color:var(--ink)}.model-select small{white-space:normal;line-height:1.6}
</style>
