<template>
  <el-dialog v-model="visible" title="定时发布 / 下架" width="min(1000px, calc(100vw - 32px))" append-to-body :close-on-click-modal="false" @closed="close">
    <el-alert title="按北京时间执行。每篇只保留一个待执行任务；修改内容后旧任务将失败。停机错过24小时内会补执行，超过则需人工重新安排。" type="info" :closable="false"/>
    <p>{{ owner?.title }} · 内容 #{{ owner?.contentId }} · 版本 {{ owner?.version }}</p>
    <el-form v-if="canPublish" inline @submit.prevent="submit">
      <el-form-item label="操作"><el-select v-model="action" style="width:130px"><el-option value="publish" label="定时发布"/><el-option value="unpublish" label="定时下架" :disabled="!owner?.isPublished"/></el-select></el-form-item>
      <el-form-item label="北京时间"><el-input v-model="dueAt" placeholder="YYYY-MM-DD HH:mm:ss" maxlength="19" style="width:205px" aria-label="定时任务北京时间"/></el-form-item>
      <el-form-item><el-button type="primary" :loading="busy" :disabled="pending" @click="submit">创建任务</el-button></el-form-item>
    </el-form>
    <el-button :loading="loading" @click="refresh">刷新任务及内容版本</el-button>
    <el-table :data="rows" v-loading="loading" style="margin-top:10px">
      <el-table-column prop="scheduleId" label="任务" width="75"/><el-table-column label="操作" width="85"><template #default="s">{{ s.row.action==='publish'?'发布':'下架' }}</template></el-table-column>
      <el-table-column prop="dueAt" label="北京时间" width="165"/><el-table-column prop="contentVersion" label="内容版本" width="85"/>
      <el-table-column label="状态" width="85"><template #default="s"><el-tag :type="tone[s.row.status]">{{ statuses[s.row.status] || s.row.status }}</el-tag></template></el-table-column>
      <el-table-column prop="createBy" label="创建人" width="100"/><el-table-column prop="resultMessage" label="执行结果" min-width="160" show-overflow-tooltip/>
      <el-table-column label="操作" width="75"><template #default="s"><el-button v-if="canPublish && s.row.status==='PENDING' && (user.id===1 || user.id===s.row.ownerUserId)" type="danger" link :disabled="busy" @click="cancel(s.row)">取消</el-button></template></el-table-column>
    </el-table>
    <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
    <template #footer><el-button @click="visible=false">关闭</el-button></template>
  </el-dialog>
</template>
<script setup>
import { ref, reactive, computed, onBeforeUnmount, onDeactivated } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import auth from '@/plugins/auth'
import useUserStore from '@/store/modules/user'
import { getContent, listSchedules, createSchedule, cancelSchedule } from '@/api/cms/content'
const props=defineProps({kind:{type:String,required:true}}), emit=defineEmits(['changed'])
const user=useUserStore(), visible=ref(false), owner=ref(null), rows=ref([]), total=ref(0), busy=ref(false), loading=ref(false), dueAt=ref(''), action=ref('publish')
const query=reactive({pageNum:1,pageSize:10}), canPublish=computed(()=>auth.hasPermi(`cms:${props.kind}:publish`))
const pending=computed(()=>rows.value.some(x=>x.status==='PENDING'))
const statuses={PENDING:'待执行',SUCCESS:'成功',FAILED:'失败',CANCELED:'已取消'},tone={PENDING:'warning',SUCCESS:'success',FAILED:'danger',CANCELED:'info'}
let epoch=0
function defaultTime(){return new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(new Date(Date.now()+600000))}
async function open(row){++epoch;owner.value={...row};rows.value=[];total.value=0;query.pageNum=1;action.value='publish';dueAt.value=defaultTime();visible.value=true;await load()}
async function load(){const ticket=++epoch;loading.value=true;try{const r=await listSchedules(props.kind,{...query,contentId:owner.value.contentId});if(ticket===epoch && visible.value){rows.value=r.rows;total.value=r.total}}finally{if(ticket===epoch)loading.value=false}}
async function refresh(){const ticket=++epoch;const r=await getContent(props.kind,owner.value.contentId);if(ticket!==epoch || !visible.value)return;owner.value={...owner.value,...r.data};await load();emit('changed')}
async function submit(){if(busy.value || !owner.value)return;if(!/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(dueAt.value)){ElMessage.warning('请填写完整的北京时间');return}const ticket=epoch, data={contentId:owner.value.contentId,contentVersion:owner.value.version,action:action.value,dueAt:dueAt.value};busy.value=true;try{await createSchedule(props.kind,data);if(ticket!==epoch || !visible.value)return;ElMessage.success('已安排；请刷新查看执行结果');query.pageNum=1;await load()}finally{busy.value=false}}
async function cancel(row){if(busy.value)return;const ticket=epoch;try{await ElMessageBox.confirm('取消这条待执行任务？','取消定时任务');if(ticket!==epoch || !visible.value)return;busy.value=true;await cancelSchedule(props.kind,row);if(ticket!==epoch || !visible.value)return;await load()}catch{}finally{busy.value=false}}
function close(){++epoch;visible.value=false;rows.value=[];owner.value=null;loading.value=false}
onBeforeUnmount(close);onDeactivated(close);defineExpose({open,close})
</script>
