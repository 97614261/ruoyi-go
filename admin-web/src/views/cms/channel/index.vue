<template>
  <div class="app-container">
    <el-form :inline="true" :model="query" @submit.prevent="search">
      <el-form-item label="栏目名称"><el-input v-model="query.name" placeholder="输入名称前缀" maxlength="60" clearable @keyup.enter="search"/></el-form-item>
      <el-form-item label="类型"><el-select v-model="query.kind" clearable placeholder="全部类型" style="width:140px"><el-option label="文章栏目" value="ARTICLE"/><el-option label="产品栏目" value="PRODUCT"/></el-select></el-form-item>
      <el-form-item label="状态"><el-select v-model="query.status" clearable placeholder="全部状态" style="width:120px"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
      <el-form-item><el-button type="primary" icon="Search" @click="search">搜索</el-button><el-button icon="Refresh" @click="reset">重置</el-button></el-form-item>
    </el-form>
    <div class="cms-toolbar"><el-button v-hasPermi="['cms:channel:add']" type="primary" plain icon="Plus" :disabled="busy" @click="edit()">新增栏目</el-button><el-button :disabled="loading" @click="load">刷新</el-button><span class="cms-note">最多8层；父栏目停用前请先停用子栏目。当前只有管理功能，尚未公开到门户。</span></div>
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="name" label="栏目名称" min-width="150"/><el-table-column prop="parentName" label="上级栏目" min-width="130"/>
      <el-table-column label="类型" width="100"><template #default="s">{{ s.row.kind === 'ARTICLE' ? '文章' : '产品' }}</template></el-table-column>
      <el-table-column prop="slug" label="栏目地址" min-width="130"/><el-table-column prop="orderNum" label="排序" width="75"/>
      <el-table-column label="状态" width="80"><template #default="s"><el-tag :type="s.row.status === '0' ? 'success' : 'info'">{{ s.row.status === '0' ? '启用' : '停用' }}</el-tag></template></el-table-column>
      <el-table-column prop="coverName" label="封面" min-width="120" show-overflow-tooltip/>
      <el-table-column label="操作" width="150"><template #default="s"><el-button v-hasPermi="['cms:channel:edit']" link type="primary" :disabled="busy" @click="edit(s.row)">修改</el-button><el-button v-hasPermi="['cms:channel:remove']" link type="danger" :disabled="busy" @click="remove(s.row)">删除</el-button></template></el-table-column>
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
    <el-dialog v-model="visible" :title="form.channelId ? '修改栏目' : '新增栏目'" width="min(760px, calc(100vw - 32px))" append-to-body :close-on-click-modal="false" :show-close="!busy">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="内容类型" prop="kind"><el-radio-group v-model="form.kind"><el-radio value="ARTICLE">文章栏目</el-radio><el-radio value="PRODUCT">产品栏目</el-radio></el-radio-group></el-form-item>
        <el-form-item label="上级栏目"><el-select v-model="form.parentId" filterable remote :remote-method="findParents" style="width:100%" @visible-change="v => v && findParents('')"><el-option label="顶级栏目" :value="0"/><el-option v-for="row in parents" :key="row.channelId" :label="`${row.name} · ${row.slug}`" :value="row.channelId"/></el-select><span class="cms-note">输入上级名称前缀查找，默认最多显示50项。</span></el-form-item>
        <el-form-item label="栏目名称" prop="name"><el-input v-model="form.name" maxlength="60" show-word-limit/></el-form-item>
        <el-form-item label="栏目地址" prop="slug"><el-input v-model="form.slug" maxlength="80" placeholder="如 company-news，只填英文短名称"/></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.orderNum" :min="0" :max="100000"/></el-form-item>
        <el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item>
        <el-form-item label="栏目封面"><AssetPicker v-model="form.coverAssetId" :label="coverName" @picked="v => coverName = v"/></el-form-item>
        <el-form-item label="SEO标题"><el-input v-model="form.seoTitle" maxlength="150"/></el-form-item>
        <el-form-item label="SEO描述"><el-input v-model="form.seoDescription" type="textarea" :rows="2" maxlength="300"/></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" maxlength="500"/></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="busy" @click="visible = false">取消</el-button><el-button type="primary" :loading="busy" @click="submit">保存</el-button></template>
    </el-dialog>
  </div>
