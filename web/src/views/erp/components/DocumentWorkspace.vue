<template>
 <div class="app-container">
  <el-alert :title="type==='receipt'?'入库单：创建草稿后确认入库。已入库不可直接修改、删除或取消。':'销售单：草稿 → 确认预占 → 确认出库；出库前取消会释放预占。'" type="info" :closable="false" class="mb12"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="单号"><el-input v-model="query.documentNo" clearable placeholder="完整单号"/></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:130px"><el-option v-for="status in statuses" :key="status" :label="statusLabels[status]" :value="status"/></el-select></el-form-item>
   <el-form-item><el-button type="primary" @click="search">查询</el-button><el-button @click="start" v-hasPermi="[`erp:${type}:add`]">新建草稿</el-button></el-form-item>
  </el-form>
  <el-table :data="rows" v-loading="loading" size="small">
   <el-table-column label="单号" prop="documentNo" min-width="290" show-overflow-tooltip/>
   <el-table-column label="仓库" prop="warehouseName" min-width="130"/>
   <el-table-column label="状态" width="100"><template #default="{row}"><el-tag>{{statusLabels[row.status]||row.status}}</el-tag></template></el-table-column>
   <el-table-column label="创建人" prop="createBy" width="100"/><el-table-column label="创建时间" prop="createTime" width="170"/>
   <el-table-column label="操作" width="270"><template #default="{row}">
    <el-button link type="primary" @click="detail(row)" v-hasPermi="[`erp:${type}:query`]">明细</el-button>
    <el-button v-for="action in documentActions(kind,row.status)" :key="action" link :type="action==='cancel'?'danger':'primary'" :disabled="working" @click="act(row,action)" v-hasPermi="[`erp:${type}:${action}`]">{{actionLabels[action]}}</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" title="新建草稿" width="780px" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
   <el-form label-width="80px"><el-form-item label="仓库" required><ReferenceSelect kind="warehouse" v-model="form.warehouseId"/></el-form-item></el-form>
   <el-table :data="form.lines" size="small">
    <el-table-column label="商品" min-width="350"><template #default="{row}"><ReferenceSelect v-model="row.skuId"/></template></el-table-column>
    <el-table-column label="数量（整数件）" width="190"><template #default="{row}"><el-input-number v-model="row.quantity" :min="1" :max="1000000000" :precision="0"/></template></el-table-column>
    <el-table-column width="80"><template #default="{$index}"><el-button link type="danger" @click="form.lines.splice($index,1)" :disabled="saving">移除</el-button></template></el-table-column>
   </el-table>
   <el-button class="mt12" @click="form.lines.push({quantity:1})" :disabled="saving||form.lines.length>=100">添加商品行</el-button>
   <el-input class="mt12" v-model="form.remark" type="textarea" maxlength="500" show-word-limit placeholder="备注（可选）"/>
   <template #footer><el-button @click="open=false" :disabled="saving">取消</el-button><el-button type="primary" @click="save" :loading="saving">保存草稿</el-button></template>
  </el-dialog>
  <el-dialog v-model="detailOpen" title="单据明细快照" width="800px">
   <p>{{selected.documentNo}} · {{selected.warehouseName}} · {{statusLabels[selected.status]}}</p>
   <el-table :data="selected.lines" size="small"><el-table-column label="SKU编码" prop="skuCode"/><el-table-column label="商品" prop="productName"/><el-table-column label="规格" prop="specification"/><el-table-column label="单位" prop="unit" width="80"/><el-table-column label="数量" prop="quantity" width="100"/></el-table>
   <p>{{selected.remark}}</p>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,computed,onMounted} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import {listDocuments,getDocument,createDocument,actOnDocument} from '@/api/erp/workflow'
import {statusLabels,actionLabels,documentActions,validateDocumentLines} from '@/utils/erp.mjs'
import ReferenceSelect from './ReferenceSelect.vue'
const props=defineProps({type:{type:String,required:true}})
const kind=computed(()=>props.type==='receipt'?'IN':'SALE'),statuses=computed(()=>props.type==='receipt'?['DRAFT','POSTED','CANCELLED']:['DRAFT','RESERVED','SHIPPED','CANCELLED'])
const query=reactive({pageNum:1,pageSize:10,status:'',documentNo:''}),rows=ref([]),total=ref(0),loading=ref(false),open=ref(false),saving=ref(false),working=ref(false),form=ref({lines:[]}),selected=ref({}),detailOpen=ref(false)
async function load(){loading.value=true;try{const res=await listDocuments(props.type,query);rows.value=res.rows;total.value=res.total}finally{loading.value=false}}
function search(){query.pageNum=1;load()}
function start(){form.value={requestKey:crypto.randomUUID(),remark:'',lines:[{quantity:1}]};open.value=true}
async function save(){
 if(saving.value)return
 const problem=validateDocumentLines(form.value.lines);if(!form.value.warehouseId||problem){ElMessage.warning(problem||'请选择仓库');return}
 saving.value=true;try{await createDocument(props.type,form.value);open.value=false;ElMessage.success('草稿已创建，请检查明细后确认');await load()}finally{saving.value=false}
}
async function detail(row){selected.value=(await getDocument(props.type,row.documentId)).data;detailOpen.value=true}
async function act(row,action){
 if(working.value)return;working.value=true
 try{try{await ElMessageBox.confirm(`确认${actionLabels[action]}？库存将按单据状态原子处理。`,'业务确认',{type:'warning'})}catch{return};await actOnDocument(props.type,row.documentId,action);ElMessage.success('操作成功');await load()}finally{working.value=false}
}
onMounted(load)
</script>
<style scoped>.mt12{margin-top:12px}.mb12{margin-bottom:12px}</style>
