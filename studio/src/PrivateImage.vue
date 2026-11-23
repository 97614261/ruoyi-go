<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { PhImage } from '@phosphor-icons/vue'
import { APIError, requestImage } from './api'
const props = withDefaults(defineProps<{ assetId: string; token: string; alt: string; variant?: 'thumbnail' | 'original'; retry?: boolean }>(), { variant: 'thumbnail', retry: true })
const emit = defineEmits<{ expired: [] }>()
const src = ref(''), error = ref('')
let sequence = 0
function revoke() { if (src.value) URL.revokeObjectURL(src.value); src.value = '' }
async function load() {
  const current = ++sequence; revoke(); error.value = ''
  try {
    const image = await requestImage(`/pod/assets/${props.assetId}/content?variant=${props.variant}`, props.token)
    if (current === sequence) src.value = URL.createObjectURL(image)
  } catch (e) { if (current === sequence) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') } }
}
watch(() => [props.assetId, props.token, props.variant], load, { immediate: true })
onBeforeUnmount(() => { sequence++; revoke() })
</script>

<template>
  <div class="private-image transparency-grid"><img v-if="src" :src="src" :alt="alt" draggable="false" @error="error = '图片无法显示'; revoke()" /><div v-else class="private-image-placeholder" role="status"><PhImage :size="26" weight="light" /><span>{{ error || '正在读取图片…' }}</span><button v-if="error && retry" class="text-button" @click.stop="load">重新读取</button></div></div>
</template>
