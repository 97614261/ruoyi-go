<template>
 <el-dialog v-model="open" :title="mode==='review'?'审核并预占库存':'取消订单'" class="erp-dialog" width="min(760px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" @close="closed">
  <el-alert :title="mode==='review'?'审核会固定SKU配对快照，按内部SKU汇总预占库存并创建一个包裹，任何一步失败都会整体回滚。':'待审核取消不动库存；已审核未发货取消会同时释放预占并取消包裹。已发货不可取消。'" type="warning" :closable="false"/>
  <el-alert v-if="pending" title="结果未确认，原请求内容与版本已固定。可用原请求重试；关闭后请先查询订单状态。" type="warning" :closable="false"/>
  <p>订单 #{{target?.orderId}} · v{{target?.version}}</p>
  <el-form label-width="95px" size="small" :disabled="busy||!!pending"><el-form-item v-if="mode==='review'" label="发货仓库"><ReferenceSelect kind="warehouse" v-model="form.warehouseId" style="width:100%"/></el-form-item><el-form-item v-else label="取消原因"><el-input v-model="form.reason" type="textarea" maxlength="500" show-word-limit placeholder="必须填写原因"/></el-form-item></el-form>
  <OrderReadinessPanel :result="checkResult"/>
  <template #footer><el-button @click="open=false" :disabled="busy">关闭</el-button><el-button v-if="mode==='review'" @click="check" :loading="checking" :disabled="busy||!!pending">检查配对 / 库存</el-button><el-button type="primary" @click="submit" :loading="saving" :disabled="checking||(mode==='review'&&checkResult&&!checkResult.ready)">{{pending?'原请求重试':mode==='review'?'审核并预占':'确认取消'}}</el-button></template>
 </el-dialog>
</template>
<script setup>
import { ref,computed,watch,onBeforeUnmount,onDeactivated,onActivated } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ReferenceSelect from './ReferenceSelect.vue'
import { reviewOrder, cancelOrder,orderReadiness } from '@/api/erp/order'
import OrderReadinessPanel from './OrderReadinessPanel.vue'
const emit=defineEmits(['completed']),open=ref(false),saving=ref(false),mode=ref('review'),target=ref(null),form=ref({})
const pending=ref(null)
const checking=ref(false),checkResult=ref(null),busy=computed(()=>saving.value||checking.value)
let checkSequence=0,lifecycle=0,active=true
const current=ticket=>active&&open.value&&!!target.value&&ticket===lifecycle
watch(()=>form.value.warehouseId,()=>{++checkSequence;checkResult.value=null;checking.value=false},{flush:'sync'})
async function check(){if(!active||busy.value||!open.value||!target.value)return;if(!form.value.warehouseId){ElMessage.warning('请选择发货仓库');return}const ticket=++checkSequence,owner=lifecycle,id=target.value.orderId,warehouseId=form.value.warehouseId;checkResult.value=null;checking.value=true;try{const r=await orderReadiness(id,{warehouseId});if(!current(owner)||ticket!==checkSequence)return;checkResult.value=r.data;if(r.data.version!==target.value.version)checkResult.value={...r.data,ready:false,issues:[{message:'订单版本已变化，请关闭后刷新订单'}]}}catch(e){if(current(owner)&&ticket===checkSequence)throw e}finally{if(owner===lifecycle&&ticket===checkSequence)checking.value=false}}
function show(row,action){if(!active||busy.value)return;++lifecycle;++checkSequence;checking.value=false;checkResult.value=null;target.value={orderId:row.orderId,version:row.version};mode.value=action;pending.value=null;form.value={warehouseId:undefined,reason:'',requestKey:crypto.randomUUID()};open.value=true}
function closed(){if(open.value)return;++lifecycle;++checkSequence;saving.value=false;checking.value=false;checkResult.value=null;target.value=null;pending.value=null;form.value={}}
function leave(){active=false;open.value=false;closed()}
onBeforeUnmount(leave);onDeactivated(leave);onActivated(()=>{active=true})
async function submit(){
 if(!active||busy.value||!open.value||!target.value)return
 const ticket=lifecycle;let command=pending.value
 if(!command){const action=mode.value,payload={version:target.value.version,requestKey:form.value.requestKey};if(action==='review'){if(checkResult.value&&!checkResult.value.ready){ElMessage.warning('请先处理审核阻断原因');return}if(!form.value.warehouseId){ElMessage.warning('请选择有权限的发货仓库');return}payload.warehouseId=form.value.warehouseId}else{if(!form.value.reason?.trim()){ElMessage.warning('请填写取消原因');return}payload.reason=form.value.reason};command=JSON.parse(JSON.stringify({action,id:target.value.orderId,payload}))}
 saving.value=true
 try{try{await ElMessageBox.confirm(command.action==='review'?'确认审核并预占库存？':'确认取消订单并释放关联预占？','确认操作',{type:'warning'})}catch{return};if(!current(ticket))return;pending.value=command;await(command.action==='review'?reviewOrder:cancelOrder)(command.id,command.payload);if(!current(ticket))return;open.value=false;closed();ElMessage.success(command.action==='review'?'已审核预占，可到包裹页维护物流':'订单已取消，关联预占已释放');emit('completed')}
 catch(e){if(current(ticket)){if(typeof e==='string'){open.value=false;closed();ElMessage.warning('请求已被拒绝，请重新读取订单')}else throw e}}
 finally{if(ticket===lifecycle)saving.value=false}
}
defineExpose({show})
</script>
