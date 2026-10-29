<template>
 <el-dialog v-model="open" :title="titles[mode]" class="erp-dialog" width="min(960px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" @close="closed">
  <el-alert title="最多选择20条，逐单提交、逐单显示结果，不是整批事务。未确认项可能已提交；先刷新核对，再用原请求键重试。成功项不会再次发送。离开页面会停止后续单据，已发出的当前请求仍须回来核对。" type="warning" :closable="false"/>
  <el-form inline class="mt8" :disabled="busy||frozen"><el-form-item v-if="mode==='review'" label="统一发货仓库"><ReferenceSelect kind="warehouse" v-model="form.warehouseId" style="width:230px"/></el-form-item><el-form-item v-if="mode==='cancel'" label="统一取消原因"><el-input v-model="form.reason" maxlength="500" style="width:340px"/></el-form-item></el-form>
  <p v-if="frozen" class="batch-hint">本批参数与各行版本已固定；重试沿用原请求键。需要改仓库/原因/版本，请关闭后刷新订单，重新选择。</p>
  <el-table :data="results" max-height="340"><el-table-column prop="label" label="订单 / 包裹" min-width="180" show-overflow-tooltip/><el-table-column prop="version" label="原版本" width="75"/><el-table-column label="结果" width="110"><template #default="{row}">{{labels[row.state]}}</template></el-table-column><el-table-column label="检查 / 说明" min-width="270"><template #default="{row}">{{row.note||checkLabel(row)}}</template></el-table-column></el-table>
  <p aria-live="polite">共 {{results.length}} 条 · 已确认成功 {{count('DONE')}} · 未确认 {{count('UNCONFIRMED')}} · 跳过 {{count('SKIPPED')}}</p>
  <template #footer><el-button @click="open=false" :disabled="busy">关闭</el-button><el-button v-if="mode==='review'&&!frozen" @click="checkAll" :loading="checking" :disabled="busy">先检查配对 / 库存</el-button><el-button v-if="!frozen" type="primary" @click="run(false)" :loading="saving" :disabled="busy">确认逐单执行</el-button><el-button v-if="frozen&&count('UNCONFIRMED')" type="warning" @click="run(true)" :loading="saving" :disabled="busy">用原请求重试未确认项</el-button></template>
 </el-dialog>
