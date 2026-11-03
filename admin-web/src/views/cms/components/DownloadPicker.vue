<template>
 <el-button @click="open">{{label||'选择资料分类'}}</el-button>
 <el-dialog v-model="visible" title="选择资料分类" width="600px" append-to-body destroy-on-close>
  <el-form inline @submit.prevent="search"><el-form-item label="名称"><el-input v-model="q.name" clearable maxlength="60"/></el-form-item><el-button @click="search">查询</el-button><el-button v-if="allowClear" @click="choose({categoryId:0,name:'全部分类'})">全部分类</el-button></el-form>
  <el-table :data="rows" v-loading="loading"><el-table-column prop="name" label="分类"/><el-table-column label="状态" width="90"><template #default="s">{{s.row.enabled?'启用':'停用'}}</template></el-table-column><el-table-column width="100"><template #default="s"><el-button link type="primary" :disabled="!s.row.enabled&&!allowDisabled" @click="choose(s.row)">选择</el-button></template></el-table-column></el-table>
  <pagination v-if="total" :total="total" v-model:page="q.pageNum" v-model:limit="q.pageSize" @pagination="load"/>
 </el-dialog>
</template>
<script setup>
import{ref,reactive,onBeforeUnmount,onDeactivated,watch}from'vue';import{workflow}from'@/api/cms/workflow'
const props=defineProps({modelValue:{type:Number,default:0},name:{type:String,default:''},allowClear:Boolean,allowDisabled:Boolean}),emit=defineEmits(['update:modelValue','selected']),visible=ref(false),rows=ref([]),total=ref(0),loading=ref(false),label=ref(props.name),q=reactive({name:'',pageNum:1,pageSize:10});let epoch=0
watch(()=>props.name,v=>label.value=v);watch(()=>props.modelValue,v=>{if(!v)label.value=props.allowClear?'全部分类':''})
async function load(){const n=++epoch;loading.value=true;try{const r=await workflow('download/categories','get',undefined,{...q});if(n===epoch){rows.value=r.rows;total.value=r.total}}finally{if(n===epoch)loading.value=false}}
function open(){visible.value=true;q.pageNum=1;load()}function search(){q.pageNum=1;load()}function choose(row){emit('update:modelValue',row.categoryId);emit('selected',row);label.value=row.name;visible.value=false}
function close(){++epoch;visible.value=false;rows.value=[]}onBeforeUnmount(close);onDeactivated(close)
</script>
