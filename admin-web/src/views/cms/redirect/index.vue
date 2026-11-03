<template>
  <div class="app-container">
    <el-alert title="只配置旧站路径到已公开内容的一跳301。先保存停用，再启用；目标下架或栏目停用后返回404。映射停用保留，启用中需先停用才能修改。" :closable="false" style="margin-bottom:12px"/>
    <el-form inline @submit.prevent="search">
      <el-form-item label="旧路径前缀"><el-input v-model="query.sourcePath" maxlength="200" clearable placeholder="/old-news" @keyup.enter="search"/></el-form-item>
      <el-form-item label="映射状态"><el-select v-model="query.status" style="width:120px" clearable><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
      <el-form-item><el-button type="primary" icon="Search" @click="search">搜索</el-button><el-button icon="Refresh" @click="reset">重置</el-button></el-form-item>
    </el-form>
    <el-row style="margin-bottom:12px;gap:10px"><el-button v-if="can('add')" type="primary" plain icon="Plus" :disabled="busy" @click="create">新增映射</el-button><el-button :disabled="loading" @click="load">刷新</el-button></el-row>
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="redirectId" label="编号" width="80"/>
      <el-table-column prop="sourcePath" label="旧路径" min-width="190" show-overflow-tooltip/>
      <el-table-column prop="targetId" label="目标编号" width="95"/>
      <el-table-column prop="targetPath" label="目标路径" min-width="190" show-overflow-tooltip/>
      <el-table-column label="映射状态" width="100"><template #default="s"><el-tag :type="s.row.status==='0'?'success':'info'">{{s.row.status==='0'?'启用':'停用'}}</el-tag></template></el-table-column>
      <el-table-column prop="remark" label="备注" min-width="130" show-overflow-tooltip/>
      <el-table-column prop="updateTime" label="修改时间" width="165"/>
      <el-table-column label="操作" min-width="215"><template #default="s">
        <el-button v-if="can('query')&&can('edit')" link type="primary" :disabled="busy||s.row.status==='0'" @click="edit(s.row)">修改</el-button>
        <el-button v-if="can('publish')" link :type="s.row.status==='0'?'warning':'primary'" :disabled="busy" @click="status(s.row)">{{s.row.status==='0'?'停用':'启用'}}</el-button>
        <el-button v-if="can('query')" link type="primary" :disabled="s.row.status!=='0'" @click="visit(s.row)">访问旧地址</el-button>
      </template></el-table-column>
    </el-table>
    <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
    <el-dialog v-model="opened" title="旧路径映射 · 保存后停用" width="min(680px,calc(100vw - 32px))" append-to-body :close-on-click-modal="false" :show-close="!busy">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px" @submit.prevent>
        <el-form-item label="旧路径" prop="sourcePath"><el-input v-model="form.sourcePath" maxlength="200" placeholder="/old-news/123.html"/></el-form-item>
        <p class="hint">只填站内路径，限小写字母、数字、/._-。不填域名、?查询参数或尾斜线。API、后台、静态资源及已有内容地址不能占用。</p>
        <el-form-item label="目标内容编号" prop="targetId"><el-input-number v-model="form.targetId" :min="1" :max="Number.MAX_SAFE_INTEGER" :precision="0" controls-position="right"/></el-form-item>
        <p class="hint">在「文章管理 / 产品展示 / 单页管理」列表复制编号；目标须已发布。访问旧路径时丢弃原查询参数，转到内容固定地址，不跳外站。</p>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" maxlength="500"/></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="busy" @click="opened=false">取消</el-button><el-button type="primary" :loading="busy" @click="save">保存为停用</el-button></template>
    </el-dialog>
  </div>
</template>
<script setup name="CmsRedirect">
import { ref,reactive,onMounted,onActivated,onDeactivated,onBeforeUnmount,nextTick } from 'vue'
import { ElMessage,ElMessageBox } from 'element-plus'
import auth from '@/plugins/auth'
import { listRedirects,getRedirect,saveRedirect,setRedirectStatus } from '@/api/cms/redirect'
const can=p=>auth.hasPermi('cms:redirect:'+p)
const query=reactive({sourcePath:'',status:'',pageNum:1,pageSize:10}),rows=ref([]),total=ref(0),loading=ref(false),busy=ref(false),opened=ref(false),formRef=ref()
const initial=()=>({redirectId:0,sourcePath:'',targetId:1,remark:'',version:0}),form=reactive(initial())
const rules={sourcePath:[{required:true,message:'请填旧路径',trigger:'blur'}],targetId:[{required:true,message:'请填已公开的目标内容编号',trigger:'change'}]}
let epoch=0,operation=0,active=true
async function load(){const ticket=++epoch;loading.value=true;try{const r=await listRedirects({...query});if(active&&ticket===epoch){rows.value=r.rows;total.value=r.total}}finally{if(ticket===epoch)loading.value=false}}
function search(){query.pageNum=1;load()}
function reset(){query.sourcePath='';query.status='';search()}
async function create(){Object.assign(form,initial());opened.value=true;await nextTick();formRef.value?.clearValidate()}
async function edit(row){if(busy.value)return;const ticket=++operation;busy.value=true;try{const r=await getRedirect(row.redirectId);if(active&&ticket===operation){Object.assign(form,r.data);opened.value=true;await nextTick();formRef.value?.clearValidate()}}finally{if(ticket===operation)busy.value=false}}
async function save(){if(busy.value)return;const ticket=++operation;busy.value=true;try{await formRef.value.validate();if(!active||ticket!==operation)return;await saveRedirect({...form});if(active&&ticket===operation){opened.value=false;ElMessage.success('已保存为停用，确认后单独启用');await load()}}finally{if(ticket===operation)busy.value=false}}
async function status(row){if(busy.value)return;const ticket=++operation;busy.value=true;const target=row.status==='0'?'1':'0';try{await ElMessageBox.confirm(target==='0'?`启用 ${row.sourcePath} 到 ${row.targetPath} 的301跳转？浏览器可能记住永久跳转。`:'停用后新请求不再跳转；已被浏览器保存的301无法强制撤回。','跳转'+(target==='0'?'启用':'停用'));if(!active||ticket!==operation)return;await setRedirectStatus(row,target);if(active&&ticket===operation){ElMessage.success('映射状态已更新');await load()}}catch(e){if(e!=='cancel'&&e!=='close')throw e}finally{if(ticket===operation)busy.value=false}}
function visit(row){window.open(row.sourcePath,'_blank','noopener,noreferrer')}
function leave(){active=false;++epoch;++operation;opened.value=false;loading.value=false;busy.value=false}
onMounted(load);onActivated(()=>{if(!active){active=true;load()}});onDeactivated(leave);onBeforeUnmount(leave)
</script>
<style scoped>.hint{font-size:12px;color:var(--el-text-color-secondary);margin:0 0 16px 100px;line-height:1.7}</style>
