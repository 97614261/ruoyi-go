<template>
  <el-dialog :model-value="modelValue" title="企业基础配额" width="580px" destroy-on-close :close-on-click-modal="false" @update:model-value="emit('update:modelValue', $event)">
    <div v-loading="loading">
      <p>{{ settings.tenantName }} ({{ settings.tenantCode }})</p>
      <el-alert title="0 表示不限。单文件仍受服务器总上限约束；保存将使该企业旧会话失效。" type="info" :closable="false" />
      <p class="usage">当前 {{ settings.usage?.userCount || 0 }} 个账号（含管理员和停用账号），已登记附件 {{ formatBytes(settings.usage?.storageBytes) }}</p>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="130px">
        <el-form-item label="账号数上限"><el-input-number v-model="form.maxUsers" :min="0" :max="100000" :precision="0" /></el-form-item>
        <el-form-item label="附件容量 MiB"><el-input-number v-model="form.storageMiB" :min="0" :max="1048576" /></el-form-item>
        <el-form-item label="单文件 MiB"><el-input-number v-model="form.uploadMiB" :min="0" :max="1024" /></el-form-item>
        <el-form-item label="确认企业编码" prop="confirmTenantCode"><el-input v-model="form.confirmTenantCode" :placeholder="`请输入 ${settings.tenantCode || '企业编码'}`" /></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model="form.reason" type="textarea" maxlength="200" show-word-limit /></el-form-item>
      </el-form>
    </div>
    <template #footer><el-button @click="emit('update:modelValue', false)">取消</el-button><el-button type="primary" :disabled="loading || !['0','1'].includes(settings.status)" :loading="saving" @click="submit">保存配额</el-button></template>
  </el-dialog>
</template>
<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getTenantQuota, saveTenantQuota } from '@/api/platform/tenant'
import { formatBytes, quotaForm, quotaPayload } from '@/utils/tenantOperations.mjs'
const props = defineProps({ modelValue: Boolean, tenantId: Number })
const emit = defineEmits(['update:modelValue', 'saved'])
const settings = ref({}), form = ref({}), formRef = ref(), loading = ref(false), saving = ref(false)
const rules = { confirmTenantCode: [{ validator: (_r,v,cb) => v && v === settings.value.tenantCode ? cb() : cb(new Error('企业编码不一致')), trigger: 'blur' }], reason: [{ required: true, message: '请填写原因', trigger: 'blur' }] }
let request = 0
watch(() => props.modelValue, async open => {
  const current = ++request
  if (!open) return
  loading.value = true; settings.value = {}
  try { const res = await getTenantQuota(props.tenantId); if (current === request) { settings.value = res.data; form.value = quotaForm(res.data.policy) } }
  catch (_) { emit('update:modelValue', false) }
  finally { if (current === request) loading.value = false }
})
async function submit() {
  await formRef.value.validate()
  try { await ElMessageBox.confirm('确认调整配额并使目标企业全部旧会话失效？', '确认配额', { type: 'warning' }) } catch (_) { return }
  saving.value = true
  try { await saveTenantQuota(props.tenantId, quotaPayload(settings.value, form.value)); ElMessage.success('配额已更新'); emit('update:modelValue', false); emit('saved') }
  finally { saving.value = false }
}
</script>
<style scoped>.usage { font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.6; }</style>
