<template>
  <div class="app-container">
    <el-form :model="queryParams" ref="queryRef" :inline="true">
      <el-form-item label="仓库编码" prop="warehouseCode"><el-input v-model="queryParams.warehouseCode" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="仓库名称" prop="warehouseName"><el-input v-model="queryParams.warehouseName" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="状态" prop="status"><el-select v-model="queryParams.status" clearable style="width:130px"><el-option value="0" label="正常"/><el-option value="1" label="停用"/></el-select></el-form-item>
      <el-form-item><el-button type="primary" icon="Search" @click="handleQuery">搜索</el-button><el-button icon="Refresh" @click="resetQuery">重置</el-button></el-form-item>
    </el-form>
    <el-row :gutter="10" class="mb8"><el-col :span="1.5"><el-button type="primary" plain icon="Plus" @click="handleAdd()" v-hasPermi="['erp:warehouse:add']">新增</el-button></el-col><el-col :span="1.5"><el-button type="danger" plain icon="Delete" :disabled="multiple" @click="handleDelete()" v-hasPermi="['erp:warehouse:remove']">删除</el-button></el-col><el-col :span="1.5"><el-button type="warning" plain icon="Download" @click="handleExport" v-hasPermi="['erp:warehouse:export']">导出</el-button></el-col></el-row>
    <WarehouseAccess class="mb8"/>
    <el-table v-loading="loading" :data="rows" @selection-change="handleSelectionChange"><el-table-column type="selection" width="55" align="center" />
      <el-table-column label="仓库编号" prop="warehouseId" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="仓库编码" prop="warehouseCode" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="仓库名称" prop="warehouseName" align="center" :show-overflow-tooltip="true" />
      <el-table-column label="状态" prop="status" align="center"><template #default="scope"><dict-tag :options="sys_normal_disable" :value="scope.row.status" /></template></el-table-column>
      <el-table-column label="操作" align="center" width="200"><template #default="scope"><el-button link type="primary" icon="Edit" @click="handleUpdate(scope.row)" v-hasPermi="['erp:warehouse:edit']">修改</el-button><el-button link type="primary" icon="Delete" @click="handleDelete(scope.row)" v-hasPermi="['erp:warehouse:remove']">删除</el-button></template></el-table-column>
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="queryParams.pageNum" v-model:limit="queryParams.pageSize" @pagination="getList" />
    <el-alert title="停用后不能普通入/出库或盘点；历史退货、取消释放仍按原规则检查。被引用的仓库不能删除。" type="info" :closable="false"/>
    <el-alert v-if="error" :title="error" type="warning" :closable="false"/>
    <el-dialog :title="title" v-model="open" class="erp-dialog" width="min(600px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" append-to-body @close="closedRequest"><el-alert v-if="pending" title="结果未确认，原内容和版本已固定。可用原内容重试；编码重复或版本冲突时请先查询确认。" type="warning" :closable="false"/><el-form ref="formRef" :disabled="busy||!!pending" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="仓库编码" prop="warehouseCode"><el-input v-model="form.warehouseCode" /></el-form-item>
      <el-form-item label="仓库名称" prop="warehouseName"><el-input v-model="form.warehouseName" /></el-form-item>
      <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio v-for="dict in sys_normal_disable" :key="dict.value" :value="dict.value">{{ dict.label }}</el-radio></el-radio-group></el-form-item>
      <el-form-item label="负责人"><el-input v-model="form.manager" maxlength="64" placeholder="可选；不是仓库授权"/></el-form-item>
      <el-form-item label="地址"><el-input v-model="form.address" maxlength="300" placeholder="可选"/></el-form-item>
      <el-form-item label="备注" prop="remark"><el-input v-model="form.remark" type="textarea" maxlength="500" /></el-form-item>
    </el-form><template #footer><el-button type="primary" :loading="busy" @click="submitForm">确定</el-button><el-button :disabled="busy" @click="open = false">取消</el-button></template></el-dialog>
  </div>
</template>

