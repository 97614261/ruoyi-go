<template>
 <div class="app-container picking-workspace">
  <header class="no-print"><div><h2>拣货与打包</h2><p>先保存运单，再扫码复核并发货；扫描枪按键盘输入＋回车使用。</p></div><el-button :disabled="busy" @click="load">刷新列表</el-button></header>
  <el-alert v-if="error" class="no-print mb12" :title="error" type="warning" :closable="false" />
  <el-alert v-if="pending" class="no-print mb12" title="请求结果未确认，原版本、内容和请求标识已冻结。只可原请求重试，或关闭后重新读取结果。" type="warning" :closable="false"><el-button :loading="saving" @click="retry">原请求重试</el-button><el-button :disabled="saving" @click="discard">关闭并刷新</el-button></el-alert>
  <div class="picking-grid">
   <section class="package-list no-print">
    <el-form inline size="small" @submit.prevent="search"><el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" :disabled="busy" style="width:190px"/></el-form-item><el-form-item label="状态"><el-select v-model="query.status" :disabled="busy" style="width:130px"><el-option v-for="(name,key) in statuses" :key="key" :label="name" :value="key"/></el-select></el-form-item><el-button size="small" :disabled="busy" @click="search">查询</el-button></el-form>
    <el-table :data="rows" size="small" v-loading="loading"><el-table-column prop="packageNo" label="包裹" min-width="160" show-overflow-tooltip/><el-table-column label="操作" width="74"><template #default="{row}"><el-button link type="primary" :disabled="busy||!can('query')" @click="openPackage(row.packageId)">拣货</el-button></template></el-table-column></el-table>
    <pagination v-show="total>0" small :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
   </section>
   <section v-if="detail" class="picking-detail">
    <div class="picking-sheet">
     <h3>拣货清单 · {{ detail.packageNo }}</h3><p>{{ detail.warehouseName }}（#{{detail.warehouseId}}） · 订单 #{{detail.orderId}} · 包裹 v{{detail.version}} · {{statuses[detail.status]}}</p>
     <p class="sheet-note">商品/单位/数量为包裹快照；扫描码和规格为当前商品配置。不混加不同单位，清单不含收件人、运单和金额。</p>
     <table class="pick-table"><thead><tr><th>SKU / 商品</th><th>扫描码</th><th>单位</th><th>应发</th><th class="no-print">本次已扫</th></tr></thead><tbody><tr v-for="item in detail.items" :key="item.skuId"><td><strong>{{item.skuCode}}</strong><small>{{item.productName}} · {{item.specification||'—'}}</small></td><td class="scan-code">{{item.scanCode}}</td><td>{{item.unit}}</td><td>{{item.quantity}}</td><td class="no-print">{{detail.status!=='READY_TO_SHIP'?'—':detail.checked?item.quantity:counts[item.skuId]||0}}</td></tr></tbody></table>
    </div>
    <div class="no-print scan-controls">
     <el-alert v-if="!detail.hasLogistics&&detail.status==='READY_TO_SHIP'" title="尚未保存物流/运单。请先到原包裹页面维护，回来重新读取并复核。" type="warning" :closable="false"/>
     <el-alert v-if="detail.blockReason" :title="detail.blockReason" type="error" :closable="false"/>
     <p><el-tag :type="detail.checked?'success':'info'">{{detail.status!=='READY_TO_SHIP'?statuses[detail.status]:detail.checked?'当前版本复核有效':'当前版本尚未有效复核'}}</el-tag><span v-if="detail.checkedAt"> 上次复核 {{detail.checkedAt}}</span></p>
     <el-form inline size="small" @submit.prevent="scan"><el-form-item label="扫码"><el-input ref="scanRef" v-model="scanCode" :disabled="busy||!detail.scannable||detail.checked||!can('check')" placeholder="条码；无条码则扫SKU编码" autocomplete="off" @keydown.enter.prevent="scan"/></el-form-item><el-form-item label="数量"><el-input-number v-model="scanQty" :min="1" :max="1000000000" :disabled="busy||detail.checked" controls-position="right" style="width:105px"/></el-form-item><el-button :disabled="busy||!detail.scannable||detail.checked||!can('check')" @click="scan">记入</el-button></el-form>
     <div class="actions"><el-button :disabled="busy||detail.checked||detail.status!=='READY_TO_SHIP'" @click="counts={}">清空本次扫描</el-button><el-button :disabled="busy||!detail.scannable||detail.checked||!can('check')" type="primary" @click="confirmCheck">保存复核</el-button><el-button :disabled="busy||!detail.checked||!detail.hasLogistics||!can('ship')" type="success" @click="confirmShip">复核后发货</el-button><el-button :disabled="busy||!can('query')" @click="printSheet">打印清单</el-button><el-button :disabled="busy||!canPackage()" @click="goPackage">维护运单 / 包裹</el-button></div>
     <p class="sheet-note">复核不扣库存；发货由原库存事务扣减一次。包裹或商品扫描信息变化会使复核失效。原手工发货为独立权限能力，不代表经过扫码复核。</p>
    </div>
   </section>
   <div v-else class="empty no-print">{{opening?'正在读取包裹…':'选择当前范围的包裹，开始拣货和复核'}}</div>
  </div>
 </div>
