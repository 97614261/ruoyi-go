<template>
 <div class="app-container overview">
  <header class="overview-head"><div><h2>经营概览</h2><p>只统计你当前可见的业务；各区域独立更新。</p></div><el-button @click="refresh">刷新全部</el-button></header>
  <el-alert title="待办和未核销金额为当前状态；今日/近7日按上海自然日统计本地已发货包裹。金额不等于银行余额或利润。" type="info" :closable="false" class="mb8"/>
  <el-empty v-if="!visible.length" description="暂未开通可查看的业务区域，请联系企业管理员"/>
  <div class="overview-grid">
   <section v-for="s in visible" :key="s.key" class="overview-section" :class="{'overview-money':s.money}">
    <header><h3>{{s.name}}</h3><el-button link @click="loadOne(s.key)" :disabled="states[s.key].loading">刷新</el-button></header>
    <p v-if="states[s.key].loading" role="status">读取中…</p>
    <el-alert v-else-if="states[s.key].error" title="读取失败或权限已变化，请刷新重试；此处不是业务零值。" type="error" :closable="false"/>
    <p v-else-if="states[s.key].data&&!states[s.key].data.available" class="overview-muted">该业务模块尚未安装</p>
    <template v-else-if="states[s.key].data?.available">
     <div v-if="s.key==='orders'" class="overview-metrics"><button @click="go(s.key,'pendingReview')"><strong>{{count(states[s.key].data.counts.pendingReview)}}</strong>待审核订单</button><button @click="go(s.key,'readyToShip')"><strong>{{count(states[s.key].data.counts.readyToShip)}}</strong>待发货订单</button></div>
     <div v-else-if="s.key==='shipments'" class="overview-metrics"><button @click="go(s.key,'today')"><strong>{{count(states[s.key].data.counts.today)}}</strong>今日发货包裹</button><button @click="go(s.key,'sevenDays')"><strong>{{count(states[s.key].data.counts.sevenDays)}}</strong>近7日发货包裹</button></div>
     <div v-else-if="s.key==='stocks'" class="overview-metrics"><button @click="go(s.key,'withStock')"><strong>{{count(states[s.key].data.counts.withStock)}}</strong>有实物记录</button><button @click="go(s.key,'withReserved')"><strong>{{count(states[s.key].data.counts.withReserved)}}</strong>存在预占记录</button><button v-if="states[s.key].data.counts.low!==null" @click="go(s.key,'low')"><strong>{{count(states[s.key].data.counts.low)}}</strong>低库存记录</button><span v-else class="overview-muted">未安装库存预警</span></div>
     <div v-else-if="s.key==='purchases'" class="overview-metrics"><button @click="go(s.key,'awaitingReceipt')"><strong>{{count(states[s.key].data.awaitingReceipt)}}</strong>尚待收货采购单</button></div>
     <template v-else-if="s.money">
      <el-table :data="states[s.key].data.rows"><el-table-column prop="currency" label="币种" width="80"/><el-table-column prop="dueAmount" :label="s.key==='payables'?'待付':'待收'" min-width="160"/><el-table-column prop="excessAmount" :label="s.key==='payables'?'超付':'超收'" min-width="160"/><el-table-column label="账单" width="90"><template #default="{row}"><el-button link @click="go(s.key,'money',row.currency)">{{count(row.documents)}} 单</el-button></template></el-table-column></el-table>
      <pagination v-if="states[s.key].data.total>0" :total="states[s.key].data.total" v-model:page="pages[s.key]" :limit="10" :page-sizes="[10]" @pagination="loadOne(s.key)"/>
     </template>
     <p v-if="s.key==='stocks'" class="overview-muted">一条记录＝一个仓库＋SKU，不混加不同商品的数量。</p>
     <p v-if="s.money" class="overview-muted">逐单待收/待付与超额分别相加；不同币种不合计。点击单数查看本币种账单。</p>
     <footer>更新于 {{states[s.key].data.checkedAt}}</footer>
    </template>
   </section>
  </div>
 </div>
</template>
<script setup>
import {reactive,computed,onMounted,onDeactivated,onActivated,onBeforeUnmount} from 'vue'
import {useRouter} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {overviewSection} from '@/api/erp/overview'
import {overviewSections,overviewCount,overviewTarget} from '@/utils/overview.mjs'
const router=useRouter(),states=reactive(Object.fromEntries(overviewSections.map(s=>[s.key,{loading:false,error:false,data:null}]))),pages=reactive({payables:1,receivables:1})
const visible=computed(()=>checkPermi(['erp:overview:list'])?overviewSections.filter(s=>checkPermi([s.permission])):[])
const sequence=Object.fromEntries(overviewSections.map(s=>[s.key,0]))
let inactive=false,wasDeactivated=false
const count=overviewCount
async function loadOne(kind){
 const spec=overviewSections.find(s=>s.key===kind);if(!spec)return
 const state=states[kind],ticket=++sequence[kind];state.data=null;state.error=false;state.loading=false
 if(inactive||!checkPermi(['erp:overview:list'])||!checkPermi([spec.permission]))return
 state.loading=true
 try{const r=await overviewSection(kind,spec.money?{pageNum:pages[kind],pageSize:10}:undefined);if(!inactive&&ticket===sequence[kind])state.data=r.data}
 catch{if(!inactive&&ticket===sequence[kind])state.error=true}
 finally{if(ticket===sequence[kind])state.loading=false}
}
function refresh(){return Promise.all(overviewSections.map(s=>loadOne(s.key)))}
function clear(){inactive=true;for(const s of overviewSections){++sequence[s.key];Object.assign(states[s.key],{loading:false,error:false,data:null})}pages.payables=1;pages.receivables=1}
function go(kind,metric,currency){if(inactive||!visible.value.some(s=>s.key===kind)||!states[kind].data?.available)return;try{return router.push(overviewTarget(kind,metric,states[kind].data,currency))}catch(e){ElMessage.warning(e.message)}}
onMounted(refresh);onBeforeUnmount(clear);onDeactivated(()=>{wasDeactivated=true;clear()});onActivated(()=>{if(wasDeactivated){wasDeactivated=false;inactive=false;refresh()}})
</script>
<style scoped>
.overview-head,.overview-section>header{display:flex;align-items:center;justify-content:space-between;gap:12px}.overview-head{margin-bottom:12px}.overview h2{font-size:20px;margin:0 0 4px}.overview-head p,.overview-muted,.overview footer{font-size:12px;color:var(--el-text-color-secondary);line-height:1.6;margin:6px 0}.overview-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.overview-section{min-width:0;padding:12px;border:1px solid var(--el-border-color-lighter);border-radius:6px;background:var(--el-bg-color)}.overview-section h3{font-size:14px;margin:0}.overview-metrics{display:flex;flex-wrap:wrap;gap:8px;margin:12px 0}.overview-metrics button{flex:1;min-width:115px;padding:10px 12px;text-align:left;border:1px solid var(--el-border-color-lighter);border-radius:4px;background:var(--el-fill-color-light);color:var(--el-text-color-regular);cursor:pointer;font:inherit;font-size:12px}.overview-metrics button:hover{border-color:var(--el-color-primary)}.overview-metrics strong{display:block;margin-bottom:6px;font-size:25px;line-height:1.1;color:var(--el-text-color-primary)}.overview footer{margin-top:12px}.overview :deep(.pagination-container){float:none;height:auto;min-height:40px;margin-top:10px;padding:0;overflow:auto}.overview :deep(.pagination-container .el-pagination){position:static;float:none}@media(max-width:850px){.overview-grid{grid-template-columns:minmax(0,1fr)}}
</style>
