<template>
 <el-button v-if="userStore.identity?.tenantAdmin" @click="start" v-hasPermi="['erp:shop:grant']">员工店铺授权</el-button>
 <el-dialog v-model="open" title="员工店铺权限" class="erp-dialog" width="min(760px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" @close="closed">
  <el-alert title="员工默认无店铺。保存替换全部店铺范围；空集合明确撤回。停用店铺仍可授权查询历史。所有者始终拥有全部店铺。" type="warning" :closable="false"/>
  <el-alert v-if="pending" title="结果未确认，原员工、范围及版本已固定。重试沿用原内容；关闭后先重新查询，版本冲突不能当作未保存。" type="warning" :closable="false"/><el-form inline class="mt12"><el-form-item label="员工"><el-select v-model="userId" filterable remote :remote-method="findUsers" @visible-change="v=>v&&findUsers('')" @change="chooseUser" :disabled="saving||!!pending" placeholder="搜索员工用户名"><el-option v-for="u in users" :key="u.userId" :value="u.userId" :label="u.userName+' · '+u.nickName"/></el-select></el-form-item><el-form-item label="店铺"><el-input v-model="query.shopName" :disabled="saving" @keyup.enter="search"/></el-form-item><el-form-item><el-button @click="search" :disabled="saving">查询</el-button></el-form-item></el-form>
  <p>已选 {{selected.length}} 个店铺（跨页保留，上限200），授权版本 v{{version||'—'}}</p>
  <el-table :data="rows" v-loading="loading"><el-table-column label="授权" width="70"><template #default="{row}"><el-checkbox :model-value="selected.includes(row.shopId)" @change="v=>toggle(row.shopId,v)" :disabled="!version||saving||loading||!!pending"/></template></el-table-column><el-table-column label="编码" prop="shopCode"/><el-table-column label="店铺" prop="shopName"/><el-table-column label="状态" width="80"><template #default="{row}">{{row.status==='0'?'启用':'停用'}}</template></el-table-column></el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <template #footer><el-button @click="open=false" :disabled="saving">取消</el-button><el-button type="primary" @click="save" :disabled="!userId||!version||loading" :loading="saving">{{pending?'用原内容重试':'保存授权'}}</el-button></template>
 </el-dialog>
</template>
<script setup>
import {ref,reactive,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import useUserStore from '@/store/modules/user'
import {listUser} from '@/api/system/user'
import {listErpShop,getShopAccess,setShopAccess} from '@/api/erp/shop'
const userStore=useUserStore(),open=ref(false),userId=ref(),users=ref([]),selected=ref([]),version=ref(),rows=ref([]),total=ref(0),loading=ref(false),saving=ref(false),pending=ref(null),query=reactive({pageNum:1,pageSize:10,shopName:''})
let userSequence=0,accessSequence=0,listSequence=0,view=0,active=true
async function findUsers(userName){if(!active||!open.value||pending.value)return;const ticket=++userSequence;const r=await listUser({userName,pageSize:100});if(active&&ticket===userSequence&&open.value)users.value=r.rows.filter(u=>String(u.userId)!==String(userStore.id))}
async function chooseUser(id){if(!active||!open.value||pending.value)return;const ticket=++accessSequence;version.value=undefined;selected.value=[];if(!id)return;const r=await getShopAccess(id);if(active&&ticket===accessSequence&&open.value&&userId.value===id){selected.value=[...r.data.shopIds];version.value=r.data.version}}
async function load(){if(!active||!open.value)return;const ticket=++listSequence;loading.value=true;try{const r=await listErpShop({...query});if(active&&ticket===listSequence&&open.value){rows.value=r.rows;total.value=r.total}}finally{if(ticket===listSequence)loading.value=false}}
function search(){if(saving.value)return;query.pageNum=1;return load()}
function start(){if(!active||saving.value)return;++view;++accessSequence;++userSequence;users.value=[];userId.value=undefined;version.value=undefined;selected.value=[];pending.value=null;open.value=true;return search()}
function closed(){if(open.value)return;++view;++accessSequence;++userSequence;++listSequence;loading.value=false;saving.value=false;pending.value=null;userId.value=undefined;version.value=undefined;selected.value=[];users.value=[];rows.value=[];total.value=0}
function leave(){active=false;open.value=false;closed()}
onBeforeUnmount(leave);onDeactivated(leave);onActivated(()=>{active=true})
function toggle(id,checked){if(saving.value||pending.value||loading.value||!version.value)return;if(checked&&!selected.value.includes(id)){if(selected.value.length>=200){ElMessage.warning('一次最多授权200个店铺');return}selected.value=[...selected.value,id]}else if(!checked)selected.value=selected.value.filter(v=>v!==id)}
async function save(){
 if(!active||!open.value||saving.value||loading.value||!pending.value&&(!userId.value||!version.value))return
 if(!pending.value&&selected.value.length>200){ElMessage.warning('一次最多授权200个店铺');return}
 const ticket=view,c=pending.value||JSON.parse(JSON.stringify({id:userId.value,payload:{shopIds:selected.value,version:version.value}}));saving.value=true
 try{try{await ElMessageBox.confirm(`将替换该员工的店铺范围，保存后拥有 ${c.payload.shopIds.length} 个店铺。`,'确认授权',{type:'warning'})}catch{return};if(!active||ticket!==view||!open.value)return;pending.value=c;await setShopAccess(c.id,c.payload);if(!active||ticket!==view||!open.value)return;open.value=false;closed();ElMessage.success('店铺权限已保存')}
 finally{if(ticket===view)saving.value=false}
}
</script>
<style scoped>.mt12{margin-top:12px}</style>