<script setup name="ErpWarehouse">
let alive=true,view=0,operation=0,wasInactive=false
import {onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import { listErpWarehouse, getErpWarehouse, addErpWarehouse, updateErpWarehouse, delErpWarehouse } from '@/api/erp/warehouse'
import WarehouseAccess from '../components/WarehouseAccess.vue'
const { proxy } = getCurrentInstance()
const formRef = ref()
const busy=ref(false),error=ref('')
let listSeq=0,detailSeq=0
const pending=ref(null)
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
  warehouseCode: [{ required: true, message: "仓库编码不能为空", trigger: 'blur' }],
  warehouseName: [{ required: true, message: "仓库名称不能为空", trigger: 'blur' }],
  status: [{ required: true, message: "状态不能为空", trigger: 'blur' }],
})
async function getList(){if(!alive)return;const seq=++listSeq;loading.value=true;try{const r=await listErpWarehouse({...queryParams});if(alive&&seq===listSeq){rows.value=r.rows;total.value=r.total}}catch(e){if(alive&&seq===listSeq){rows.value=[];total.value=0;if(typeof error!=="undefined")error.value=e.message||"读取失败"}}finally{if(seq===listSeq)loading.value=false}}
function handleQuery() { queryParams.pageNum = 1; getList() }
function resetQuery() { proxy.resetForm('queryRef'); handleQuery() }
function handleSelectionChange(selection) { ids.value = selection.map(item => item.warehouseId); multiple.value = !selection.length }
function handleAdd() {if(!alive||busy.value||pending.value&&open.value)return;pending.value=null;if(busy.value)return;++detailSeq;error.value='';form.value={status:'0',address:'',manager:''};title.value='新增仓库';open.value=true}
async function handleUpdate(row,readOnly=false){if(!alive||busy.value)return;const seq=++detailSeq;pending.value=null;try{const r=await getErpWarehouse(row.warehouseId);if(!alive||seq!==detailSeq)return;form.value=r.data;title.value="修改仓库 · v"+r.data.version;open.value=true}catch(e){if(alive&&seq===detailSeq&&typeof error!=="undefined")error.value=e.message||"读取失败"}}
async function submitForm(){
 if(!alive||busy.value||!open.value)return
 const owner=view,op=++operation;busy.value=true
 try{let c=pending.value;if(!c){await formRef.value.validate();if(!alive||owner!==view||op!==operation||!open.value)return;c=JSON.parse(JSON.stringify({id:form.value.warehouseId,payload:Object.fromEntries(['warehouseId','warehouseCode','warehouseName','status','version','address','manager','remark'].map(k=>[k,form.value[k]]))}));pending.value=c};if(c.id!=null){await updateErpWarehouse(c.payload)}else await addErpWarehouse(c.payload);if(!alive||owner!==view||op!==operation)return;open.value=false;closedRequest();if(typeof ElMessage!=="undefined")ElMessage.success('已保存');else proxy.$modal.msgSuccess('操作成功');await getList()}
 catch(e){if(alive&&owner===view&&op===operation){if(typeof error!=="undefined")error.value='结果未确认，原内容已固定；请先查询确认。';else throw e}}
 finally{if(op===operation)busy.value=false}
}
async function handleDelete(row={}){if(!alive||busy.value)return;const owner=view,op=++operation,target=JSON.parse(JSON.stringify(row.warehouseId??ids.value));busy.value=true;try{await proxy.$modal.confirm("确认删除选中数据？被引用的数据不能删除。");if(!alive||owner!==view||op!==operation)return;await delErpWarehouse(target);if(!alive||owner!==view||op!==operation)return;proxy.$modal.msgSuccess("删除成功");await getList()}finally{if(op===operation)busy.value=false}}
function handleExport() { proxy.download('erp/warehouse/export', { ...queryParams }, 'warehouse_' + new Date().getTime() + '.xlsx') }
getList()

function closedRequest(){if(open.value)return;++detailSeq;form.value={};pending.value=null}
function leave(){alive=false;++view;++operation;++listSeq;++detailSeq;busy.value=false;loading.value=false;open.value=false;form.value={};pending.value=null;rows.value=[];total.value=0;if(typeof channels!=="undefined")channels.value=[];if(typeof ids!=="undefined")ids.value=[]}
onBeforeUnmount(leave);onDeactivated(()=>{wasInactive=true;leave()});onActivated(()=>{if(wasInactive){wasInactive=false;alive=true;return getList()}})
</script>
