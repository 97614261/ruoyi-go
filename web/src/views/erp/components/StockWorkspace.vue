<template>
 <div class="app-container">
  <el-alert :title="ledger?'流水只读；名称沿用单据快照，来源详情另查功能及店铺/仓库权限。':'可用 = 实物 − 预占；零库存只指已有余额行。不同SKU/单位不相加，不提供覆盖余额入口。'" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" @update:model-value="search" style="width:180px"/></el-form-item>
   <el-form-item label="商品"><ReferenceSelect v-model="query.skuId" @update:model-value="search" style="width:220px"/></el-form-item>
   <el-form-item v-if="!ledger" label="库存"><el-select v-model="query.stockState" style="width:140px" @change="search"><el-option v-for="(label,key) in states" :key="key" :value="key" :label="label"/></el-select></el-form-item>
   <template v-else><el-form-item label="单号"><el-input v-model="query.documentNo" clearable placeholder="完整来源单号"/></el-form-item><el-form-item label="事件"><el-select v-model="query.event" clearable style="width:130px"><el-option v-for="(v,k) in eventLabels" :key="k" :value="k" :label="v"/></el-select></el-form-item><el-form-item label="时间"><el-date-picker v-model="times" type="datetimerange" value-format="YYYY-MM-DD HH:mm:ss" start-placeholder="开始" end-placeholder="结束" style="max-width:100%;width:340px"/></el-form-item></template>
   <el-form-item><el-button type="primary" @click="search">查询</el-button><el-button @click="reset">重置</el-button><el-button v-if="can(ledger?'erp:ledger:export':'erp:stock:export')" @click="exportRows">导出</el-button><el-button v-if="!ledger&&can('erp:stock:reconcile')" @click="openReconcile">只读核对</el-button></el-form-item>
  </el-form>
  <el-alert v-if="listError" title="读取失败，旧数据已清除，请重试；不是零条记录。" type="error" :closable="false"/>
  <el-table :data="rows" v-loading="loading" size="small">
   <el-table-column prop="warehouseName" label="仓库" min-width="130"/><el-table-column prop="skuCode" label="SKU编码" min-width="130"/><el-table-column prop="productName" label="商品" min-width="150"/><el-table-column prop="unit" label="单位" width="65"/>
   <template v-if="!ledger"><el-table-column label="实物" prop="onHand" width="95"/><el-table-column label="预占" width="100"><template #default="{row}"><el-button v-if="can('erp:sales:query')&&row.reserved>0" link type="primary" @click="showReservations(row)">{{row.reserved}}</el-button><span v-else>{{row.reserved}}</span></template></el-table-column><el-table-column label="可用" prop="available" width="95"/><el-table-column label="最低库存" width="110"><template #default="{row}"><el-tag v-if="row.lowStock" type="danger">{{thresholdLabel(row.thresholdQuantity)}} · 低</el-tag><span v-else>{{thresholdLabel(row.thresholdQuantity)}}</span></template></el-table-column><el-table-column label="操作" width="150" fixed="right"><template #default="{row}"><el-button v-if="can('erp:ledger:list')" link @click="ledgerFor(row)">流水</el-button><el-button v-if="can('erp:stock:alert')" link type="primary" @click="openAlert(row)">预警</el-button></template></el-table-column></template>
   <template v-else><el-table-column label="来源单号" min-width="300"><template #default="{row}"><el-button v-if="canSource(row)" link type="primary" @click="showSource(row)">{{row.documentNo}}</el-button><span v-else>{{row.documentNo}}</span></template></el-table-column><el-table-column label="事件" width="110"><template #default="{row}">{{eventLabels[row.event]||row.event}}</template></el-table-column><el-table-column prop="onHandDelta" label="实物变化" width="100"/><el-table-column prop="reservedDelta" label="预占变化" width="100"/><el-table-column prop="onHandAfter" label="实物结余" width="100"/><el-table-column prop="reservedAfter" label="预占结余" width="100"/><el-table-column prop="createBy" label="操作人" width="110"/><el-table-column prop="createTime" label="时间" width="170"/></template>
  </el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="alertOpen" title="最低可用库存 · 仓库＋SKU" class="erp-dialog" width="min(500px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" @close="detailTicket++">
   <p>仓库 #{{alert.warehouseId}} · SKU #{{alert.skuId}} · v{{alert.version}}</p><el-checkbox v-model="alert.enabled" :disabled="busy">启用本项预警</el-checkbox><el-input-number v-if="alert.enabled" v-model="alert.thresholdQuantity" :min="0" :max="1000000000000" :precision="0" :disabled="busy"/><p>0是有效阈值；取消启用表示未配置，不会自动采购。</p><el-alert v-if="detailError" :title="detailError" type="warning" :closable="false"/><template #footer><el-button :disabled="busy" @click="alertOpen=false">关闭</el-button><el-button type="primary" :loading="busy" @click="saveAlert">保存</el-button></template>
  </el-dialog>
  <el-dialog v-model="inspectOpen" :title="inspectTitle" class="erp-dialog" width="min(1050px, calc(100vw - 24px))" @close="clearInspection" @closed="inspectClosing=false">
   <el-alert v-if="detailError" :title="detailError" type="warning" :closable="false"/>
   <p v-if="inspectMode==='reservations'">仅列出你可读的预占来源，可能不是该仓库预占的全部来源；不显示订单收件资料。</p>
   <p v-if="inspectMode==='reconcile'">只读快照核对当前页，不是全仓已通过；差异只报告，不自动修账。</p>
   <el-table v-if="inspectMode==='reconcile'" :data="inspectRows" size="small"><el-table-column prop="warehouseId" label="仓库ID"/><el-table-column prop="skuId" label="SKU ID"/><el-table-column prop="onHand" label="实物"/><el-table-column prop="ledgerOnHand" label="流水实物"/><el-table-column prop="reserved" label="预占"/><el-table-column prop="ledgerReserved" label="流水预占"/><el-table-column label="核对"><template #default="{row}"><el-tag :type="row.consistent?'success':'danger'">{{row.balanceMissing?'余额缺失':row.consistent?'一致':'有差异'}}</el-tag></template></el-table-column></el-table>
   <el-table v-else-if="inspectMode==='reservations'" :data="inspectRows" size="small"><el-table-column label="单号" min-width="300"><template #default="{row}"><el-button link type="primary" @click="showSource({...row,documentKind:'SALE'})">{{row.documentNo}}</el-button></template></el-table-column><el-table-column prop="warehouseName" label="仓库"/><el-table-column prop="status" label="状态"/></el-table>
   <template v-else-if="source"><p>{{source.documentNo||source.transferNo}} · {{source.status}} · v{{source.version}}</p><el-table :data="source.lines||[]" size="small"><el-table-column prop="skuCode" label="SKU编码"/><el-table-column prop="productName" label="商品快照"/><el-table-column prop="unit" label="单位"/><el-table-column prop="quantity" label="数量"/></el-table></template>
   <pagination v-if="inspectMode!=='source'&&inspectTotal>0" :total="inspectTotal" v-model:page="inspectQuery.pageNum" v-model:limit="inspectQuery.pageSize" @pagination="loadInspection"/>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,watch,onActivated,onBeforeUnmount,onDeactivated} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {checkPermi} from '@/utils/permission'
