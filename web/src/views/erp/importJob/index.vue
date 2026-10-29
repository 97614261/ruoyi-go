<template>
 <div class="app-container">
  <el-button class="mb8" @click="router.push('/erp/order')" v-hasPermi="['erp:order:list']">返回订单工作台</el-button>
  <el-alert title="任务仅对创建者或当前企业所有者开放，并检查实时店铺范围。原文件24小时后回收；失败或中断可显式重试最多5次。已提交的成功订单不会回滚或重复创建。" type="info" :closable="false" class="mb8"/>
  <el-alert v-if="retryPending" title="任务重排结果未确认，请原请求重试或关闭后先查任务，勿另换请求键。" type="warning" :closable="false"><el-button :disabled="saving" @click="retry({jobId:retryPending.id})">原请求重试</el-button><el-button :disabled="saving" @click="retryPending=null;load()">关闭并查询</el-button></el-alert><ERPQueryBar :model="query" :loading="loading"  @search="search" @reset="reset">
   <el-form-item label="店铺ID"><el-input v-model="shopText" placeholder="可选正整数"/></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable ><el-option v-for="(name,key) in statuses" :key="key" :value="key" :label="name"/></el-select></el-form-item>
  </ERPQueryBar>
  <el-table :data="rows" v-loading="loading"><el-table-column prop="jobId" label="任务" width="85"/><el-table-column prop="shopId" label="店铺ID" width="85"/><el-table-column label="状态" min-width="120"><template #default="{row}">{{statuses[row.status]||row.status}}</template></el-table-column><el-table-column prop="totalRows" label="数据行" width="85"/><el-table-column prop="createdOrders" label="新建单" width="85"/><el-table-column prop="existingOrders" label="已存在" width="85"/><el-table-column prop="failedOrders" label="失败单" width="85"/><el-table-column prop="retainUntil" label="文件保留至" width="170"/><el-table-column label="操作" width="220" fixed="right"><template #default="{row}"><el-button link type="primary" @click="showJob(row)" :disabled="busy" v-hasPermi="['erp:importJob:query']">详情 / 结果</el-button><el-button v-if="canRetry(row)" link type="warning" @click="retry(row)" :disabled="busy" v-hasPermi="['erp:order:import','erp:importJob:query']">重试</el-button></template></el-table-column></el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" title="导入任务及结果" class="erp-dialog" width="min(980px, calc(100vw - 24px))" :close-on-click-modal="false" @close="closeDetail"><div v-if="job"><p>任务 #{{job.jobId}} · {{statuses[job.status]}} · 第 {{job.attempt}} 次执行 · v{{job.version}}</p><p>文件：{{job.fileState==='PRESENT'?'私有保留中':'已回收'}}，到期 {{job.retainUntil}}</p><el-button v-if="job.fileState==='PRESENT'" @click="sourceFile" :disabled="busy">下载原文件（含收件信息）</el-button><el-button @click="refreshDetail" :disabled="busy">刷新任务</el-button><el-table :data="results" class="mt8" v-loading="detailLoading"><el-table-column prop="sourceRow" label="源行" width="70"/><el-table-column prop="sourceOrderNo" label="来源订单号" min-width="150"/><el-table-column prop="resultCode" label="结果" width="120"/><el-table-column label="订单" width="110"><template #default="{row}"><el-button v-if="row.orderId&&checkPermi(['erp:order:list'])&&checkPermi(['erp:order:query'])" link type="primary" @click="goOrder(row)">#{{row.orderId}}</el-button><span v-else>{{row.orderId||'—'}}</span></template></el-table-column><el-table-column prop="errorSummary" label="说明" min-width="260"/></el-table><pagination v-show="resultTotal>0" :total="resultTotal" v-model:page="resultQuery.pageNum" v-model:limit="resultQuery.pageSize" @pagination="refreshDetail"/></div><template #footer><el-button @click="open=false">关闭</el-button></template></el-dialog>
 </div>
</template>
<script setup>
import ERPQueryBar from '../components/ERPQueryBar.vue'
import { resetERPQuery } from '@/utils/erp-query.mjs'

