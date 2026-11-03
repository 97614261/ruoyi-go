<template>
  <el-form class="asset-search" inline @submit.prevent="search">
    <el-form-item label="名称前缀"><el-input v-model="form.name" maxlength="150" clearable @keyup.enter="search"/></el-form-item>
    <el-form-item label="分组"><el-select v-model="form.groupMode" style="width:125px"><el-option value="all" label="全部分组"/><el-option value="none" label="未分组"/><el-option value="named" label="指定分组"/></el-select></el-form-item>
    <el-form-item v-if="form.groupMode==='named'" label="分组名"><el-input v-model="form.groupName" maxlength="40" placeholder="填写完整分组名" clearable @keyup.enter="search"/></el-form-item>
    <el-form-item label="存储格式"><el-select v-model="form.mimeType" clearable style="width:110px"><el-option value="image/png" label="PNG"/><el-option value="image/jpeg" label="JPEG"/></el-select></el-form-item>
    <el-form-item label="宽度(px)"><el-input-number v-model="form.minWidth" :min="0" :max="12000000" :precision="0" :controls="false" aria-label="最小宽度"/><span class="range">至</span><el-input-number v-model="form.maxWidth" :min="0" :max="12000000" :precision="0" :controls="false" aria-label="最大宽度"/></el-form-item>
    <el-form-item label="大小(KB)"><el-input-number v-model="form.minKB" :min="0" :max="8192" :precision="0" :controls="false" aria-label="最小大小KB"/><span class="range">至</span><el-input-number v-model="form.maxKB" :min="0" :max="8192" :precision="0" :controls="false" aria-label="最大大小KB"/></el-form-item>
    <el-form-item><el-button type="primary" icon="Search" @click="search">搜索</el-button><el-button icon="Refresh" @click="reset">重置</el-button></el-form-item>
  </el-form>
</template>
<script setup>
import { reactive } from 'vue'
import { ElMessage } from 'element-plus'
const emit=defineEmits(['search'])
const initial=()=>({name:'',groupMode:'all',groupName:'',mimeType:'',minWidth:0,maxWidth:0,minKB:0,maxKB:0})
const form=reactive(initial())
function search(){
 if(form.groupMode==='named'&&!form.groupName.trim()){ElMessage.warning('请填写分组名');return}
 const q={name:form.name.trim(),mimeType:form.mimeType||'',minWidth:form.minWidth||0,maxWidth:form.maxWidth||0,minBytes:(form.minKB||0)*1024,maxBytes:(form.maxKB||0)*1024}
 if(q.maxWidth&&q.minWidth>q.maxWidth||q.maxBytes&&q.minBytes>q.maxBytes){ElMessage.warning('筛选下限不能大于上限');return}
 if(form.groupMode==='none')q.ungrouped=true;else if(form.groupMode==='named')q.groupName=form.groupName.trim()
 emit('search',q)
}
function reset(){Object.assign(form,initial());search()}
</script>
<style scoped>
.asset-search :deep(.el-form-item){margin-right:14px;margin-bottom:10px}.asset-search :deep(.el-input){width:155px}.asset-search :deep(.el-input-number){width:85px}.asset-search :deep(.el-input-number .el-input){width:100%}.range{margin:0 6px;color:var(--el-text-color-secondary)}
</style>
