<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhDownloadSimple, PhX } from '@phosphor-icons/vue'
import { APIError, requestImage } from './api'
import type { Asset } from './contracts'
import PrivateImage from './PrivateImage.vue'
const props = defineProps<{ asset: Asset; token: string }>()
const emit = defineEmits<{ close: []; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), busy = ref(false), error = ref('')
let focus: HTMLElement | null = null
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
async function download() {
  busy.value = true; error.value = ''
  try {
    const blob = await requestImage(`/pod/assets/${props.asset.assetId}/download`, props.token)
    const url = URL.createObjectURL(blob), link = document.createElement('a'); link.href = url; link.download = props.asset.originalName; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
  finally { busy.value = false }
}
</script>

<template>
  <dialog ref="dialog" class="asset-preview-dialog" aria-labelledby="asset-preview-title" @cancel.prevent="emit('close')">
    <header class="asset-dialog-header"><div><h2 id="asset-preview-title">{{ asset.originalName }}</h2><p>{{ asset.width }} × {{ asset.height }} px · {{ (asset.byteSize / 1024 / 1024).toFixed(2) }} MB · 原图</p></div><button class="icon-button" aria-label="关闭预览" @click="emit('close')"><PhX :size="21" /></button></header>
    <PrivateImage :asset-id="asset.assetId" :token="token" :alt="asset.originalName" variant="original" @expired="emit('expired')" />
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <footer><span>下载保留原始文件与透明通道</span><button class="primary-button" :disabled="busy" @click="download"><PhDownloadSimple :size="17" />{{ busy ? '正在下载…' : '下载原图' }}</button></footer>
  </dialog>
</template>
