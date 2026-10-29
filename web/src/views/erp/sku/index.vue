<template>
  <div class="app-container">
    <ERPQueryBar ref="queryRef" :model="queryParams" :loading="loading"  @search="handleQuery" @reset="resetQuery">
   <el-form-item label="SKU编码" prop="skuCode"><el-input v-model="queryParams.skuCode" clearable  /></el-form-item>
   <el-form-item label="商品名称" prop="productName"><el-input v-model="queryParams.productName" clearable  /></el-form-item>
   <el-form-item label="规格" prop="specification"><el-input v-model="queryParams.specification" clearable  /></el-form-item>
   <el-form-item label="单位" prop="unit"><el-input v-model="queryParams.unit" clearable  /></el-form-item>
  </ERPQueryBar>
    <el-row :gutter="10" class="mb8"><el-col :span="1.5"><el-button type="primary" plain icon="Plus" @click="handleAdd()" v-hasPermi="['erp:sku:add']">新增</el-button></el-col><el-col :span="1.5"><el-button type="danger" plain icon="Delete" :disabled="multiple" @click="handleDelete()" v-hasPermi="['erp:sku:remove']">删除</el-button></el-col><el-col :span="1.5"><el-button type="warning" plain icon="Download" @click="handleExport" v-hasPermi="['erp:sku:export']">导出</el-button></el-col></el-row>
    <el-table v-loading="loading" :data="rows" @selection-change="handleSelectionChange"><el-table-column type="selection" width="55" align="center" />
      <el-table-column label="SKU编号" prop="skuId" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="SKU编码" prop="skuCode" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="商品名称" prop="productName" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="规格" prop="specification" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="单位" prop="unit" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="状态" prop="status" align="center"><template #default="scope"><dict-tag :options="sys_normal_disable" :value="scope.row.status" /></template></el-table-column>
      <el-table-column label="操作" align="center" width="200"><template #default="scope"><el-button link type="primary" icon="Edit" @click="handleUpdate(scope.row)" v-hasPermi="['erp:sku:edit']">修改</el-button><el-button link type="primary" icon="Delete" @click="handleDelete(scope.row)" v-hasPermi="['erp:sku:remove']">删除</el-button></template></el-table-column>
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="queryParams.pageNum" v-model:limit="queryParams.pageSize" @pagination="getList" />
    <el-dialog :title="title" v-model="open" class="erp-dialog" width="min(600px, calc(100vw - 24px))" append-to-body @close="closedRequest"><el-alert v-if="pending" title="结果未确认，原内容和版本已固定。可用原内容重试；编码重复或版本冲突时请先查询确认。" type="warning" :closable="false"/><el-form ref="formRef" :disabled="busy||!!pending" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="SKU编码" prop="skuCode"><el-input v-model="form.skuCode" /></el-form-item>
      <el-form-item label="商品名称" prop="productName"><el-input v-model="form.productName" /></el-form-item>
      <el-form-item label="规格" prop="specification"><el-input v-model="form.specification" /></el-form-item>
      <el-form-item label="单位" prop="unit"><el-input v-model="form.unit" /></el-form-item>
      <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio v-for="dict in sys_normal_disable" :key="dict.value" :value="dict.value">{{ dict.label }}</el-radio></el-radio-group></el-form-item>
      <el-form-item label="备注" prop="remark"><el-input v-model="form.remark" type="textarea" maxlength="500" /></el-form-item>
    </el-form><template #footer><el-button type="primary" @click="submitForm">确定</el-button><el-button @click="open = false">取消</el-button></template></el-dialog>
  </div>
</template>

<script setup name="ErpSku">
import ERPQueryBar from '../components/ERPQueryBar.vue'

