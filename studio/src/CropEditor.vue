<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { cropConfig, cropPatch, cropPixels, drawnCrop, movedCrop, resizedCrop, rotatedSize, rotationTransform, validCrop } from './image-geometry'
import type { Corner, Crop, Point } from './image-geometry'
import { usePrivateAsset } from './use-private-asset'
const props = defineProps<{ assetId: string; token: string; config: Record<string, unknown>; disabled?: boolean }>()
const emit = defineEmits<{ patch: [values: Record<string, number>]; gesture: [active: boolean]; expired: [] }>()
const { asset, src, loading, error, load, decodeError } = usePrivateAsset(() => props.assetId, () => props.token, () => emit('expired'))
const svg = ref<SVGSVGElement>(), draft = ref<Crop | null>(null), drawMode = ref(false), message = ref('')
const rotation = computed(() => Number(props.config.rotate || 0))
const size = computed(() => rotatedSize(asset.value?.width || 1, asset.value?.height || 1, rotation.value))
const rectangle = computed(() => draft.value || cropConfig(props.config))
const pixels = computed(() => cropPixels(rectangle.value, size.value.width, size.value.height))
const valid = computed(() => validCrop(rectangle.value) && pixels.value.width >= 1 && pixels.value.height >= 1)
const ready = computed(() => !!src.value && !props.disabled)
const handleSize = computed(() => Math.max(size.value.width, size.value.height) * .045)
const corners = computed(() => ([['nw', rectangle.value.x, rectangle.value.y], ['ne', rectangle.value.x + rectangle.value.width, rectangle.value.y], ['sw', rectangle.value.x, rectangle.value.y + rectangle.value.height], ['se', rectangle.value.x + rectangle.value.width, rectangle.value.y + rectangle.value.height]] as const))
const shade = computed(() => {
  const r = rectangle.value, w = size.value.width, h = size.value.height
  return `M0 0H${w}V${h}H0Z M${r.x * w} ${r.y * h}h${r.width * w}v${r.height * h}h${-r.width * w}Z`
})
let drag: { pointer: number; start: Point; rect: Crop; mode: Corner | 'draw' | 'move' } | null = null
function point(event: PointerEvent): Point | null {
  const box = svg.value?.getBoundingClientRect()
  if (!box?.width || !box.height) return null
  const scale = Math.min(box.width / size.value.width, box.height / size.value.height)
  return { x: (event.clientX - box.left - (box.width - size.value.width * scale) / 2) / (size.value.width * scale), y: (event.clientY - box.top - (box.height - size.value.height * scale) / 2) / (size.value.height * scale) }
}
function cancel() {
  if (!drag) return
  const pointer = drag.pointer; drag = null; draft.value = null
  if (svg.value?.hasPointerCapture(pointer)) svg.value.releasePointerCapture(pointer)
  emit('gesture', false)
}
function start(event: PointerEvent) {
  if (!ready.value || drag || (event.button !== 0 && event.button !== undefined)) return
  const p = point(event)
  if (!p || p.x < 0 || p.x > 1 || p.y < 0 || p.y > 1) return
  const r = cropConfig(props.config), target = event.target as Element
  const corner = target.closest('[data-corner]')?.getAttribute('data-corner') as Corner | undefined
  const full = r.width === 1 && r.height === 1
  const mode = drawMode.value || !validCrop(r) ? 'draw' : corner || (!full && target.closest('[data-crop-body]') ? 'move' : 'draw')
  drag = { pointer: event.pointerId, start: p, rect: r, mode }; message.value = ''
  draft.value = mode === 'draw' ? drawnCrop(p, p) : { ...r }
  svg.value?.setPointerCapture(event.pointerId); svg.value?.focus(); emit('gesture', true); event.preventDefault()
}
function move(event: PointerEvent) {
  if (!drag || event.pointerId !== drag.pointer || !ready.value) return
  const p = point(event); if (!p) return
  draft.value = drag.mode === 'draw' ? drawnCrop(drag.start, p) : drag.mode === 'move' ? movedCrop(drag.rect, { x: p.x - drag.start.x, y: p.y - drag.start.y }) : resizedCrop(drag.rect, drag.mode, p, 1 / size.value.width, 1 / size.value.height)
}
function finish(event: PointerEvent) {
  if (!drag || event.pointerId !== drag.pointer) return
  move(event)
  if (ready.value && draft.value && valid.value) { emit('patch', cropPatch(draft.value)); drawMode.value = false }
  else message.value = '选框至少保留 1 × 1 像素，请重新框选。'
  cancel()
}
function reset() { cancel(); message.value = ''; drawMode.value = false; emit('patch', cropPatch({ x: 0, y: 0, width: 1, height: 1 })) }
function key(event: KeyboardEvent) {
  if (event.key === 'Escape' && drag) { event.preventDefault(); event.stopPropagation(); cancel(); return }
  if (drag || !ready.value || !valid.value || !event.key.startsWith('Arrow')) return
  event.preventDefault(); event.stopPropagation()
  const step = event.shiftKey ? .1 : .01, direction = { ArrowLeft: { x: -step, y: 0 }, ArrowRight: { x: step, y: 0 }, ArrowUp: { x: 0, y: -step }, ArrowDown: { x: 0, y: step } }[event.key]
  if (direction) emit('patch', cropPatch(movedCrop(rectangle.value, direction)))
}
watch(() => [props.assetId, props.token, rotation.value, props.disabled, src.value], cancel)
onBeforeUnmount(cancel)
</script>
<template>
  <section class="crop-editor" aria-label="可视化裁剪">
    <div class="image-tool-actions"><button type="button" class="text-button" :disabled="!ready" :aria-pressed="drawMode" @click="cancel(); drawMode = !drawMode">{{ drawMode ? '切回移动选框' : '重新框选' }}</button><button type="button" class="text-button" :disabled="disabled" @click="reset">恢复全图</button></div>
    <svg v-if="src && asset" ref="svg" class="crop-surface transparency-grid" :class="{ 'crop-drawing': drawMode }" :viewBox="`0 0 ${size.width} ${size.height}`" role="group" aria-label="裁剪图片，拖动框选或移动选框，方向键微调" tabindex="0" @pointerdown="start" @pointermove="move" @pointerup="finish" @pointercancel="cancel" @lostpointercapture="cancel" @keydown="key">
      <image :href="src" :width="asset.width" :height="asset.height" :transform="rotationTransform(asset.width, asset.height, rotation)" @error="decodeError" />
      <template v-if="validCrop(rectangle)"><path :d="shade" fill-rule="evenodd" class="crop-shade" pointer-events="none" /><rect data-crop-body :x="rectangle.x * size.width" :y="rectangle.y * size.height" :width="rectangle.width * size.width" :height="rectangle.height * size.height" class="crop-selection" vector-effect="non-scaling-stroke" />
        <rect v-for="[corner, x, y] in corners" :key="corner" :data-corner="corner" :x="x * size.width - handleSize / 2" :y="y * size.height - handleSize / 2" :width="handleSize" :height="handleSize" class="crop-handle" :class="`crop-${corner}`" vector-effect="non-scaling-stroke" />
      </template>
    </svg>
    <p v-else class="image-tool-placeholder" role="status">{{ loading ? '正在读取裁剪原图…' : error || '先连接图片输入，或执行上游图片节点，再框选裁剪。' }}<button v-if="error" type="button" class="text-button" @click="load">重新读取</button></p>
    <p v-if="asset && valid" class="image-tool-caption" role="status">保留 {{ pixels.width }} × {{ pixels.height }} px · 旋转后 {{ size.width }} × {{ size.height }} px</p>
    <p class="muted">拖出新选框，框内拖动位置，四角调整大小。方向键微调，Shift 加快；也可修改下方数字。多图使用相同比例区域。</p>
    <p v-if="message || (src && !valid && !drag)" class="error-message" role="alert">{{ message || '裁剪区域超出图片或不足 1 像素，请调整数字或恢复全图。' }}</p>
  </section>
</template>
