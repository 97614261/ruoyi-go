<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhX } from '@phosphor-icons/vue'
import type { Asset } from './contracts'
import { usePrivateAsset } from './use-private-asset'
const props = defineProps<{ result: Asset; token: string }>()
const emit = defineEmits<{ close: []; expired: [] }>()
const before = usePrivateAsset(() => props.result.parentAssetId, () => props.token, () => emit('expired'))
const after = usePrivateAsset(() => props.result.assetId, () => props.token, () => emit('expired'))
const dialog = ref<HTMLDialogElement>(), mode = ref('both'), zoom = ref(0)
const panels = computed(() => [
  { key: 'before', label: '处理前 · 直接输入', image: before },
  { key: 'after', label: '处理后 · 本次结果', image: after },
].filter(panel => mode.value === 'both' || mode.value === panel.key))
let focus: HTMLElement | null = null
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="image-compare-dialog" aria-labelledby="image-compare-title" @cancel.prevent="emit('close')">
    <header class="asset-dialog-header"><div><h2 id="image-compare-title">处理前后对比</h2><p>{{ result.originalName }} · 棋盘格表示透明区域</p></div><button class="icon-button" aria-label="关闭图片对比" @click="emit('close')"><PhX :size="21" /></button></header>
    <div class="compare-toolbar"><div class="image-tool-actions" role="group" aria-label="对比显示方式"><button v-for="[value, label] in [['both', '并排对比'], ['before', '只看处理前'], ['after', '只看处理后']]" :key="value" class="secondary-button" :aria-pressed="mode === value" @click="mode = value!">{{ label }}</button></div><label>查看比例<select v-model.number="zoom"><option :value="0">适应窗口</option><option :value="1">100% · 实际像素</option><option :value="2">200%</option><option :value="4">400%</option></select></label></div>
    <div class="compare-panels" :class="{ 'compare-single': panels.length === 1 }">
      <section v-for="panel in panels" :key="panel.key" class="compare-panel"><header><strong>{{ panel.label }}</strong><span v-if="panel.image.asset.value">{{ panel.image.asset.value.width }} × {{ panel.image.asset.value.height }} px</span></header>
        <div class="compare-viewport transparency-grid" :class="{ 'compare-fit': !zoom, 'compare-pixel': zoom > 0 }" tabindex="0" :aria-label="`${panel.label}图片，可滚动检查`">
          <img v-if="panel.image.src.value && panel.image.asset.value" :src="panel.image.src.value" :alt="panel.label" :width="panel.image.asset.value.width" :height="panel.image.asset.value.height" :style="zoom ? { width: `${panel.image.asset.value.width * zoom}px`, height: `${panel.image.asset.value.height * zoom}px` } : undefined" @error="panel.image.decodeError" />
          <p v-else role="status">{{ panel.image.loading.value ? '正在读取原图…' : panel.image.error.value || '图片不可用' }}<button v-if="panel.image.error.value" class="text-button" @click="panel.image.load">重新读取</button></p>
        </div>
      </section>
    </div>
    <footer>处理前来自本次结果的直接输入。裁剪、旋转或放大后尺寸可能不同；100% 按图片像素显示。</footer>
  </dialog>
</template>
