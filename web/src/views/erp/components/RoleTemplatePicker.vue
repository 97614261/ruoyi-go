<template>
 <div v-if="owner" class="template-picker">
  <p>ERP角色模板只预填新角色，不分配员工或仓库、店铺范围；确认保存后才生效。</p>
  <el-select v-model="selected" placeholder="选择模板预览" :loading="loading" clearable @change="applied=false"><el-option v-for="item in templates" :key="item.templateKey" :label="item.name" :value="item.templateKey" /></el-select>
  <span v-if="error">模板加载失败，可继续手工选择权限。</span>
  <template v-if="current">
   <p>{{ current.description }}</p>
   <details><summary>将预填 {{ current.permissions.length }} 项权限（点击展开核对）</summary><ul><li v-for="p in current.permissions" :key="p.permission">{{ p.label }} · {{ p.permission }}</li></ul></details>
   <details v-if="current.unavailablePermissions.length"><summary>本企业未开通 {{ current.unavailablePermissions.length }} 项，不会选中</summary><ul><li v-for="p in current.unavailablePermissions" :key="p">{{ p }}</li></ul></details>
   <el-button size="small" :disabled="!current.menuIds.length" @click="apply">预填当前模板</el-button><span v-if="applied"> 已预填，请核对菜单权限再保存。</span>
  </template>
 </div>
</template>
<script setup>
import {ref,computed,watch,onBeforeUnmount,onDeactivated} from 'vue'
import useUserStore from '@/store/modules/user'
import {getERPRoleTemplates} from '@/api/erp/setup'
import {roleTemplateDraft} from '@/utils/erpSetup.mjs'
const props=defineProps({active:Boolean}),emit=defineEmits(['apply'])
const user=useUserStore(),owner=computed(()=>user.identity?.tenantAdmin===true&&!user.identity?.platformAdmin)
const templates=ref([]),selected=ref(''),loading=ref(false),error=ref(false),applied=ref(false)
const current=computed(()=>templates.value.find(t=>t.templateKey===selected.value))
let epoch=0
function clear(){++epoch;templates.value=[];selected.value='';loading.value=false;error.value=false;applied.value=false}
async function load(){clear();if(!props.active||!owner.value)return;const id=epoch;loading.value=true;try{const r=await getERPRoleTemplates();if(id===epoch&&props.active&&owner.value)templates.value=r.data}catch{if(id===epoch)error.value=true}finally{if(id===epoch)loading.value=false}}
function apply(){if(props.active&&owner.value&&current.value){emit('apply',roleTemplateDraft(current.value));applied.value=true}}
watch(()=>[props.active,owner.value],load,{immediate:true})
onBeforeUnmount(clear);onDeactivated(clear)
</script>
<style scoped>
.template-picker{background:var(--el-fill-color-light);padding:10px;border-radius:4px;margin-bottom:12px;font-size:12px}.template-picker p{margin:0 0 8px;line-height:1.6}.template-picker details{margin:8px 0}.template-picker ul{max-height:180px;overflow:auto;padding-left:18px;overflow-wrap:anywhere}.template-picker .el-select{width:100%}
</style>
