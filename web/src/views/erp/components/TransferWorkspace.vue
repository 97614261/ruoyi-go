<template>
 <div class="app-container">
  <el-alert title="整单调出 → 在途 → 整单收货；在途不可售，不挤占订单预占。已调出不可删除或直接取消，暂不含损耗、分批或调拨退回。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search"><el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" style="width:200px"/></el-form-item><el-form-item label="状态"><el-select v-model="query.status" clearable style="width:140px"><el-option v-for="(label,key) in transferStatuses" :key="key" :value="key" :label="label"/></el-select></el-form-item><el-form-item label="单号"><el-input v-model="query.transferNo" clearable placeholder="完整单号"/></el-form-item><el-form-item><el-button @click="search">查询</el-button><el-button type="primary" v-if="can('erp:transfer:add')" :disabled="busy||opening" @click="start">新建调拨</el-button></el-form-item></el-form>
  <el-alert v-if="listError" title="读取失败，旧列表已清空，请重试。" type="error" :closable="false"/>
  <el-alert v-if="lastResult" :title="'调拨 #'+lastResult.transferId+' · '+transferStatuses[lastResult.status]+' · v'+lastResult.version" type="success" closable @close="lastResult=null"/>
  <el-table :data="rows" size="small" v-loading="loading"><el-table-column prop="transferNo" label="调拨单号" min-width="300"/><el-table-column prop="sourceWarehouseName" label="调出仓" min-width="130"/><el-table-column prop="targetWarehouseName" label="接收仓" min-width="130"/><el-table-column label="状态" width="120"><template #default="{row}">{{transferStatuses[row.status]||row.status}}</template></el-table-column><el-table-column prop="createTime" label="创建时间" width="170"/><el-table-column label="操作" width="265" fixed="right"><template #default="{row}"><el-button v-if="can('erp:transfer:query')" link @click="show(row.transferId,'detail')">详情</el-button><el-button v-if="row.status==='DRAFT'&&can('erp:transfer:edit')" link @click="show(row.transferId,'edit')">编辑</el-button><el-button v-if="row.status==='DRAFT'&&can('erp:transfer:dispatch')" link type="warning" @click="show(row.transferId,'dispatch')">调出</el-button><el-button v-if="row.status==='IN_TRANSIT'&&can('erp:transfer:receive')" link type="primary" @click="show(row.transferId,'receive')">收货</el-button><el-button v-if="row.status==='DRAFT'&&can('erp:transfer:cancel')" link type="danger" @click="show(row.transferId,'cancel')">取消</el-button></template></el-table-column></el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-alert v-if="error&&!open" :title="error" type="warning" :closable="false"/>
  <el-dialog v-model="open" :title="titles[mode]" class="erp-dialog" width="min(1000px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" :before-close="beforeClose" @close="closed">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/>
   <p v-if="form.transferId">{{form.transferNo}} · {{transferStatuses[form.status]}} · v{{form.version}}</p>
   <el-form size="small" label-width="80px" :disabled="busy||!!pending||mode!=='create'">
    <el-form-item label="调出仓"><ReferenceSelect v-if="mode==='create'" kind="warehouse" v-model="form.sourceWarehouseId"/><span v-else>{{form.sourceWarehouseName}} · #{{form.sourceWarehouseId}}</span></el-form-item><el-form-item label="接收仓"><ReferenceSelect v-if="mode==='create'" kind="warehouse" v-model="form.targetWarehouseId"/><span v-else>{{form.targetWarehouseName}} · #{{form.targetWarehouseId}}</span></el-form-item>
   </el-form>
   <el-table :data="form.lines" size="small"><el-table-column label="商品 / SKU快照" min-width="240"><template #default="{row}"><ReferenceSelect v-if="editable" v-model="row.skuId" :disabled="busy||!!pending"/><span v-else>{{row.skuCode}} · {{row.productName}} · {{row.unit}}</span></template></el-table-column><el-table-column label="数量（整数）" width="190"><template #default="{row}"><el-input-number v-if="editable" v-model="row.quantity" :min="1" :max="1000000000" :precision="0" :disabled="busy||!!pending" style="width:165px"/><span v-else>{{row.quantity}}</span></template></el-table-column><el-table-column v-if="editable" label="操作" width="70"><template #default="{ $index }"><el-button link type="danger" :disabled="busy||!!pending" @click="form.lines.splice($index,1)">移除</el-button></template></el-table-column></el-table>
   <el-button v-if="editable" size="small" class="mt8" :disabled="busy||!!pending||form.lines.length>=100" @click="form.lines.push({quantity:1})">添加商品</el-button>
   <el-input v-if="editable" v-model="form.remark" type="textarea" maxlength="500" placeholder="备注（可选）" class="mt8" :disabled="busy||!!pending"/><p v-else>备注：{{form.remark||'—'}}</p>
   <p v-if="mode==='dispatch'">确认实物已离开调出仓；本次扣源仓可用库存，不修改其他订单预占。</p><p v-if="mode==='receive'">核对整单实物已到接收仓；本次只增加接收仓实物。</p><p v-if="mode==='cancel'">仅取消草稿，不改变任何库存。</p>
   <template #footer><el-button :disabled="busy" @click="close">{{pending?'关闭 / 放弃本次重试':'关闭'}}</el-button><el-button v-if="mode!=='detail'" type="primary" :loading="busy" @click="submit">{{pending?'沿用原请求重试':titles[mode]}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,computed,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRoute} from 'vue-router'
