<template>
 <div class="app-container">
  <el-alert title="收货确认会立即增加实物库存，不改变预占。采购收货凭据不可直接撤销；需退货请使用独立采购退货作业，本页不展示价格。" type="warning" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search"><el-form-item label="采购单号"><el-input v-model="query.purchaseNo" clearable placeholder="完整采购单号"/></el-form-item><el-form-item v-if="can('erp:warehouse:list')" label="仓库"><PurchasingReferenceSelect kind="warehouse" v-model="query.warehouseId" style="width:180px"/></el-form-item><el-form-item><el-button @click="search">查询可收货采购</el-button><el-button v-if="can('erp:purchaseReceipt:list')" @click="router.push('/erp/purchaseReceipt')">收货记录</el-button></el-form-item></el-form>
  <el-alert v-if="listError" title="可收货列表读取失败，不是零条数据。" type="error" :closable="false"/>
  <el-alert v-if="lastResult" :title="'已入库：收货 #'+lastResult.receiptId+'，库存凭据 #'+lastResult.stockDocumentId+'，采购v'+lastResult.version+' · '+purchaseStates[lastResult.status]" type="success" :closable="true" @close="lastResult=null"/>
  <el-table :data="rows" size="small" v-loading="loading"><el-table-column prop="purchaseNo" label="采购单号" min-width="240" show-overflow-tooltip/><el-table-column prop="supplierName" label="供应商" min-width="130"/><el-table-column prop="warehouseName" label="到货仓库" min-width="120"/><el-table-column label="状态" width="110"><template #default="{row}">{{purchaseStates[row.status]}}</template></el-table-column><el-table-column label="已收/采购" width="120"><template #default="{row}">{{row.receivedQuantity}} / {{row.orderedQuantity}}</template></el-table-column><el-table-column label="操作" width="120" fixed="right"><template #default="{row}"><el-button link type="primary" :disabled="busy||opening||closing" @click="start(row.purchaseId)">登记到货</el-button></template></el-table-column></el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" title="登记本次到货 · 确认即入库" class="erp-dialog" width="min(900px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" :before-close="beforeClose" @close="closed" @closed="closing=false">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/><p>{{form.purchaseNo}} · {{form.supplierName}} → {{form.warehouseName}} · v{{form.version}}</p>
   <el-table :data="form.lines" size="small"><el-table-column prop="skuCode" label="SKU" min-width="120"/><el-table-column prop="productName" label="商品快照" min-width="180"/><el-table-column prop="unit" label="单位" width="65"/><el-table-column prop="orderedQuantity" label="采购" width="80"/><el-table-column prop="receivedQuantity" label="已收" width="80"/><el-table-column label="余量" width="85"><template #default="{row}">{{row.orderedQuantity-row.receivedQuantity}}</template></el-table-column><el-table-column label="本次数量（0不提交）" width="190"><template #default="{row}"><el-input-number v-model="row.quantity" :min="0" :max="row.orderedQuantity-row.receivedQuantity" :precision="0" :disabled="busy||!!pending" style="width:160px"/></template></el-table-column></el-table>
   <p>确认后将立即入库；不可从手工凭据撤销；退货请使用独立采购退货作业。请核对实物数量，不要把余量自动当到货。</p>
   <template #footer><el-button :disabled="busy" @click="closeDialog">关闭 / 放弃本次重试</el-button><el-button type="primary" :loading="busy" @click="submit">{{pending?'沿用原请求重试':'确认本次到货并入库'}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="PurchaseReceiveWorkspace">
import {ref,reactive,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRoute,useRouter} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {listReceivables,getReceivable,receivePurchase} from '@/api/erp/purchase'
import {purchaseStates,receivePayload,immutablePayload,safeID} from '@/utils/purchasing.mjs'
import PurchasingReferenceSelect from './PurchasingReferenceSelect.vue'
const router=useRouter(),route=useRoute(),can=p=>checkPermi([p]),query=reactive({pageNum:1,pageSize:10,purchaseNo:'',warehouseId:undefined})
const rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),open=ref(false),closing=ref(false),busy=ref(false),opening=ref(false),form=ref({lines:[]}),pending=ref(null),error=ref(''),lastResult=ref(null)
let listSeq=0,detailSeq=0,viewSeq=0,inactive=false
async function load(){if(inactive)return;const t=++listSeq;loading.value=true;listError.value=false;try{const r=await listReceivables({...query});if(t===listSeq){rows.value=r.rows;total.value=r.total}}catch{if(t===listSeq){rows.value=[];total.value=0;listError.value=true}}finally{if(t===listSeq)loading.value=false}}
function search(){query.pageNum=1;return load()}
async function start(id){if(inactive)return;if(busy.value||opening.value||closing.value||open.value||!can('erp:purchase:receive')||!safeID(id))return;const t=++detailSeq;opening.value=true;try{const r=await getReceivable(id);if(t!==detailSeq)return;form.value={...r.data,requestKey:crypto.randomUUID(),lines:r.data.lines.map(l=>({...l,quantity:0}))};pending.value=null;error.value='';open.value=true}finally{if(t===detailSeq)opening.value=false}}
async function submit(){if(inactive)return;if(busy.value||!open.value)return;const view=viewSeq;busy.value=true;error.value='';try{if(!pending.value)pending.value=immutablePayload({id:form.value.purchaseId,payload:receivePayload(form.value)});const r=await receivePurchase(pending.value.id,pending.value.payload);if(inactive||view!==viewSeq)return;lastResult.value=r.data;closing.value=true;open.value=false;form.value={lines:[]};pending.value=null;ElMessage.success('本次到货已入库');await load()}catch(e){if(!inactive&&view===viewSeq)error.value=pending.value?'结果未确认：数量、版本和请求键已锁定。请沿用原请求重试；关闭后先查收货记录，勿换键重复入库。':e.message||'请检查到货数量'}finally{if(view===viewSeq)busy.value=false}}
function closed(){if(open.value)return;closing.value=true;++detailSeq;opening.value=false;form.value={lines:[]};pending.value=null;error.value=''}
function closeDialog(){if(busy.value)return;closing.value=true;open.value=false;closed()}
function beforeClose(done){if(busy.value)return;closeDialog();done()}
function clear(){busy.value=false;++viewSeq;++listSeq;++detailSeq;open.value=false;form.value={lines:[]};pending.value=null;rows.value=[];total.value=0;opening.value=false;loading.value=false;lastResult.value=null}
function openLink(){if(inactive)return;const id=safeID(route.query.purchaseId);if(id)return start(id)}watch(()=>route.query.purchaseId,openLink)
onBeforeUnmount(()=>{inactive=true;clear()});onDeactivated(()=>{inactive=true;clear()});onActivated(()=>{if(inactive){inactive=false;closing.value=false;const pendingLoad=can('erp:purchase:receive')?load():undefined;openLink();return pendingLoad}})
if(can('erp:purchase:receive'))load();openLink()
</script>
