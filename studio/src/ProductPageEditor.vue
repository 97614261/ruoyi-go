<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { PhArrowClockwise, PhDownloadSimple, PhImage, PhX } from '@phosphor-icons/vue'
import { APIError, request, requestImage } from './api'
import AssetPicker from './AssetPicker.vue'
import type { Asset, AssetLimits } from './contracts'
import { pageFilename, pageImageLabels, pageTemplates, pageTextLimits, readPageDraft } from './product-page'
import type { PageDraft, PageImageSlot } from './product-page'
import { drawProductPage } from './product-page-canvas'

const props = defineProps<{ projectId: string; token: string; limits: AssetLimits; assetAccess: boolean; storageKey: string; sku: string; initial: PageDraft }>()
const emit = defineEmits<{ close: []; expired: []; saved: []; viewSaved: [] }>()
const dialog = ref<HTMLDialogElement>(), canvas = ref<HTMLCanvasElement>()
const saveUnavailable = ref(false)
function restore() {
  try { return readPageDraft(localStorage.getItem(props.storageKey), props.initial) }
  catch { saveUnavailable.value = true; return props.initial }
}
const initial = restore()
const draft = reactive({ ...initial, images: { ...initial.images } })
const picker = ref<PageImageSlot>()
const mobilePanel = ref<'edit' | 'preview'>('edit')
const fontsReady = ref(false), exporting = ref(false), loading = ref(true)
const previewError = ref(''), exportError = ref('')
const saving = ref(false), saveError = ref(''), savedRevision = ref(0)
const previewRevision = ref(0)
const dimensions = ref('')
const failures = ref<string[]>([])
const retry = ref(0)
const loaded = shallowRef(new Map<string, HTMLImageElement>())
const activeSlots = computed<readonly PageImageSlot[]>(() => pageTemplates[draft.template].images)
const activeImages = computed(() => [...new Set(activeSlots.value.map(slot => draft.images[slot]).filter(Boolean))].join(','))
const canExport = computed(() => !!dimensions.value && !!draft.images.hero && !exporting.value && !saving.value)
const currentSaved = computed(() => !!savedRevision.value && savedRevision.value === previewRevision.value)
let mounted = true
let previousFocus: HTMLElement | null = null

watch(draft, value => {
  try { localStorage.setItem(props.storageKey, JSON.stringify(value)); saveUnavailable.value = false }
  catch { saveUnavailable.value = true }
}, { flush: 'sync' })

watch([activeImages, retry, () => props.token], async ([key, , token], previous, onCleanup) => {
  let current = true
  onCleanup(() => { current = false })
  const cache = previous[2] === token ? loaded.value : new Map<string, HTMLImageElement>()
  loaded.value = new Map()
  failures.value = []
  loading.value = true
  const ids = key ? key.split(',') : []
  const images = await Promise.all(ids.map(async id => {
    const existing = cache.get(id)
    if (existing) return existing
    let url = ''
    try {
      const blob = await requestImage(`/pod/assets/${id}/content?variant=original`, token)
      if (!current) return null
      url = URL.createObjectURL(blob)
      const image = new Image()
      image.src = url
      await image.decode()
      return image.naturalWidth && image.naturalHeight ? image : null
    } catch (error) {
      if (current && error instanceof APIError && error.code === 401) emit('expired')
      return null
    } finally {
      if (url) URL.revokeObjectURL(url)
    }
  }))
  if (!current) return
  const ready = new Map<string, HTMLImageElement>()
  const failed: string[] = []
  ids.forEach((id, index) => {
    const image = images[index]
    if (image) ready.set(id, image)
    else failed.push(id)
  })
  loaded.value = ready
  failures.value = failed
  loading.value = false
}, { immediate: true })

