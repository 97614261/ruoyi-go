<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { APIError, request, requestImage } from './api'
import type { Asset, ImageInfo } from './contracts'
import PrivateImage from './PrivateImage.vue'
import AssetPreview from './AssetPreview.vue'
import ImageCompareDialog from './ImageCompareDialog.vue'
const props = defineProps<{ ids: string[]; token: string; info?: ImageInfo[]; compare?: boolean }>()
const emit = defineEmits<{ expired: [] }>()
const preview = ref<Asset | null>(null), busy = ref(''), error = ref('')
const comparison = ref<Asset | null>(null)
let sequence = 0
watch(() => [props.ids.join(','), props.token, props.compare], () => { ++sequence; busy.value = ''; preview.value = null; comparison.value = null; error.value = '' })
onBeforeUnmount(() => { ++sequence })
async function use(id: string, action: 'preview' | 'download' | 'compare') {
  if (busy.value) return
  const current = ++sequence; busy.value = id; error.value = ''
  try {
    const { data } = await request<{ data: Asset }>(`/pod/assets/${id}`, {}, props.token)
    if (current !== sequence) return
    if (action === 'preview') { preview.value = data; return }
    if (action === 'compare') { if (!data.parentAssetId || data.parentAssetId === '0') throw new Error('这张结果没有可读取的直接输入，无法对比。'); comparison.value = data; return }
    const blob = await requestImage(`/pod/assets/${id}/download`, props.token)
    if (current !== sequence) return
    const url = URL.createObjectURL(blob), link = document.createElement('a'); link.href = url; link.download = data.originalName; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) { if (current === sequence) { error.value = e instanceof Error ? e.message : '无法读取结果'; if (e instanceof APIError && e.code === 401) emit('expired') } }
  finally { if (current === sequence) busy.value = '' }
}
</script>
<template>
  <div class="result-images nodrag nowheel" @click.stop>
    <figure v-for="(id, index) in ids" :key="id">
      <button class="result-image-open" :aria-label="`查看结果原图 ${index + 1}`" :disabled="!!busy" @click="use(id, 'preview')"><PrivateImage :asset-id="id" :token="token" alt="节点处理结果" @expired="emit('expired')" /></button>
      <figcaption><small v-if="info?.find(item => item.assetId === id)">{{ info.find(item => item.assetId === id)!.width }} × {{ info.find(item => item.assetId === id)!.height }} px<span v-if="info.find(item => item.assetId === id)!.effectiveDpi"> · {{ info.find(item => item.assetId === id)!.effectiveDpi }} DPI</span></small><button v-if="compare" class="text-button" :disabled="!!busy" :aria-label="`对比处理结果 ${index + 1}`" @click="use(id, 'compare')">对比</button><button class="text-button" :disabled="!!busy" @click="use(id, 'download')">下载</button></figcaption>
      <p v-if="info?.find(item => item.assetId === id)?.warning" class="image-warning">{{ info.find(item => item.assetId === id)!.warning }}</p>
    </figure>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <AssetPreview v-if="preview" :asset="preview" :token="token" @close="preview = null" @expired="emit('expired')" />
    <ImageCompareDialog v-if="comparison" :result="comparison" :token="token" @close="comparison = null" @expired="emit('expired')" />
  </div>
</template>
