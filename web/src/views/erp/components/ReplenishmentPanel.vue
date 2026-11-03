<template>
  <el-dialog :model-value="modelValue" title="采购补货建议" width="min(1100px, calc(100vw - 24px))" class="erp-dialog" destroy-on-close :show-close="!busy" :close-on-click-modal="false" :before-close="beforeClose" @update:model-value="v => !v && close()">
    <el-alert title="建议补到预警线＋1，扣除可用库存、采购草稿和已确认未收数量；确认采购不入库，实际收货才入库。只列当前仓库已配置预警且启用的商品。" type="info" :closable="false" class="mb8" />
    <ERPQueryBar :model="query" :loading="loading" :disabled="busy || !!pending" @search="search" @reset="resetQuery">
      <el-form-item label="到货仓库"><PurchasingReferenceSelect kind="warehouse" v-model="query.warehouseId" :disabled="busy || !!pending" /></el-form-item>
    </ERPQueryBar>
    <el-alert v-if="error" :title="error" type="error" :closable="false" class="mb8" />
    <p>重新查询或翻页会清空当前选择；“缺口”为0时，已有库存或采购计划已覆盖目标。采购量可人工调整，每行最多十亿。</p>
    <el-table :data="rows" v-loading="loading" row-key="skuId" @selection-change="choose">
      <el-table-column type="selection" width="42" :selectable="selectable" />
      <el-table-column prop="skuCode" label="SKU" min-width="110" />
      <el-table-column prop="productName" label="商品" min-width="130" show-overflow-tooltip />
      <el-table-column prop="unit" label="单位" width="60" />
      <el-table-column prop="available" label="可用" width="85" />
      <el-table-column prop="thresholdQuantity" label="预警线" width="85" />
      <el-table-column prop="targetQuantity" label="补货目标" width="95" />
      <el-table-column prop="draftQuantity" label="草稿待采购" width="105" />
      <el-table-column prop="incomingQuantity" label="在途未收" width="95" />
      <el-table-column prop="neededQuantity" label="缺口" width="95" />
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load" />
    <template v-if="form.lines.length">
      <h4>确认生成采购草稿 · 已选 {{ form.lines.length }} 项</h4>
      <el-form :model="form" label-width="90px" :disabled="busy || !!pending">
        <el-form-item label="供应商"><PurchasingReferenceSelect kind="supplier" v-model="form.supplierId" :disabled="busy || !!pending || !canCreate" /></el-form-item>
        <el-form-item label="币种"><el-input v-model="form.currency" maxlength="3" placeholder="例如 CNY" /></el-form-item>
        <el-table :data="form.lines">
          <el-table-column prop="skuCode" label="SKU" min-width="110" />
          <el-table-column prop="unit" label="单位" width="60" />
          <el-table-column label="本次采购量" min-width="150"><template #default="{row}"><el-input-number v-model="row.quantity" :min="1" :max="1000000000" :precision="0" :disabled="busy || !!pending" /></template></el-table-column>
          <el-table-column label="单价" min-width="150"><template #default="{row}"><el-input v-model="row.unitPrice" maxlength="21" placeholder="手填，允许0" :disabled="busy || !!pending" /></template></el-table-column>
        </el-table>
        <el-form-item label="备注" class="mt8"><el-input v-model="form.remark" maxlength="500" placeholder="最多500字，不换行" /></el-form-item>
      </el-form>
      <p>价格由你填写，不自动读取历史采购价；仓库或建议依据变化时，需重新查询核对。</p>
    </template>
    <template #footer>
      <el-button :disabled="busy" @click="close">关闭</el-button>
      <el-button v-if="canCreate" type="primary" :loading="busy" :disabled="loading || !form.lines.length" @click="submit">{{ pending ? '沿用原请求重试' : '生成采购草稿' }}</el-button>
    </template>
  </el-dialog>
