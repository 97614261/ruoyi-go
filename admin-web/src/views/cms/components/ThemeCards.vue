<template>
  <div class="theme-grid"><article v-for="t in themes" :key="t.id" class="theme-card" :class="{ selected: modelValue === t.id }"><img :src="pictures[t.id]" :alt="t.name+'布局示意'" /><div class="theme-info"><strong>{{ t.name }}</strong><el-tag v-if="modelValue === t.id" size="small">当前草稿选择</el-tag><p>{{ t.description }}</p><small>资源版本 {{ t.version }} · 示意图，实际以预览为准</small><el-button :disabled="disabled || modelValue === t.id" type="primary" plain @click="$emit('update:modelValue',t.id)">{{ modelValue === t.id ? '已选择' : '选择此模板' }}</el-button></div></article></div>
  <p>选择只修改当前表单。保存草稿后可对照预览，再发布生效；不会生成示例文章、重置首页区块或改动线上。</p>
</template>
<script setup>
import business from '@/assets/portal/business.svg'
import editorial from '@/assets/portal/editorial.svg'
import industrial from '@/assets/portal/industrial.svg'
defineProps({ modelValue: String, themes: { type: Array, default: () => [] }, disabled: Boolean });defineEmits(['update:modelValue'])
const pictures={business,editorial,industrial}
</script>
<style scoped>
.theme-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px;margin:12px 0}.theme-card{border:1px solid var(--el-border-color);border-radius:6px;overflow:hidden}.theme-card.selected{border-color:var(--el-color-primary);box-shadow:0 0 0 1px var(--el-color-primary)}.theme-card>img{width:100%;display:block;aspect-ratio:16/9;object-fit:cover;border-bottom:1px solid var(--el-border-color)}.theme-info{padding:14px}.theme-info strong{margin-right:8px}.theme-info p{min-height:40px;color:var(--el-text-color-regular)}.theme-info small{display:block;margin-bottom:12px;color:var(--el-text-color-secondary)}@media(max-width:1000px){.theme-grid{grid-template-columns:1fr}}
</style>