</template>
<script setup>
import { ref,computed,watch,onBeforeUnmount,onDeactivated,onActivated } from 'vue'
import { ElMessage,ElMessageBox } from 'element-plus'
import ReferenceSelect from './ReferenceSelect.vue'
import { reviewOrder,cancelOrder,shipPackage,orderReadiness } from '@/api/erp/order'
const emit=defineEmits(['completed']),open=ref(false),saving=ref(false),checking=ref(false),frozen=ref(false),mode=ref('review'),form=ref({}),results=ref([])
const busy=computed(()=>saving.value||checking.value),titles={review:'批量审核预占',cancel:'批量取消订单',ship:'批量确认本地发货'},labels={PENDING:'待执行',RUNNING:'提交中',DONE:'已确认成功',UNCONFIRMED:'未确认',SKIPPED:'已跳过'}
let generation=0,lifecycle=0,active=true
const current=ticket=>active&&open.value&&ticket===lifecycle
function show(rows,action){if(!active||busy.value)return;if(!Array.isArray(rows)||rows.length<1||rows.length>20){ElMessage.warning('请选择1—20条当前页记录');return}++generation;++lifecycle;mode.value=action;frozen.value=false;form.value={warehouseId:undefined,reason:''};const seen=new Set();results.value=rows.filter(row=>{const id=action==='ship'?row.packageId:row.orderId;if(seen.has(id))return false;seen.add(id);return true}).map(row=>({id:action==='ship'?row.packageId:row.orderId,orderId:row.orderId,version:row.version,status:row.status,label:row.sourceOrderNo||row.packageNo||`#${row.orderId}`,requestKey:crypto.randomUUID(),state:'PENDING',note:'',check:null,payload:null}));open.value=true}
function closed(){if(open.value)return;++generation;++lifecycle;saving.value=false;checking.value=false;results.value=[];form.value={};frozen.value=false}
function leave(){active=false;open.value=false;closed()}
onBeforeUnmount(leave);onDeactivated(leave);onActivated(()=>{active=true})
function count(state){return results.value.filter(row=>row.state===state).length}
function checkLabel(row){if(!row.check)return '尚未检查 / 后端提交时重新校验';return row.check.ready?'当前可审核，未预占':row.check.issues?.map(i=>i.message).join('；')||'当前不能审核'}
watch(()=>form.value.warehouseId,()=>{if(!frozen.value){++generation;results.value.forEach(row=>{row.check=null;row.note=''})}},{flush:'sync'})
async function checkAll(){if(!active||busy.value||frozen.value||!open.value)return;if(!form.value.warehouseId){ElMessage.warning('请选择发货仓库');return}const ticket=generation,owner=lifecycle,warehouseId=form.value.warehouseId;checking.value=true;try{for(const row of results.value){if(!current(owner)||ticket!==generation)return;if(row.status!=='PENDING_REVIEW'){row.note='非待审核记录，将跳过';continue}try{const r=await orderReadiness(row.id,{warehouseId});if(!current(owner)||ticket!==generation)return;row.check=r.data;if(r.data.version!==row.version){row.check={...r.data,ready:false,issues:[{message:'订单版本已变化，请关闭并刷新后重新选择'}]}}}catch{if(!current(owner)||ticket!==generation)return;row.note='检查失败：请核对权限或连接后重新检查'}}}finally{if(owner===lifecycle)checking.value=false}}
async function run(retry=false){if(!active||busy.value||!open.value)return;if(retry&&!frozen.value)return;if(!retry&&frozen.value)return;const action=mode.value;if(!retry&&action==='review'&&!form.value.warehouseId){ElMessage.warning('请选择发货仓库');return}if(!retry&&action==='cancel'&&!form.value.reason?.trim()){ElMessage.warning('请填写取消原因');return}
 const parameters={warehouseId:form.value.warehouseId,reason:form.value.reason},batch=results.value.filter(row=>retry?row.state==='UNCONFIRMED':row.state==='PENDING'),ticket=lifecycle;if(!batch.length)return;saving.value=true
 try{try{await ElMessageBox.confirm(`确认逐单执行${batch.length}条？成功项不回滚，请逐条查看结果。`,titles[action],{type:'warning'})}catch{return}if(!current(ticket))return;frozen.value=true
  for(const row of batch){if(!current(ticket))return;const eligible=action==='review'?row.status==='PENDING_REVIEW':action==='cancel'?['PENDING_REVIEW','READY_TO_SHIP'].includes(row.status):row.status==='READY_TO_SHIP';if(!eligible){row.state='SKIPPED';row.note='当前记录状态不适用此动作';continue}if(!retry&&action==='review'&&row.check&&!row.check.ready){row.state='SKIPPED';row.note=checkLabel(row);continue}
   if(!row.payload){row.payload={version:row.version,requestKey:row.requestKey};if(action==='review')row.payload.warehouseId=parameters.warehouseId;if(action==='cancel')row.payload.reason=parameters.reason}row.state='RUNNING';row.note=''
   try{const reply=await (action==='review'?reviewOrder:action==='cancel'?cancelOrder:shipPackage)(row.id,{...row.payload});if(!current(ticket))return;row.state='DONE';row.note=reply?.data?.existing?'原请求已成功，本次未重复记账':'后端已确认成功'}catch{if(!current(ticket))return;row.state='UNCONFIRMED';row.note='未确认成功：请核对权限、版本、库存或连接，再用原请求重试'}
  }
  if(current(ticket))emit('completed')
 }finally{if(ticket===lifecycle)saving.value=false}
}
defineExpose({show})
</script>
<style scoped>
.batch-hint{font-size:12px;color:var(--el-text-color-secondary)}
</style>
