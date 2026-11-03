<template>
  <el-button @click="open">{{ caption }}</el-button>
  <el-dialog v-model="visible" title="选择导航目标" width="min(780px,calc(100vw - 24px))" append-to-body destroy-on-close @closed="close">
    <el-tabs v-model="tab"><el-tab-pane label="固定页面" name="fixed"><el-button v-for="p in pages" :key="p.url" @click="choose(0,p.url,p.title)">{{ p.title }}</el-button></el-tab-pane><el-tab-pane label="栏目" name="channel"><SiteReferencePicker type="channel" :allow-clear="false" @selected="r=>r&&choose(0,r.url,r.title)" /></el-tab-pane><el-tab-pane label="文章与单页" name="content"><SiteReferencePicker :allow-clear="false" @selected="r=>r&&choose(r.refId,'',r.title)" /></el-tab-pane><el-tab-pane label="自定义地址" name="custom"><el-form @submit.prevent="custom"><el-form-item label="地址"><el-input v-model="address" maxlength="500" placeholder="站内路径 /... 或外链 https://..." /></el-form-item><el-button type="primary" @click="custom">使用此地址</el-button></el-form><p>外链使用HTTPS，站内地址以单个 / 开头。</p></el-tab-pane></el-tabs>
  </el-dialog>
</template>
<script setup>
import { ref, computed, inject, onBeforeUnmount, onDeactivated } from 'vue'
import { ElMessage } from 'element-plus'
import SiteReferencePicker from './SiteReferencePicker.vue'
const props = defineProps({ contentId: { type: Number, default: 0 }, url: { type: String, default: '' } }), emit = defineEmits(['update:contentId', 'update:url', 'selected'])
const references = inject('siteReferenceNames', ref({ channel: {}, content: {} })), visible = ref(false), tab = ref('fixed'), address = ref('')
const pages = [{ title: '首页', url: '/' }, { title: '文章列表', url: '/news' }, { title: '产品列表', url: '/products' }, { title: '资料下载', url: '/downloads' }, { title: '站内搜索', url: '/search' }, { title: '联系留言', url: '/inquiry' }]
const caption = computed(() => props.contentId ? references.value.content[props.contentId]?.title || `已选内容 #${props.contentId}` : pages.find(p => p.url === props.url)?.title || props.url || '选择链接目标')
function open() { address.value = props.url; tab.value = props.contentId ? 'content' : pages.some(p=>p.url===props.url) ? 'fixed' : props.url ? 'custom' : 'fixed'; visible.value = true }
function choose(id,url,title) { emit('update:contentId',id); emit('update:url',url); emit('selected',{title}); visible.value=false }
function custom() { let valid=false; try { const u=new URL(address.value,location.origin); valid=address.value.startsWith('/')&&!address.value.startsWith('//')&&!address.value.includes('\\') || u.protocol==='https:'&&!u.username&&!u.password&&address.value.startsWith('https://') } catch {} if(!valid){ElMessage.warning('请填写站内路径或HTTPS地址');return} choose(0,address.value,'') }
function close(){visible.value=false}onBeforeUnmount(close);onDeactivated(close)
</script>
