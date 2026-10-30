<template>
  <div class="app-container">
    <el-form inline @submit.prevent="search"><el-form-item label="素材名称"><el-input v-model="query.name" placeholder="输入名称前缀" clearable maxlength="150" @keyup.enter="search"/></el-form-item><el-form-item><el-button type="primary" icon="Search" @click="search">搜索</el-button><el-button icon="Refresh" @click="reset">重置</el-button></el-form-item></el-form>
    <div class="cms-toolbar"><el-button v-hasPermi="['cms:asset:upload']" type="primary" plain icon="Upload" :loading="busy" @click="fileInput.click()">上传图片</el-button><input ref="fileInput" type="file" accept="image/png,image/jpeg,image/webp" hidden @change="upload"><el-button :disabled="loading" @click="load">刷新</el-button><span class="cms-note">图片仅在授权后台预览；PNG/JPEG/WebP，单张8MB、1200万像素以内，上传后重编码并移除原始元数据。</span></div>
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="assetId" label="编号" width="80"/><el-table-column prop="name" label="素材名称" min-width="180" show-overflow-tooltip/>
      <el-table-column label="尺寸" width="120"><template #default="s">{{ s.row.width }}×{{ s.row.height }}</template></el-table-column>
      <el-table-column label="大小" width="100"><template #default="s">{{ Math.ceil(s.row.byteSize / 1024) }} KB</template></el-table-column>
      <el-table-column prop="altText" label="替代文字" min-width="140" show-overflow-tooltip/><el-table-column prop="referenceCount" label="引用数" width="85"/>
      <el-table-column prop="createTime" label="上传时间" width="165"/>
      <el-table-column label="操作" min-width="220"><template #default="s"><el-button v-hasPermi="['cms:asset:query']" link type="primary" @click="preview.open(s.row.assetId)">预览</el-button><el-button v-hasPermi="['cms:asset:edit']" link type="primary" :disabled="busy" @click="edit(s.row)">修改</el-button><el-button v-if="canSeeReferences" link type="primary" :disabled="busy" @click="references(s.row)">引用</el-button><el-button v-hasPermi="['cms:asset:remove']" link type="danger" :disabled="busy || s.row.referenceCount > 0" @click="remove(s.row)">删除</el-button></template></el-table-column>
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
    <el-dialog v-model="visible" title="修改素材信息" width="min(600px, calc(100vw - 32px))" append-to-body :close-on-click-modal="false" :show-close="!busy">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="95px"><el-form-item label="素材名称" prop="name"><el-input v-model="form.name" maxlength="150"/></el-form-item><el-form-item label="替代文字"><el-input v-model="form.altText" maxlength="200" placeholder="描述图片内容，供无障碍和门户展示使用"/></el-form-item><el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="3" maxlength="500"/></el-form-item></el-form>
      <template #footer><el-button :disabled="busy" @click="visible = false">取消</el-button><el-button type="primary" :loading="busy" @click="submit">保存</el-button></template>
    </el-dialog>
    <el-dialog v-model="referenceVisible" title="素材使用位置" width="min(650px, calc(100vw - 32px))" append-to-body><el-table :data="referenceRows"><el-table-column prop="ownerType" label="引用类型"/><el-table-column prop="ownerId" label="编号"/><el-table-column prop="ownerName" label="使用位置"/></el-table><pagination v-show="referenceTotal > 0" :total="referenceTotal" v-model:page="referenceQuery.pageNum" v-model:limit="referenceQuery.pageSize" @pagination="loadReferences"/></el-dialog>
    <PrivateImage ref="preview"/>
  </div>
</template>
<script setup name="CmsAsset">
import { ref, reactive, computed, onMounted, onActivated, onDeactivated, onBeforeUnmount, nextTick, getCurrentInstance } from 'vue'
import { listAssets, getAsset, saveAsset, deleteAsset, uploadAsset, listAssetReferences } from '@/api/cms/content'
import { checkPermi } from '@/utils/permission'
import PrivateImage from '../components/PrivateImage.vue'
const { proxy } = getCurrentInstance()
const rows = ref([]), total = ref(0), loading = ref(false), busy = ref(false), visible = ref(false), formRef = ref(), preview = ref(), fileInput = ref()
const query = reactive({ name: '', pageNum: 1, pageSize: 10 }), form = reactive({ assetId: 0, name: '', altText: '', remark: '', version: 0 })
const rules = { name: [{ required: true, message: '请输入素材名称', trigger: 'blur' }] }
const referenceVisible = ref(false), referenceRows = ref([]), referenceTotal = ref(0), referenceId = ref(0), referenceQuery = reactive({ pageNum: 1, pageSize: 10 })
const canSeeReferences = computed(() => checkPermi(['cms:asset:query']) && checkPermi(['cms:channel:query']))
let active = true, epoch = 0, operation = 0, referenceEpoch = 0
async function load() { const ticket = ++epoch; loading.value = true; try { const data = await listAssets({ ...query }); if (active && ticket === epoch) { rows.value = data.rows; total.value = data.total } } finally { if (ticket === epoch) loading.value = false } }
function search() { query.pageNum = 1; load() }
function reset() { query.name = ''; search() }
async function upload(event) { const file = event.target.files?.[0]; event.target.value = ''; if (!file || busy.value) return; if (file.size > 8 * 1024 * 1024) { proxy.$modal.msgError('图片不得超过8MB'); return } const ticket = ++operation; busy.value = true; try { await uploadAsset(file); if (active && ticket === operation) { proxy.$modal.msgSuccess('上传成功'); await load() } } finally { if (ticket === operation) busy.value = false } }
async function edit(row) { if (busy.value) return; const ticket = ++operation; busy.value = true; try { const data = await getAsset(row.assetId); if (active && ticket === operation) { Object.assign(form, data.data); visible.value = true; await nextTick(); formRef.value?.clearValidate() } } finally { if (ticket === operation) busy.value = false } }
async function submit() { if (busy.value) return; const ticket = ++operation; busy.value = true; try { await formRef.value.validate(); if (!active || ticket !== operation) return; await saveAsset(form.assetId, { name: form.name, altText: form.altText, remark: form.remark, version: form.version }); if (active && ticket === operation) { visible.value = false; proxy.$modal.msgSuccess('保存成功'); await load() } } catch {} finally { if (ticket === operation) busy.value = false } }
async function remove(row) { if (busy.value) return; const ticket = ++operation; busy.value = true; try { await proxy.$modal.confirm(`确认删除素材“${row.name}”？有引用的素材不能删除。`); if (!active || ticket !== operation) return; await deleteAsset({ ...row }); if (active && ticket === operation) { proxy.$modal.msgSuccess('删除成功'); await load() } } catch {} finally { if (ticket === operation) busy.value = false } }
async function loadReferences() { const ticket = ++referenceEpoch; const data = await listAssetReferences(referenceId.value, { ...referenceQuery }); if (active && referenceVisible.value && ticket === referenceEpoch) { referenceRows.value = data.rows; referenceTotal.value = data.total } }
function references(row) { referenceId.value = row.assetId; referenceQuery.pageNum = 1; referenceVisible.value = true; loadReferences() }
function leave() { active = false; ++epoch; ++operation; ++referenceEpoch; visible.value = false; referenceVisible.value = false; busy.value = false; loading.value = false }
onMounted(load); onActivated(() => { if (!active) { active = true; load() } }); onDeactivated(leave); onBeforeUnmount(leave)
</script>
<style scoped>.cms-toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:12px}.cms-note{font-size:12px;color:var(--el-text-color-secondary)}</style>
