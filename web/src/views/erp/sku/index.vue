<template>
  <div class="app-container">
    <el-form :model="queryParams" ref="queryRef" :inline="true">
      <el-form-item label="SKU编码" prop="skuCode"><el-input v-model="queryParams.skuCode" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="商品名称" prop="productName"><el-input v-model="queryParams.productName" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="规格" prop="specification"><el-input v-model="queryParams.specification" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item label="单位" prop="unit"><el-input v-model="queryParams.unit" clearable @keyup.enter="handleQuery" /></el-form-item>
      <el-form-item><el-button type="primary" icon="Search" @click="handleQuery">搜索</el-button><el-button icon="Refresh" @click="resetQuery">重置</el-button></el-form-item>
    </el-form>
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
    <el-dialog :title="title" v-model="open" class="erp-dialog" width="min(600px, calc(100vw - 24px))" append-to-body><el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
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
import { listErpSku, getErpSku, addErpSku, updateErpSku, delErpSku } from '@/api/erp/sku'
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
  skuCode: [{ required: true, message: "SKU编码不能为空", trigger: 'blur' }],
  productName: [{ required: true, message: "商品名称不能为空", trigger: 'blur' }],
  unit: [{ required: true, message: "单位不能为空", trigger: 'blur' }],
  status: [{ required: true, message: "状态不能为空", trigger: 'blur' }],
})
async function getList() { loading.value = true; try { const res = await listErpSku(queryParams); rows.value = res.rows; total.value = res.total } finally { loading.value = false } }
function handleQuery() { queryParams.pageNum = 1; getList() }
function resetQuery() { proxy.resetForm('queryRef'); handleQuery() }
function handleSelectionChange(selection) { ids.value = selection.map(item => item.skuId); multiple.value = !selection.length }
function handleAdd() { form.value = {status:'0',unit:'件',specification:''}; title.value = '新增商品SKU'; open.value = true }
async function handleUpdate(row) { const res = await getErpSku(row.skuId); form.value = res.data; title.value = '修改商品SKU'; open.value = true }
async function submitForm() { await formRef.value.validate(); if (form.value.skuId != null) await updateErpSku(form.value); else await addErpSku(form.value); proxy.$modal.msgSuccess('操作成功'); open.value = false; getList() }
async function handleDelete(row = {}) { const target = row.skuId ?? ids.value; await proxy.$modal.confirm('确认删除选中数据？'); await delErpSku(target); proxy.$modal.msgSuccess('删除成功'); getList() }
function handleExport() { proxy.download('erp/sku/export', { ...queryParams }, 'sku_' + new Date().getTime() + '.xlsx') }
getList()
</script>
