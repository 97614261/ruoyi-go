<template>
  <div class="app-container pod-overview">
    <div class="heading">
      <div><h2>运营概览</h2><p>管理模型、创作者与平台资源。普通用户在独立创作端完成工作。</p></div>
      <el-tag type="info">模型创作 · 第 5 步</el-tag>
    </div>
    <el-alert title="正式执行已接通" description="创作端支持模型选择、服务端报价和正式点数结算。启用模式以服务端配置为准；当前概览沿用已有记录，管理任务和用量明细将在后续开放。" type="info" :closable="false" show-icon />
    <el-tabs v-model="tab" class="overview-tabs" @tab-change="loadTab">
      <el-tab-pane v-if="canOverview" label="平台概况" name="overview">
        <el-form :inline="true" class="query-form" @submit.prevent="loadOverview">
          <el-form-item label="统计日期"><el-date-picker v-model="overviewDates" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" :clearable="false" /></el-form-item>
          <el-form-item><el-button type="primary" :loading="overviewLoading" @click="loadOverview">刷新统计</el-button></el-form-item>
        </el-form>
        <el-alert v-if="overviewError" :title="overviewError" type="error" :closable="false" show-icon />
        <template v-if="overview">
          <div class="stat-grid" :aria-busy="overviewLoading">
            <el-card v-for="item in cards" :key="item.label" shadow="never"><span class="stat-label">{{ item.label }}</span><strong>{{ item.value }}</strong><small>{{ item.hint }}</small></el-card>
          </div>
          <el-card shadow="never">
            <template #header><span>运行状态</span><small class="range-hint">{{ overview.beginDate }} 至 {{ overview.endDate }}（北京时间）</small></template>
            <el-table :data="overview.states" empty-text="这段时间没有运行记录"><el-table-column prop="state" label="状态" :formatter="stateLabel" /><el-table-column prop="count" label="运行数" align="right" /></el-table>
          </el-card>
          <p class="note">创作者、项目、素材为当前有效数量；存储包含回收站保留文件。运行、批次及估算费用按所选日期内创建的记录汇总，费用以供应商账单为准。此页只读元信息。</p>
        </template>
      </el-tab-pane>
      <el-tab-pane v-if="canAudit" label="管理操作审计" name="audits">
        <el-form :inline="true" class="query-form" @submit.prevent="searchAudits">
          <el-form-item label="操作日期"><el-date-picker v-model="auditDates" type="daterange" value-format="YYYY-MM-DD" :clearable="false" start-placeholder="开始日期" end-placeholder="结束日期" /></el-form-item>
          <el-form-item label="操作者 ID"><el-input v-model.trim="auditActor" placeholder="留空查看全部" clearable /></el-form-item>
          <el-form-item label="结果"><el-select v-model="auditOutcome" clearable placeholder="全部" style="width: 130px"><el-option label="成功" value="success" /><el-option label="拒绝" value="denied" /><el-option label="失败" value="failure" /></el-select></el-form-item>
          <el-form-item><el-button type="primary" :loading="auditLoading" @click="searchAudits">查询</el-button></el-form-item>
        </el-form>
        <el-alert v-if="auditError" :title="auditError" type="error" :closable="false" show-icon />
        <p class="note">记录管理访问、模型配置及创作者权益变更及未通过的写操作。没有请求正文、密码、密钥或用户作品内容。</p>
        <el-table v-loading="auditLoading" :data="audits" empty-text="没有匹配的审计记录">
          <el-table-column prop="auditId" label="编号" min-width="80" />
          <el-table-column prop="createTime" label="时间（北京时间）" min-width="170" />
          <el-table-column prop="actorUserId" label="操作者 ID" min-width="100" />
          <el-table-column label="动作" min-width="140"><template #default="{ row }">{{ row.actionType === 'admin.access' ? '访问管理端' : row.actionType }}</template></el-table-column>
          <el-table-column label="结果" min-width="90"><template #default="{ row }"><el-tag :type="row.outcome === 'success' ? 'success' : 'warning'">{{ row.outcome === 'success' ? '成功' : row.outcome === 'denied' ? '拒绝' : '失败' }}</el-tag></template></el-table-column>
          <el-table-column prop="traceId" label="追踪编号" min-width="200" show-overflow-tooltip />
          <el-table-column v-if="canAuditDetail" label="操作" width="90"><template #default="{ row }"><el-button link type="primary" @click="showAudit(row.auditId)">详情</el-button></template></el-table-column>
        </el-table>
        <pagination v-show="auditTotal > 0 && !auditError" v-model:page="auditPage" v-model:limit="auditSize" :total="auditTotal" @pagination="loadAudits" />
      </el-tab-pane>
    </el-tabs>
    <el-empty v-if="!canOverview && !canAudit" description="当前账号已获管理入口权限，请从左侧进入已授权模块。" />
    <el-dialog v-model="detailOpen" title="审计详情" width="680px" append-to-body>
      <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" />
      <el-descriptions v-if="detail" :column="1" border>
        <el-descriptions-item label="审计编号">{{ detail.auditId }}</el-descriptions-item>
        <el-descriptions-item label="操作者 ID">{{ detail.actorUserId }}</el-descriptions-item>
        <el-descriptions-item label="时间">{{ detail.createTime }}</el-descriptions-item>
        <el-descriptions-item label="动作">{{ detail.actionType }}</el-descriptions-item>
        <el-descriptions-item label="结果">{{ detail.outcome }}</el-descriptions-item>
        <el-descriptions-item label="资源">{{ detail.resourceType }} / {{ detail.resourceId }}</el-descriptions-item>
        <el-descriptions-item label="追踪编号">{{ detail.traceId }}</el-descriptions-item>
        <el-descriptions-item label="安全快照"><pre>{{ JSON.stringify(detail.after || {}, null, 2) }}</pre></el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>
