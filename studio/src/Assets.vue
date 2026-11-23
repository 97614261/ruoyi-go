<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowClockwise, PhArrowCounterClockwise, PhCheck, PhImage, PhTrash, PhUploadSimple } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { Asset, AssetLimits } from './contracts'
import PrivateImage from './PrivateImage.vue'
import AssetPreview from './AssetPreview.vue'

const props = withDefaults(defineProps<{ token: string; limits: AssetLimits; picker?: boolean; single?: boolean; selectedIds?: string[]; productProjectId?: string }>(), { picker: false, selectedIds: () => [] })
const emit = defineEmits<{ expired: []; select: [asset: Asset] }>()
const rows = ref<Asset[]>([]), total = ref(0), page = ref(1), deleted = ref(false)
const loading = ref(false), busy = ref(false), error = ref(''), notice = ref(''), fileInput = ref<HTMLInputElement>(), preview = ref<Asset | null>(null)
let sequence = 0, alive = true
function report(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
async function load() {
  const current = ++sequence; loading.value = true; error.value = ''
  try {
    const path = props.productProjectId ? `/pod/product-jobs/${props.productProjectId}/page-assets` : '/pod/assets'
    const result = await request<{ rows: Asset[]; total: number }>(`${path}?pageNum=${page.value}&pageSize=24&deleted=${deleted.value}`, {}, props.token)
    if (!alive || current !== sequence) return
    if (!result.rows.length && page.value > 1) { page.value--; await load(); return }
    rows.value = result.rows; total.value = result.total
  } catch (e) { if (alive && current === sequence) report(e) }
  finally { if (alive && current === sequence) loading.value = false }
}
function tab(value: boolean) { deleted.value = value; page.value = 1; rows.value = []; notice.value = ''; void load() }
async function upload(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]; input.value = ''
  if (!file || busy.value) return
  error.value = ''; notice.value = ''
  if (!/\.(png|jpe?g|webp)$/i.test(file.name) || file.size > props.limits.maxUploadBytes) { error.value = `请选择 PNG、JPEG 或 WebP 图片，最大 ${(props.limits.maxUploadBytes / 1024 / 1024).toFixed(0)} MB`; return }
  busy.value = true
  const form = new FormData(); form.append('file', file)
  try {
    const result = await request<{ data: Asset }>('/pod/assets', { method: 'POST', body: form }, props.token)
    if (!alive) return
    deleted.value = false; page.value = 1; await load(); notice.value = `已上传 ${result.data.originalName}`
    if (props.picker && !props.selectedIds.includes(result.data.assetId)) emit('select', result.data)
  } catch (e) { if (alive) report(e) } finally { if (alive) busy.value = false }
}
async function deletion(asset: Asset) {
  if (busy.value) return
  if (!deleted.value && !window.confirm(`将“${asset.originalName}”移入回收站？可以恢复，被项目引用的素材无法删除。`)) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await request(`/pod/assets/${asset.assetId}${deleted.value ? '/restore' : ''}`, { method: deleted.value ? 'POST' : 'DELETE', body: JSON.stringify({ revision: asset.revision }) }, props.token); await load() }
  catch (e) { report(e) } finally { busy.value = false }
}
onMounted(load)
onBeforeUnmount(() => { alive = false; sequence++ })
</script>