import {checkPermi} from '@/utils/permission'
import {ElMessage} from 'element-plus'
import {listTransfers,getTransfer,addTransfer,editTransfer,actTransfer} from '@/api/erp/warehouseOperations'
import {transferPayload,transferStatuses} from '@/utils/warehouse-operations.mjs'
import {safeID,immutablePayload} from '@/utils/purchasing.mjs'
import ReferenceSelect from './ReferenceSelect.vue'
const can=p=>checkPermi([p]),route=useRoute(),titles={create:'保存调拨草稿',edit:'保存草稿修改',detail:'调拨详情 · 只读',dispatch:'确认整单调出',receive:'确认整单收货',cancel:'取消草稿'}
const query=reactive({pageNum:1,pageSize:10}),rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),open=ref(false),mode=ref('detail'),form=ref({lines:[]}),pending=ref(null),busy=ref(false),opening=ref(false),error=ref(''),lastResult=ref(null),editable=computed(()=>mode.value==='create'||mode.value==='edit')
let listSeq=0,detailSeq=0,viewSeq=0,inactive=false
async function load(){const t=++listSeq;rows.value=[];total.value=0;loading.value=true;listError.value=false;try{const v=await listTransfers({...query});if(t===listSeq&&!inactive){rows.value=v.rows;total.value=v.total}}catch{if(t===listSeq&&!inactive)listError.value=true}finally{if(t===listSeq)loading.value=false}}
function search(){query.pageNum=1;load()}
function start(){if(busy.value||opening.value||!can('erp:transfer:add'))return;++detailSeq;mode.value='create';form.value={requestKey:crypto.randomUUID(),version:0,remark:'',lines:[{quantity:1}]};pending.value=null;error.value='';open.value=true}
async function show(id,action){if(busy.value||opening.value||!safeID(id)||!can('erp:transfer:query')||action!=='detail'&&!can('erp:transfer:'+action))return;const seq=++detailSeq;opening.value=true;error.value='';open.value=false;form.value={lines:[]};pending.value=null;try{const v=await getTransfer(id);if(seq!==detailSeq||inactive)return;form.value={...v.data,requestKey:crypto.randomUUID()};mode.value=action;open.value=true}catch(e){if(seq===detailSeq)error.value=e.message||'调拨读取失败'}finally{if(seq===detailSeq)opening.value=false}}
async function submit(){if(busy.value||!open.value||mode.value==='detail')return;const view=viewSeq;busy.value=true;error.value='';try{if(!pending.value)pending.value=immutablePayload({id:form.value.transferId,action:mode.value,payload:transferPayload(form.value,mode.value)});const p=pending.value,v=await(p.action==='create'?addTransfer(p.payload):p.action==='edit'?editTransfer(p.id,p.payload):actTransfer(p.id,p.action,p.payload));if(view!==viewSeq||inactive)return;lastResult.value=v.data;open.value=false;closed();ElMessage.success('调拨操作已确认');await load()}catch(e){if(view===viewSeq&&!inactive)error.value=pending.value?'结果未确认：仓库、行、版本和请求键已锁定。请原请求重试；关闭后先查单据，不要换键重复操作。':e.message||'请核对调拨参数'}finally{busy.value=false}}
function closed(){if(open.value)return;++detailSeq;form.value={lines:[]};pending.value=null}
function close(){if(busy.value)return;open.value=false;closed()}
function beforeClose(done){if(busy.value)return;close();done()}
function clear(){inactive=true;++listSeq;++detailSeq;++viewSeq;open.value=false;form.value={lines:[]};pending.value=null;rows.value=[];total.value=0;lastResult.value=null;error.value='';opening.value=false;loading.value=false}
async function activate(){inactive=false;await load();if(route.query.transferId!==undefined){const id=safeID(route.query.transferId);if(id)await show(id,'detail');else error.value='调拨链接不合法'}}
onBeforeUnmount(clear);onDeactivated(clear);onActivated(()=>{if(inactive)activate()});watch(()=>route.query.transferId,()=>{if(!inactive)activate()});activate()
</script>
<style scoped>.mb8{margin-bottom:8px}.mt8{margin-top:8px}</style>
