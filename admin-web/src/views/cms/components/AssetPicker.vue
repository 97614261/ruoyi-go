<template>
  <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
    <span>{{ modelValue ? (label || `素材 #${modelValue}`) : '未选择' }}</span>
    <el-button v-if="canChoose" @click="open">选择素材</el-button>
    <el-button v-if="modelValue" @click="$emit('update:modelValue', 0); $emit('picked', '')">清除</el-button>
    <el-button v-if="modelValue" v-hasPermi="['cms:asset:query']" link type="primary" @click="preview.open(modelValue)">预览</el-button>
    <el-dialog v-model="visible" title="选择门户图片" width="min(820px, calc(100vw - 32px))" append-to-body>
      <el-form inline @submit.prevent="search"><el-form-item label="名称前缀"><el-input v-model="query.name" clearable maxlength="150" @keyup.enter="search" /></el-form-item><el-form-item><el-button type="primary" @click="search">搜索</el-button></el-form-item></el-form>
      <el-table :data="rows" v-loading="loading"><el-table-column prop="assetId" label="编号" width="90"/><el-table-column prop="name" label="素材名称" min-width="180"/><el-table-column label="尺寸" width="120"><template #default="s">{{ s.row.width }}×{{ s.row.height }}</template></el-table-column><el-table-column label="操作" width="140"><template #default="s"><el-button link type="primary" @click="preview.open(s.row.assetId)">预览</el-button><el-button link type="primary" @click="choose(s.row)">选择</el-button></template></el-table-column></el-table>
      <pagination v-show="total > 0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
    </el-dialog>
    <PrivateImage ref="preview"/>
  </div>
</template>
<script setup>
import { ref, reactive, onDeactivated, onBeforeUnmount } from 'vue'
import { listAssets } from '@/api/cms/content'
import PrivateImage from './PrivateImage.vue'
import auth from '@/plugins/auth'
const canChoose = auth.hasPermi('cms:asset:list') && auth.hasPermi('cms:asset:query')
defineProps({ modelValue: Number, label: String })
const emit = defineEmits(['update:modelValue','picked'])
const visible = ref(false), rows = ref([]), total = ref(0), loading = ref(false), preview = ref()
const query = reactive({ name: '', pageNum: 1, pageSize: 10 }); let epoch = 0
async function load() { const ticket = ++epoch; loading.value = true; try { const data = await listAssets({ ...query }); if (ticket === epoch && visible.value) { rows.value = data.rows; total.value = data.total } } finally { if (ticket === epoch) loading.value = false } }
function open() { visible.value = true; load() }
function search() { query.pageNum = 1; load() }
function choose(row) { emit('update:modelValue', row.assetId); emit('picked', row.name); visible.value = false }
function cleanup() { ++epoch; visible.value = false; loading.value = false }
onDeactivated(cleanup); onBeforeUnmount(cleanup)
</script>
