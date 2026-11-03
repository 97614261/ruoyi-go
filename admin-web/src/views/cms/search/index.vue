<template>
  <div class="app-container">
    <h3>公开内容搜索索引</h3>
    <el-alert title="仅索引已标记发布的版本，草稿不会写入。前台检索会再次检查当前发布版本与栏目状态；后台更新期间标题、摘要仍可搜索。" :closable="false" />
    <p v-if="status">已就绪 {{ status.ready }} · 待更新 {{ status.pending }} · 失败 {{ status.failed }}（每轮最多2篇，失败最多尝试{{ status.maxAttempts }}次）</p>
    <el-tabs v-model="statusTab" @tab-change="search"><el-tab-pane label="全部任务" name="all" /><el-tab-pane label="待更新" name="PENDING" /><el-tab-pane label="已就绪" name="READY" /><el-tab-pane label="失败" name="FAILED" /></el-tabs>
    <div style="margin:12px 0"><el-button @click="load(true)">刷新</el-button><el-button v-hasPermi="['cms:search:edit']" :loading="busy" @click="rebuild">重建有权查看的公开索引</el-button></div>
    <el-table :data="rows" v-loading="loading"><el-table-column prop="contentId" label="内容" width="90" /><el-table-column prop="kind" label="类型" width="100" /><el-table-column prop="title" label="当前公开标题" /><el-table-column label="状态" width="100"><template #default="s">{{ labels[s.row.status] || s.row.status }}</template></el-table-column><el-table-column prop="attempts" label="尝试" width="80" /><el-table-column prop="errorMessage" label="结果" /><el-table-column prop="updateTime" label="更新时间" width="170" /></el-table>
    <pagination v-if="total" :total="total" v-model:page="q.pageNum" v-model:limit="q.pageSize" @pagination="()=>load(true)" />
    <p>只显示你有查询权限的内容类型。自动刷新每5秒一次，离开页面后停止。</p>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { workflow } from '@/api/cms/workflow'
const rows = ref([]), total = ref(0), status = ref(), loading = ref(false), busy = ref(false), q = reactive({ status: '', pageNum: 1, pageSize: 20 })
const labels = { PENDING: '待更新', READY: '已就绪', FAILED: '失败' }
const statusTab = computed({ get: () => q.status || 'all', set: v => q.status = v === 'all' ? '' : v })
let epoch = 0, life = 0, timer, flight = false, resume = false
async function load(force = false) {
  if (flight && !force) return
  const n = ++epoch
  flight = true; loading.value = true; rows.value = []; total.value = 0
  try { const [r, s] = await Promise.all([workflow('search-index/list', 'get', undefined, { ...q }), workflow('search-index/status')]); if (n === epoch) { rows.value = r.rows; total.value = r.total; status.value = s.data } }
  finally { if (n === epoch) { flight = false; loading.value = false } }
}
function search() { q.pageNum = 1; load(true) }
async function rebuild() {
  if (busy.value) return
  const n = life
  try { await ElMessageBox.confirm('重新安排当前有查询权限的内容索引？不会修改草稿或发布内容。', '索引重建'); if (n !== life) return; busy.value = true; const r = await workflow('search-index/rebuild', 'post', {}); if (n !== life) return; ElMessage.success('已安排' + r.data + '篇，请稍后刷新'); await load(true) }
  catch {} finally { busy.value = false }
}
function start() { load(true); clearInterval(timer); timer = setInterval(() => { if (!document.hidden) load() }, 5000) }
function close() { ++epoch; ++life; clearInterval(timer); flight = false; loading.value = false }
onMounted(start); onBeforeUnmount(close); onDeactivated(() => { resume = true; close() }); onActivated(() => { if (resume) { resume = false; start() } })
</script>
