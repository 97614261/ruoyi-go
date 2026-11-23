<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { PhLightning } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { FormalPoints } from './contracts'

const props = defineProps<{ token: string; managed: boolean; refreshKey: string }>()
const emit = defineEmits<{ open: []; expired: [] }>()
const available = ref<string | null>(null)
let sequence = 0
watch(() => [props.token, props.managed, props.refreshKey], async () => {
  const current = ++sequence
  available.value = null
  if (!props.managed) return
  try {
    const result = await request<{ data: FormalPoints }>('/pod/points?pageNum=1&pageSize=1', {}, props.token)
    if (current === sequence) available.value = result.data.account.available
  } catch (error) { if (current === sequence && error instanceof APIError && error.code === 401) emit('expired') }
}, { immediate: true })
onBeforeUnmount(() => ++sequence)
</script>

<template>
  <button class="point-balance" :aria-label="available !== null ? `可用 ${available} 点，查看资格与点数` : '查看资格与点数'" @click="emit('open')"><PhLightning :size="17" weight="fill" aria-hidden="true" /><strong v-if="available !== null">{{ available }}</strong><span v-else>点数</span></button>
</template>
