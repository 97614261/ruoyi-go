<template>
 <div class="app-container">
  <el-alert title="店铺仅登记企业经营资料，不代表平台已授权。所有者维护资料，员工只查看已授权店铺；停用保留历史，不提供删除。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="编码"><el-input v-model="query.shopCode" clearable maxlength="64" @keyup.enter="search"/></el-form-item>
   <el-form-item label="名称"><el-input v-model="query.shopName" clearable maxlength="120" @keyup.enter="search"/></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:100px"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
   <el-form-item><el-button @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item>
  </el-form>
  <div class="mb8"><el-button v-if="owner" type="primary" size="small" @click="start" v-hasPermi="['erp:shop:add']" :disabled="saving||detailLoading">新增店铺</el-button> <ShopAccess/></div>
  <el-table :data="rows" v-loading="loading" size="small">
   <el-table-column label="编码" prop="shopCode" min-width="130" show-overflow-tooltip/>
   <el-table-column label="店铺" prop="shopName" min-width="160" show-overflow-tooltip/>
   <el-table-column label="渠道" prop="channelCode" width="130"/>
   <el-table-column label="站点" prop="siteCode" width="100"/>
   <el-table-column label="币种" prop="currency" width="80"/>
   <el-table-column label="状态" width="90"><template #default="{row}"><el-tag :type="row.status==='0'?'success':'info'">{{row.status==='0'?'启用':'停用'}}</el-tag></template></el-table-column>
   <el-table-column label="版本" prop="version" width="65"/>
   <el-table-column label="操作" width="140"><template #default="{row}"><el-button link type="primary" @click="edit(row,true)" :disabled="saving||detailLoading" v-hasPermi="['erp:shop:query']">详情</el-button><el-button v-if="owner" link type="primary" @click="edit(row,false)" :disabled="saving||detailLoading" v-hasPermi="['erp:shop:query','erp:shop:edit']">修改</el-button></template></el-table-column>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="readonly?'店铺详情':form.shopId?'修改店铺':'新增店铺'" class="erp-dialog" width="min(620px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" @close="closed">
   <el-form ref="formRef" :model="form" :rules="rules" label-width="90px" size="small" :disabled="saving||readonly">
    <el-form-item label="店铺编码" prop="shopCode"><el-input v-model="form.shopCode" maxlength="64" placeholder="企业内唯一，字母数字及._-"/></el-form-item>
    <el-form-item label="店铺名称" prop="shopName"><el-input v-model="form.shopName" maxlength="120"/></el-form-item>
    <el-form-item label="渠道" prop="channelCode"><el-select v-if="channels.length" v-model="form.channelCode"><el-option v-for="c in channels" :key="c.channelCode" :value="c.channelCode" :label="c.channelName+'（未接API）'"/></el-select><el-input v-else v-model="form.channelCode" maxlength="32" placeholder="渠道目录编码，如 MANUAL"/></el-form-item>
    <el-form-item label="站点编码" prop="siteCode"><el-input v-model="form.siteCode" maxlength="32" placeholder="自定义归类，如 CN、US、GLOBAL；不校验平台站点"/></el-form-item>
    <el-form-item label="币种" prop="currency"><el-input v-model="form.currency" maxlength="3" placeholder="如 CNY、USD；不提供换汇"/></el-form-item>
    <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item>
    <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" maxlength="500" show-word-limit/></el-form-item>
   </el-form>
   <p v-if="form.shopId">版本 v{{form.version}}；已被引用后，编码、渠道、站点和币种不可变更。版本冲突请关闭后重新打开。</p>
   <template #footer><el-button @click="open=false" :disabled="saving">{{readonly?'关闭':'取消'}}</el-button><el-button v-if="!readonly" type="primary" @click="save" :loading="saving">保存</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="ErpShop">
import { ref, reactive, computed } from 'vue'
import { ElMessage } from 'element-plus'
import useUserStore from '@/store/modules/user'
import { checkPermi } from '@/utils/permission'
import { listChannels, listErpShop, getErpShop, addErpShop, updateErpShop } from '@/api/erp/shop'
import ShopAccess from '../components/ShopAccess.vue'
const user=useUserStore(),owner=computed(()=>!!user.identity?.tenantAdmin)
const rows=ref([]),total=ref(0),loading=ref(false),detailLoading=ref(false),saving=ref(false),open=ref(false),readonly=ref(false),form=ref({}),formRef=ref(),channels=ref([])
const query=reactive({pageNum:1,pageSize:10,shopCode:'',shopName:'',status:''})
const rules={shopCode:[{required:true,message:'请填写店铺编码',trigger:'blur'}],shopName:[{required:true,message:'请填写店铺名称',trigger:'blur'}],channelCode:[{required:true,message:'请选择渠道',trigger:'change'}],siteCode:[{required:true,message:'请填写站点编码',trigger:'blur'}],currency:[{required:true,message:'请填写币种',trigger:'blur'}],status:[{required:true,message:'请选择状态',trigger:'change'}]}
let listSequence=0,detailSequence=0
async function load(){const ticket=++listSequence;loading.value=true;try{const r=await listErpShop({...query});if(ticket===listSequence){rows.value=r.rows;total.value=r.total}}finally{if(ticket===listSequence)loading.value=false}}
function search(){query.pageNum=1;return load()}
function reset(){Object.assign(query,{shopCode:'',shopName:'',status:''});return search()}
function start(){if(saving.value||detailLoading.value)return;++detailSequence;form.value={shopCode:'',shopName:'',channelCode:'MANUAL',siteCode:'GLOBAL',currency:'CNY',status:'0',remark:''};readonly.value=false;open.value=true;formRef.value?.clearValidate()}
async function edit(row,readOnly){if(saving.value||detailLoading.value)return;const ticket=++detailSequence;detailLoading.value=true;try{const r=await getErpShop(row.shopId);if(ticket===detailSequence){form.value={...r.data};readonly.value=readOnly;open.value=true;formRef.value?.clearValidate()}}finally{if(ticket===detailSequence)detailLoading.value=false}}
// Invalidate at close-start, not after animation: a delayed `closed` event
// must never cancel the next dialog's detail request or leave it busy forever.
function closed(){if(open.value)return;++detailSequence;detailLoading.value=false;formRef.value?.clearValidate()}
async function save(){
 if(saving.value||readonly.value||!open.value)return
 // Include async form validation in busy state and preserve the draft on error.
 saving.value=true
 try{
  await formRef.value.validate()
  const id=form.value.shopId,payload=Object.fromEntries(['shopCode','shopName','channelCode','siteCode','currency','status','remark','version'].map(k=>[k,form.value[k]]))
  if(id)await updateErpShop(id,payload);else await addErpShop(payload)
  open.value=false;ElMessage.success('店铺已保存');await load()
 }finally{saving.value=false}
}
async function loadChannels(){if(checkPermi(['erp:channel:list'])){const r=await listChannels({pageNum:1,pageSize:100});channels.value=r.rows}}
load();loadChannels()
</script>
