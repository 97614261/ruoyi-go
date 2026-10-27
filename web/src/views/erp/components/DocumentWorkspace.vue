<template>
 <div class="app-container">
  <el-alert :title="notice" type="info" :closable="false" class="mb12"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="单号"><el-input v-model="query.documentNo" clearable placeholder="完整单号"/></el-form-item>
   <el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" style="width:220px"/></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:130px"><el-option v-for="status in statuses" :key="status" :label="label(status)" :value="status"/></el-select></el-form-item>
   <el-form-item><el-button type="primary" @click="search">查询</el-button><el-button @click="start" v-hasPermi="[`erp:${type}:add`]">新建草稿</el-button></el-form-item>
  </el-form>
  <el-table :data="rows" v-loading="loading" size="small">
   <el-table-column label="单号" prop="documentNo" min-width="280" show-overflow-tooltip/><el-table-column label="仓库" prop="warehouseName" min-width="120"/>
   <el-table-column label="状态" width="100"><template #default="{row}"><el-tag>{{label(row.status)}}</el-tag></template></el-table-column>
   <el-table-column label="类型" width="80" v-if="correction"><template #default="{row}">{{row.reversal?'整单冲销':'退货'}}</template></el-table-column>
   <el-table-column label="版本" prop="version" width="60"/><el-table-column label="创建人" prop="createBy" width="90"/>
   <el-table-column label="操作" width="290"><template #default="{row}">
    <el-button link type="primary" @click="detail(row)" v-hasPermi="[`erp:${type}:query`]">明细</el-button>
    <el-button v-if="row.status==='DRAFT'" link type="primary" @click="edit(row)" v-hasPermi="[`erp:${type}:edit`]">编辑</el-button>
    <el-button v-for="action in documentActions(kind,row.status)" :key="action" link :type="action==='cancel'?'danger':'primary'" :disabled="working" @click="act(row,action)" v-hasPermi="[`erp:${type}:${action}`]">{{actionLabels[action]}}</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="form.documentId?'编辑草稿 · v'+form.version:'新建草稿'" width="min(850px, calc(100vw - 32px))" class="erp-document-dialog erp-dialog" align-center :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" @close="closeDraft">
   <el-alert v-if="kind==='COUNT'" title="保存时记录账面快照；期间有库存流转将拒绝过账。编辑会重新取快照，请重新核对实盘。" type="warning" :closable="false" class="mb12"/>
   <el-alert v-if="pending" title="结果未确认，原内容已固定。新建沿原请求键，编辑版本冲突时先查单；关闭后先核对记录。" type="warning" :closable="false"/><el-form label-width="85px" :disabled="saving||!!pending">
    <template v-if="correction">
     <el-form-item label="关联原单" required><el-select v-model="form.sourceDocumentId" :disabled="!!form.documentId" remote filterable :remote-method="findSources" :loading="sourceLoading" @visible-change="v=>v&&findSources('')" @change="chooseSource" style="width:100%" placeholder="搜索完整原单号；最多100条"><el-option v-for="d in sources" :key="d.documentId" :value="d.documentId" :label="d.documentNo+' · '+d.warehouseName"/></el-select></el-form-item>
     <el-form-item label="整单冲销"><el-checkbox v-model="form.reversal" :disabled="!!form.documentId||!form.sourceDocumentId" @change="resetSourceLines">仅未退货原单；按全部原数量反向过账</el-checkbox></el-form-item>
    </template>
    <el-form-item label="仓库" required><span v-if="correction">{{form.warehouseName||'请先选择原单'}}</span><ReferenceSelect v-else kind="warehouse" v-model="form.warehouseId" :disabled="saving||!!pending"/></el-form-item>
   </el-form>
   <el-table :data="form.lines" size="small">
    <el-table-column label="商品" min-width="300"><template #default="{row}"><span v-if="correction">{{row.skuCode}} · {{row.productName}}</span><ReferenceSelect v-else v-model="row.skuId" :disabled="saving||!!pending"/></template></el-table-column>
    <el-table-column :label="kind==='COUNT'?'实盘数量（可为0）':'数量（整数）'" width="200"><template #default="{row}"><el-input-number v-if="kind==='COUNT'" v-model="row.countedQuantity" :disabled="saving||!!pending" :min="0" :max="1000000000" :precision="0"/><el-input-number v-else v-model="row.quantity" :disabled="saving||!!pending||form.reversal" :min="1" :max="1000000000" :precision="0"/></template></el-table-column>
    <el-table-column v-if="!form.reversal" width="75"><template #default="{$index}"><el-button link type="danger" @click="form.lines.splice($index,1)" :disabled="saving||!!pending">移除</el-button></template></el-table-column>
   </el-table>
   <el-button v-if="!correction" class="mt12" @click="form.lines.push(kind==='COUNT'?{countedQuantity:0}:{quantity:1})" :disabled="saving||!!pending||form.lines.length>=100">添加商品行</el-button>
   <el-input class="mt12" v-model="form.remark" :disabled="saving||!!pending" type="textarea" maxlength="500" show-word-limit :placeholder="correction||kind==='COUNT'?'必填：退货／冲销／盘点原因':'备注（可选）'"/>
   <template #footer><el-button @click="open=false" :disabled="saving">取消</el-button><el-button type="primary" @click="save" :loading="saving">保存草稿</el-button></template>
  </el-dialog>
  <el-dialog v-model="detailOpen" title="单据明细快照" width="min(900px, calc(100vw - 32px))" class="erp-document-dialog erp-dialog" align-center>
   <p>{{selected.documentNo}} · {{selected.warehouseName}} · {{label(selected.status)}} · v{{selected.version}}</p>
   <p v-if="selected.sourceDocumentId">关联原单 ID：{{selected.sourceDocumentId}} · {{selected.reversal?'整单冲销':'退货'}}</p>
   <el-table :data="selected.lines" size="small"><el-table-column label="SKU编码" prop="skuCode"/><el-table-column label="商品" prop="productName"/><el-table-column label="规格" prop="specification"/><el-table-column label="单位" prop="unit" width="60"/><el-table-column :label="kind==='COUNT'?'实盘':'数量'" prop="quantity" width="100"/><template v-if="kind==='COUNT'"><el-table-column label="账面快照" prop="expectedOnHand" width="100"/><el-table-column label="预占快照" prop="expectedReserved" width="100"/><el-table-column label="差异" width="100"><template #default="{row}">{{row.quantity-row.expectedOnHand}}</template></el-table-column></template></el-table>
   <p>{{selected.remark}}</p>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,computed,onMounted,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import {listDocuments,getDocument,createDocument,editDocument,actOnDocument} from '@/api/erp/workflow'
