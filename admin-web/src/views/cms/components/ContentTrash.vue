<template><el-button @click="open">回收站</el-button><el-dialog v-model="opened" title="内容回收站 · 恢复后需重新发布" width="min(950px,calc(100vw - 32px))" append-to-body><el-alert title="恢复后为未发布草稿，原地址与历史版本继续保留；不会自动发布或重新安排定时任务。" :closable="false"/><el-form inline @submit.prevent="search"><el-form-item label="标题前缀"><el-input v-model="q.title" maxlength="120" clearable/></el-form-item><el-button @click="search">搜索</el-button></el-form><el-table :data="rows" v-loading="loading"><el-table-column prop="contentId" label="编号" width="90"/><el-table-column prop="title" label="草稿标题"/><el-table-column prop="slug" label="访问地址"/><el-table-column prop="updateTime" label="删除时间" width="170"/><el-table-column label="操作" width="120"><template #default="s"><el-button v-if="canRecover" link type="primary" :disabled="busy" @click="recover(s.row)">恢复为草稿</el-button></template></el-table-column></el-table><pagination v-show="total" :total="total" v-model:page="q.pageNum" v-model:limit="q.pageSize" @pagination="load"/></el-dialog></template>
<script setup>
import {ref,reactive,computed,onBeforeUnmount,onDeactivated} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import auth from '@/plugins/auth'
import request from '@/utils/request'
const props=defineProps({kind:String}),emit=defineEmits(['changed']),opened=ref(false),rows=ref([]),total=ref(0),busy=ref(false),loading=ref(false),q=reactive({title:'',pageNum:1,pageSize:10});let epoch=0
const canRecover=computed(()=>auth.hasPermiAnd([`cms:${props.kind}:query`,`cms:${props.kind}:edit`,`cms:${props.kind}:remove`]))
function open(){q.title='';q.pageNum=1;opened.value=true;load()}function search(){q.pageNum=1;load()}
async function load(){const ticket=++epoch;loading.value=true;try{const r=await request({url:`/cms/${props.kind}/trash`,params:{...q}});if(ticket===epoch&&opened.value){rows.value=r.rows;total.value=r.total}}finally{if(ticket===epoch)loading.value=false}}
async function recover(row){try{await ElMessageBox.confirm('恢复该内容为未发布草稿？','恢复确认');busy.value=true;await request({url:`/cms/${props.kind}/${row.contentId}/recover`,method:'post',data:{version:row.version}});ElMessage.success('已恢复为草稿，尚未发布');emit('changed');await load()}catch{}finally{busy.value=false}}
function cleanup(){++epoch;opened.value=false}onBeforeUnmount(cleanup);onDeactivated(cleanup)
</script>