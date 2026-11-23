<script setup lang="ts">
import {computed,onMounted,onBeforeUnmount,ref} from 'vue'
import {PhCopy,PhArrowClockwise,PhX} from '@phosphor-icons/vue'
import {APIError,request} from './api'
import type {AssetLimits,Graph,TemplateSlot} from './contracts'
import TemplateGraph from './TemplateGraph.vue'
import AssetPicker from './AssetPicker.vue'
import {pendingOfficialUse,retryOfficialUse,submitOfficialUse} from './official-requests'
interface Official {templateId:string;name:string;description:string;publishedVersionId?:string}
interface Snapshot {versionId:string;version:string;graph:Graph;slots:TemplateSlot[];outputs:{nodeId:string;type:string;label:string}[]}
const props=defineProps<{token:string;scope:string;limits:AssetLimits}>(),emit=defineEmits<{open:[id:string];expired:[]}>()
const rows=ref<Official[]>([]),page=ref(1),total=ref(0),loading=ref(false),busy=ref(false),error=ref(''),pending=ref(false),name=ref(''),selected=ref<Official|null>(null),snapshot=ref<Snapshot|null>(null),dialog=ref<HTMLDialogElement>(),projectName=ref(''),texts=ref<Record<string,string>>({}),images=ref<Record<string,string[]>>({}),replaceImages=ref<Record<string,boolean>>({}),picker=ref('')
let sequence=0,detailSequence=0,focus:HTMLElement|null=null
const canUse=computed(()=>!!snapshot.value&&!busy.value&&!pending.value&&!!projectName.value.trim()&&(snapshot.value.slots||[]).every(slot=>slot.type!=='imageInput'||((replaceImages.value[slot.nodeId]||!slot.assetIds?.length)?!!images.value[slot.nodeId]?.length:true)))
function showError(e:unknown){error.value=(e as Error).message;if(e instanceof APIError&&e.code===401)emit('expired')}
function refreshPending(){try{pending.value=!!pendingOfficialUse(props.scope)}catch(e){pending.value=true;showError(e)}}
async function load(){const seq=++sequence;loading.value=true;error.value='';try{const r=await request<{rows:Official[];total:number}>(`/pod/catalog/templates?${new URLSearchParams({pageNum:String(page.value),pageSize:'12',name:name.value})}`,{},props.token);if(seq===sequence){rows.value=r.rows;total.value=r.total}}catch(e){if(seq===sequence)showError(e)}finally{if(seq===sequence)loading.value=false}}
async function show(row:Official){focus=document.activeElement as HTMLElement;selected.value=row;snapshot.value=null;projectName.value=row.name;texts.value={};images.value={};replaceImages.value={};error.value='';const seq=++detailSequence;dialog.value?.showModal();try{const r=await request<{data:{template:Official;snapshot:Snapshot}}>(`/pod/catalog/templates/${row.templateId}`,{},props.token);if(seq!==detailSequence)return;snapshot.value=r.data.snapshot;for(const slot of snapshot.value.slots||[]){if(slot.type==='textInput')texts.value[slot.nodeId]=slot.text||'';else{images.value[slot.nodeId]=[];replaceImages.value[slot.nodeId]=!slot.assetIds?.length}}}catch(e){if(seq===detailSequence)showError(e)}}
function close(){if(busy.value)return;++detailSequence;dialog.value?.close();selected.value=null;snapshot.value=null;picker.value='';focus?.focus()}
async function use(){if(!canUse.value||!selected.value||!snapshot.value)return;busy.value=true;error.value='';try{const bindings=(snapshot.value.slots||[]).flatMap<unknown>(slot=>slot.type==='textInput'?[{nodeId:slot.nodeId,text:texts.value[slot.nodeId]||''}]:replaceImages.value[slot.nodeId]?[{nodeId:slot.nodeId,assetIds:images.value[slot.nodeId]||[]}]:[]) as unknown[];const r=await submitOfficialUse(props.scope,props.token,selected.value.templateId,{version:snapshot.value.versionId,name:projectName.value,bindings});if(r.state==='completed')emit('open',r.projectId)}catch(e){showError(e)}finally{busy.value=false;refreshPending()}}
async function retry(){busy.value=true;error.value='';try{const r=await retryOfficialUse(props.scope,props.token);if(r.state==='completed')emit('open',r.projectId)}catch(e){showError(e)}finally{busy.value=false;refreshPending()}}
onMounted(()=>{refreshPending();void load()});onBeforeUnmount(()=>{++sequence;++detailSequence;dialog.value?.close()})
</script>
<template>
 <div class="official-library">
  <div class="official-toolbar"><form @submit.prevent="page=1;load()"><input v-model="name" aria-label="搜索官方模板" placeholder="搜索模板" maxlength="100"/><button class="secondary-button" :disabled="loading">查询</button></form><button class="icon-button" aria-label="刷新官方模板" :disabled="loading" @click="load"><PhArrowClockwise :size="18"/></button></div>
  <div v-if="pending" class="editor-alert"><span>上次复制结果待确认，请继续原操作。</span><button class="text-button" :disabled="busy" @click="retry">继续原操作</button></div><p v-if="error" class="error-message" role="alert">{{ error }}</p>
  <p v-if="loading" class="muted" role="status">正在读取已发布模板…</p>
  <div v-else-if="rows.length" class="personal-template-list"><article v-for="row in rows" :key="row.templateId" class="personal-template-row"><span class="template-symbol"><PhCopy :size="24" weight="duotone"/></span><div class="personal-template-copy"><h2>{{ row.name }}</h2><p v-if="row.description">{{ row.description.trim().split(/\r?\n/)[0] }}</p></div><div class="personal-template-actions"><button class="primary-button" :disabled="busy||pending" @click="show(row)">查看并使用</button></div></article></div>
  <div v-else class="template-empty"><PhCopy :size="36" weight="light"/><h2>暂无官方模板</h2><p>可先使用个人模板或内置预设。</p></div>
  <div v-if="total>12" class="template-pagination"><button class="secondary-button" :disabled="page===1||loading" @click="page--;load()">上一页</button><span>第 {{ page }} / {{ Math.ceil(total/12) }} 页</span><button class="secondary-button" :disabled="page*12>=total||loading" @click="page++;load()">下一页</button></div>
  <dialog ref="dialog" class="template-use-dialog official-dialog" aria-labelledby="official-template-title" @cancel.prevent="close">
    <header class="template-dialog-header"><div><h2 id="official-template-title">{{ selected?.name }}</h2></div><button class="icon-button" aria-label="关闭模板" :disabled="busy" @click="close"><PhX :size="20"/></button></header>
    <div v-if="snapshot" class="template-use-layout official-detail">
      <section class="template-preview">
        <details class="template-disclosure"><summary>流程预览 · {{ snapshot.graph.nodes.length }} 个节点</summary><TemplateGraph :graph="snapshot.graph"/></details>
        <details v-if="selected?.description" class="template-disclosure"><summary>使用说明</summary><p class="official-instructions">{{ selected.description }}</p></details>
      </section>
      <section class="template-inputs official-inputs">
        <label class="field">项目名称<input v-model="projectName" maxlength="100" :disabled="busy"/></label>
        <template v-for="slot in snapshot.slots||[]" :key="slot.nodeId">
          <label v-if="slot.type==='textInput'" class="field">{{ slot.label }}<textarea v-model="texts[slot.nodeId]" :disabled="busy" rows="4"/></label>
          <section v-else class="official-image-slot"><label v-if="slot.assetIds?.length"><input v-model="replaceImages[slot.nodeId]" type="checkbox" :disabled="busy"/>替换「{{ slot.label }}」的默认图片</label><strong v-if="!slot.assetIds?.length">{{ slot.label }}</strong><p class="muted">{{ replaceImages[slot.nodeId] ? (images[slot.nodeId]?.length ? `已选 ${images[slot.nodeId].length} 张图片` : '请先选择个人图片') : `使用 ${slot.assetIds?.length||0} 张模板图片` }}</p><button v-if="replaceImages[slot.nodeId]" class="secondary-button" :disabled="busy" @click="picker=slot.nodeId">从我的素材选择</button></section>
        </template>
      </section>
    </div>
    <p v-else class="template-loading muted" role="status">正在读取模板…</p>
    <footer class="template-use-footer"><p>仅创建项目，不执行模型；模板图片占用个人存储。</p><p v-if="error" class="error-message" role="alert">{{ error }}</p><div class="dialog-actions"><button class="secondary-button" :disabled="busy" @click="close">取消</button><button class="primary-button" :disabled="!canUse" @click="use">{{ busy?'正在复制…':'复制并打开画布' }}</button></div></footer>
  </dialog>
  <AssetPicker v-if="picker" :token="token" :limits="limits" :initial="images[picker]||[]" @close="picker=''" @confirm="ids=>{images[picker]=ids;picker=''}" @expired="emit('expired')"/>
 </div>
</template>
<style scoped>
.official-toolbar { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; }
.official-toolbar form { display: flex; flex: 1; min-width: 0; gap: 8px; }
.official-toolbar input { flex: 1; width: 100%; min-width: 0; border: 1px solid var(--line); border-radius: 7px; padding: 10px 12px; }
.official-toolbar button { flex-shrink: 0; }
.official-instructions { white-space: pre-wrap; font-size: 13px; line-height: 1.8; color: var(--muted); overflow-wrap: anywhere; }
.official-image-slot { padding: 14px; border: 1px solid var(--line); border-radius: 8px; }
.official-image-slot label { display: flex; gap: 8px; align-items: flex-start; }
.official-image-slot .muted { margin-block: 10px; }
</style>
