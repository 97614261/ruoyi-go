<template>
 <div class="app-container">
  <el-alert title="退货确认立即增加实物，不改预占；原包裹、发货事实不回退，不含退款及订单价格。" type="warning" :closable="false" class="mb8"/>
  <div class="mb8"><el-radio-group v-model="tab" @change="switchTab"><el-radio-button value="packages">原包裹退货</el-radio-button><el-radio-button value="returns">退货历史</el-radio-button></el-radio-group></div>
  <ERPQueryBar :model="query" :loading="loading"  @search="search" @reset="reset">
   <el-form-item :label="tab==='packages'?'包裹单号':'退货单号'"><el-input v-model="numberFilter" clearable placeholder="完整单号"/></el-form-item>
  </ERPQueryBar>
  <el-alert v-if="listError" title="读取失败，请重试；不是零条数据。" type="error" :closable="false"/>
  <el-alert v-if="lastResult" :title="'已退货 #'+lastResult.returnId+' · 库存凭据 #'+lastResult.stockDocumentId+' · 原包裹退货v'+lastResult.version" type="success" closable @close="lastResult=null"/>
  <el-table :data="rows" v-loading="loading">
   <el-table-column :prop="tab==='packages'?'packageNo':'returnNo'" :label="tab==='packages'?'包裹单号':'退货单号'" min-width="250" show-overflow-tooltip/>
   <el-table-column prop="orderId" label="订单ID" width="90"/><el-table-column prop="warehouseId" label="仓库ID" width="90"/>
   <el-table-column v-if="tab==='packages'" label="发 / 退 / 净发" width="160"><template #default="{row}">{{row.shippedQuantity}} / {{row.legacyStockReturn?'需核对':row.returnedQuantity}} / {{row.netShippedQuantity??'—'}}</template></el-table-column>
   <el-table-column v-else prop="packageId" label="原包裹ID" width="110"/>
   <el-table-column :prop="tab==='packages'?'shippedAt':'postedAt'" :label="tab==='packages'?'发货时间':'退货时间'" min-width="165"/>
   <el-table-column label="操作" width="120" fixed="right"><template #default="{row}">
    <span v-if="tab==='packages'&&row.legacyStockReturn">有旧库存退货，需核对</span>
    <el-button v-if="tab==='packages'&&can('erp:salesReturnOrder:return')&&!row.legacyStockReturn" link type="danger" :disabled="locked||row.netShippedQuantity===0" @click="start(row.packageId)">登记退货</el-button>
    <el-button v-if="tab==='returns'&&can('erp:salesReturnOrder:query')" link type="primary" :disabled="locked" @click="show(row.returnId)">查看明细</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="mode==='return'?'原包裹退货 · 确认即入库':'订单退货事实 · 只读'" class="erp-dialog" width="min(1000px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" :before-close="beforeClose" @close="closed" @closed="closing=false">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/>
   <p>原包裹 #{{form.packageId}} · 订单 #{{form.orderId}} · 仓库 #{{form.warehouseId}} <span v-if="mode==='return'">· 原包裹退货v{{form.version}}</span><span v-else>· 退货 #{{form.returnId}} · {{form.postedAt}}</span></p>
   <el-table :data="form.lines">
    <el-table-column prop="orderLineId" label="原订单行" width="100"/><el-table-column prop="skuCode" label="SKU快照" min-width="120"/><el-table-column prop="productName" label="商品快照" min-width="160"/><el-table-column prop="unit" label="单位" width="60"/>
    <template v-if="mode==='return'">
     <el-table-column prop="quantity" label="原包裹发货" width="90"/><el-table-column prop="returnedQuantity" label="已退" width="80"/><el-table-column prop="netShippedQuantity" label="可退" width="80"/>
     <el-table-column label="本次（0不提交）" width="175"><template #default="{row}"><el-input-number v-model="row.returnQuantity" :min="0" :max="row.netShippedQuantity" :precision="0" :disabled="busy||!!pending" style="width:150px"/></template></el-table-column>
    </template>
    <el-table-column v-else prop="quantity" label="本次退货" width="100"/>
   </el-table>
   <el-form v-if="mode==='return'" label-width="80px" class="mt8"><el-form-item label="退货原因" required><el-input v-model="form.reason" maxlength="500" placeholder="必填，最多500字，不换行" :disabled="busy||!!pending"/></el-form-item></el-form>
   <p v-else>退货原因：{{form.reason}} · 库存凭据 #{{form.stockDocumentId}}</p>
   <p v-if="mode==='return'">请核对实物；入库不改变已预占数量。退货过账后不可修改、删除或撤销，不会退款或重开原订单。</p>
   <template #footer><el-button :disabled="busy" @click="closeDialog">{{pending?'关闭 / 放弃本次重试':'关闭'}}</el-button><el-button v-if="mode==='return'" type="danger" :loading="busy" @click="submit">{{pending?'沿用原请求重试':'确认退货并入库'}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="SalesReturnWorkspace">
