<template>
 <div class="app-container supplier-workspace">
  <el-alert title="供应商是本企业共享资料；列表不展示联系人，详情需单独授权。编码创建后固定，停用保留历史，不提供删除。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="编码"><el-input v-model="query.supplierCode" clearable maxlength="64" @keyup.enter="search"/></el-form-item>
   <el-form-item label="名称"><el-input v-model="query.supplierName" clearable maxlength="120" @keyup.enter="search"/></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:100px"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
   <el-form-item><el-button @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item>
  </el-form>
  <div class="mb8"><el-button type="primary" size="small" @click="start" :disabled="saving||detailLoading||closing" v-if="checkPermi(['erp:supplier:add'])">新增供应商</el-button></div>
  <el-alert v-if="listError" type="error" title="供应商读取失败，请重试；当前不是零条数据的结论。" :closable="false" class="mb8"/>
  <el-table :data="rows" v-loading="loading" size="small">
   <el-table-column label="编码" prop="supplierCode" min-width="130" show-overflow-tooltip/>
   <el-table-column label="供应商名称" prop="supplierName" min-width="190" show-overflow-tooltip/>
   <el-table-column label="状态" width="85"><template #default="{row}"><el-tag :type="row.status==='0'?'success':'info'">{{row.status==='0'?'启用':'停用'}}</el-tag></template></el-table-column>
   <el-table-column label="版本" prop="version" width="65"/>
   <el-table-column label="创建时间" prop="createTime" width="165"/>
   <el-table-column label="操作" width="145" fixed="right"><template #default="{row}">
    <el-button link type="primary" v-if="checkPermi(['erp:supplier:query'])" :disabled="saving||detailLoading||closing" @click="edit(row,true)">详情</el-button>
    <el-button link type="primary" v-if="canEdit" :disabled="saving||detailLoading||closing" @click="edit(row,false)">修改 / 启停</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="readonly?'供应商详情':form.supplierId?'修改供应商':'新增供应商'" class="erp-dialog" width="min(680px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" :before-close="beforeClose" @close="closed" @closed="closing=false">
   <el-alert v-if="saveError" type="warning" :title="saveError" :closable="false" class="mb8"/>
   <el-alert v-if="pending" title="结果未确认，原内容和版本已固定；编码重复或版本冲突时先查询确认。" type="warning" :closable="false"/><el-form ref="formRef" :model="form" :rules="rules" label-width="90px" size="small" :disabled="saving||readonly||!!pending">
    <el-form-item label="供应商编码" prop="supplierCode"><el-input v-model="form.supplierCode" maxlength="64" :disabled="!!form.supplierId" placeholder="企业内唯一，字母数字及._-"/></el-form-item>
    <el-form-item label="供应商名称" prop="supplierName"><el-input v-model="form.supplierName" maxlength="120"/></el-form-item>
    <el-row :gutter="16"><el-col :span="12"><el-form-item label="联系人"><el-input v-model="form.contactName" maxlength="64"/></el-form-item></el-col><el-col :span="12"><el-form-item label="联系电话"><el-input v-model="form.phone" maxlength="64"/></el-form-item></el-col></el-row>
    <el-form-item label="联系邮箱" prop="email"><el-input v-model="form.email" maxlength="128"/></el-form-item>
    <el-form-item label="联系地址"><el-input v-model="form.address" type="textarea" :rows="2" maxlength="500" show-word-limit/></el-form-item>
    <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item>
    <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" maxlength="500" show-word-limit/></el-form-item>
   </el-form>
   <p v-if="form.supplierId">版本 v{{form.version}}；修改与启停共用版本检查，冲突请关闭后重新打开。</p>
   <template #footer><el-button :disabled="saving" @click="closeDialog">{{readonly?'关闭':'取消'}}</el-button><el-button v-if="!readonly" type="primary" :loading="saving" @click="save">保存</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup name="SupplierWorkspace">
