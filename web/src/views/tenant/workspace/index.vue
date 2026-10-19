<template>
  <div class="app-container" v-loading="loading">
    <h2>企业信息</h2>
    <el-descriptions :column="1" border>
      <el-descriptions-item label="企业">{{ workspace.tenantName }}（{{ workspace.tenantCode }}）</el-descriptions-item>
      <el-descriptions-item label="状态">{{ tenantStatus(workspace.status) }}</el-descriptions-item>
      <el-descriptions-item label="账号用量">{{ workspace.usage?.userCount || 0 }} / {{ workspace.policy?.maxUsers || '不限' }}（含管理员和停用账号）</el-descriptions-item>
      <el-descriptions-item label="附件容量">{{ formatBytes(workspace.usage?.storageBytes) }} / {{ workspace.policy?.maxStorageBytes ? formatBytes(workspace.policy.maxStorageBytes) : '不限' }}</el-descriptions-item>
      <el-descriptions-item label="单文件限制">{{ workspace.policy?.maxUploadBytes ? formatBytes(workspace.policy.maxUploadBytes) : '随服务器设置' }}</el-descriptions-item>
    </el-descriptions>
    <section v-if="userStore.identity?.tenantAdmin" class="cancellation">
      <h3>申请注销企业</h3>
      <el-alert title="提交后企业立即暂停使用并退出登录。平台后续归档，数据保留、不物理删除；恢复需平台审核并重新启用。" type="warning" :closable="false" />
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="action-form">
        <el-form-item label="确认企业编码" prop="confirmTenantCode"><el-input v-model="form.confirmTenantCode" :placeholder="`请输入 ${workspace.tenantCode || '企业编码'}`" /></el-form-item>
        <el-form-item label="申请原因" prop="reason"><el-input v-model="form.reason" type="textarea" maxlength="200" /></el-form-item>
        <el-form-item><el-button type="danger" :loading="saving" @click="submit">提交注销申请</el-button></el-form-item>
      </el-form>
    </section>
  </div>
</template>
<script setup>
import { ref, onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import useUserStore from '@/store/modules/user'
import { getWorkspace, requestCancellation } from '@/api/tenant/workspace'
import { tenantStatus, formatBytes } from '@/utils/tenantOperations.mjs'
const userStore=useUserStore(),router=useRouter(),workspace=ref({}),form=ref({confirmTenantCode:'',reason:''}),formRef=ref(),loading=ref(false),saving=ref(false)
const rules={confirmTenantCode:[{validator:(_r,v,cb)=>v&&v===workspace.value.tenantCode?cb():cb(new Error('企业编码不一致')),trigger:'blur'}],reason:[{required:true,message:'请填写申请原因',trigger:'blur'}]}
onMounted(async()=>{loading.value=true;try{workspace.value=(await getWorkspace()).data}finally{loading.value=false}})
async function submit(){
  await formRef.value.validate()
  try{await ElMessageBox.confirm('提交后企业所有用户将无法继续使用，直到平台处理并重新启用。确认申请？','确认申请注销',{type:'warning'})}catch(_){return}
  saving.value=true
  try{await requestCancellation({version:workspace.value.version,action:'requestCancellation',...form.value});await userStore.logOut();await router.replace('/login')}
  finally{saving.value=false}
}
</script>
<style scoped>.cancellation{margin-top:28px;max-width:680px}.action-form{margin-top:18px}</style>
