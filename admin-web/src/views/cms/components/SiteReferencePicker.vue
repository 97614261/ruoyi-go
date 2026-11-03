<template>
  <el-button :disabled="disabled || !available" @click="open">{{ caption }}</el-button>
  <el-dialog v-model="visible" :title="type === 'channel' ? '选择栏目' : '选择内容'" width="min(760px,calc(100vw - 24px))" append-to-body destroy-on-close @closed="close">
    <p>按名称前缀搜索。{{ type === 'content' ? '未发布内容也可选，发布外观前须先发布目标内容。' : '栏目及上级需启用才能在前台展示。' }}</p>
    <el-form inline @submit.prevent="search"><el-form-item label="名称"><el-input v-model="query.name" placeholder="输入名称前缀" clearable maxlength="120" @keyup.enter="search" /></el-form-item><el-form-item v-if="!kind" label="类型"><el-select v-model="selectedKind" style="width:130px" @change="search"><el-option v-for="k in kinds" :key="k" :label="kindNames[k]" :value="k" /></el-select></el-form-item><el-button type="primary" @click="search">搜索</el-button><el-button v-if="allowClear" @click="choose(null)">{{ clearLabel }}</el-button></el-form>
    <el-table :data="rows" v-loading="loading"><el-table-column prop="title" label="名称" min-width="200" /><el-table-column prop="url" label="前台地址" show-overflow-tooltip /><el-table-column label="状态" width="120"><template #default="s">{{ type === 'channel' ? (s.row.enabled ? '启用' : '停用') : (s.row.enabled ? '已标记发布' : '未发布') }}</template></el-table-column><el-table-column width="85"><template #default="s"><el-button link type="primary" @click="choose(s.row)">选择</el-button></template></el-table-column></el-table>
    <pagination v-if="total" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load" />
  </el-dialog>
</template>
<script setup>
import { ref, reactive, computed, inject, watch, onBeforeUnmount, onDeactivated } from 'vue'
import { workflow } from '@/api/cms/workflow'
import auth from '@/plugins/auth'
const props = defineProps({ modelValue: { type: Number, default: 0 }, type: { type: String, default: 'content' }, kind: { type: String, default: '' }, allowClear: { type: Boolean, default: true }, clearLabel: { type: String, default: '不跳转' }, disabled: Boolean })
const emit = defineEmits(['update:modelValue', 'selected'])
const references = inject('siteReferenceNames', ref({ channel: {}, content: {} }))
const local = ref(null), visible = ref(false), rows = ref([]), total = ref(0), loading = ref(false), query = reactive({ name: '', pageNum: 1, pageSize: 10 })
const kindNames = { ARTICLE: '文章', PRODUCT: '产品', PAGE: '单页' }
const kinds = computed(() => ['ARTICLE', 'PRODUCT', ...(props.type === 'content' ? ['PAGE'] : [])].filter(k => (!props.kind || props.kind === k) && auth.hasPermi(props.type === 'channel' ? 'cms:channel:list' : `cms:${k.toLowerCase()}:list`)))
const selectedKind = ref(''), available = computed(() => auth.hasPermi('cms:site:query') && kinds.value.length > 0)
const caption = computed(() => {
  if (!props.modelValue) return props.clearLabel
  const row = local.value?.refId === props.modelValue ? local.value : references.value[props.type]?.[props.modelValue]
  return row ? row.title : `已选${props.type === 'channel' ? '栏目' : '内容'} #${props.modelValue}（点击更换）`
})
let epoch = 0
watch(() => props.modelValue, () => { local.value = null })
async function open() { if (!available.value) return; visible.value = true; selectedKind.value = kinds.value[0]; query.pageNum = 1; query.name = ''; await load() }
function search() { query.pageNum = 1; load() }
async function load() { const ticket = ++epoch; loading.value = true; try { const r = await workflow('site/references', 'get', undefined, { ...query, type: props.type, kind: props.kind || selectedKind.value }); if (ticket === epoch && visible.value) { rows.value = r.rows; total.value = r.total } } finally { if (ticket === epoch) loading.value = false } }
function choose(row) { local.value = row; if (row) references.value[props.type][row.refId] = row; emit('update:modelValue', row?.refId || 0); emit('selected', row); visible.value = false }
function close() { ++epoch; visible.value = false; loading.value = false; rows.value = [] }
onBeforeUnmount(close); onDeactivated(close)
</script>