watch([canvas, draft, fontsReady, loaded, loading, failures], () => {
  previewRevision.value++
  dimensions.value = ''
  previewError.value = ''
  exportError.value = ''
  saveError.value = ''
  if (!canvas.value || !fontsReady.value || loading.value || failures.value.length || !draft.title.trim()) return
  const images: Partial<Record<PageImageSlot, HTMLImageElement>> = {}
  for (const slot of activeSlots.value) images[slot] = loaded.value.get(draft.images[slot])
  try {
    drawProductPage(canvas.value, draft, images)
    dimensions.value = `${canvas.value.width} × ${canvas.value.height} px`
  } catch (error) { previewError.value = error instanceof Error ? error.message : '无法绘制预览，请重试。' }
}, { flush: 'post' })

onMounted(async () => {
  previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  dialog.value?.showModal()
  try {
    await document.fonts?.load('750 76px "Manrope Variable"')
    await document.fonts?.load('500 40px "Manrope Variable"')
  } catch { /* The browser's fallback fonts are available for preview and export. */ }
  if (mounted) fontsReady.value = true
})
onBeforeUnmount(() => {
  mounted = false
  dialog.value?.close()
  previousFocus?.focus()
})

function applyImage(ids: string[]) {
  if (picker.value && ids[0]) draft.images[picker.value] = ids[0]
  picker.value = undefined
}

function previewPNG(preview: HTMLCanvasElement) {
  return new Promise<Blob>((resolve, reject) => {
    preview.toBlob(blob => {
      if (blob) resolve(blob)
      else reject(new Error('无法生成 PNG，请重试。'))
    }, 'image/png')
  })
}

async function saveToLibrary() {
  if (!props.assetAccess || !canExport.value || !canvas.value || currentSaved.value) return
  saving.value = true
  saveError.value = ''
  exportError.value = ''
  const revision = previewRevision.value
  const name = pageFilename(props.sku, draft.title, draft.template)
  const preview = canvas.value
  try {
    if (Math.max(preview.width, preview.height) > props.limits.maxDimension || preview.width * preview.height > props.limits.maxPixels) {
      throw new Error('成品尺寸超过素材库上限，请缩短内容或下载 PNG。')
    }
    const blob = await previewPNG(preview)
    if (!mounted) return
    if (blob.size > props.limits.maxUploadBytes) throw new Error('成品超过素材库单张大小上限，可以直接下载 PNG。')
    const form = new FormData()
    form.append('file', blob, name)
    await request<{ data: Asset }>(`/pod/product-jobs/${props.projectId}/page-assets`, { method: 'POST', body: form }, props.token)
    if (mounted) {
      savedRevision.value = revision
      emit('saved')
    }
  } catch (error) {
    if (!mounted) return
    if (error instanceof APIError && error.code === 401) emit('expired')
    saveError.value = error instanceof Error ? error.message : '保存成品失败，请重试。'
  } finally { if (mounted) saving.value = false }
}

