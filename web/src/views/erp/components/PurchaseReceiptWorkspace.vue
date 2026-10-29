<template>
 <div class="app-container">
  <el-alert title="这里是已过账的收货事实，不含采购价格；原收货不可修改、删除、撤销；退货使用独立采购退货作业。" type="info" :closable="false" class="mb8"/>
  <ERPQueryBar :model="query" :loading="loading"  @search="search" @reset="reset">
   <el-form-item label="采购ID"><el-input v-model="purchaseFilter" clearable placeholder="关联采购ID"/></el-form-item>
   <el-form-item label="收货单号"><el-input v-model="query.receiptNo" clearable placeholder="完整收货单号"/></el-form-item>
  </ERPQueryBar>
  <el-alert v-if="listError" title="收货记录读取失败，请重试；并非零条数据。" type="error" :closable="false"/>
  <el-table :data="rows" v-loading="loading"><el-table-column prop="receiptNo" label="收货单号" min-width="250" show-overflow-tooltip/><el-table-column prop="purchaseId" label="采购ID" width="100"/><el-table-column prop="warehouseId" label="仓库ID" width="95"/><el-table-column prop="postedAt" label="入库时间" min-width="170"/><el-table-column prop="stockDocumentId" label="库存凭据" width="100"/><el-table-column label="操作" width="325" fixed="right"><template #default="{row}"><el-button link type="primary" v-if="can('erp:purchaseReceipt:query')" :disabled="opening||closing" @click="show(row,'receipt')">明细</el-button><el-button link v-if="can('erp:purchase:query')" @click="router.push({path:'/erp/purchase',query:{purchaseId:String(row.purchaseId)}})">采购单</el-button><el-button link v-if="can('erp:receipt:query')" :disabled="opening||closing" @click="show(row,'voucher')">库存凭据</el-button><el-button link type="danger" v-if="can('erp:purchaseReturnOrder:return')" @click="router.push({path:'/erp/purchaseReturnOrder',query:{receiptId:String(row.receiptId)}})">退货</el-button></template></el-table-column></el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="mode==='voucher'?'关联入库凭据 · 只读':'收货明细 · 不含价格'" class="erp-dialog" width="min(900px, calc(100vw - 24px))" @close="closed" @closed="closing=false" :before-close="beforeClose">
   <p v-if="mode==='receipt'">收货 #{{selected.receiptId}} · 采购 #{{selected.purchaseId}} · 入库凭据 #{{selected.stockDocumentId}}</p><p v-else>库存凭据 #{{selected.documentId}} · {{selected.documentNo}} · {{selected.status}}</p>
   <el-table :data="selected.lines"><el-table-column prop="skuCode" label="SKU快照" min-width="130"/><el-table-column prop="productName" label="商品快照" min-width="160"/><el-table-column prop="specification" label="规格" min-width="130"/><el-table-column prop="unit" label="单位" width="70"/><el-table-column prop="quantity" label="本次数量" width="100"/></el-table><template #footer><el-button @click="closeDialog">关闭</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="PurchaseReceiptWorkspace">
import ERPQueryBar from './ERPQueryBar.vue'
import { resetERPQuery } from '@/utils/erp-query.mjs'

import {ref,reactive,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRoute,useRouter} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {listPurchaseReceipts,getPurchaseReceipt} from '@/api/erp/purchase'
import {getDocument} from '@/api/erp/workflow'
import {safeID} from '@/utils/purchasing.mjs'
const router=useRouter(),route=useRoute(),can=p=>checkPermi([p]),purchaseFilter=ref(''),query=reactive({pageNum:1,pageSize:10,purchaseId:undefined,receiptNo:''}),rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),opening=ref(false),closing=ref(false),open=ref(false),selected=ref({lines:[]}),mode=ref('receipt')
let listSeq=0,detailSeq=0,inactive=false
async function load(){if(inactive)return;const t=++listSeq;loading.value=true;listError.value=false;try{const r=await listPurchaseReceipts({...query});if(t===listSeq){rows.value=r.rows;total.value=r.total}}catch{if(t===listSeq){rows.value=[];total.value=0;listError.value=true}}finally{if(t===listSeq)loading.value=false}}
function search(){if(inactive)return;const id=safeID(purchaseFilter.value);if(purchaseFilter.value&&!id){ElMessage.warning('采购ID须为有效正整数');return};query.purchaseId=id;query.pageNum=1;return load()}
async function show(row,next){if(inactive)return;if(opening.value||closing.value||open.value||!can(next==='voucher'?'erp:receipt:query':'erp:purchaseReceipt:query'))return;const t=++detailSeq;opening.value=true;try{const r=await(next==='voucher'?getDocument('receipt',row.stockDocumentId):getPurchaseReceipt(row.receiptId));if(t!==detailSeq)return;selected.value=r.data;mode.value=next;open.value=true}finally{if(t===detailSeq)opening.value=false}}
function closed(){if(open.value)return;closing.value=true;++detailSeq;opening.value=false;selected.value={lines:[]}}
function closeDialog(){closing.value=true;open.value=false;closed()}function beforeClose(done){closeDialog();done()}
function clear(){++listSeq;++detailSeq;open.value=false;selected.value={lines:[]};rows.value=[];total.value=0;loading.value=false;opening.value=false}
function link(){if(inactive)return;const id=safeID(route.query.purchaseId);purchaseFilter.value=id?String(id):'';return search()}watch(()=>route.query.purchaseId,link)
onBeforeUnmount(()=>{inactive=true;clear()});onDeactivated(()=>{inactive=true;clear()});onActivated(()=>{if(inactive){inactive=false;closing.value=false;if(can('erp:purchaseReceipt:list'))return link()}})
if(can('erp:purchaseReceipt:list'))link()
function reset(){purchaseFilter.value='';resetERPQuery(query,{pageNum:1,pageSize:10,purchaseId:undefined,receiptNo:''});return search()}
</script>
