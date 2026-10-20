<template>
 <el-button v-if="userStore.identity?.tenantAdmin" @click="start" v-hasPermi="['erp:warehouse:grant']">员工仓库授权</el-button>
 <el-dialog v-model="open" title="员工仓库权限" width="760px" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
  <el-alert title="员工默认无仓库。保存会替换该员工的仓库集合；空集合表示全部撤回，后端立即按新集合校验。所有者始终拥有全部仓库。" type="warning" :closable="false"/>
  <el-form class="mt12" inline size="small"><el-form-item label="员工"><el-select v-model="userId" filterable remote :remote-method="findUsers" @visible-change="v=>v&&findUsers('')" @change="chooseUser" :disabled="saving" placeholder="搜索员工用户名"><el-option v-for="u in users" :key="u.userId" :value="u.userId" :label="u.userName+' · '+u.nickName"/></el-select></el-form-item><el-form-item label="仓库"><el-input v-model="query.warehouseName" :disabled="saving" @keyup.enter="search" placeholder="搜索仓库名称"/></el-form-item><el-form-item><el-button @click="search" :disabled="saving">查询</el-button></el-form-item></el-form>
  <p>已选 {{selected.length}} 个仓库（跨页保留），授权版本 v{{version||'—'}}</p>
  <el-table :data="rows" size="small" v-loading="loading"><el-table-column label="授权" width="70"><template #default="{row}"><el-checkbox :model-value="selected.includes(row.warehouseId)" @change="v=>toggle(row.warehouseId,v)" :disabled="!version||saving||loading"/></template></el-table-column><el-table-column label="编码" prop="warehouseCode"/><el-table-column label="仓库" prop="warehouseName"/><el-table-column label="状态" width="80"><template #default="{row}">{{row.status==='0'?'启用':'停用'}}</template></el-table-column></el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <template #footer><el-button @click="open=false" :disabled="saving">取消</el-button><el-button type="primary" @click="save" :disabled="!userId||!version||loading" :loading="saving">保存授权</el-button></template>
 </el-dialog>
</template>
<script setup>
import {ref,reactive} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import useUserStore from '@/store/modules/user'
import {listUser} from '@/api/system/user'
import {listErpWarehouse} from '@/api/erp/warehouse'
import {getWarehouseAccess,setWarehouseAccess} from '@/api/erp/workflow'
const userStore=useUserStore(),open=ref(false),userId=ref(),users=ref([]),selected=ref([]),version=ref(),rows=ref([]),total=ref(0),loading=ref(false),saving=ref(false),query=reactive({pageNum:1,pageSize:10,warehouseName:''})
let userSequence=0,accessSequence=0
async function findUsers(userName){const ticket=++userSequence;const r=await listUser({userName,pageSize:100});if(ticket===userSequence)users.value=r.rows.filter(u=>String(u.userId)!==String(userStore.id))}
async function chooseUser(id){const ticket=++accessSequence;version.value=undefined;selected.value=[];const r=await getWarehouseAccess(id);if(ticket===accessSequence&&open.value&&userId.value===id){selected.value=r.data.warehouseIds;version.value=r.data.version}}
async function load(){loading.value=true;try{const r=await listErpWarehouse(query);rows.value=r.rows;total.value=r.total}finally{loading.value=false}}
function search(){query.pageNum=1;load()}
function start(){++accessSequence;userId.value=undefined;version.value=undefined;selected.value=[];open.value=true;search()}
function toggle(id,checked){selected.value=checked?[...selected.value,id]:selected.value.filter(v=>v!==id)}
async function save(){
 if(saving.value||!userId.value||!version.value||loading.value)return
 if(selected.value.length>200){ElMessage.warning('一次最多授权200个仓库');return}
 const targetUserId=userId.value,payload={warehouseIds:[...selected.value],version:version.value}
 // Include the confirmation phase in the busy window; snapshot its target and payload.
 saving.value=true
 try{
  try{await ElMessageBox.confirm(`将替换该员工的仓库范围，保存后拥有 ${payload.warehouseIds.length} 个仓库。`,'确认授权',{type:'warning'})}catch{return}
  await setWarehouseAccess(targetUserId,payload);open.value=false;ElMessage.success('仓库权限已保存')
 }finally{saving.value=false}
}
</script>
<style scoped>.mt12{margin-top:12px}</style>
