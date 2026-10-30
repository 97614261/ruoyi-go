<template>
  <el-dialog v-model="visible" title="私有素材预览" width="min(900px, calc(100vw - 32px))" append-to-body @closed="release">
    <div v-loading="loading" style="min-height:120px;text-align:center"><img v-if="url" :src="url" alt="选中的私有素材" style="max-width:100%;max-height:70vh;object-fit:contain"></div>
  </el-dialog>
</template>
<script setup>
import { ref, onBeforeUnmount, onDeactivated, onActivated } from 'vue'
import { ElMessage } from 'element-plus'
import { privateImage } from '@/api/cms/content'
const visible = ref(false), loading = ref(false), url = ref('')
let epoch = 0, active = true
function release() { ++epoch; if (url.value) URL.revokeObjectURL(url.value); url.value = ''; loading.value = false }
async function open(id) {
  release(); visible.value = true; loading.value = true
  const ticket = epoch
  try { const blob = await privateImage(id); if (active && ticket === epoch && visible.value) url.value = URL.createObjectURL(blob) }
  catch (error) { if (active && ticket === epoch) ElMessage.error(error.message || '预览失败') }
  finally { if (ticket === epoch) loading.value = false }
}
onActivated(() => { active = true })
onDeactivated(() => { active = false; visible.value = false; release() })
onBeforeUnmount(() => { active = false; release() })
defineExpose({ open })
</script>
