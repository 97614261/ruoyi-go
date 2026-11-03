<template>
 <el-dialog v-model="visible" title="送审快照与处理记录" width="min(1080px,calc(100vw - 32px))" append-to-body destroy-on-close @closed="close">
  <div v-loading="loading" v-if="row">
   <el-alert :title="`${kindLabels[row.kind]} #${row.objectId} · ${reviewLabels[row.status]} · ${row.isCurrent?'仍是当前工作稿':'已不是当前工作稿'}`" :type="row.isCurrent?'info':'warning'" :closable="false"/>
   <VersionCompare ref="versionCompare"/><el-button link type="primary" @click="versionCompare.open({kind:row.kind,objectId:row.objectId,left:{source:'REVIEW',id:row.reviewId}})">送审稿与当前稿对比</el-button><p>申请 #{{row.reviewId}} · 工作版本 {{row.contentVersion}} · 保存人 #{{row.authorUserId}} · 提交人 #{{row.submitterUserId}} · 审核人 #{{row.reviewerUserId||'—'}}</p>
   <el-alert v-if="row.opinion" :title="row.opinion" :closable="false"/>
   <el-tabs>
    <el-tab-pane label="送审内容"><iframe v-if="html" :srcdoc="html" sandbox="" referrerpolicy="no-referrer" title="不可变送审快照" style="width:100%;height:50vh;border:1px solid #eee"/><p v-else>正在准备只读快照…</p></el-tab-pane>
    <el-tab-pane label="完整配置与运营设置"><pre style="white-space:pre-wrap;overflow-wrap:anywhere;max-height:50vh;overflow:auto">{{JSON.stringify(row.snapshot,null,2)}}</pre></el-tab-pane>
    <el-tab-pane label="处理记录"><el-table :data="events"><el-table-column prop="action" label="动作" width="150"/><el-table-column prop="actorUserId" label="操作人ID" width="100"/><el-table-column prop="reason" label="意见/原因"/><el-table-column prop="createTime" label="时间" width="170"/></el-table><pagination v-if="eventTotal" :total="eventTotal" v-model:page="pg.pageNum" v-model:limit="pg.pageSize" @pagination="loadEvents"/></el-tab-pane>
   </el-tabs>
   <el-form v-if="canDecide" label-width="80px"><el-form-item label="审核意见"><el-input v-model="opinion" type="textarea" maxlength="1000" placeholder="退回必须填写原因；审核通过不自动发布"/></el-form-item></el-form>
  </div>
  <template #footer><el-button @click="visible=false">关闭</el-button><el-button v-if="canWithdraw" :loading="busy" @click="decide('withdraw')">撤回申请</el-button><el-button v-if="canDecide" type="danger" :loading="busy" @click="decide('reject')">退回</el-button><el-button v-if="canDecide" type="primary" :loading="busy" @click="decide('approve')">审核通过</el-button><el-button v-if="canPublish" type="success" :loading="busy" @click="publish">发布已批准版本</el-button></template>
 </el-dialog>
