<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { Handle } from '@vue-flow/core'
import type { Position } from '@vue-flow/core'
import { PhPlus } from '@phosphor-icons/vue'

const props = withDefaults(defineProps<{
  id: string; type: 'source' | 'target'; position: Position; top: string
  label: string; valueType: string; disabled?: boolean; following?: boolean; quick?: boolean; pressedMenu?: boolean; existingConnection?: boolean
}>(), { following: true })
const emit = defineEmits<{ quick: [anchor: HTMLElement] }>()
const followingPointer = ref(false)
let frame = 0, surface: HTMLElement | undefined, pressed = false
let initialPointer = { x: 0, y: 0 }, dragged = false
let offset = { x: 0, y: 0 }

function cancelFrame() { cancelAnimationFrame(frame); frame = 0 }
function reset() {
  cancelFrame(); followingPointer.value = false
  surface?.style.removeProperty('--port-follow-x'); surface?.style.removeProperty('--port-follow-y')
}
function move(event: PointerEvent) {
  if (pressed && Math.hypot(event.clientX - initialPointer.x, event.clientY - initialPointer.y) > 6) dragged = true
  if (!props.following || props.disabled || pressed || event.buttons || event.pointerType !== 'mouse') { reset(); return }
  const element = event.currentTarget as HTMLElement
  const bounds = element.getBoundingClientRect()
  if (!bounds.width || !element.offsetWidth) return
  // Work in canvas units, so zoom does not change the permitted travel radius.
  const zoom = bounds.width / element.offsetWidth
  const x = (event.clientX - bounds.left - bounds.width / 2) / zoom * .5
  const y = (event.clientY - bounds.top - bounds.height / 2) / zoom * .5
  const distance = Math.hypot(x, y), limit = 6
  offset = { x: distance > limit ? x * limit / distance : x, y: distance > limit ? y * limit / distance : y }
  surface = element
  if (frame) return
  frame = requestAnimationFrame(() => {
    frame = 0
    if (!props.following || props.disabled || pressed) return
    surface?.style.setProperty('--port-follow-x', `${offset.x.toFixed(2)}px`)
    surface?.style.setProperty('--port-follow-y', `${offset.y.toFixed(2)}px`)
    followingPointer.value = true
  })
}
function leave() { if (pressed) dragged = true; pressed = false; reset() }
function press(event: PointerEvent) { pressed = true; dragged = false; initialPointer = { x: event.clientX, y: event.clientY }; reset() }
function openQuick(event: MouseEvent) {
  if (!props.quick || event.shiftKey || props.existingConnection) return
  event.stopImmediatePropagation()
  const wasDrag = dragged && event.detail !== 0
  dragged = false
  if (!props.disabled && !wasDrag) emit('quick', event.currentTarget as HTMLElement)
}
function activate(event: KeyboardEvent) {
  if (props.disabled || event.repeat) return
  reset(); (event.currentTarget as HTMLElement).click()
}
watch(() => [props.following, props.disabled], () => { if (!props.following || props.disabled) reset() })
onBeforeUnmount(reset)
</script>

<template>
  <Handle :id="id" :type="type" :position="position" :connectable="!disabled" class="magnetic-handle"
    :class="{ 'is-following': followingPointer }" :style="{ top }" :data-type="valueType"
    :aria-label="label" :aria-disabled="disabled || undefined" :aria-expanded="quick ? !!pressedMenu : undefined" :tabindex="disabled ? -1 : 0" role="button"
    @pointermove="move" @pointerleave="leave" @pointercancel="leave" @pointerdown.capture="press"
    @pointerup="pressed = false" @focus="reset" @click.capture="openQuick" @click.stop
    @keydown.enter.prevent.stop="activate" @keydown.space.prevent.stop="activate">
    <span class="magnetic-handle-button" aria-hidden="true"><PhPlus :size="13" weight="bold" /></span>
    <span class="magnetic-handle-label" aria-hidden="true">{{ label }}</span>
  </Handle>
</template>