import { ref, reactive, computed, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import { ElMessage } from 'element-plus'
import { checkPermi } from '@/utils/permission'
import { listSuppliers, getSupplier, addSupplier, editSupplier } from '@/api/erp/supplier'
// Extended from the generated erp_supplier Vue3 CRUD; no delete/export routes.
const rows=ref([]),total=ref(0),loading=ref(false),listError=ref(false),detailLoading=ref(false),saving=ref(false),open=ref(false),closing=ref(false),readonly=ref(false),form=ref({}),formRef=ref(),saveError=ref('')
const query=reactive({pageNum:1,pageSize:10,supplierCode:'',supplierName:'',status:''})
const canEdit=computed(()=>checkPermi(['erp:supplier:query'])&&checkPermi(['erp:supplier:edit']))
const rules={supplierCode:[{required:true,message:'请填写供应商编码',trigger:'blur'}],supplierName:[{required:true,message:'请填写供应商名称',trigger:'blur'}],status:[{required:true,message:'请选择状态',trigger:'change'}],email:[{type:'email',message:'邮箱格式不正确',trigger:'blur'}]}
const pending=ref(null)
let listSequence=0,detailSequence=0,viewSequence=0,deactivated=false
async function load(){if(deactivated)return;
 const ticket=++listSequence;loading.value=true;listError.value=false
 try{const r=await listSuppliers({...query});if(ticket===listSequence){rows.value=r.rows;total.value=r.total}}
 catch{if(ticket===listSequence){rows.value=[];total.value=0;listError.value=true}}
 finally{if(ticket===listSequence)loading.value=false}
}
function search(){query.pageNum=1;return load()}
function reset(){Object.assign(query,{supplierCode:'',supplierName:'',status:''});return search()}
function start(){if(deactivated||saving.value||open.value)return;pending.value=null;if(saving.value||detailLoading.value||closing.value||open.value||!checkPermi(['erp:supplier:add']))return;++detailSequence;saveError.value='';form.value={supplierCode:'',supplierName:'',contactName:'',phone:'',email:'',address:'',status:'0',remark:''};readonly.value=false;open.value=true;formRef.value?.clearValidate()}
async function edit(row,readOnly){if(deactivated)return;
 if(saving.value||detailLoading.value||closing.value||open.value||!checkPermi(['erp:supplier:query'])||(!readOnly&&!canEdit.value))return
 const ticket=++detailSequence;detailLoading.value=true;saveError.value=''
 try{const r=await getSupplier(row.supplierId);if(ticket===detailSequence){form.value={...r.data};readonly.value=readOnly;open.value=true;formRef.value?.clearValidate()}}
 finally{if(ticket===detailSequence)detailLoading.value=false}
}
// Set the guard before Vue emits close: a new fetch must not race the old close event.
function closeDialog(){if(saving.value)return;closing.value=true;open.value=false;closed()}
function beforeClose(done){if(saving.value)return;closeDialog();done()}
function closed(){if(open.value)return;pending.value=null;closing.value=true;++detailSequence;detailLoading.value=false;form.value={};saveError.value='';formRef.value?.clearValidate()}
function clearPrivate(){pending.value=null;deactivated=true;saving.value=false;++viewSequence;++detailSequence;++listSequence;open.value=false;form.value={};rows.value=[];total.value=0;detailLoading.value=false;loading.value=false;saveError.value=''}
async function save(){
 if(deactivated||saving.value||readonly.value||!open.value)return
 const ticket=viewSequence;saving.value=true;saveError.value=''
 try{let c=pending.value;if(!c){try{await formRef.value.validate()}catch{return};if(ticket!==viewSequence||!open.value)return;const payload=Object.fromEntries(['supplierCode','supplierName','contactName','phone','email','address','status','remark'].map(k=>[k,form.value[k]]));if(form.value.supplierId)payload.version=form.value.version;c=JSON.parse(JSON.stringify({id:form.value.supplierId,payload}));pending.value=c};if(c.id)await editSupplier(c.id,c.payload);else await addSupplier(c.payload);if(ticket===viewSequence){closing.value=true;open.value=false;form.value={};pending.value=null;ElMessage.success('供应商已保存');await load()}}
 catch{if(ticket===viewSequence)saveError.value='结果未确认，原内容已保留；版本冲突或编码重复请先查询，不要改变原请求继续重试。'}
 finally{if(ticket===viewSequence)saving.value=false}
}
onBeforeUnmount(clearPrivate)
onDeactivated(()=>{deactivated=true;clearPrivate()})
onActivated(()=>{if(deactivated){deactivated=false;closing.value=false;if(checkPermi(['erp:supplier:list']))return load()}})
if(checkPermi(['erp:supplier:list']))load()
</script>