</template>
<script setup>
import {ref,reactive,computed,onBeforeUnmount,onDeactivated} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import {workflow,kindLabels,reviewLabels} from '@/api/cms/workflow'
import {privateImage} from '@/api/cms/content'
import auth from '@/plugins/auth'
import VersionCompare from './VersionCompare.vue'
const versionCompare=ref()
import useUserStore from '@/store/modules/user'
const emit=defineEmits(['changed']),user=useUserStore(),visible=ref(false),row=ref(),loading=ref(false),busy=ref(false),html=ref(''),events=ref([]),eventTotal=ref(0),opinion=ref(''),pg=reactive({pageNum:1,pageSize:10});let epoch=0;const urls=[]
const self=computed(()=>row.value&&(user.id===row.value.authorUserId||user.id===row.value.submitterUserId))
const canDecide=computed(()=>row.value?.status==='PENDING'&&row.value.isCurrent&&!self.value&&auth.hasPermi(`cms:${row.value.kind.toLowerCase()}:review`))
const canWithdraw=computed(()=>row.value?.submitterUserId===user.id&&['PENDING','APPROVED'].includes(row.value.status))
const canPublish=computed(()=>row.value?.status==='APPROVED'&&row.value.isCurrent&&auth.hasPermi(`cms:${row.value.kind.toLowerCase()}:publish`))
function release(){for(const u of urls)URL.revokeObjectURL(u);urls.length=0;html.value=''}
async function open(id){close();visible.value=true;loading.value=true;const n=epoch;try{const r=await workflow('review/'+id);if(n!==epoch||!visible.value)return;row.value=r.data;opinion.value='';pg.pageNum=1;await Promise.all([preview(n),loadEvents()])}finally{if(n===epoch)loading.value=false}}
async function loadEvents(){if(!row.value)return;const n=epoch,r=await workflow(`review/${row.value.reviewId}/events`,'get',undefined,{...pg});if(n===epoch){events.value=r.rows;eventTotal.value=r.total}}
async function preview(n){release();const snap=row.value.snapshot;let source
 if(snap.site){const r=await workflow(`review/${row.value.reviewId}/preview`);if(n!==epoch)return;source=r.data.html}else{const doc=document.implementation.createHTMLDocument('送审快照');const r=snap.content.draft;doc.body.innerHTML=r.bodyHtml;const title=doc.createElement('h1');title.textContent=r.title;doc.body.prepend(title);for(const [key,label]of[['summary','摘要'],['productCode','产品编号'],['productParams','产品参数'],['seoTitle','SEO标题'],['seoDescription','SEO描述']]){if(r[key]){const p=doc.createElement('p');p.textContent=label+'：'+r[key];doc.body.append(p)}};for(const id of [r.coverAssetId,...(r.galleryIds||[]),snap.operations?.shareAssetId].filter(Boolean)){const im=doc.createElement('img');im.src='/media/'+id;doc.body.append(im)};source=doc.documentElement.outerHTML}
 const doc=new DOMParser().parseFromString(source,'text/html');for(const el of doc.querySelectorAll('script,iframe,object,embed,form,base,meta[http-equiv]'))el.remove();for(const a of doc.querySelectorAll('a'))a.removeAttribute('href')
 for(const img of doc.querySelectorAll('img')){const m=/^\/media\/([1-9][0-9]*)$/.exec(img.getAttribute('src')||'');if(!m||!auth.hasPermi('cms:asset:query')){img.replaceWith(doc.createTextNode('[图片需要素材查询权限]'));continue};try{const blob=await privateImage(Number(m[1]));if(n!==epoch)return;const u=URL.createObjectURL(blob);urls.push(u);img.src=u}catch{img.replaceWith(doc.createTextNode('[图片不可用]'))}}
 for(const link of doc.querySelectorAll('link[rel="stylesheet"]'))link.href=new URL(link.getAttribute('href'),location.origin).href
 const meta=doc.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content=`default-src 'none'; img-src blob:; style-src 'unsafe-inline' ${location.origin}; base-uri 'none'`;doc.head.prepend(meta);if(n===epoch)html.value='<!doctype html>'+doc.documentElement.outerHTML
}
async function decide(action){if(busy.value)return;if(action==='reject'&&!opinion.value.trim()){ElMessage.warning('请填写退回原因');return};const id=row.value.reviewId,n=epoch;busy.value=true;try{await workflow(`review/${id}/decision`,'post',{version:row.value.version,action,opinion:opinion.value});if(n!==epoch)return;ElMessage.success('处理成功');emit('changed');await open(id)}finally{busy.value=false}}
async function publish(){if(busy.value)return;const r=row.value,n=epoch;try{await ElMessageBox.confirm('发布此已批准版本？批准只供本次使用。','发布确认');if(n!==epoch)return;busy.value=true;await workflow(r.kind==='SITE'?'site/publish':`${r.kind.toLowerCase()}/${r.objectId}/publish`,'post',{version:r.contentVersion});if(n!==epoch)return;ElMessage.success('已发布');emit('changed');await open(r.reviewId)}catch{}finally{busy.value=false}}
function close(){versionCompare.value?.close();++epoch;visible.value=false;row.value=null;events.value=[];eventTotal.value=0;release();loading.value=false}onBeforeUnmount(close);onDeactivated(close);defineExpose({open,close})
</script>
