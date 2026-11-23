<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowRight, PhPackage, PhPlus } from '@phosphor-icons/vue'

const props = defineProps<{ createDisabled?: boolean }>()
const emit = defineEmits<{ create: []; templates: []; product: [] }>()
const panel = ref<HTMLElement>()
let reducedMotion: MediaQueryList | undefined
let frame = 0
let pointerX = 0, pointerY = 0

function move(event: PointerEvent) {
  if (event.pointerType !== 'mouse' || props.createDisabled || reducedMotion?.matches) return
  pointerX = event.clientX; pointerY = event.clientY
  if (frame) return
  frame = requestAnimationFrame(() => {
    frame = 0
    const element = panel.value
    if (!element || props.createDisabled) return
    const bounds = element.getBoundingClientRect()
    element.style.setProperty('--launch-light-x', `${pointerX - bounds.left}px`)
    element.style.setProperty('--launch-light-y', `${pointerY - bounds.top}px`)
    element.classList.add('has-pointer-light')
  })
}
function leave() {
  if (frame) cancelAnimationFrame(frame)
  frame = 0
  panel.value?.classList.remove('has-pointer-light')
}
onMounted(() => { reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)') })
onBeforeUnmount(leave)
</script>

<template>
  <div ref="panel" class="creation-launchpad" :class="{ 'is-create-disabled': createDisabled }" @pointermove.passive="move" @pointerleave="leave" @pointercancel="leave">
    <span class="launch-light" aria-hidden="true" />
    <button class="launch-canvas" type="button" :disabled="createDisabled" @click="emit('create')"><span class="launch-plus"><PhPlus :size="32" /></span><strong>新建画布创作</strong></button>
    <button class="launch-template" type="button" @click="emit('templates')">从模板开始<PhArrowRight :size="16" /></button>
  </div>
  <button type="button" class="product-launch" @click="emit('product')"><PhPackage :size="21" /><span><strong>制作商品图</strong><small>商品图、印花和 T 恤样机</small></span><PhArrowRight :size="17" /></button>
</template>
