<template>
 <div class="app-container">
  <header class="logistics-head"><h2>物流设置</h2><span>企业共享档案；停用阻止新选择，已有包裹沿原快照保留。</span></header>
  <el-alert title="这里只维护手工发货使用的物流公司和配送方式，不连接承运商、不申请面单或扣运费。" type="info" :closable="false" class="mb8" />
  <el-tabs v-model="tab" @tab-change="changed"><el-tab-pane label="物流公司" name="carrier" :disabled="busy||open"/><el-tab-pane label="配送方式" name="method" :disabled="busy||open"/></el-tabs>
  <el-form inline size="small" @submit.prevent="search"><el-form-item label="编码"><el-input v-model="query.code" maxlength="64" clearable/></el-form-item><el-form-item label="名称"><el-input v-model="query.name" maxlength="120" clearable/></el-form-item><el-form-item label="状态"><el-select v-model="query.status" clearable style="width:100px"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item><el-button @click="search" :disabled="busy">查询</el-button><el-button type="primary" @click="start" v-hasPermi="['erp:logistics:add']" :disabled="busy">新增{{isMethod?'配送方式':'物流公司'}}</el-button></el-form>
  <el-table :data="rows" size="small" v-loading="loading"><el-table-column prop="code" label="编码" min-width="120"/><el-table-column prop="name" label="名称" min-width="160"/><el-table-column v-if="isMethod" prop="carrierName" label="物流公司" min-width="150"/><el-table-column label="状态" width="75"><template #default="s">{{s.row.status==='0'?'启用':'停用'}}</template></el-table-column><el-table-column prop="version" label="版本" width="70"/><el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip/><el-table-column label="操作" width="130"><template #default="s"><el-button link @click="show(s.row,false)" v-hasPermi="['erp:logistics:query']" :disabled="busy">详情</el-button><el-button link type="primary" @click="show(s.row,true)" v-hasPermi="['erp:logistics:edit']" :disabled="busy||!checkPermi(['erp:logistics:query'])">编辑</el-button></template></el-table-column></el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="(form.id?'物流档案':'新增档案')+(editable?'':' · 只读')" class="erp-dialog" width="min(620px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving" @close="closed">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/>
   <el-form label-width="100px" :disabled="!editable||saving||!!pending">
    <el-form-item label="编码" required><el-input v-model="form.code" maxlength="64" :disabled="!!form.id||!editable||saving||!!pending"/></el-form-item>
    <el-form-item label="名称" required><el-input v-model="form.name" maxlength="120"/></el-form-item>
    <el-form-item v-if="formMethod" label="物流公司" required><el-select v-model="form.carrierId" filterable remote :remote-method="loadCarriers" :loading="carrierLoading" :disabled="!!form.id||!editable||saving||!!pending" @visible-change="visible=>visible&&!form.id&&loadCarriers('')" style="width:100%"><el-option v-for="c in carriers" :key="c.id" :value="c.id" :label="c.code+' · '+c.name"/></el-select></el-form-item>
    <el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item><el-form-item label="备注"><el-input v-model="form.remark" maxlength="500"/></el-form-item>
   </el-form>
   <template #footer><el-button @click="open=false" :disabled="saving">关闭</el-button><el-button v-if="editable&&!pending" type="primary" :loading="saving" @click="submit">保存</el-button><el-button v-if="pending" type="warning" :loading="saving" @click="submit">原请求重试</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import { ref,reactive,computed,onMounted,onBeforeUnmount,onDeactivated,onActivated } from 'vue'
import { ElMessage } from 'element-plus'
import { checkPermi } from '@/utils/permission'
import { logisticsList,logisticsGet,logisticsWrite } from '@/api/erp/logistics'
import { logisticsKey,logisticsMasterPayload,logisticsCommand,logisticsRefused } from '@/utils/logistics.mjs'
const tab=ref('carrier'),rows=ref([]),total=ref(0),loading=ref(false),saving=ref(false),open=ref(false),editable=ref(false),form=ref({}),pending=ref(null),error=ref(''),formMethod=ref(false),carriers=ref([]),carrierLoading=ref(false)
const query=reactive({pageNum:1,pageSize:20,code:'',name:'',status:''}),isMethod=computed(()=>tab.value==='method'),busy=computed(()=>saving.value)
let listSequence=0,detailSequence=0,carrierSequence=0,wasDeactivated=false
function clear(){++detailSequence;++carrierSequence;open.value=false;form.value={};pending.value=null;carriers.value=[];error.value=''}
async function load(){const ticket=++listSequence;loading.value=true;try{const r=await logisticsList(isMethod.value,{...query});if(ticket===listSequence){rows.value=r.rows;total.value=r.total}}catch{if(ticket===listSequence){rows.value=[];total.value=0;clear()}}finally{if(ticket===listSequence)loading.value=false}}
function search(){query.pageNum=1;return load()}
function changed(){rows.value=[];total.value=0;Object.assign(query,{pageNum:1,code:'',name:'',status:''});return load()}
function start(){if(busy.value)return;clear();formMethod.value=isMethod.value;form.value={id:0,code:'',name:'',carrierId:undefined,status:'0',remark:'',version:0,requestKey:logisticsKey()};editable.value=true;open.value=true}
async function show(row,edit){if(busy.value)return;clear();const ticket=++detailSequence;formMethod.value=isMethod.value;try{const r=await logisticsGet(formMethod.value,row.id);if(ticket!==detailSequence)return;form.value={...r.data,requestKey:logisticsKey()};if(formMethod.value)carriers.value=[{id:r.data.carrierId,code:r.data.carrierCode,name:r.data.carrierName}];editable.value=edit;open.value=true}catch{if(ticket===detailSequence){rows.value=[];total.value=0;clear()}}}
async function loadCarriers(keyword){const ticket=++carrierSequence;carrierLoading.value=true;try{const r=await logisticsList(false,{pageSize:100,status:'0',name:keyword});if(ticket===carrierSequence)carriers.value=r.rows}catch{if(ticket===carrierSequence)carriers.value=[]}finally{if(ticket===carrierSequence)carrierLoading.value=false}}
async function submit(){if(saving.value||!open.value||!editable.value)return;let command=pending.value;try{if(!command)command=logisticsCommand(formMethod.value?'METHOD':'CARRIER',form.value.id,logisticsMasterPayload(form.value,formMethod.value))}catch(e){ElMessage.warning(e.message);return}pending.value=command;const ticket=detailSequence;saving.value=true;try{await logisticsWrite(command.kind==='METHOD',command.id,command.payload);if(ticket!==detailSequence)return;clear();ElMessage.success('物流档案已保存');await load()}catch(e){if(ticket!==detailSequence)return;if(logisticsRefused(e)){clear();rows.value=[];total.value=0;ElMessage.warning('服务端已拒绝，请重新查询后操作')}else error.value='请求结果未确认，表单已冻结。可用原请求重试，或关闭后先查询档案。'}finally{saving.value=false}}
function closed(){if(open.value)return;clear()}
function deactivate(){++listSequence;clear();rows.value=[];total.value=0}
onMounted(load);onBeforeUnmount(deactivate);onDeactivated(()=>{wasDeactivated=true;deactivate()});onActivated(()=>{if(wasDeactivated){wasDeactivated=false;load()}})
</script>
<style scoped>.logistics-head{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:10px}.logistics-head h2{font-size:18px;margin:0}.logistics-head span{font-size:12px;color:var(--el-text-color-secondary)}</style>
