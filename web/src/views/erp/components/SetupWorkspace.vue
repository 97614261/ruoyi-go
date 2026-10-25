<template>
 <div class="app-container setup-workspace">
  <header><div><h2>企业启用</h2><p>把已有配置串起来，按需准备，沿原页面保存。</p></div><el-button :disabled="!canRead()" :loading="loading" @click="refresh">重新检查</el-button></header>
  <el-alert title="此页只检查是否存在可用记录，不会创建账号、授权、入库或记账。已有记录不代表每位员工配置完整，也不保证某笔业务一定能提交。" type="info" :closable="false" />
  <el-alert v-if="!canRead()" class="mt12" title="仅获企业启用权限的当前企业所有者可以查看。" type="warning" :closable="false" />
  <div v-else-if="error" class="empty-state">检查失败，旧结果已清除。请重试或联系管理员。</div>
  <div v-else-if="!rows.length" class="empty-state">{{ loading?'正在检查…':'点击重新检查获取当前状态' }}</div>
  <template v-else>
   <p class="stamp">更新于 {{ checkedAt }}。功能未开通请联系平台；模块未安装请联系部署人员。</p>
   <section v-for="(group,index) in groups" :key="group"><h3>{{ index+1 }} · {{ group }}</h3>
    <div v-for="row in rows.filter(r=>r.group===index)" :key="row.key" class="setup-row">
     <div><strong>{{ row.label }}</strong><p>{{ row.help }}</p></div>
     <el-tag :type="row.state==='READY'?'success':row.state==='MISSING'?'warning':'info'" effect="plain">{{ stateLabels[row.state] }}</el-tag>
     <el-button :disabled="!canGo(row)" @click="go(row)">去配置 / 查看</el-button>
    </div>
   </section>
   <footer><p>首笔业务：商品/配对 → 采购确认收货 → 订单审核发货 → 显式登记应付/应收 → 按币种记账核销。</p><p>无员工的个人企业可由所有者直接操作；物流目录也可暂不配置，不把可选项误当强制门槛。</p></footer>
  </template>
 </div>
</template>
<script setup>
import {ref,onMounted,onActivated,onDeactivated,onBeforeUnmount} from 'vue'
import {useRouter} from 'vue-router'
import useUserStore from '@/store/modules/user'
import {checkPermi} from '@/utils/permission'
import {getERPSetup} from '@/api/erp/setup'
import {setupRows,setupStates} from '@/utils/erpSetup.mjs'
const router=useRouter(),user=useUserStore(),rows=ref([]),checkedAt=ref(''),loading=ref(false),error=ref(false)
const groups=['人员与授权','业务资料','第一笔业务'],stateLabels=setupStates
let epoch=0,active=true
function canRead(){return user.identity?.tenantAdmin===true&&!user.identity?.platformAdmin&&checkPermi(['erp:setup:list'])}
function clear(){rows.value=[];checkedAt.value='';loading.value=false;error.value=false}
async function refresh(){const id=++epoch;clear();if(!active||!canRead())return;loading.value=true;try{const r=await getERPSetup();if(id!==epoch||!active)return;if(!canRead()){clear();return};rows.value=setupRows(r.data);checkedAt.value=r.data.checkedAt}catch{if(id===epoch&&active){clear();error.value=true}}finally{if(id===epoch)loading.value=false}}
function canGo(row){return active&&canRead()&&['READY','MISSING'].includes(row.state)&&checkPermi([row.permission])}
function go(row){if(canGo(row))router.push(row.path)}
function leave(){active=false;++epoch;clear()}
onMounted(refresh)
onActivated(()=>{active=true;refresh()})
onDeactivated(leave)
onBeforeUnmount(leave)
</script>
<style scoped>
header{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:12px}h2{margin:0 0 5px;font-size:20px}header p,.stamp{color:var(--el-text-color-secondary);font-size:12px;margin:0}section{border:1px solid var(--el-border-color-light);border-radius:6px;margin-top:12px;padding:0 14px}h3{font-size:14px;margin:12px 0}.setup-row{display:grid;grid-template-columns:minmax(0,1fr) auto auto;gap:12px;align-items:center;padding:10px 0;border-top:1px solid var(--el-border-color-lighter)}.setup-row p,footer p{font-size:12px;color:var(--el-text-color-secondary);line-height:1.6;margin:5px 0 0;overflow-wrap:anywhere}.stamp{margin-top:12px}.empty-state{padding:24px;color:var(--el-text-color-secondary)}footer{margin-top:16px}.mt12{margin-top:12px}@media(max-width:600px){.setup-row{grid-template-columns:minmax(0,1fr) auto}.setup-row>div{grid-column:1/-1}.setup-row .el-button{justify-self:end}}
</style>
