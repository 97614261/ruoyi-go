<template>
  <div class="app-container">
    <h3>前台访问统计</h3>
    <el-alert title="统计访客浏览文章、产品、单页和其他前台页面的次数；后台管理页、后台接口和后台预览不计入。这里是查看报表的入口。" :closable="false" />
    <p>默认关闭，在「站点与外观 → 统计设置」开启并发布后才记录。刷新、重复访问和机器人也会计数，不等于独立访客数。</p>
    <el-form inline @submit.prevent="search"><el-form-item label="日期范围"><el-date-picker v-model="dates" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" :clearable="false" /></el-form-item><el-form-item><el-button type="primary" @click="search">查询</el-button></el-form-item></el-form>
    <el-tabs v-model="q.dimension" @tab-change="search"><el-tab-pane label="每日访问" name="day" /><el-tab-pane label="内容与页面排行" name="page" /><el-tab-pane label="访问来源" name="source" /><el-tab-pane label="站内搜索词" name="keyword" /></el-tabs>
    <p>当前页合计 <b>{{ sum }}</b> 次 · 共 {{ total }} 项；最多查询90天。来源仅保留域名，关键词需单独开启。</p>
    <el-table :data="rows" v-loading="loading"><el-table-column v-if="q.dimension === 'page'" label="页面类型" width="130"><template #default="s">{{ pageKind(s.row.value) }}</template></el-table-column><el-table-column prop="value" :label="labels[q.dimension]" min-width="250" show-overflow-tooltip /><el-table-column prop="hits" label="访问次数" width="130" /><el-table-column label="本页相对占比" min-width="200"><template #default="s"><el-progress :percentage="sum ? Math.round(s.row.hits / sum * 100) : 0" /></template></el-table-column></el-table>
    <pagination v-show="total" :total="total" v-model:page="q.pageNum" v-model:limit="q.pageSize" @pagination="load" />
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import request from '@/utils/request'
const format = d => new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d)
const dates = ref([format(new Date(Date.now() - 6 * 86400000)), format(new Date())]), q = reactive({ pageNum: 1, pageSize: 10, dimension: 'day' }), rows = ref([]), total = ref(0), loading = ref(false)
const labels = { day: '日期（北京时间）', page: '前台页面路径', source: '来源域名（仅供参考）', keyword: '站内搜索词' }
const sum = computed(() => rows.value.reduce((n, row) => n + row.hits, 0))
let epoch = 0, resumed = false
function pageKind(path) {
  if (path === '/') return '首页'
  if (/^\/news\/c\//.test(path)) return '文章栏目'
  if (/^\/news\/[^/]+$/.test(path)) return '文章'
  if (path === '/news') return '文章列表'
  if (/^\/products\/c\//.test(path)) return '产品栏目'
  if (/^\/products\/[^/]+$/.test(path)) return '产品'
  if (path === '/products') return '产品列表'
  if (/^\/p\/[^/]+$/.test(path)) return '单页'
  if (path === '/downloads') return '资料下载中心'
  if (path === '/search') return '站内搜索'
  return '前台页面'
}
function search() { q.pageNum = 1; load() }
async function load() {
  const ticket = ++epoch
  loading.value = true; rows.value = []; total.value = 0
  try { const r = await request({ url: '/cms/statistics/list', params: { ...q, from: dates.value[0], to: dates.value[1] } }); if (ticket === epoch) { rows.value = r.rows; total.value = r.total } }
  finally { if (ticket === epoch) loading.value = false }
}
function close() { ++epoch; loading.value = false }
onMounted(load); onBeforeUnmount(close); onDeactivated(() => { resumed = true; close() }); onActivated(() => { if (resumed) { resumed = false; load() } })
</script>