import { ref, reactive, computed, onMounted,onBeforeUnmount,onDeactivated,onActivated } from 'vue'
import { useRouter } from 'vue-router'
import { checkPermi } from '@/utils/permission'
import { ElMessage, ElMessageBox } from 'element-plus'
import { saveAs } from 'file-saver'
import { listImportJobs, getImportJob, importResults, retryImportJob, importSourceFile } from '@/api/erp/order'
const rows=ref([]),total=ref(0),loading=ref(false),saving=ref(false),detailLoading=ref(false),open=ref(false),job=ref(null),results=ref([]),resultTotal=ref(0),shopText=ref('')
const router=useRouter()
function goOrder(row){if(checkPermi(['erp:order:list'])&&checkPermi(['erp:order:query']))return router.push({path:'/erp/order',query:{orderId:String(row.orderId)}})}
const busy=computed(()=>saving.value||detailLoading.value),query=reactive({pageNum:1,pageSize:10,shopId:undefined,status:''}),resultQuery=reactive({pageNum:1,pageSize:10})
const statuses={PENDING:'排队中',RUNNING:'处理中',SUCCEEDED:'全部成功',PARTIAL_FAILED:'部分失败',FAILED:'失败',INTERRUPTED:'已中断'}
let listSequence=0,detailSequence=0,view=0,operation=0,alive=true,wasInactive=false
const retryPending=ref(null)
async function load(){if(!alive)return;const ticket=++listSequence;loading.value=true;try{const r=await listImportJobs({...query});if(alive&&ticket===listSequence){rows.value=r.rows;total.value=r.total}}finally{if(ticket===listSequence)loading.value=false}}
function search(){if(shopText.value&&!/^[1-9]\d*$/.test(shopText.value)){ElMessage.warning('店铺ID须为正整数');return}query.shopId=shopText.value?Number(shopText.value):undefined;query.pageNum=1;return load()}
function canRetry(row){return ['FAILED','PARTIAL_FAILED','INTERRUPTED'].includes(row.status)&&row.fileState==='PRESENT'}
async function showJob(row){if(!alive||busy.value)return;job.value={...row};results.value=[];resultQuery.pageNum=1;open.value=true;return refreshDetail()}
async function refreshDetail(){if(!alive||!open.value||!job.value)return;const ticket=++detailSequence,id=job.value.jobId;detailLoading.value=true;try{const [d,r]=await Promise.all([getImportJob(id),importResults(id,{...resultQuery})]);if(alive&&ticket===detailSequence&&open.value){job.value=d.data;results.value=r.rows;resultTotal.value=r.total}}finally{if(ticket===detailSequence)detailLoading.value=false}}
function closeDetail(){if(open.value)return;++detailSequence;detailLoading.value=false;job.value=null;results.value=[]}
async function retry(row){if(!alive||busy.value)return;if(retryPending.value&&retryPending.value.id!==row.jobId){ElMessage.warning('请先核对上次未确认的导入任务');return}const c=retryPending.value||{id:row.jobId,payload:{version:row.version,requestKey:crypto.randomUUID()}},owner=view,op=++operation;saving.value=true;try{try{await ElMessageBox.confirm('只重试失败或中断部分，已经成功的订单保持不变。确认继续？','重试导入',{type:'warning'})}catch{return};if(!alive||owner!==view||op!==operation)return;retryPending.value=c;await retryImportJob(c.id,c.payload);if(!alive||owner!==view||op!==operation)return;retryPending.value=null;ElMessage.success('已重新排队');await load()}catch(e){if(alive&&owner===view&&op===operation){if(typeof e==='string'){retryPending.value=null;rows.value=[];total.value=0}else throw e}}finally{if(op===operation)saving.value=false}}
async function sourceFile(){if(!alive||busy.value||!job.value)return;const owner=view,ticket=detailSequence,op=++operation,id=job.value.jobId;saving.value=true;try{const b=await importSourceFile(id);if(!alive||owner!==view||ticket!==detailSequence||!open.value)return;if(b.type==='application/json'){const v=JSON.parse(await b.text());if(alive&&owner===view&&ticket===detailSequence&&open.value)ElMessage.warning(v.msg||'下载失败');return}saveAs(b,'订单导入原文件.xlsx')}finally{if(op===operation)saving.value=false}}
onMounted(load)
function leave(){alive=false;++view;++operation;++listSequence;++detailSequence;saving.value=false;loading.value=false;detailLoading.value=false;open.value=false;job.value=null;results.value=[];resultTotal.value=0;rows.value=[];total.value=0;retryPending.value=null}
onBeforeUnmount(leave);onDeactivated(()=>{wasInactive=true;leave()});onActivated(()=>{if(wasInactive){wasInactive=false;alive=true;return load()}})
function reset(){shopText.value='';resetERPQuery(query,{pageNum:1,pageSize:10,shopId:undefined,status:''});return search()}
</script>
