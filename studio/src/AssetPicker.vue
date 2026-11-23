<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhX } from '@phosphor-icons/vue'
import Assets from './Assets.vue'
import type { Asset, AssetLimits } from './contracts'
const props = defineProps<{ token: string; limits: AssetLimits; initial: string[]; single?: boolean; title?: string; confirmLabel?: string }>()
const emit = defineEmits<{ close: []; confirm: [ids: string[]]; expired: [] }>()
const selected = ref([...props.initial]), error = ref(''), dialog = ref<HTMLDialogElement>()
let focus: HTMLElement | null = null
function choose(asset: Asset) {
  error.value = ''
  if (props.single) { selected.value = selected.value.includes(asset.assetId) ? [] : [asset.assetId]; return }
  if (selected.value.includes(asset.assetId)) selected.value = selected.value.filter(id => id !== asset.assetId)
  else if (selected.value.length >= props.limits.maxImagesPerNode) error.value = `一个图片节点最多选择 ${props.limits.maxImagesPerNode} 张图片`
  else selected.value.push(asset.assetId)
}
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
</script>

<template>
  <dialog ref="dialog" class="asset-picker-dialog" aria-labelledby="asset-picker-title" @cancel.prevent="emit('close')">
    <header class="asset-dialog-header"><div><h2 id="asset-picker-title">{{ title || (single ? '选择商品参考图' : '选择图片素材') }}</h2><p v-if="!single">每个节点最多 {{ limits.maxImagesPerNode }} 张，按选择顺序保存。</p></div><button class="icon-button" aria-label="关闭素材选择" @click="emit('close')"><PhX :size="21" /></button></header>
    <Assets :token="token" :limits="limits" picker :single="single" :selected-ids="selected" @select="choose" @expired="emit('expired')" />
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <footer><span>已选 {{ selected.length }} / {{ single ? 1 : limits.maxImagesPerNode }} 张</span><button class="text-button" @click="selected = []; error = ''">清空选择</button><button class="secondary-button" @click="emit('close')">取消</button><button class="primary-button" :disabled="single && !selected.length" @click="emit('confirm', selected)">{{ confirmLabel || (single ? '用作参考图' : '加入节点') }}</button></footer>
  </dialog>
</template>
