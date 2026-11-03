<template>
  <div class="app-container" v-loading="loading">
    <h3>询盘数据保留与审计</h3>
    <el-alert title="这里管理客户询盘的保存期限，记录导出、清理等操作。默认不限期；保存策略不会自动删除，只有预览后再次确认才会擦除。" :closable="false" />
    <el-tabs v-model="tab" :before-leave="canLeave" @tab-change="loadTab">
      <el-tab-pane label="保留设置" name="policy">
        <el-alert v-if="policy && formVersion !== policy.version" title="已保存策略有更新，当前表单保留了你的输入。请重新读取后确认。" type="warning" :closable="false" />
        <el-form label-width="140px" style="max-width:850px;margin-top:16px" @submit.prevent>
          <el-form-item label="无活动保留天数"><el-input-number v-model="days" :min="0" :max="3650" :precision="0" :disabled="!canEdit || busy" /><span>0表示不限；从最后一次提交、处理、改派或跟进计算。</span></el-form-item>
          <el-form-item label="适用状态"><el-checkbox-group v-model="statuses" :disabled="!canEdit || busy"><el-checkbox label="已处理" value="DONE" /><el-checkbox label="垃圾留言" value="SPAM" /><el-checkbox label="待处理（慎选）" value="NEW" /></el-checkbox-group></el-form-item>
          <el-form-item v-if="canEdit" label="变更原因"><el-input v-model="reason" maxlength="300" placeholder="填写设置保留策略的原因" /></el-form-item>
          <el-form-item><el-button v-if="canEdit" type="primary" :loading="busy" @click="save">保存保留策略</el-button><el-button :disabled="busy" @click="loadPolicy(true)">重新读取</el-button></el-form-item>
        </el-form>
      </el-tab-pane>
      <el-tab-pane v-if="canClean" label="手动清理" name="cleanup">
        <p>当前已保存：{{ policy?.retentionDays || 0 }}天；状态 {{ (policy?.statuses || []).map(s => labels[s]).join('、') }}。未保存的表单不参与清理。</p>
        <el-alert title="每批最多100条，预览15分钟内有效。执行会擦除联系人、正文、备注及跟进文字并取消提醒，无法撤销；历史备份需要另行管理。" type="warning" :closable="false" />
        <div style="display:flex;gap:8px;margin:14px 0"><el-input v-model="cleanReason" maxlength="300" placeholder="本次手动清理的原因" style="max-width:500px" /><el-button :disabled="!policy?.retentionDays" :loading="busy" @click="preview">生成预览</el-button></div>
        <template v-if="batch">
          <el-alert :title="`预览 #${batch.batchId}：符合条件${batch.totalEligible}条，本批${batch.previewCount}条；有效期至${batch.expiresAt}，${batchLabels[batch.status] || batch.status}`" :closable="false" />
          <el-table :data="items"><el-table-column prop="inquiryId" label="询盘" width="90" /><el-table-column prop="name" label="称呼" /><el-table-column prop="contact" label="联系方式（遮蔽）" /><el-table-column prop="activeAt" label="最后活动" width="170" /><el-table-column label="版本"><template #default="s">预览 {{ s.row.sourceVersion }} / 当前 {{ s.row.currentVersion }}<el-tag v-if="s.row.sourceVersion !== s.row.currentVersion" type="danger">已变化</el-tag></template></el-table-column></el-table>
          <pagination v-if="itemTotal" :total="itemTotal" v-model:page="ipg.pageNum" v-model:limit="ipg.pageSize" @pagination="loadItems" />
          <el-button type="danger" :disabled="batch.status !== 'PREVIEWED' || !batch.previewCount" :loading="busy" @click="execute">确认清理此预览</el-button>
        </template>
        <el-empty v-else description="生成预览或从清理记录中选择一个批次，尚未擦除数据" />
      </el-tab-pane>
      <el-tab-pane v-if="canClean" label="清理记录" name="batches">
        <el-table :data="batches"><el-table-column prop="batchId" label="批次" width="90" /><el-table-column label="状态" width="120"><template #default="s">{{ batchLabels[s.row.status] || s.row.status }}</template></el-table-column><el-table-column prop="previewCount" label="预览行数" /><el-table-column prop="erasedCount" label="已擦除行数" /><el-table-column prop="expiresAt" label="预览截止" width="170" /><el-table-column label="操作"><template #default="s"><el-button link type="primary" @click="selectBatch(s.row)">查看此批</el-button></template></el-table-column></el-table>
        <pagination v-if="batchTotal" :total="batchTotal" v-model:page="bpg.pageNum" v-model:limit="bpg.pageSize" @pagination="loadBatches" />
      </el-tab-pane>
      <el-tab-pane v-if="canAudit" label="操作审计" name="audit">
        <p>查看谁设置过策略、导出或清理过多少条询盘；不在此记录客户原文。</p>
        <el-table :data="events"><el-table-column prop="eventId" label="记录" width="80" /><el-table-column label="动作" width="110"><template #default="s">{{ actionLabels[s.row.action] || s.row.action }}</template></el-table-column><el-table-column prop="createBy" label="操作者" width="110" /><el-table-column prop="rowCount" label="行数" width="90" /><el-table-column prop="createTime" label="时间" width="170" /><el-table-column prop="details" label="筛选 / 原因" show-overflow-tooltip /><el-table-column prop="fileSha256" label="文件SHA256" show-overflow-tooltip /></el-table>
        <pagination v-if="eventTotal" :total="eventTotal" v-model:page="epg.pageNum" v-model:limit="epg.pageSize" @pagination="loadEvents" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>
<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { workflow } from '@/api/cms/workflow'
import auth from '@/plugins/auth'

