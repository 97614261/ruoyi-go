<template>
  <section class="erp-query-bar">
    <el-form v-show="showSearch" ref="formRef" :model="model" label-width="auto" class="erp-query-form" role="search" aria-label="列表筛选" @submit.prevent="search">
      <div class="erp-query-fields"><slot /></div>
      <div class="erp-query-actions">
        <el-button type="primary" :icon="Search" native-type="submit" :loading="loading" :disabled="disabled">查询</el-button>
        <el-button :icon="Refresh" :disabled="loading || disabled" @click="reset">重置</el-button>
      </div>
    </el-form>
    <div v-if="$slots.toolbar" class="erp-query-toolbar"><slot name="toolbar" /></div>
  </section>
</template>

<script setup>
import { ref } from 'vue'
import { Search, Refresh } from '@element-plus/icons-vue'
const props = defineProps({
  model: Object,
  loading: Boolean,
  disabled: Boolean,
  showSearch: { type: Boolean, default: true }
})
const emit = defineEmits(['search', 'reset'])
const formRef = ref()
function search() { if (!props.loading && !props.disabled) emit('search') }
function reset() { if (!props.loading && !props.disabled) emit('reset') }
defineExpose({ resetFields: () => formRef.value?.resetFields() })
</script>

<style scoped>
.erp-query-bar { margin-bottom: 12px; }
.erp-query-form { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 12px; padding: 12px; background: var(--el-fill-color-extra-light); border: 1px solid var(--el-border-color-lighter); border-radius: var(--el-border-radius-base); }
.erp-query-fields { flex: 1 1 480px; min-width: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr)); gap: 12px 16px; }
.erp-query-fields :deep(.el-form-item) { min-width: 0; margin: 0; }
.erp-query-fields :deep(.el-form-item__content) { min-width: 0; }
.erp-query-fields :deep(.el-input), .erp-query-fields :deep(.el-input-number), .erp-query-fields :deep(.el-select), .erp-query-fields :deep(.el-date-editor) { width: 100%; min-width: 0; }
.erp-query-fields :deep(.erp-query-wide) { grid-column: 1 / -1; }
.erp-query-actions, .erp-query-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.erp-query-actions { flex-shrink: 0; margin-left: auto; }
.erp-query-toolbar { margin-top: 10px; }
.erp-query-toolbar:empty { display: none; }
.erp-query-actions :deep(.el-button + .el-button), .erp-query-toolbar :deep(.el-button + .el-button) { margin-left: 0; }
@media (max-width: 600px) { .erp-query-fields { grid-template-columns: minmax(0, 1fr); } .erp-query-fields :deep(.erp-query-wide) { grid-column: auto; } }
</style>
