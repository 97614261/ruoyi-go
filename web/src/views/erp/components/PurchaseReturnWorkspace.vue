<template>
 <div class="app-container">
  <el-alert title="退货确认立即扣可用实物，不改预占；原收货、采购进度不回退，不含退款及采购价格。" type="warning" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item><el-radio-group v-model="tab" size="small" @change="switchTab"><el-radio-button value="receipts">原收货退货</el-radio-button><el-radio-button value="returns">退货历史</el-radio-button></el-radio-group></el-form-item>
   <el-form-item :label="tab==='receipts'?'收货单号':'退货单号'"><el-input v-model="numberFilter" clearable placeholder="完整单号"/></el-form-item>
   <el-form-item><el-button @click="search">查询</el-button></el-form-item>
  </el-form>
  <el-alert v-if="listError" title="读取失败，请重试；不是零条数据。" type="error" :closable="false"/>
  <el-alert v-if="lastResult" :title="'已退货 #'+lastResult.returnId+' · 库存凭据 #'+lastResult.stockDocumentId+' · 原批退货v'+lastResult.version" type="success" closable @close="lastResult=null"/>
  <el-table :data="rows" size="small" v-loading="loading">
   <el-table-column :prop="tab==='receipts'?'receiptNo':'returnNo'" :label="tab==='receipts'?'收货单号':'退货单号'" min-width="250" show-overflow-tooltip/>
   <el-table-column prop="purchaseId" label="采购ID" width="90"/><el-table-column prop="warehouseId" label="仓库ID" width="90"/>
   <el-table-column v-if="tab==='receipts'" label="收 / 退 / 净收" width="160"><template #default="{row}">{{row.receivedQuantity}} / {{row.returnedQuantity}} / {{row.netReceivedQuantity}}</template></el-table-column>
   <el-table-column v-else prop="receiptId" label="原收货ID" width="110"/>
   <el-table-column prop="postedAt" :label="tab==='receipts'?'收货时间':'退货时间'" min-width="165"/>
   <el-table-column label="操作" width="120" fixed="right"><template #default="{row}">
    <el-button v-if="tab==='receipts'&&can('erp:purchaseReturnOrder:return')" link type="danger" :disabled="locked||row.netReceivedQuantity===0" @click="start(row.receiptId)">登记退货</el-button>
    <el-button v-if="tab==='returns'&&can('erp:purchaseReturnOrder:query')" link type="primary" :disabled="locked" @click="show(row.returnId)">查看明细</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="mode==='return'?'原批次退货 · 确认即扣库存':'采购退货事实 · 只读'" class="erp-dialog" width="min(1000px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" :before-close="beforeClose" @close="closed" @closed="closing=false">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/>
   <p>原收货 #{{form.receiptId}} · 采购 #{{form.purchaseId}} · 仓库 #{{form.warehouseId}} <span v-if="mode==='return'">· 原批退货v{{form.version}}</span><span v-else>· 退货 #{{form.returnId}} · {{form.postedAt}}</span></p>
   <el-table :data="form.lines" size="small">
    <el-table-column prop="skuCode" label="SKU快照" min-width="120"/><el-table-column prop="productName" label="商品快照" min-width="160"/><el-table-column prop="specification" label="规格" min-width="120"/><el-table-column prop="unit" label="单位" width="60"/>
    <template v-if="mode==='return'">
     <el-table-column prop="quantity" label="原批收货" width="90"/><el-table-column prop="returnedQuantity" label="已退" width="80"/><el-table-column prop="netReceivedQuantity" label="可退" width="80"/>
     <el-table-column label="本次（0不提交）" width="175"><template #default="{row}"><el-input-number v-model="row.returnQuantity" :min="0" :max="row.netReceivedQuantity" :precision="0" :disabled="busy||!!pending" style="width:150px"/></template></el-table-column>
    </template>
    <el-table-column v-else prop="quantity" label="本次退货" width="100"/>
   </el-table>
   <el-form v-if="mode==='return'" label-width="80px" size="small" class="mt8"><el-form-item label="退货原因" required><el-input v-model="form.reason" maxlength="500" placeholder="必填，最多500字，不换行" :disabled="busy||!!pending"/></el-form-item></el-form>
   <p v-else>退货原因：{{form.reason}} · 库存凭据 #{{form.stockDocumentId}}</p>
   <p v-if="mode==='return'">请核对实物；不能占用已预占数量。退货过账后不可修改、删除或撤销，不会自动重新采购。</p>
   <template #footer><el-button :disabled="busy" @click="closeDialog">{{pending?'关闭 / 放弃本次重试':'关闭'}}</el-button><el-button v-if="mode==='return'" type="danger" :loading="busy" @click="submit">{{pending?'沿用原请求重试':'确认退货并扣库存'}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="PurchaseReturnWorkspace">