async function download() {
  if (!canExport.value || !canvas.value) return
  exporting.value = true
  exportError.value = ''
  saveError.value = ''
  const name = pageFilename(props.sku, draft.title, draft.template)
  const preview = canvas.value
  try {
    const blob = await previewPNG(preview)
    if (!mounted) return
    const url = URL.createObjectURL(blob), link = document.createElement('a')
    link.href = url
    link.download = name
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch {
    if (mounted) exportError.value = '图片导出失败，请重试。'
  } finally { if (mounted) exporting.value = false }
}
</script>

<template>
  <dialog ref="dialog" class="product-page-dialog" aria-labelledby="product-page-title" @cancel.prevent="!saving && emit('close')">
    <header class="product-page-header">
      <h2 id="product-page-title">卖点图 / 详情页</h2>
      <button class="icon-button" type="button" aria-label="关闭排版编辑器" :disabled="saving" @click="emit('close')"><PhX :size="21" /></button>
    </header>
    <nav class="product-page-mobile" aria-label="编辑器视图">
      <button class="secondary-button" type="button" :aria-pressed="mobilePanel === 'edit'" @click="mobilePanel = 'edit'">编辑</button>
      <button class="secondary-button" type="button" :aria-pressed="mobilePanel === 'preview'" @click="mobilePanel = 'preview'">预览</button>
    </nav>
    <div class="product-page-body" :data-panel="mobilePanel">
      <div class="product-page-fields">
        <fieldset class="product-page-templates">
          <legend>版式</legend>
          <label v-for="(template, key) in pageTemplates" :key="key">
            <input v-model="draft.template" type="radio" name="product-page-template" :value="key" />
            {{ template.label }}
          </label>
        </fieldset>
        <label>标题 <input v-model="draft.title" :maxlength="pageTextLimits.title" required /></label>
        <label>规格 <textarea v-model="draft.specs" :maxlength="pageTextLimits.specs" rows="2" placeholder="例如 SKU、尺寸、材质" /></label>
        <label>卖点 <textarea v-model="draft.points" :maxlength="pageTextLimits.points" rows="3" placeholder="每行写一条卖点" /></label>
        <section class="product-page-slots" aria-label="配图">
          <div v-for="slot in activeSlots" :key="slot" class="product-page-slot">
            <PhImage :size="18" aria-hidden="true" />
            <span>{{ pageImageLabels[slot] }}<small>{{ failures.includes(draft.images[slot]) ? '读取失败' : draft.images[slot] ? '已选择' : slot === 'hero' ? '必选' : '选填' }}</small></span>
            <button v-if="assetAccess" class="text-button" type="button" :aria-label="(draft.images[slot] ? '更换' : '选择') + pageImageLabels[slot]" @click="picker = slot">{{ draft.images[slot] ? '更换' : '选择' }}</button>
            <button v-if="draft.images[slot]" class="icon-button" type="button" :aria-label="'移除' + pageImageLabels[slot]" @click="draft.images[slot] = ''"><PhX :size="15" /></button>
          </div>
        </section>
      </div>
      <section class="product-page-preview" aria-label="图片预览">
        <p v-if="loading || !fontsReady" class="product-page-placeholder" role="status">正在准备预览…</p>
        <div v-else-if="failures.length" class="product-page-placeholder" role="alert">
          <p>部分图片无法读取，请重试或更换这张图。</p>
          <button class="secondary-button" type="button" @click="retry++"><PhArrowClockwise :size="17" />重试</button>
        </div>
        <p v-else-if="previewError" class="product-page-placeholder" role="alert">{{ previewError }}</p>
        <p v-else-if="!draft.title.trim()" class="product-page-placeholder">填写标题后可预览。</p>
        <canvas v-show="dimensions" ref="canvas" role="img" :aria-label="draft.title + ' · ' + pageTemplates[draft.template].label + '预览'" />
      </section>
    </div>
    <footer class="product-page-footer">
      <div>
        <p v-if="saveError || exportError" class="error-message" role="alert">{{ saveError || exportError }}</p>
        <p v-else-if="saveUnavailable" class="error-message" role="status">浏览器无法保存草稿</p>
        <p v-else>草稿自动保存到本机浏览器</p>
        <small v-if="!draft.images.hero">选择主图后可导出</small>
        <small v-else-if="dimensions">{{ dimensions }}</small>
        <button v-if="savedRevision" class="text-button product-page-see-saved" type="button" :disabled="saving" @click="emit('viewSaved')">查看排版成品</button>
        <span v-if="currentSaved" class="sr-only" role="status">已保存到素材库</span>
      </div>
      <div class="product-page-footer-actions">
        <button :class="assetAccess ? 'secondary-button' : 'primary-button'" type="button" :disabled="!canExport" @click="download"><PhDownloadSimple :size="18" />{{ exporting ? '正在导出…' : '下载 PNG' }}</button>
        <button v-if="assetAccess" class="primary-button" type="button" :disabled="!canExport || currentSaved" @click="saveToLibrary">{{ saving ? '正在保存…' : currentSaved ? '已保存到素材库' : '保存到素材库' }}</button>
      </div>
    </footer>
    <AssetPicker v-if="picker" single :token="token" :limits="limits" :initial="draft.images[picker] ? [draft.images[picker]] : []" :title="'选择' + pageImageLabels[picker]" confirm-label="使用这张图" @close="picker = undefined" @confirm="applyImage" @expired="emit('expired')" />
  </dialog>
</template>

<style src="./product-page-editor.css"></style>