import ERPQueryBar from './ERPQueryBar.vue'
import { resetERPQuery } from '@/utils/erp-query.mjs'

import {ref,reactive,computed,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {listReturnablePackages,getReturnablePackage,postSalesReturn,listSalesReturns,getSalesReturn} from '@/api/erp/salesReturn'
import {safeID,immutablePayload} from '@/utils/purchasing.mjs'
import {salesReturnPayload} from '@/utils/sales-return.mjs'
const route=useRoute(),can=p=>checkPermi([p]),query=reactive({pageNum:1,pageSize:10})
const tab=ref('packages'),numberFilter=ref(''),rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),open=ref(false),mode=ref('return'),closing=ref(false),busy=ref(false),opening=ref(false),form=ref({lines:[]}),pending=ref(null),error=ref(''),lastResult=ref(null)
const locked=computed(()=>busy.value||opening.value||closing.value||open.value)
let listSeq=0,detailSeq=0,viewSeq=0,inactive=false
async function load(){if(inactive)return;if(!can('erp:salesReturnOrder:list'))return;const t=++listSeq;loading.value=true;listError.value=false;const params={...query,[tab.value==='packages'?'packageNo':'returnNo']:numberFilter.value};try{const r=await(tab.value==='packages'?listReturnablePackages(params):listSalesReturns(params));if(t===listSeq){rows.value=r.rows;total.value=r.total}}catch{if(t===listSeq){rows.value=[];total.value=0;listError.value=true}}finally{if(t===listSeq)loading.value=false}}
function search(){query.pageNum=1;return load()}
function switchTab(){numberFilter.value='';rows.value=[];total.value=0;return search()}
async function start(id){if(inactive)return;if(locked.value||!can('erp:salesReturnOrder:return')||!safeID(id))return;const t=++detailSeq;opening.value=true;try{const r=await getReturnablePackage(id);if(t!==detailSeq)return;if(query.orderId&&r.data.orderId!==query.orderId){ElMessage.warning('包裹与链接订单不一致');return}form.value={...r.data,requestKey:crypto.randomUUID(),reason:'',lines:r.data.lines.map(l=>({...l,returnQuantity:0}))};mode.value='return';pending.value=null;error.value='';open.value=true}catch(e){if(t===detailSeq)error.value=e.message||'原包裹读取失败'}finally{if(t===detailSeq)opening.value=false}}
async function show(id){if(inactive)return;if(locked.value||!can('erp:salesReturnOrder:query')||!safeID(id))return;const t=++detailSeq;opening.value=true;try{const r=await getSalesReturn(id);if(t!==detailSeq)return;form.value=r.data;mode.value='detail';pending.value=null;error.value='';open.value=true}finally{if(t===detailSeq)opening.value=false}}
async function submit(){if(inactive)return;if(busy.value||!open.value||mode.value!=='return')return;const view=viewSeq;busy.value=true;error.value='';try{if(!pending.value)pending.value=immutablePayload({id:form.value.packageId,payload:salesReturnPayload(form.value)});const r=await postSalesReturn(pending.value.id,pending.value.payload);if(inactive||view!==viewSeq)return;lastResult.value=r.data;closing.value=true;open.value=false;form.value={lines:[]};pending.value=null;ElMessage.success('订单退货已入库');await load()}catch(e){if(!inactive&&view===viewSeq)error.value=pending.value?'结果未确认：版本、原因、数量和请求键已锁定。请沿用原请求重试；关闭后先查退货历史，不要换键重复入库。':e.message||'请核对本次退货'}finally{if(view===viewSeq)busy.value=false}}
function closed(){if(open.value)return;closing.value=true;++detailSeq;opening.value=false;form.value={lines:[]};pending.value=null;error.value=''}
function closeDialog(){if(busy.value)return;closing.value=true;open.value=false;closed()}
function beforeClose(done){if(busy.value)return;closeDialog();done()}
function clear(){busy.value=false;++viewSeq;++listSeq;++detailSeq;open.value=false;form.value={lines:[]};pending.value=null;rows.value=[];total.value=0;opening.value=false;loading.value=false;lastResult.value=null;error.value=''}
async function openLink(){if(inactive)return;clear();closing.value=false;const rawOrder=route.query.orderId,rawPackage=route.query.packageId;const oid=safeID(rawOrder),pid=safeID(rawPackage);if((rawOrder!==undefined&&!oid)||(rawPackage!==undefined&&!pid)){listError.value=true;return}query.orderId=oid||undefined;query.packageId=pid||undefined;const view=viewSeq;await load();if(view===viewSeq&&pid)return start(pid)}watch(()=>[route.query.orderId,route.query.packageId],openLink)
onBeforeUnmount(()=>{inactive=true;clear()});onDeactivated(()=>{inactive=true;clear()});onActivated(()=>{if(inactive){inactive=false;closing.value=false;const p=load();openLink();return p}})
openLink()
function reset(){numberFilter.value='';resetERPQuery(query,{pageNum:1,pageSize:10});return search()}
</script>
