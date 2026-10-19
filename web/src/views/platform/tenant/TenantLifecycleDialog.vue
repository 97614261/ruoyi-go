<template>
  <el-dialog :model-value="modelValue" :title="lifecycleLabels[action] || '企业生命周期'" width="540px" destroy-on-close :close-on-click-modal="false" @update:model-value="emit('update:modelValue',$event)">
    <div v-loading="loading">
      <p>{{ target.tenantName }} ({{ target.tenantCode }}) · {{ tenantStatus(target.status) }}</p>
      <el-alert title="全过程保留数据，不物理删除。申请后立即暂停使用；恢复/驳回后为停用状态，须另行启用。旧会话不会恢复。" type="warning" :closable="false" />
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="action-form">
        <el-form-item label="确认企业编码" prop="confirmTenantCode"><el-input v-model="form.confirmTenantCode" :placeholder="`请输入 ${target.tenantCode || '企业编码'}`" /></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model="form.reason" type="textarea" maxlength="200" show-word-limit /></el-form-item>
      </el-form>
    </div>
    <template #footer><el-button @click="emit('update:modelValue',false)">取消</el-button><el-button type="danger" :disabled="loading || !lifecycleActions(target.status).includes(action)" :loading="saving" @click="submit">确认{{ lifecycleLabels[action] }}</el-button></template>
  </el-dialog>
</template>
<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getTenant, changeTenantLifecycle } from '@/api/platform/tenant'
import { tenantStatus, lifecycleLabels, lifecycleActions } from '@/utils/tenantOperations.mjs'
const props = defineProps({ modelValue: Boolean, tenantId: Number, action: String })
const emit = defineEmits(['update:modelValue','saved'])
const target=ref({}),form=ref({}),formRef=ref(),loading=ref(false),saving=ref(false)
const rules={confirmTenantCode:[{validator:(_r,v,cb)=>v&&v===target.value.tenantCode?cb():cb(new Error('企业编码不一致')),trigger:'blur'}],reason:[{required:true,message:'请填写原因',trigger:'blur'}]}
let request=0
watch(()=>props.modelValue,async open=>{
  const current=++request;if(!open)return
  loading.value=true;target.value={};form.value={confirmTenantCode:'',reason:''}
  try{const res=await getTenant(props.tenantId);if(current===request)target.value=res.data}
  catch(_){emit('update:modelValue',false)}finally{if(current===request)loading.value=false}
})
async function submit(){
  await formRef.value.validate()
  try{await ElMessageBox.confirm('确认执行该生命周期操作？数据保留，但目标企业旧会话将失效。','二次确认',{type:'warning'})}catch(_){return}
  saving.value=true
  try{await changeTenantLifecycle(props.tenantId,{version:target.value.version,action:props.action,...form.value});ElMessage.success('状态已变更，数据保留');emit('update:modelValue',false);emit('saved')}
  finally{saving.value=false}
}
</script>
<style scoped>.action-form{margin-top:18px}</style>