const manager = auth.hasPermiAnd(['cms:inquiry:list', 'cms:inquiry:query']) && auth.hasPermiOr(['cms:inquiry:all', 'cms:inquiry:assign'])
const canEdit = manager && auth.hasPermi('cms:data:edit')
const canClean = manager && auth.hasPermiAnd(['cms:data:cleanup', 'cms:inquiry:remove'])
const canAudit = manager && auth.hasPermi('cms:data:audit')
const tab = ref('policy'), policy = ref(), formVersion = ref(0), days = ref(0), statuses = ref(['DONE', 'SPAM']), reason = ref(''), cleanReason = ref('')
const batch = ref(), items = ref([]), itemTotal = ref(0), batches = ref([]), batchTotal = ref(0), events = ref([]), eventTotal = ref(0), busy = ref(false), loading = ref(false)
const bpg = reactive({ pageNum: 1, pageSize: 10 }), ipg = reactive({ pageNum: 1, pageSize: 10 }), epg = reactive({ pageNum: 1, pageSize: 10 })
const labels = { NEW: '待处理', DONE: '已处理', SPAM: '垃圾留言' }, batchLabels = { PREVIEWED: '已生成预览', EXECUTED: '已清理' }, actionLabels = { POLICY: '设置策略', PREVIEW: '生成预览', ERASE: '执行清理', EXPORT: '导出询盘' }
let epoch = 0, resume = false, policyTicket = 0, itemTicket = 0, batchTicket = 0, eventTicket = 0
const canLeave = () => !busy.value

async function loadPolicy(reset = false) {
  const n = epoch, ticket = ++policyTicket, r = await workflow('data/policy')
  if (n !== epoch || ticket !== policyTicket) return
  policy.value = r.data
  if (reset) { formVersion.value = r.data.version; days.value = r.data.retentionDays; statuses.value = [...r.data.statuses]; reason.value = '' }
}
async function loadBatches() {
  if (!canClean) return
  const n = epoch, ticket = ++batchTicket, r = await workflow('data/cleanup/list', 'get', undefined, { ...bpg })
  if (n === epoch && ticket === batchTicket) { batches.value = r.rows; batchTotal.value = r.total; if (batch.value) batch.value = r.rows.find(b => b.batchId === batch.value.batchId) || batch.value }
}
async function loadEvents() {
  if (!canAudit) return
  const n = epoch, ticket = ++eventTicket, r = await workflow('data/events', 'get', undefined, { ...epg })
  if (n === epoch && ticket === eventTicket) { events.value = r.rows; eventTotal.value = r.total }
}
async function loadItems() {
  if (!batch.value) return
  const n = epoch, id = batch.value.batchId, ticket = ++itemTicket, r = await workflow(`data/cleanup/${id}/items`, 'get', undefined, { ...ipg })
  if (n === epoch && ticket === itemTicket && batch.value?.batchId === id) { items.value = r.rows; itemTotal.value = r.total }
}
async function loadTab() {
  const n = epoch, current = tab.value
  loading.value = true
  try {
    if (current === 'policy') { if (!formVersion.value) await loadPolicy(true) }
    else if (current === 'cleanup') { await loadPolicy(); await loadItems() }
    else if (current === 'batches') await loadBatches()
    else if (current === 'audit') await loadEvents()
  } finally { if (n === epoch && current === tab.value) loading.value = false }
}
async function save() {
  if (busy.value || !canEdit) return
  if (!reason.value.trim() || !statuses.value.length) { ElMessage.warning('请选择状态并填写变更原因'); return }
  const n = epoch
  busy.value = true
  try {
    await workflow('data/policy', 'put', { version: formVersion.value, retentionDays: days.value, statuses: statuses.value, reason: reason.value })
    if (n !== epoch) return
    batch.value = null; await loadPolicy(true); ElMessage.success('策略已保存，没有清理数据')
  } finally { busy.value = false }
}
async function preview() {
  if (busy.value) return
  if (!cleanReason.value.trim()) { ElMessage.warning('请填写本次清理原因'); return }
  const n = epoch
  busy.value = true
  try { const r = await workflow('data/cleanup/preview', 'post', { reason: cleanReason.value }); if (n !== epoch) return; batch.value = r.data; ipg.pageNum = 1; await loadItems(); ElMessage.success('预览已生成，尚未擦除数据') }
  finally { busy.value = false }
}
async function selectBatch(row) { batch.value = row; items.value = []; itemTotal.value = 0; ipg.pageNum = 1; if (tab.value === 'cleanup') await loadItems(); else tab.value = 'cleanup' }
async function execute() {
  if (busy.value || !batch.value) return
  const n = epoch, b = { ...batch.value }
  try {
    const p = await ElMessageBox.prompt(`将擦除本批${b.previewCount}条询盘的联系人、正文、备注和跟进文字，并取消提醒。请输入“确认清除”。`, '确认手动清理', { inputPattern: /^确认清除$/, inputErrorMessage: '必须完整输入确认清除', confirmButtonText: '确认清除', type: 'warning' })
    if (n !== epoch) return
    busy.value = true
    await workflow(`data/cleanup/${b.batchId}/execute`, 'post', { version: b.version, confirmation: p.value })
    if (n !== epoch) return
    batch.value = { ...b, status: 'EXECUTED', version: b.version + 1, erasedCount: b.previewCount }; await loadItems(); ElMessage.success('此批已清理')
  } catch {} finally { busy.value = false }
}
function close() { ++epoch; loading.value = false; batch.value = null; items.value = []; cleanReason.value = '' }
onMounted(loadTab); onBeforeUnmount(close); onDeactivated(() => { resume = true; close() }); onActivated(() => { if (resume) { resume = false; loadTab() } })
</script>
