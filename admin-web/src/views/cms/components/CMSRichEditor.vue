<template>
  <div class="cms-editor">
    <div class="image-tools" v-if="canImages"><AssetPicker v-model="imageId" label="选择后点击插入正文"/><el-button :disabled="!imageId || loading" @click="insertImage">插入正文图片</el-button></div>
    <div ref="host"/>
    <div class="tip">图片从素材库选择；外部粘贴按纯文本处理。正文最多 256KiB。</div>
  </div>
</template>
<script setup>
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import Quill from 'quill'
import 'quill/dist/quill.snow.css'
import { ElMessage } from 'element-plus'
import { privateImage } from '@/api/cms/content'
import AssetPicker from './AssetPicker.vue'
import auth from '@/plugins/auth'
const props = defineProps({ modelValue: { type: String, default: '' } })
const emit = defineEmits(['update:modelValue'])
const host = ref(), imageId = ref(0), loading = ref(false)
const canImages = auth.hasPermi('cms:asset:list') && auth.hasPermi('cms:asset:query')
const Embed = Quill.import('formats/image'), Delta = Quill.import('delta')
// A separate blot leaves the existing system editor's Image behavior unchanged.
class PrivateImage extends Embed {
  static blotName = 'cmsImage'; static tagName = 'IMG'; static className = 'cms-private-image'
  static create(value) { const n = document.createElement('img'); n.className = this.className; n.setAttribute('src', value.url); n.setAttribute('data-cms-id', String(value.id)); n.setAttribute('alt', '正文图片'); return n }
  static value(n) { return { id: Number(n.getAttribute('data-cms-id')), url: n.getAttribute('src') } }
}
Quill.register(PrivateImage, true)
let quill, epoch = 0, applying = false, emitted = '', alive = true
let loadFailed = false
const urls = new Map(), minted = new Set()
function release() { for (const u of urls.values()) URL.revokeObjectURL(u); urls.clear(); minted.clear() }
async function imageURL(id, ticket) { if (urls.has(id)) return urls.get(id); const blob = await privateImage(id); if (!alive || ticket !== epoch) return null; const u = URL.createObjectURL(blob); urls.set(id, u); minted.add(u); return u }
function serialize() { const copy = quill.root.cloneNode(true); for (const n of copy.querySelectorAll('img')) { const id = Number(n.getAttribute('data-cms-id')); if (id > 0 && minted.has(n.getAttribute('src'))) { n.setAttribute('src', `/media/${id}`); n.removeAttribute('class'); n.removeAttribute('data-cms-id') } else n.remove() }; return copy.innerHTML }
async function load(value) {
  if (!quill) return
  const ticket = ++epoch; applying = true; loadFailed = false; loading.value = true; quill.enable(false); release()
  try {
    const doc = new DOMParser().parseFromString(value || '<p><br></p>', 'text/html')
    const images = [...doc.querySelectorAll('img')]
    for (const n of images) {
      const match = /^\/media\/([1-9][0-9]*)$/.exec(n.getAttribute('src') || '')
      if (!match) { n.remove(); continue }
      const id = Number(match[1]); const u = await imageURL(id, ticket)
      if (ticket !== epoch || !alive) return
      if (!u) { n.remove(); continue }; n.setAttribute('src', u); n.setAttribute('data-cms-id', String(id)); n.className = 'cms-private-image'
    }
    if (ticket !== epoch || !alive) return
    quill.clipboard.dangerouslyPasteHTML(doc.body.innerHTML, 'silent')
  } catch { if (ticket === epoch) { loadFailed = true; quill.setText('', 'silent'); ElMessage.error('正文图片加载失败，请确认素材查询权限后重新打开；未加载内容不能保存') } }
  finally { if (ticket === epoch && alive) { applying = false; loading.value = false; quill.enable(true) } }
}
async function insertImage() { const ticket = epoch; const id = imageId.value; try { const u = await imageURL(id, ticket); if (!u || ticket !== epoch || !alive) return; const at = quill.getSelection(true)?.index ?? quill.getLength() - 1; quill.insertEmbed(at, 'cmsImage', { id, url: u }, 'user'); quill.setSelection(at + 1) } catch { ElMessage.error('图片加载失败') } }
function paste(event) { event.preventDefault(); event.stopPropagation(); const text = event.clipboardData?.getData('text/plain') || ''; if (!text) { ElMessage.info('图片请从素材库选择'); return }; const range = quill.getSelection(true); const at = range?.index ?? quill.getLength() - 1; if (range?.length) quill.deleteText(at, range.length, 'user'); quill.insertText(at, text, 'user'); quill.setSelection(at + text.length) }
function drop(event) { event.preventDefault(); event.stopPropagation(); ElMessage.info('图片请从素材库选择') }
onMounted(async () => {
  quill = new Quill(host.value, { theme: 'snow', placeholder: '输入正文', modules: { toolbar: [[{ header: [2, 3, false] }], ['bold', 'italic', 'underline', 'strike'], [{ list: 'ordered' }, { list: 'bullet' }], ['blockquote', 'code-block', 'link'], [{ color: [] }, { background: [] }], ['clean']] }, formats: ['header', 'bold', 'italic', 'underline', 'strike', 'list', 'blockquote', 'code-block', 'link', 'color', 'background', 'cmsImage'] })
  quill.clipboard.addMatcher('IMG', node => { const id = Number(node.getAttribute('data-cms-id')), url = node.getAttribute('src'); return id > 0 && minted.has(url) ? new Delta().insert({ cmsImage: { id, url } }) : new Delta() })
  quill.root.addEventListener('paste', paste, true); quill.root.addEventListener('drop', drop, true)
  quill.on('text-change', () => { if (!applying) { emitted = serialize(); emit('update:modelValue', emitted) } })
  await load(props.modelValue)
})
watch(() => props.modelValue, value => { if (value !== emitted) load(value) })
onBeforeUnmount(() => { alive = false; ++epoch; release(); quill?.root.removeEventListener('paste', paste, true); quill?.root.removeEventListener('drop', drop, true); quill = null })
defineExpose({ ready: () => !loading.value && !loadFailed })
</script>
<style scoped>.image-tools{display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap}.cms-editor{width:100%}.cms-editor :deep(.ql-editor){min-height:260px;max-height:500px}.cms-editor :deep(img){max-width:100%}.tip{font-size:12px;color:#777;margin-top:4px}</style>
