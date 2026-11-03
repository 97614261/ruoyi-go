<template>
 <div class="recovery-bar">
  <el-alert :type="['conflict','error'].includes(state.phase)?'warning':'info'" :title="state.message" :closable="false"/>
  <div class="recovery-actions">
   <span v-if="state.view?.found">副本时间 {{state.view.savedAt}} · 到期 {{state.view.expiresAt}}</span>
   <el-button v-if="state.phase==='review'&&state.view?.baseMatches" @click="$emit('restore')">恢复到表单</el-button>
   <el-button v-if="state.view?.found" @click="inspect=true">查看个人副本</el-button>
   <el-button v-if="state.view?.found" @click="$emit('clear')">清除个人副本</el-button>
   <el-button v-if="['error','conflict'].includes(state.phase)" @click="$emit('reload')">重新核对副本</el-button>
   <el-button v-else :disabled="busy||['loading','review','saving'].includes(state.phase)" @click="$emit('flush')">立即保存副本</el-button>
  </div>
  <el-dialog v-model="inspect" title="个人恢复副本 · 只读核对" width="min(900px,calc(100vw - 32px))" append-to-body><p>正文按HTML源码显示，可复制核对；不会执行图片、脚本或替换服务端新稿。</p><pre class="recovery-data">{{JSON.stringify(state.view?.draft,null,2)}}</pre></el-dialog>
 </div>
</template>
<script setup>
import {ref} from 'vue'
defineProps({state:{type:Object,required:true},busy:Boolean});defineEmits(['restore','clear','reload','flush']);const inspect=ref(false)
</script>
<style scoped>.recovery-bar{margin-bottom:16px}.recovery-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:8px}.recovery-actions span{color:var(--el-text-color-secondary);font-size:12px}.recovery-data{white-space:pre-wrap;overflow-wrap:anywhere;max-height:60vh;overflow:auto;font:13px/1.7 monospace}</style>