import {ref,reactive,computed,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {listReturnableReceipts,getReturnableReceipt,postPurchaseReturn,listPurchaseReturns,getPurchaseReturn} from '@/api/erp/purchaseReturn'
import {safeID,immutablePayload} from '@/utils/purchasing.mjs'
import {purchaseReturnPayload} from '@/utils/purchase-return.mjs'
const route=useRoute(),can=p=>checkPermi([p]),query=reactive({pageNum:1,pageSize:10})
const tab=ref('receipts'),numberFilter=ref(''),rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),open=ref(false),mode=ref('return'),closing=ref(false),busy=ref(false),opening=ref(false),form=ref({lines:[]}),pending=ref(null),error=ref(''),lastResult=ref(null)
const locked=computed(()=>busy.value||opening.value||closing.value||open.value)
let listSeq=0,detailSeq=0,viewSeq=0,inactive=false
async function load(){if(!can('erp:purchaseReturnOrder:list'))return;const t=++listSeq;loading.value=true;listError.value=false;const params={...query,[tab.value==='receipts'?'receiptNo':'returnNo']:numberFilter.value};try{const r=await(tab.value==='receipts'?listReturnableReceipts(params):listPurchaseReturns(params));if(t===listSeq){rows.value=r.rows;total.value=r.total}}catch{if(t===listSeq){rows.value=[];total.value=0;listError.value=true}}finally{if(t===listSeq)loading.value=false}}
function search(){query.pageNum=1;return load()}
function switchTab(){numberFilter.value='';rows.value=[];total.value=0;return search()}
async function start(id){if(locked.value||!can('erp:purchaseReturnOrder:return')||!safeID(id))return;const t=++detailSeq;opening.value=true;try{const r=await getReturnableReceipt(id);if(t!==detailSeq)return;form.value={...r.data,requestKey:crypto.randomUUID(),reason:'',lines:r.data.lines.map(l=>({...l,returnQuantity:0}))};mode.value='return';pending.value=null;error.value='';open.value=true}finally{if(t===detailSeq)opening.value=false}}
async function show(id){if(locked.value||!can('erp:purchaseReturnOrder:query')||!safeID(id))return;const t=++detailSeq;opening.value=true;try{const r=await getPurchaseReturn(id);if(t!==detailSeq)return;form.value=r.data;mode.value='detail';pending.value=null;error.value='';open.value=true}finally{if(t===detailSeq)opening.value=false}}
async function submit(){if(busy.value||!open.value||mode.value!=='return')return;const view=viewSeq;busy.value=true;error.value='';try{if(!pending.value)pending.value=immutablePayload({id:form.value.receiptId,payload:purchaseReturnPayload(form.value)});const r=await postPurchaseReturn(pending.value.id,pending.value.payload);if(view!==viewSeq)return;lastResult.value=r.data;closing.value=true;open.value=false;form.value={lines:[]};pending.value=null;ElMessage.success('采购退货已扣库存');await load()}catch(e){if(view===viewSeq)error.value=pending.value?'结果未确认：版本、原因、数量和请求键已锁定。请沿用原请求重试；关闭后先查退货历史，不要换键重复扣库存。':e.message||'请核对本次退货'}finally{busy.value=false}}
function closed(){if(open.value)return;closing.value=true;++detailSeq;opening.value=false;form.value={lines:[]};pending.value=null;error.value=''}
function closeDialog(){if(busy.value)return;closing.value=true;open.value=false;closed()}
function beforeClose(done){if(busy.value)return;closeDialog();done()}
function clear(){++viewSeq;++listSeq;++detailSeq;open.value=false;form.value={lines:[]};pending.value=null;rows.value=[];total.value=0;opening.value=false;loading.value=false;lastResult.value=null;error.value=''}
function openLink(){const id=safeID(route.query.receiptId);if(id)return start(id)}watch(()=>route.query.receiptId,openLink)
onBeforeUnmount(clear);onDeactivated(()=>{inactive=true;clear()});onActivated(()=>{if(inactive){inactive=false;closing.value=false;const p=load();openLink();return p}})
load();openLink()
</script>
