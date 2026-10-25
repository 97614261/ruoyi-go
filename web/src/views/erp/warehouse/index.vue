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
    <el-dialog :title="title" v-model="open" class="erp-dialog" width="min(600px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" append-to-body><el-form ref="formRef" :model="form" :rules="rules" :disabled="busy" label-width="100px">
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
import { listErpWarehouse, getErpWarehouse, addErpWarehouse, updateErpWarehouse, delErpWarehouse } from '@/api/erp/warehouse'
import WarehouseAccess from '../components/WarehouseAccess.vue'
const { proxy } = getCurrentInstance()
const formRef = ref()
const busy=ref(false),error=ref('')
let listSeq=0,detailSeq=0
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
async function getList() { const seq=++listSeq;loading.value=true;rows.value=[];total.value=0;try {const res=await listErpWarehouse({...queryParams});if(seq===listSeq){rows.value=res.rows;total.value=res.total}}catch(e){if(seq===listSeq)error.value=e.message||'读取失败'}finally{if(seq===listSeq)loading.value=false} }
function handleQuery() { queryParams.pageNum = 1; getList() }
function resetQuery() { proxy.resetForm('queryRef'); handleQuery() }
function handleSelectionChange(selection) { ids.value = selection.map(item => item.warehouseId); multiple.value = !selection.length }
function handleAdd() {if(busy.value)return;++detailSeq;error.value='';form.value={status:'0',address:'',manager:''};title.value='新增仓库';open.value=true}
async function handleUpdate(row) {if(busy.value)return;const seq=++detailSeq;open.value=false;error.value='';try{const res=await getErpWarehouse(row.warehouseId);if(seq!==detailSeq)return;form.value=res.data;title.value='修改仓库 · v'+res.data.version;open.value=true}catch(e){error.value=e.message||'读取失败'}}
async function submitForm() {if(busy.value)return;busy.value=true;try{await formRef.value.validate();const f=form.value;const body={warehouseId:f.warehouseId,warehouseCode:f.warehouseCode,warehouseName:f.warehouseName,status:f.status,version:f.version,address:f.address||'',manager:f.manager||'',remark:f.remark||''};if(body.warehouseId!=null)await updateErpWarehouse(body);else await addErpWarehouse(body);proxy.$modal.msgSuccess('操作成功');open.value=false;await getList()}catch(e){error.value=e.message||'未确认保存，请重读核对后再修改'}finally{busy.value=false}}
async function handleDelete(row = {}) {if(busy.value)return;busy.value=true;const target=row.warehouseId??[...ids.value];try{await proxy.$modal.confirm('确认删除选中数据？被引用的仓库不能删除。');await delErpWarehouse(target);proxy.$modal.msgSuccess('删除成功');await getList()}finally{busy.value=false}}
function handleExport() { proxy.download('erp/warehouse/export', { ...queryParams }, 'warehouse_' + new Date().getTime() + '.xlsx') }
getList()
onBeforeUnmount(()=>{++listSeq;++detailSeq})
</script>