let alive=true,view=0,operation=0,wasInactive=false
import {onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import { listErpSku, getErpSku, addErpSku, updateErpSku, delErpSku } from '@/api/erp/sku'
const { proxy } = getCurrentInstance()
const formRef = ref()
const busy=ref(false),error=ref(""),pending=ref(null)
const loading = ref(false)
const rows = ref([])
const total = ref(0)
const open = ref(false)
const title = ref('')
const ids = ref([])
const multiple = ref(true)
const form = ref({})
const queryParams = reactive({ pageNum: 1, pageSize: 10 })
const { sys_normal_disable } = proxy.useDict('sys_normal_disable')
const rules = reactive({
  skuCode: [{ required: true, message: "SKU编码不能为空", trigger: 'blur' }],
  productName: [{ required: true, message: "商品名称不能为空", trigger: 'blur' }],
  unit: [{ required: true, message: "单位不能为空", trigger: 'blur' }],
  status: [{ required: true, message: "状态不能为空", trigger: 'blur' }],
})
let listSeq=0,detailSeq=0
async function getList(){if(!alive)return;const seq=++listSeq;loading.value=true;try{const r=await listErpSku({...queryParams});if(alive&&seq===listSeq){rows.value=r.rows;total.value=r.total}}catch(e){if(alive&&seq===listSeq){rows.value=[];total.value=0;if(typeof error!=="undefined")error.value=e.message||"读取失败"}}finally{if(seq===listSeq)loading.value=false}}
function handleQuery() { queryParams.pageNum = 1; getList() }
function resetQuery() { proxy.resetForm('queryRef'); handleQuery() }
function handleSelectionChange(selection) { ids.value = selection.map(item => item.skuId); multiple.value = !selection.length }
function handleAdd() {if(!alive||busy.value||pending.value&&open.value)return;pending.value=null; form.value = {status:'0',unit:'件',specification:''}; title.value = '新增商品SKU'; open.value = true }
async function handleUpdate(row,readOnly=false){if(!alive||busy.value)return;const seq=++detailSeq;pending.value=null;try{const r=await getErpSku(row.skuId);if(!alive||seq!==detailSeq)return;form.value=r.data;title.value="修改商品SKU";open.value=true}catch(e){if(alive&&seq===detailSeq&&typeof error!=="undefined")error.value=e.message||"读取失败"}}
async function submitForm(){
 if(!alive||busy.value||!open.value)return
 const owner=view,op=++operation;busy.value=true
 try{let c=pending.value;if(!c){await formRef.value.validate();if(!alive||owner!==view||op!==operation||!open.value)return;c=JSON.parse(JSON.stringify({id:form.value.skuId,payload:Object.fromEntries(['skuId','skuCode','productName','specification','unit','status','remark','version'].map(k=>[k,form.value[k]]))}));pending.value=c};if(c.id!=null){await updateErpSku(c.payload)}else await addErpSku(c.payload);if(!alive||owner!==view||op!==operation)return;open.value=false;closedRequest();if(typeof ElMessage!=="undefined")ElMessage.success('已保存');else proxy.$modal.msgSuccess('操作成功');await getList()}
 catch(e){if(alive&&owner===view&&op===operation){if(typeof error!=="undefined")error.value='结果未确认，原内容已固定；请先查询确认。';else throw e}}
 finally{if(op===operation)busy.value=false}
}
async function handleDelete(row={}){if(!alive||busy.value)return;const owner=view,op=++operation,target=JSON.parse(JSON.stringify(row.skuId??ids.value));busy.value=true;try{await proxy.$modal.confirm("确认删除选中数据？被引用的数据不能删除。");if(!alive||owner!==view||op!==operation)return;await delErpSku(target);if(!alive||owner!==view||op!==operation)return;proxy.$modal.msgSuccess("删除成功");await getList()}finally{if(op===operation)busy.value=false}}
function handleExport() { proxy.download('erp/sku/export', { ...queryParams }, 'sku_' + new Date().getTime() + '.xlsx') }
getList()
function closedRequest(){if(open.value)return;++detailSeq;form.value={};pending.value=null}
function leave(){alive=false;++view;++operation;++listSeq;++detailSeq;busy.value=false;loading.value=false;open.value=false;form.value={};pending.value=null;rows.value=[];total.value=0;if(typeof channels!=="undefined")channels.value=[];if(typeof ids!=="undefined")ids.value=[]}
onBeforeUnmount(leave);onDeactivated(()=>{wasInactive=true;leave()});onActivated(()=>{if(wasInactive){wasInactive=false;alive=true;return getList()}})
</script>
