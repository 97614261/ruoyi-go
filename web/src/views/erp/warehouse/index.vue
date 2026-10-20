<template>
  <div class="app-container">
    <el-form :model="queryParams" ref="queryRef" :inline="true">
      <el-form-item label="仓库编码" prop="warehouseCode"><el-input v-model="queryParams.warehouseCode" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="仓库名称" prop="warehouseName"><el-input v-model="queryParams.warehouseName" clearable @keyup.enter="handleQuery" /></el-form-item>
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
    <el-dialog :title="title" v-model="open" width="600px" append-to-body><el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="仓库编码" prop="warehouseCode"><el-input v-model="form.warehouseCode" /></el-form-item>
      <el-form-item label="仓库名称" prop="warehouseName"><el-input v-model="form.warehouseName" /></el-form-item>
      <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio v-for="dict in sys_normal_disable" :key="dict.value" :value="dict.value">{{ dict.label }}</el-radio></el-radio-group></el-form-item>
      <el-form-item label="备注" prop="remark"><el-input v-model="form.remark" type="textarea" maxlength="500" /></el-form-item>
    </el-form><template #footer><el-button type="primary" @click="submitForm">确定</el-button><el-button @click="open = false">取消</el-button></template></el-dialog>
  </div>
</template>

<script setup name="ErpWarehouse">
import { listErpWarehouse, getErpWarehouse, addErpWarehouse, updateErpWarehouse, delErpWarehouse } from '@/api/erp/warehouse'
import WarehouseAccess from '../components/WarehouseAccess.vue'
const { proxy } = getCurrentInstance()
const formRef = ref()
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
async function getList() { loading.value = true; try { const res = await listErpWarehouse(queryParams); rows.value = res.rows; total.value = res.total } finally { loading.value = false } }
function handleQuery() { queryParams.pageNum = 1; getList() }
function resetQuery() { proxy.resetForm('queryRef'); handleQuery() }
function handleSelectionChange(selection) { ids.value = selection.map(item => item.warehouseId); multiple.value = !selection.length }
function handleAdd() { form.value = {status:'0'}; title.value = '新增仓库'; open.value = true }
async function handleUpdate(row) { const res = await getErpWarehouse(row.warehouseId); form.value = res.data; title.value = '修改仓库'; open.value = true }
async function submitForm() { await formRef.value.validate(); if (form.value.warehouseId != null) await updateErpWarehouse(form.value); else await addErpWarehouse(form.value); proxy.$modal.msgSuccess('操作成功'); open.value = false; getList() }
async function handleDelete(row = {}) { const target = row.warehouseId ?? ids.value; await proxy.$modal.confirm('确认删除选中数据？'); await delErpWarehouse(target); proxy.$modal.msgSuccess('删除成功'); getList() }
function handleExport() { proxy.download('erp/warehouse/export', { ...queryParams }, 'warehouse_' + new Date().getTime() + '.xlsx') }
getList()
</script>
