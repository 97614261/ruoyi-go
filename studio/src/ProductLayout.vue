<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { usePrivateAsset } from './use-private-asset'
import { printGeometry } from './image-geometry'
const props = defineProps<{ token: string; assetId: string; config: Record<string, unknown>; disabled?: boolean }>()
const emit = defineEmits<{ config: [key: string, value: unknown]; patch: [values: Record<string, number>]; expired: []; gesture: [active: boolean] }>()
const { asset, src, loading, error, load, decodeError } = usePrivateAsset(() => props.assetId, () => props.token, () => emit('expired'))
const svg = ref<SVGSVGElement>(), draft = ref<Record<string, number> | null>(null)
const placement = computed(() => ({ ...props.config, ...draft.value }))
const geometry = computed(() => printGeometry(asset.value?.width || 1, asset.value?.height || 1, placement.value))
const w = computed(() => geometry.value.width), h = computed(() => geometry.value.height)
const x = computed(() => geometry.value.x), y = computed(() => geometry.value.y)
let dragging = false, pointer = 0, offsetX = 0, offsetY = 0
function point(event: PointerEvent) { const rect = (event.currentTarget as SVGSVGElement).getBoundingClientRect(); const ratio = Math.min(rect.width / 1000, rect.height / 1200); return { x: (event.clientX - rect.left - (rect.width - ratio * 1000) / 2) / ratio, y: (event.clientY - rect.top - (rect.height - ratio * 1200) / 2) / ratio } }
function start(event: PointerEvent) { if (props.disabled || !src.value || dragging || (event.button !== 0 && event.button !== undefined)) return; const p = point(event); if (p.x < x.value || p.x > x.value + w.value || p.y < y.value || p.y > y.value + h.value) return; dragging = true; pointer = event.pointerId; emit('gesture', true); offsetX = p.x - x.value; offsetY = p.y - y.value; svg.value?.setPointerCapture(pointer); svg.value?.focus(); event.preventDefault() }
function stop() { if (!dragging) return; dragging = false; draft.value = null; if (svg.value?.hasPointerCapture(pointer)) svg.value.releasePointerCapture(pointer); emit('gesture', false) }
function finish(event: PointerEvent) { if (!dragging || event.pointerId !== pointer) return; move(event); if (draft.value) emit('patch', draft.value); stop() }
function move(event: PointerEvent) { if (!dragging || props.disabled || event.pointerId !== pointer) return; const p = point(event); draft.value = { printX: Math.round(Math.max(0, Math.min(1, (p.x - offsetX - 325) / Math.max(1, 350 - w.value))) * 100) / 100, printY: Math.round(Math.max(0, Math.min(1, (p.y - offsetY - 300) / Math.max(1, 550 - h.value))) * 100) / 100 } }
function key(event: KeyboardEvent) { if (event.key === 'Escape' && dragging) { event.preventDefault(); event.stopPropagation(); stop(); return }; if (dragging || props.disabled || !src.value || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return; event.preventDefault(); event.stopPropagation(); const field = ['ArrowLeft', 'ArrowRight'].includes(event.key) ? 'printX' : 'printY'; emit('config', field, Math.max(0, Math.min(1, Number(props.config[field] ?? (field === 'printY' ? .4 : .5)) + (['ArrowLeft', 'ArrowUp'].includes(event.key) ? -1 : 1) * (event.shiftKey ? .1 : .01)))) }
function patch(values: Record<string, number>) { stop(); emit('patch', values) }
watch(() => [props.assetId, props.token, props.disabled, src.value], stop)
onBeforeUnmount(stop)
</script>
<template>
  <div class="product-layout">
    <div class="image-tool-actions"><button type="button" class="text-button" :disabled="disabled" @click="patch({ printX: .5 })">水平居中</button><button type="button" class="text-button" :disabled="disabled" @click="patch({ printY: .5 })">垂直居中</button><button type="button" class="text-button" :disabled="disabled" @click="patch({ printX: .5, printY: .5 })">全部居中</button><button type="button" class="text-button" :disabled="disabled" @click="patch({ printX: .5, printY: .4, printScale: .6 })">布局复位</button></div>
    <svg ref="svg" viewBox="0 0 1000 1200" role="group" aria-label="T 恤印花位置，拖动印花或使用方向键调整" tabindex="0" @pointerdown="start" @pointermove="move" @pointerup="finish" @pointercancel="stop" @lostpointercapture="stop" @keydown="key">
      <rect width="1000" height="1200" fill="#f1f4f8" />
      <path d="M350 120L430 100L570 100L650 120L870 300L755 445L685 390L685 1060L315 1060L315 390L245 445L130 300Z" :fill="String(config.color || '#ffffff')" stroke="#b5beca" stroke-width="3" />
      <path :d="config.side === 'back' ? 'M430 100Q500 145 570 100Z' : 'M430 100C440 220 560 220 570 100Z'" fill="#e1e6ec" />
      <path d="M330 410V1030M670 410V1030M330 1040H670" fill="none" stroke="#828c96" stroke-opacity=".28" />
      <rect x="325" y="300" width="350" height="550" fill="none" stroke="#6c89b1" stroke-width="3" stroke-dasharray="10 10" />
      <image v-if="src" :href="src" :x="x" :y="y" :width="w" :height="h" preserveAspectRatio="none" @error="decodeError" />
    </svg>
    <p>{{ assetId ? '拖动印花调整位置，虚线为最大印刷区域。' : '连接图片输入或已执行的图片节点，查看印花位置。' }}二维效果示意。</p>
    <p v-if="src" class="image-tool-caption">横向 {{ Math.round(Number(placement.printX ?? .5) * 100) }}% · 纵向 {{ Math.round(Number(placement.printY ?? .4) * 100) }}%<br>预览坐标 {{ x }}, {{ y }} · {{ w }} × {{ h }} px（1000 × 1200 画布）</p>
    <p v-if="loading" role="status">正在读取印花…</p>
    <p v-if="error" class="error-message" role="alert">{{ error }} <button class="text-button" @click="load">重新读取</button></p>
  </div>
</template>
