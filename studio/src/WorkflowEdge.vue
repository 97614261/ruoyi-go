<script setup lang="ts">
import { computed } from 'vue'
import { BaseEdge, EdgeLabelRenderer, getBezierPath } from '@vue-flow/core'
import type { EdgeProps } from '@vue-flow/core'
import { PhX } from '@phosphor-icons/vue'

defineOptions({ inheritAttrs: false })
const props = defineProps<Pick<EdgeProps, 'id' | 'sourceX' | 'sourceY' | 'targetX' | 'targetY' | 'sourcePosition' | 'targetPosition' | 'markerStart' | 'markerEnd' | 'selected'> & { highlighted?: boolean; removable?: boolean; zoom?: number }>()
const emit = defineEmits<{ remove: [id: string] }>()
const curve = computed(() => getBezierPath(props))
const path = computed(() => curve.value[0])
const buttonPosition = computed(() => ({
  left: `${curve.value[1]}px`, top: `${curve.value[2]}px`,
  transform: `translate(-50%, -50%) scale(${1 / (props.zoom || 1)})`,
}))
// Stagger streams by identity, so parallel connections don't flash in unison.
const phase = computed(() => -([...props.id].reduce((hash, char) => (hash * 31 + char.charCodeAt(0)) >>> 0, 0) % 3400) / 1000)
</script>

<template>
  <g class="workflow-edge" :class="{ 'is-related': highlighted }" :style="{ '--edge-phase': `${phase}s` }">
    <BaseEdge :id="id" :path="path" :marker-start="markerStart" :marker-end="markerEnd" :interaction-width="24" />
    <path class="workflow-edge-halo" :d="path" fill="none" aria-hidden="true" pointer-events="none" />
    <path class="workflow-edge-stream" :d="path" pathLength="100" fill="none" aria-hidden="true" pointer-events="none" />
  </g>
  <EdgeLabelRenderer v-if="selected && removable">
    <button class="edge-disconnect nodrag nopan" type="button" :style="buttonPosition" aria-label="断开这条连线" title="断开这条连线" @mousedown.stop @touchstart.stop @click.stop="emit('remove', id)">
      <PhX :size="13" weight="bold" aria-hidden="true" />
    </button>
  </EdgeLabelRenderer>
</template>