</template>
<script setup name="ReplenishmentPanel">
import { ref, reactive, computed, watch, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import { ElMessage } from 'element-plus'
import { checkPermi } from '@/utils/permission'
import { listReplenishments, createReplenishment } from '@/api/erp/replenishment'
import { replenishmentPayload } from '@/utils/replenishment.mjs'
import { safeID } from '@/utils/purchasing.mjs'
import ERPQueryBar from './ERPQueryBar.vue'
import PurchasingReferenceSelect from './PurchasingReferenceSelect.vue'
const props = defineProps({ modelValue: Boolean })
const emit = defineEmits(['update:modelValue', 'created'])
const query = reactive({ warehouseId: undefined, pageNum: 1, pageSize: 10 })
const rows = ref([]), total = ref(0), loading = ref(false), busy = ref(false), pending = ref(null), error = ref('')
const form = ref({ lines: [], currency: 'CNY', remark: '' })
const canRead = computed(() => checkPermi(['erp:stock:list']) && checkPermi(['erp:purchase:list']))
const canCreate = computed(() => canRead.value && checkPermi(['erp:purchase:add']) && checkPermi(['erp:supplier:list']))
let sequence = 0, view = 0, inactive = false
function clearRows() { ++sequence; rows.value = []; total.value = 0; loading.value = false; form.value.lines = [] }
function clear() { ++view; clearRows(); busy.value = false; pending.value = null; error.value = ''; form.value = { lines: [], currency: 'CNY', remark: '' } }
watch(() => props.modelValue, () => { clear(); query.warehouseId = undefined; query.pageNum = 1 })
watch(() => query.warehouseId, () => { if (!busy.value && !pending.value) { clearRows(); error.value = ''; query.pageNum = 1 } })
async function load() {
  if (inactive || !props.modelValue || busy.value || pending.value || !canRead.value) return
  if (!safeID(query.warehouseId)) { error.value = '请先选择到货仓库'; return }
  clearRows(); const ticket = ++sequence, warehouseId = query.warehouseId
  loading.value = true; error.value = ''
  try {
    const result = await listReplenishments({ ...query })
    if (inactive || !props.modelValue || ticket !== sequence || warehouseId !== query.warehouseId) return
    rows.value = result.rows; total.value = result.total
  } catch (e) {
    if (!inactive && ticket === sequence) { rows.value = []; total.value = 0; error.value = e.message || '补货建议读取失败，请重试；并非无缺货' }
  } finally { if (ticket === sequence) loading.value = false }
}
function search() { query.pageNum = 1; return load() }
function resetQuery() { if (busy.value || pending.value) return; query.warehouseId = undefined; query.pageNum = 1; clearRows(); error.value = '' }
const selectable = row => !busy.value && !pending.value && !loading.value && row.neededQuantity > 0
function choose(selected) {
  if (inactive || busy.value || pending.value || !props.modelValue) return
  const previous = new Map(form.value.lines.map(line => [line.skuId, line]))
  form.value.lines = selected.filter(row => row.neededQuantity > 0).slice(0, 100).map(row => ({ skuId: row.skuId, skuCode: row.skuCode, unit: row.unit, basis: row.basis, quantity: previous.get(row.skuId)?.quantity ?? row.suggestedQuantity, unitPrice: previous.get(row.skuId)?.unitPrice ?? '' }))
}
async function submit() {
  if (inactive || !props.modelValue || busy.value || loading.value || !canCreate.value) return
  const ticket = view; busy.value = true; error.value = ''
  try {
    if (!pending.value) pending.value = replenishmentPayload({ ...form.value, warehouseId: query.warehouseId, requestKey: crypto.randomUUID() })
    const result = await createReplenishment(pending.value)
    if (inactive || !props.modelValue || ticket !== view) return
    ElMessage.success('采购草稿已生成，请在采购列表确认后安排收货')
    emit('created', result.data); emit('update:modelValue', false)
  } catch (e) {
    if (!inactive && ticket === view) error.value = pending.value ? `${e.message || '结果未确认'}；原内容已锁定，可沿原请求重试。关闭后先查采购列表，建议变化时重新查询。` : (e.message || '请检查输入')
  } finally { if (ticket === view) busy.value = false }
}
function close() { if (!busy.value) emit('update:modelValue', false) }
function beforeClose(done) { if (!busy.value) { close(); done() } }
function leave() { inactive = true; clear(); if (props.modelValue) emit('update:modelValue', false) }
onBeforeUnmount(leave); onDeactivated(leave); onActivated(() => { inactive = false })
</script>