</template>
<script setup>
import {ref,reactive,computed,onMounted,onBeforeUnmount,onDeactivated,onActivated,watch,nextTick} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {ElMessage,ElMessageBox} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import ReferenceSelect from './ReferenceSelect.vue'
import {listPicking,getPicking,checkPicking,shipPicking} from '@/api/erp/picking'
import {pickingID,recordPickingScan,pickingCommand} from '@/utils/picking.mjs'
const router=useRouter(),route=useRoute(),rows=ref([]),total=ref(0),loading=ref(false),opening=ref(false),saving=ref(false),detail=ref(null),counts=ref({}),scanCode=ref(''),scanQty=ref(1),scanRef=ref(),pending=ref(null),error=ref('')
const query=reactive({pageNum:1,pageSize:10,warehouseId:undefined,status:'READY_TO_SHIP'}),statuses={READY_TO_SHIP:'待发货',SHIPPED:'已发货',CANCELLED:'已取消'},busy=computed(()=>loading.value||opening.value||saving.value||!!pending.value)
let epoch=0,listEpoch=0,linkEpoch=0,active=true,wasDeactivated=false
const can=action=>checkPermi(['erp:picking:'+action])
function clearDetail(){++epoch;detail.value=null;counts.value={};scanCode.value='';scanQty.value=1;pending.value=null;opening.value=false}
async function load(){const id=++listEpoch;loading.value=false;if(!active||!can('list')){rows.value=[];total.value=0;clearDetail();return};loading.value=true;try{const r=await listPicking({...query});if(id===listEpoch&&active){rows.value=r.rows;total.value=r.total}}catch{if(id===listEpoch&&active){rows.value=[];total.value=0;clearDetail();error.value='读取失败，旧数据已清除，请刷新'}}finally{if(id===listEpoch)loading.value=false}}
function search(){if(busy.value)return;clearDetail();error.value='';query.pageNum=1;return load()}
async function openPackage(id,{preserveScans=false}={}){
 if(saving.value||pending.value)return
 // Printing must reread authority, but an identical fresh sheet does not
 // invalidate the operator's in-memory work. Any change clears that work.
 const previous=preserveScans&&detail.value?{sheet:JSON.stringify(detail.value),counts:{...counts.value},code:scanCode.value,quantity:scanQty.value}:null
 ++linkEpoch;clearDetail();if(!active||!can('query'))return
 const ticket=epoch;opening.value=true;error.value=''
 try{
  const r=await getPicking(id);if(ticket!==epoch||!active)return
  detail.value=r.data
  if(previous){
   if(previous.sheet===JSON.stringify(r.data)){counts.value=previous.counts;scanCode.value=previous.code;scanQty.value=previous.quantity}
   else if(Object.keys(previous.counts).length||previous.code)ElMessage.warning('清单已变化，本次扫描已清空，请按最新清单核对')
  }
  await nextTick();if(ticket!==epoch||!active)return
  scanRef.value?.focus();return ticket
 }catch{if(ticket===epoch&&active){clearDetail();rows.value=[];total.value=0;error.value='包裹不可读取，请核对权限、范围或状态'}}
 finally{if(ticket===epoch)opening.value=false}
}
function scan(){if(busy.value||!can('check'))return;try{counts.value=recordPickingScan(detail.value,counts.value,scanCode.value,scanQty.value);scanCode.value='';scanQty.value=1;scanRef.value?.focus()}catch(e){ElMessage.warning(e.message)}}
async function perform(command){const ticket=epoch;saving.value=true;pending.value=command;error.value='';try{await(command.kind==='CHECK'?checkPicking(command.id,command.payload):shipPicking(command.id,command.payload));if(ticket!==epoch||!active)return;pending.value=null;saving.value=false;ElMessage.success(command.kind==='CHECK'?'复核已记录，库存未变化':'本地发货已完成');await openPackage(command.id);await load()}catch(e){if(ticket!==epoch||!active)return;if(typeof e==='string'){saving.value=false;clearDetail();rows.value=[];total.value=0;error.value='请求已拒绝，请重新读取包裹后核对'}else error.value='结果未确认，请用原请求重试或关闭后刷新'}finally{if(ticket===epoch)saving.value=false}}
async function confirmCheck(){if(busy.value||!can('check'))return;try{const command=pickingCommand('CHECK',detail.value,counts.value);return perform(command)}catch(e){ElMessage.warning(e.message)}}
async function confirmShip(){if(busy.value||!can('ship'))return;let command;try{command=pickingCommand('SHIP',detail.value)}catch(e){ElMessage.warning(e.message);return};const ticket=epoch;saving.value=true;try{await ElMessageBox.confirm('确认该复核包裹交付出库？将原子扣减实物与预占，只登记本地发货。','复核后发货',{type:'warning'})}catch{if(ticket===epoch)saving.value=false;return};if(ticket===epoch)saving.value=false;if(ticket===epoch&&active)return perform(command)}
function retry(){if(!saving.value&&pending.value)return perform(pending.value)}
async function discard(){if(saving.value)return;clearDetail();error.value='';await load()}
const canPackage=()=>checkPermi(['erp:package:list'])&&checkPermi(['erp:package:query'])
function goPackage(){if(!busy.value&&detail.value&&canPackage())router.push({path:'/erp/package',query:{packageId:String(detail.value.packageId)}})}
async function printSheet(){
 if(busy.value||!detail.value||!can('query'))return
 const id=detail.value.packageId,ticket=await openPackage(id,{preserveScans:true})
 if(ticket===undefined||ticket!==epoch||!active||detail.value?.packageId!==id)return
 await nextTick()
 if(ticket===epoch&&active&&can('query')&&detail.value?.packageId===id)window.print()
}
async function followLink(){if(saving.value)return;const link=++linkEpoch;clearDetail();const raw=route.query.packageId;if(raw!==undefined&&!pickingID(raw)){++listEpoch;rows.value=[];total.value=0;error.value='包裹链接编号无效';return};await load();if(raw&&active&&link===linkEpoch)await openPackage(pickingID(raw))}
function leave(){active=false;++listEpoch;++linkEpoch;saving.value=false;clearDetail();rows.value=[];total.value=0;error.value=''}
watch(()=>route.query.packageId,followLink);onMounted(followLink);onDeactivated(()=>{wasDeactivated=true;leave()});onBeforeUnmount(leave);onActivated(()=>{if(wasDeactivated){wasDeactivated=false;active=true;followLink()}})
</script>
<style scoped>
header{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:12px}h2{font-size:20px;margin:0 0 6px}header p,.sheet-note,.scan-controls>p{font-size:12px;color:var(--el-text-color-secondary);line-height:1.65;overflow-wrap:anywhere}.picking-grid{display:grid;grid-template-columns:330px minmax(0,1fr);gap:16px}.package-list,.picking-detail{min-width:0;border:1px solid var(--el-border-color-light);border-radius:6px;padding:12px}.package-list :deep(.pagination-container){padding:8px 0;height:auto}.picking-sheet h3{margin:0 0 10px;font-size:16px;overflow-wrap:anywhere}.pick-table{width:100%;border-collapse:collapse;font-size:12px;table-layout:fixed}.pick-table th,.pick-table td{padding:8px 6px;border-bottom:1px solid var(--el-border-color-lighter);text-align:left;overflow-wrap:anywhere}.pick-table th:first-child{width:36%}.pick-table td small{display:block;margin-top:4px;color:var(--el-text-color-secondary)}.scan-code{font-family:monospace}.scan-controls{margin-top:12px}.scan-controls .el-form{margin-top:12px}.actions{display:flex;gap:8px;flex-wrap:wrap}.actions .el-button{margin-left:0}.empty{padding:28px;color:var(--el-text-color-secondary)}.mb12{margin-bottom:12px}@media(max-width:1000px){.picking-grid{grid-template-columns:minmax(0,1fr)}}
@media print{.no-print{display:none!important}.picking-grid{display:block}.picking-detail{border:0;padding:0}.pick-table{font-size:11px}.pick-table tr{break-inside:avoid} :global(.sidebar-container),:global(.navbar),:global(.tags-view-container){display:none!important} :global(.main-container){margin-left:0!important} :global(body),:global(.app-wrapper),:global(.app-main),:global(.el-scrollbar),:global(.el-scrollbar__wrap){height:auto!important;overflow:visible!important;min-height:0!important}.picking-workspace{padding:0}}
</style>