import {ElMessage} from 'element-plus'
import {listStock,listLedger,getDocument} from '@/api/erp/workflow'
import {getStockAlert,setStockAlert,reconcileStock,stockReservations,getTransfer} from '@/api/erp/warehouseOperations'
import {eventLabels} from '@/utils/erp.mjs'
import {sourceType,thresholdLabel} from '@/utils/warehouse-operations.mjs'
import {safeID} from '@/utils/purchasing.mjs'
import {download} from '@/utils/request'
import ReferenceSelect from './ReferenceSelect.vue'
const props=defineProps({ledger:Boolean}),router=useRouter(),route=useRoute(),can=p=>checkPermi([p])
const query=reactive({pageNum:1,pageSize:10,stockState:'ALL'}),times=ref([]),rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false)
const states={HAS_STOCK:'有实物库存',HAS_RESERVED:'存在预占',ALL:'全部余额',IN_STOCK:'有可用库存',ZERO:'零实物库存',RESERVED_OUT:'全部被预占',LOW:'低于/等于阈值'}
const alertOpen=ref(false),alert=ref({}),busy=ref(false),detailError=ref(''),inspectOpen=ref(false),inspectClosing=ref(false),inspectMode=ref(''),inspectTitle=ref(''),inspectRows=ref([]),inspectTotal=ref(0),inspectQuery=reactive({pageNum:1,pageSize:10}),source=ref(null)
let ticket=0,detailTicket=0,alive=true
const params=()=>({...query,beginTime:times.value?.[0],endTime:times.value?.[1]})
async function load(){const seq=++ticket;rows.value=[];total.value=0;loading.value=true;listError.value=false;try{const r=await(props.ledger?listLedger(params()):listStock(params()));if(alive&&seq===ticket){rows.value=r.rows;total.value=r.total}}catch{if(alive&&seq===ticket)listError.value=true}finally{if(alive&&seq===ticket)loading.value=false}}
function search(){query.pageNum=1;inspectOpen.value=false;source.value=null;++detailTicket;load()}
function reset(){Object.assign(query,{warehouseId:undefined,skuId:undefined,documentNo:undefined,event:undefined,stockState:'ALL',documentId:undefined});times.value=[];search()}
function exportRows(){return download(props.ledger?'/erp/ledger/export':'/erp/stock/export',params(),props.ledger?'库存流水.xlsx':'库存余额.xlsx',{headers:{repeatSubmit:false}})}
function ledgerFor(r){router.push({path:'/erp/ledger',query:{warehouseId:r.warehouseId,skuId:r.skuId}})}
function canSource(r){return r.transferId?can('erp:transfer:query'):!!sourceType(r.documentKind)&&can(`erp:${sourceType(r.documentKind)}:query`)}
async function showSource(r){if(inspectClosing.value||!canSource(r))return;const seq=++detailTicket;inspectMode.value='source';inspectTitle.value='来源单据 · 只读';inspectRows.value=[];source.value=null;detailError.value='';inspectOpen.value=true;try{const v=await(r.transferId?getTransfer(r.transferId):getDocument(sourceType(r.documentKind),r.documentId));if(alive&&seq===detailTicket&&inspectOpen.value)source.value=v.data}catch(e){if(alive&&seq===detailTicket){source.value=null;detailError.value=e.message||'无权读取来源单据'}}}
async function openAlert(r){if(busy.value)return;const seq=++detailTicket;detailError.value='';alertOpen.value=false;try{const v=await getStockAlert({warehouseId:r.warehouseId,skuId:r.skuId});if(alive&&seq===detailTicket){alert.value={...v.data,enabled:v.data.thresholdQuantity!==null,thresholdQuantity:v.data.thresholdQuantity??0};alertOpen.value=true}}catch(e){if(alive&&seq===detailTicket)ElMessage.warning(e.message||'预警读取失败')}}
async function saveAlert(){if(busy.value)return;const f=alert.value;if(f.enabled&&(!Number.isSafeInteger(f.thresholdQuantity)||f.thresholdQuantity<0||f.thresholdQuantity>1000000000000)){detailError.value='阈值须为0—一万亿整数';return}const body={warehouseId:f.warehouseId,skuId:f.skuId,version:f.version,thresholdQuantity:f.enabled?f.thresholdQuantity:null};const seq=detailTicket;busy.value=true;try{await setStockAlert(body);if(alive&&seq===detailTicket){alertOpen.value=false;ElMessage.success('已保存');await load()}}catch(e){if(alive&&seq===detailTicket)detailError.value=(e.message||'保存未确认')+'；请关闭重读核对后再修改'}finally{busy.value=false}}
function showReservations(r){if(inspectClosing.value)return;inspectMode.value='reservations';inspectTitle.value='当前可读预占来源';Object.assign(inspectQuery,{warehouseId:r.warehouseId,skuId:r.skuId,pageNum:1});inspectOpen.value=true;loadInspection()}
function openReconcile(){if(inspectClosing.value)return;inspectMode.value='reconcile';inspectTitle.value='库存余额与流水核对';Object.assign(inspectQuery,{warehouseId:query.warehouseId,skuId:query.skuId,pageNum:1});inspectOpen.value=true;loadInspection()}
async function loadInspection(){const seq=++detailTicket;inspectRows.value=[];inspectTotal.value=0;detailError.value='';try{const v=await(inspectMode.value==='reconcile'?reconcileStock({...inspectQuery}):stockReservations({...inspectQuery}));if(alive&&seq===detailTicket&&inspectOpen.value){inspectRows.value=v.rows;inspectTotal.value=v.total}}catch(e){if(alive&&seq===detailTicket)detailError.value=e.message||'读取失败'}}
function stop(){alive=false;++ticket;++detailTicket;rows.value=[];total.value=0;inspectRows.value=[];inspectOpen.value=false;alertOpen.value=false;source.value=null}
function clearInspection(){inspectClosing.value=true;++detailTicket;source.value=null;inspectRows.value=[];inspectTotal.value=0}
onBeforeUnmount(stop);onDeactivated(stop)
function activate(){alive=true;if(!props.ledger){const raw=route.query.stockState;if(raw!==undefined&&(typeof raw!=="string"||!Object.hasOwn(states,raw))){++ticket;rows.value=[];total.value=0;listError.value=true;return}query.stockState=raw||"ALL";query.pageNum=1;}inspectClosing.value=false;if(props.ledger){query.warehouseId=safeID(route.query.warehouseId);query.skuId=safeID(route.query.skuId);if((route.query.warehouseId!==undefined&&!query.warehouseId)||(route.query.skuId!==undefined&&!query.skuId)){++ticket;rows.value=[];total.value=0;listError.value=true;return}}load()}
onActivated(()=>{if(!alive)activate()});watch(()=>[route.query.warehouseId,route.query.skuId,route.query.stockState],()=>{if(alive)activate()});activate()
</script>
<style scoped>.mb8{margin-bottom:8px}</style>