<template>
  <section class="asset-library" :class="{ 'asset-library-picker': picker }" :aria-label="productProjectId ? '排版成品' : picker ? '选择素材' : '我的素材'" :aria-busy="busy">
    <div v-if="!productProjectId" class="asset-library-heading"><div><h1 v-if="!picker">我的素材</h1><p v-if="picker && !single">选择图片后加入当前节点，可跨页选择。</p></div><button class="primary-button" :disabled="busy" @click="fileInput?.click()"><PhUploadSimple :size="18" />{{ busy ? '正在处理…' : '上传图片' }}</button><input ref="fileInput" class="sr-only" type="file" accept=".png,.jpg,.jpeg,.webp" aria-label="选择上传图片" :disabled="busy" @change="upload" /></div>
    <div v-if="!productProjectId" class="project-tabs"><div><button :class="{ active: !deleted }" :aria-pressed="!deleted" :disabled="busy" @click="tab(false)">图片</button><button v-if="!picker" :class="{ active: deleted }" :aria-pressed="deleted" :disabled="busy" @click="tab(true)"><PhTrash :size="15" />回收站</button></div><button class="icon-button" aria-label="刷新素材列表" :disabled="loading || busy" @click="load"><PhArrowClockwise :size="18" /></button></div>
    <p v-if="error" class="error-message" role="alert">{{ error }} <button v-if="productProjectId" type="button" class="text-button" @click="load">重试</button></p><p v-if="notice" class="asset-notice" role="status">{{ notice }}</p>
    <p v-if="loading" class="muted" role="status">正在读取素材…</p>
    <div v-else-if="rows.length" class="asset-grid">
      <article v-for="asset in rows" :key="asset.assetId" class="asset-item" :class="{ 'asset-selected': selectedIds.includes(asset.assetId) }">
        <button v-if="!deleted" class="asset-cover" :aria-label="`${picker ? '选择' : '预览'} ${asset.originalName}`" :aria-pressed="picker ? selectedIds.includes(asset.assetId) : undefined" @click="picker ? emit('select', asset) : preview = asset"><PrivateImage :asset-id="asset.assetId" :token="token" :alt="asset.originalName" :retry="false" @expired="emit('expired')" /><span v-if="!picker && asset.source === 'product-page'" class="asset-source-mark">商品排版</span><span v-else-if="!picker && ['generated', 'processed'].includes(asset.source)" class="asset-source-mark">{{ asset.source === 'generated' ? 'AI 生成' : '处理结果' }}</span><span v-if="picker && selectedIds.includes(asset.assetId)" class="asset-selected-mark"><PhCheck :size="17" weight="bold" /></span></button>
        <div v-else class="asset-trashed"><PhImage :size="35" weight="light" /><span>原图保留，可恢复</span></div>
        <div class="asset-item-info"><strong :title="asset.originalName">{{ asset.originalName }}</strong><p>{{ asset.width }} × {{ asset.height }} px<span>{{ (asset.byteSize / 1024 / 1024).toFixed(2) }} MB</span></p><div class="asset-item-actions"><small>{{ asset.referenceCount ? `${asset.referenceCount} 处项目引用` : '未被项目引用' }}</small><button v-if="!picker" class="icon-button" :disabled="busy || (!deleted && asset.referenceCount > 0)" :aria-label="`${deleted ? '恢复' : '移入回收站'} ${asset.originalName}`" :title="asset.referenceCount ? '先从项目中移除后再删除，回收站项目也保留引用' : undefined" @click="deletion(asset)"><PhArrowCounterClockwise v-if="deleted" :size="17" /><PhTrash v-else :size="17" /></button></div></div>
      </article>
    </div>
    <div v-else-if="productProjectId && !error" class="product-page-empty">暂无成品，可在「卖点图 / 详情页」里保存。</div>
    <div v-else-if="!productProjectId" class="asset-empty"><PhImage :size="44" weight="light" /><h2>{{ deleted ? '素材回收站是空的' : '放入你的第一张图片' }}</h2><p>{{ deleted ? '删除的图片会保留原图，恢复后可继续使用。' : '上传参考图或印花图案，再把它们放进工作流。' }}</p></div>
    <div v-if="total > 24" class="project-pagination"><button class="secondary-button" :disabled="busy || loading || page === 1" @click="page--; load()">上一页</button><span>第 {{ page }} / {{ Math.ceil(total / 24) }} 页，共 {{ total }} 张</span><button class="secondary-button" :disabled="busy || loading || page * 24 >= total" @click="page++; load()">下一页</button></div>
    <details v-if="!productProjectId" class="asset-specifications"><summary>上传规格</summary><p class="asset-footnote">PNG / JPEG / WebP，单张不超过 {{ (limits.maxUploadBytes / 1024 / 1024).toFixed(0) }} MB、{{ Math.round(limits.maxPixels / 10000) }} 万像素，最长边 {{ limits.maxDimension }} px。原图保持不变。</p></details>
    <AssetPreview v-if="preview" :asset="preview" :token="token" @close="preview = null" @expired="emit('expired')" />
  </section>
</template>
