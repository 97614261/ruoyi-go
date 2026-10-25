<template>
 <div class="app-container">
  <h2 class="shipment-title">发货记录</h2><el-alert title="仅展示当前店铺和仓库范围内的本地已发货包裹，不含收件资料、金额或承运商揽收/妥投信息。目录名称来自保存时快照，旧手工记录只显示原代码。" type="info" :closable="false" class="mb8"/>
  <el-form class="shipment-filters" inline size="small" @submit.prevent="search"><el-form-item label="物流代码"><el-input v-model="query.carrierCode" maxlength="64" clearable/></el-form-item><el-form-item label="运单号"><el-input v-model="query.trackingNo" maxlength="128" clearable/></el-form-item><el-form-item label="方式编号"><el-input-number v-model="query.shippingMethodId" :min="1" :precision="0" :controls="false" placeholder="可点下方方式筛选"/></el-form-item><el-form-item label="录入方式"><el-select v-model="query.mode" clearable style="width:115px"><el-option label="手工填写" value="MANUAL"/><el-option label="目录选择" value="CATALOG"/></el-select></el-form-item><el-form-item label="发货起始"><el-date-picker v-model="dates[0]" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" placeholder="起始时间" class="shipment-dates" popper-class="shipment-date-panel"/></el-form-item><el-form-item label="发货结束"><el-date-picker v-model="dates[1]" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" placeholder="结束时间" class="shipment-dates" popper-class="shipment-date-panel"/></el-form-item><el-button @click="search" :disabled="loading">查询</el-button><el-button @click="reset" :disabled="loading">重置</el-button><el-button @click="exportRows" v-hasPermi="['erp:shipment:export']" :disabled="loading">导出</el-button></el-form>
  <el-table :data="rows" size="small" v-loading="loading"><el-table-column prop="packageNo" label="包裹号" min-width="190"/><el-table-column prop="orderId" label="订单编号" width="100"/><el-table-column label="物流公司" min-width="145"><template #default="s">{{s.row.carrierName||s.row.carrierCode}}</template></el-table-column><el-table-column label="配送方式快照" min-width="150"><template #default="s"><el-button v-if="s.row.shippingMethodId" link @click="filterMethod(s.row.shippingMethodId)">{{s.row.methodName}} · {{s.row.methodCode}}</el-button><span v-else>手工填写</span></template></el-table-column><el-table-column prop="trackingNo" label="运单号" min-width="180"/><el-table-column prop="shippedAt" label="本地发货时间" width="170"/><el-table-column label="操作" width="95"><template #default="s"><el-button link v-hasPermi="['erp:package:query']" @click="openPackage(s.row)">查看包裹</el-button></template></el-table-column></el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
 </div>
</template>
<script setup>
import {ref,reactive,getCurrentInstance,onMounted,onBeforeUnmount,onDeactivated,onActivated,watch} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {logisticsShipments} from '@/api/erp/logistics'
const {proxy}=getCurrentInstance(),router=useRouter(),route=useRoute(),rows=ref([]),total=ref(0),loading=ref(false),dates=ref([])
const query=reactive({pageNum:1,pageSize:20,carrierCode:'',trackingNo:'',shippingMethodId:undefined,mode:''})
let sequence=0,wasDeactivated=false
function params(){return{...query,shippedFrom:dates.value?.[0]||'',shippedTo:dates.value?.[1]||''}}
async function load(){const ticket=++sequence;loading.value=true;try{const r=await logisticsShipments(params());if(ticket===sequence){rows.value=r.rows;total.value=r.total}}catch{if(ticket===sequence){rows.value=[];total.value=0}}finally{if(ticket===sequence)loading.value=false}}
function search(){query.pageNum=1;return load()}
function reset(){Object.assign(query,{pageNum:1,carrierCode:'',trackingNo:'',shippingMethodId:undefined,mode:''});dates.value=[];return load()}
function filterMethod(id){query.shippingMethodId=id;return search()}
function openPackage(row){return router.push({path:'/erp/package',query:{packageId:String(row.packageId),orderId:String(row.orderId)}})}
function exportRows(){return proxy.download('/erp/logistics/shipments/export',params(),'发货记录.xlsx')}
function deactivate(){++sequence;rows.value=[];total.value=0;Object.assign(query,{carrierCode:'',trackingNo:'',shippingMethodId:undefined,mode:''});dates.value=[]}
function applyDateLink(){const a=route.query.shippedFrom,b=route.query.shippedTo;const valid=v=>v===undefined||(typeof v==='string'&&/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(v));if(!valid(a)||!valid(b)){++sequence;rows.value=[];total.value=0;return false}dates.value=[a||'',b||''];query.pageNum=1;return true}
watch(()=>[route.query.shippedFrom,route.query.shippedTo],()=>{if(!wasDeactivated&&applyDateLink())load()})
onMounted(()=>{if(applyDateLink())load()});onBeforeUnmount(deactivate);onDeactivated(()=>{wasDeactivated=true;deactivate()});onActivated(()=>{if(wasDeactivated){wasDeactivated=false;if(applyDateLink())load()}})
</script>
<style scoped>
.shipment-title{font-size:18px;margin:0 0 10px}
.shipment-dates{width:200px;max-width:100%}
.shipment-filters :deep(.el-form-item){max-width:100%}
.shipment-filters :deep(.el-form-item__content){min-width:0}
@media(max-width:600px){
 :global(.shipment-date-panel){position:fixed!important;left:50%!important;top:50%!important;transform:translate(-50%,-50%)!important;max-width:calc(100vw - 24px);max-height:calc(100dvh - 24px);overflow:auto}
 :global(.shipment-date-panel .el-popper__arrow){display:none}
 .shipment-filters :deep(.el-form-item){display:flex;width:100%;margin-right:0}
 .shipment-filters :deep(.el-form-item__label){width:72px;flex-shrink:0}
 .shipment-filters :deep(.el-form-item__content){flex:1;min-width:0}
 .shipment-filters :deep(.el-input),.shipment-filters :deep(.el-input-number),.shipment-filters :deep(.el-select),.shipment-filters :deep(.el-date-editor){width:100%!important;min-width:0}
}
</style>