<script setup>
import { computed, ref, onMounted } from 'vue'
import { getAdminOverview, getAdminAudits, getAdminAudit } from '@/api/pod/admin'
import useUserStore from '@/store/modules/user'
import { formatMicros, formatBytes, localDate } from '@/utils/pod-admin'

const user = useUserStore()
const has = permission => user.podAdmin?.permissions?.includes(permission)
const canOverview = computed(() => has('podAdmin:overview:query'))
const canAudit = computed(() => has('podAdmin:audit:list'))
const canAuditDetail = computed(() => has('podAdmin:audit:query'))
const tab = ref(canOverview.value ? 'overview' : 'audits')
const initialDates = () => {
  const end = new Date(); const begin = new Date(end.getTime() - 6 * 24 * 60 * 60 * 1000)
  return [localDate(begin), localDate(end)]
}
const overviewDates = ref(initialDates()), auditDates = ref(initialDates())
const overview = ref(null), overviewLoading = ref(false), overviewError = ref('')
const audits = ref([]), auditLoading = ref(false), auditError = ref('')
const auditActor = ref(''), auditOutcome = ref(''), auditPage = ref(1), auditSize = ref(10), auditTotal = ref(0)
const detail = ref(null), detailOpen = ref(false), detailError = ref('')
let overviewRequest = 0, auditRequest = 0, detailRequest = 0
const cards = computed(() => overview.value ? [
  { label: '启用的工作空间', value: overview.value.creators, hint: '按个人空间状态统计' },
  { label: '项目 / 素材', value: `${overview.value.projects} / ${overview.value.assets}`, hint: '当前未删除记录' },
  { label: '已存文件', value: formatBytes(overview.value.storageBytes), hint: '包含回收站与缩略图' },
  { label: '运行 / 批次', value: `${overview.value.runs} / ${overview.value.batches}`, hint: '所选日期内创建' },
  { label: '已确认估算成本', value: `¥ ${formatMicros(overview.value.chargedMicros)}`, hint: '人民币，非用户点数' },
  { label: '待确认费用预占', value: `¥ ${formatMicros(overview.value.reservedMicros)}`, hint: '结果不明时继续保留' }
] : [])
const labels = { queued: '排队中', running: '运行中', succeeded: '成功', failed: '失败', cancelled: '已取消', cancelRequested: '取消中', awaitingReconciliation: '待核对', completed: '已完成' }
const stateLabel = row => labels[row.state] || row.state
const range = dates => ({ beginDate: dates?.[0] || '', endDate: dates?.[1] || '' })
async function loadOverview() {
  if (!canOverview.value) return
  const request = ++overviewRequest; overviewLoading.value = true; overviewError.value = ''; overview.value = null
  try { const result = await getAdminOverview(range(overviewDates.value)); if (request === overviewRequest) overview.value = result.data }
  catch (error) { if (request === overviewRequest) overviewError.value = error.message }
  finally { if (request === overviewRequest) overviewLoading.value = false }
}
async function loadAudits() {
  if (!canAudit.value) return
  // Keep the total while loading: Element Plus otherwise clamps the selected page back to 1.
  const request = ++auditRequest; auditLoading.value = true; auditError.value = ''; audits.value = []
  if (auditActor.value && !/^[1-9][0-9]*$/.test(auditActor.value)) { auditError.value = '操作者 ID 应为正整数'; auditLoading.value = false; return }
  try {
    const result = await getAdminAudits({ ...range(auditDates.value), actorUserId: auditActor.value, outcome: auditOutcome.value, pageNum: auditPage.value, pageSize: auditSize.value })
    if (request === auditRequest) { audits.value = result.rows; auditTotal.value = result.total }
  } catch (error) { if (request === auditRequest) auditError.value = error.message }
  finally { if (request === auditRequest) auditLoading.value = false }
}
function searchAudits() { auditPage.value = 1; loadAudits() }
function loadTab() { if (tab.value === 'overview') loadOverview(); else loadAudits() }
async function showAudit(id) {
  const request = ++detailRequest; detail.value = null; detailError.value = ''; detailOpen.value = true
  try { const result = await getAdminAudit(id); if (request === detailRequest) detail.value = result.data }
  catch (error) { if (request === detailRequest) detailError.value = error.message }
}
onMounted(loadTab)
</script>
<style scoped>
.heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
h2 { margin: 0 0 8px; font-size: 22px; }
.heading p, .note { color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.7; }
.heading p { margin: 0; }
.overview-tabs { margin-top: 16px; }
.query-form { margin: 10px 0 0; }
.stat-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin: 10px 0 18px; }
.stat-grid strong { display: block; margin: 12px 0; font-size: 25px; font-variant-numeric: tabular-nums; color: var(--el-text-color-primary); }
.stat-label { color: var(--el-text-color-regular); font-size: 14px; }
small { color: var(--el-text-color-secondary); font-size: 12px; }
.range-hint { float: right; line-height: 20px; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 900px) { .stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 600px) { .heading { align-items: flex-start; flex-direction: column; } .stat-grid { grid-template-columns: minmax(0, 1fr); } .range-hint { float: none; display: block; } .query-form :deep(.el-form-item) { display: flex; margin-right: 0; } .query-form :deep(.el-date-editor) { max-width: 100%; } }
</style>