</template>
<script setup name="CmsChannel">
import { ref, reactive, onMounted, onActivated, onDeactivated, onBeforeUnmount, nextTick, getCurrentInstance } from 'vue'
import { listChannels, getChannel, saveChannel, deleteChannel } from '@/api/cms/content'
import AssetPicker from '../components/AssetPicker.vue'
const { proxy } = getCurrentInstance()
const rows = ref([]), total = ref(0), loading = ref(false), busy = ref(false), visible = ref(false), formRef = ref(), parents = ref([]), coverName = ref('')
const query = reactive({ name: '', kind: '', status: '', pageNum: 1, pageSize: 10 })
const empty = () => ({ channelId: 0, parentId: 0, name: '', kind: 'ARTICLE', slug: '', orderNum: 0, status: '0', coverAssetId: 0, seoTitle: '', seoDescription: '', remark: '', version: 0 })
const form = reactive(empty()), rules = { name: [{ required: true, message: '请输入栏目名称', trigger: 'blur' }], slug: [{ required: true, pattern: /^[a-z][a-z0-9-]{0,79}$/, message: '请输入小写英文地址，可含数字和连字符', trigger: 'blur' }] }
let active = true, epoch = 0, parentEpoch = 0, operation = 0
async function load() { const ticket = ++epoch; loading.value = true; try { const data = await listChannels({ ...query }); if (active && ticket === epoch) { rows.value = data.rows; total.value = data.total } } finally { if (ticket === epoch) loading.value = false } }
function search() { query.pageNum = 1; load() }
function reset() { Object.assign(query, { name: '', kind: '', status: '', pageNum: 1 }); load() }
async function findParents(name) { const ticket = ++parentEpoch; const data = await listChannels({ name, kind: form.kind, pageNum: 1, pageSize: 50 }); if (active && ticket === parentEpoch && visible.value) { const current = parents.value.find(v => v.channelId === form.parentId); parents.value = data.rows.filter(v => v.channelId !== form.channelId); if (current && !parents.value.some(v => v.channelId === current.channelId)) parents.value.push(current) } }
async function edit(row) { if (busy.value) return; const ticket = ++operation; busy.value = true; try { const detail = row ? (await getChannel(row.channelId)).data : empty(); if (!active || ticket !== operation) return; Object.assign(form, empty(), detail, { coverAssetId: detail.coverAssetId || 0 }); coverName.value = detail.coverName || ''; parents.value = detail.parentId ? [{ channelId: detail.parentId, name: detail.parentName, slug: '当前上级' }] : []; visible.value = true; await nextTick(); formRef.value?.clearValidate() } finally { if (ticket === operation) busy.value = false } }
async function submit() { if (busy.value) return; const ticket = ++operation; busy.value = true; try { await formRef.value.validate(); if (!active || ticket !== operation) return; await saveChannel({ ...form }); if (active && ticket === operation) { visible.value = false; proxy.$modal.msgSuccess('保存成功'); await load() } } catch {} finally { if (ticket === operation) busy.value = false } }
async function remove(row) { if (busy.value) return; const ticket = ++operation; busy.value = true; try { await proxy.$modal.confirm(`确认删除栏目“${row.name}”？存在子栏目时不能删除。`); if (!active || ticket !== operation) return; await deleteChannel({ ...row }); if (active && ticket === operation) { proxy.$modal.msgSuccess('删除成功'); await load() } } catch {} finally { if (ticket === operation) busy.value = false } }
function leave() { active = false; ++epoch; ++parentEpoch; ++operation; visible.value = false; busy.value = false; loading.value = false }
onMounted(load); onActivated(() => { if (!active) { active = true; load() } }); onDeactivated(leave); onBeforeUnmount(leave)
</script>
<style scoped>.cms-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:8px;margin-bottom:12px}.cms-note{font-size:12px;color:var(--el-text-color-secondary)}.el-form--inline .el-form-item{margin-right:16px;margin-bottom:12px}</style>