import {documentKinds,documentStatusLabel,actionLabels,documentActions,documentActionPrompt,validateDocumentLines} from '@/utils/erp.mjs'
import ReferenceSelect from './ReferenceSelect.vue'
const props=defineProps({type:{type:String,required:true}})
const kind=computed(()=>documentKinds[props.type]),correction=computed(()=>['SR','PR'].includes(kind.value)),sourceType=computed(()=>props.type==='salesReturn'?'sales':'receipt')
const statuses=computed(()=>kind.value==='SALE'?['DRAFT','RESERVED','SHIPPED','CANCELLED']:['DRAFT','POSTED','CANCELLED'])
const notice=computed(()=>correction.value?'必须关联已出／入库原单；确认时复核剩余可退数量。冲销不删除原单和流水。':kind.value==='COUNT'?'盘点记录账面快照，确认时写差异流水；有库存变动则拒绝，不能覆盖预占。':'草稿可编辑并校验版本；确认后不可修改。已入库／出库请用退货或整单冲销纠错。')
const query=reactive({pageNum:1,pageSize:10,status:'',documentNo:''}),rows=ref([]),total=ref(0),loading=ref(false),open=ref(false),saving=ref(false),working=ref(false),form=ref({lines:[]}),selected=ref({}),detailOpen=ref(false),sources=ref([]),sourceLoading=ref(false),sourceLines=ref([])
const label=status=>documentStatusLabel(kind.value,status)
async function load(){if(!alive)return;const seq=++listSeq;loading.value=true;try{const res=await listDocuments(props.type,{...query});if(alive&&seq===listSeq){rows.value=res.rows;total.value=res.total}}finally{if(seq===listSeq)loading.value=false}}
function search(){query.pageNum=1;load()}
function start(){if(!alive||saving.value||working.value||pending.value&&open.value)return;pending.value=null;++detailSeq;sourceLines.value=[];sources.value=[];form.value={requestKey:crypto.randomUUID(),remark:'',reversal:false,lines:correction.value?[]:[kind.value==='COUNT'?{countedQuantity:0}:{quantity:1}]};open.value=true}
const pending=ref(null)
let sourceSequence=0,listSeq=0,detailSeq=0,view=0,operation=0,alive=true,wasInactive=false
async function findSources(documentNo){const seq=++sourceSequence;sourceLoading.value=true;try{const r=await listDocuments(sourceType.value,{pageSize:100,status:sourceType.value==='sales'?'SHIPPED':'POSTED',documentNo});if(seq===sourceSequence)sources.value=r.rows}finally{if(seq===sourceSequence)sourceLoading.value=false}}
async function chooseSource(id){if(!alive||pending.value)return;const owner=view,seq=detailSeq;const data=(await getDocument(sourceType.value,id)).data;if(!alive||owner!==view||seq!==detailSeq||form.value.sourceDocumentId!==id||!open.value)return;form.value.warehouseId=data.warehouseId;form.value.warehouseName=data.warehouseName;sourceLines.value=data.lines;resetSourceLines()}
function resetSourceLines(){form.value.lines=sourceLines.value.map(l=>({...l}))}
async function edit(row){if(!alive||saving.value||working.value)return;const seq=++detailSeq;const data=(await getDocument(props.type,row.documentId)).data;if(!alive||seq!==detailSeq)return;form.value={...data,lines:data.lines.map(l=>({...l,...(kind.value==='COUNT'?{countedQuantity:l.quantity}:{})}))};if(correction.value)sources.value=[{documentId:data.sourceDocumentId,documentNo:'原单 ID '+data.sourceDocumentId,warehouseName:data.warehouseName}];open.value=true}
async function save(){
 if(!alive||saving.value)return
 let c=pending.value
 if(!c){const problem=validateDocumentLines(form.value.lines,kind.value);if(!form.value.warehouseId||problem){ElMessage.warning(problem||'请选择仓库');return}if((correction.value||kind.value==='COUNT')&&!form.value.remark?.trim()){ElMessage.warning('请填写原因');return};c=JSON.parse(JSON.stringify({id:form.value.documentId,payload:{warehouseId:form.value.warehouseId,requestKey:form.value.requestKey,remark:form.value.remark,sourceDocumentId:form.value.sourceDocumentId,reversal:!!form.value.reversal,version:form.value.version,lines:form.value.lines.map(l=>({skuId:l.skuId,...(kind.value==='COUNT'?{countedQuantity:l.countedQuantity}:{quantity:l.quantity})}))}}));pending.value=c}
 const owner=view,op=++operation;saving.value=true
 try{if(c.id)await editDocument(props.type,c.id,c.payload);else await createDocument(props.type,c.payload);if(!alive||owner!==view||op!==operation)return;open.value=false;pending.value=null;ElMessage.success('草稿已保存，请检查明细后确认');await load()}finally{if(op===operation)saving.value=false}
}
async function detail(row){if(!alive)return;const seq=++detailSeq;const r=await getDocument(props.type,row.documentId);if(!alive||seq!==detailSeq)return;selected.value=r.data;detailOpen.value=true}
async function act(row,action){if(!alive||working.value)return;const owner=view,op=++operation,id=row.documentId;working.value=true;try{try{await ElMessageBox.confirm(documentActionPrompt(kind.value,action),'业务确认',{type:'warning'})}catch{return};if(!alive||owner!==view||op!==operation)return;await actOnDocument(props.type,id,action);if(!alive||owner!==view||op!==operation)return;ElMessage.success('操作成功');await load()}finally{if(op===operation)working.value=false}}
onMounted(load)
function leave(){pending.value=null;alive=false;++view;++operation;++listSeq;++detailSeq;++sourceSequence;loading.value=false;saving.value=false;working.value=false;open.value=false;detailOpen.value=false;rows.value=[];total.value=0;form.value={lines:[]};selected.value={};sources.value=[];sourceLines.value=[];sourceLoading.value=false}
onBeforeUnmount(leave);onDeactivated(()=>{wasInactive=true;leave()});onActivated(()=>{if(wasInactive){wasInactive=false;alive=true;return load()}})
function closeDraft(){if(open.value)return;++detailSeq;++sourceSequence;pending.value=null;form.value={lines:[]};sources.value=[];sourceLines.value=[]}
</script>
<style scoped>.mt12{margin-top:12px}.mb12{margin-bottom:12px}</style>
<!-- Dialog content can be teleported: limit these global rules to ERP document dialogs. -->
<style>
.erp-document-dialog { display: flex; flex-direction: column; max-height: calc(100vh - 32px); max-height: calc(100dvh - 32px); overflow: hidden; }
.erp-document-dialog .el-dialog__header, .erp-document-dialog .el-dialog__footer { flex-shrink: 0; }
.erp-document-dialog .el-dialog__body { min-height: 0; overflow: auto; }
.erp-document-dialog .el-dialog__body > p { overflow-wrap: anywhere; }
</style>
