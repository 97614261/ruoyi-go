<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PhX } from '@phosphor-icons/vue'
import type { Bootstrap } from './contracts'
import Templates from './Templates.vue'
defineProps<{ token: string; bootstrap: Bootstrap }>()
const emit = defineEmits<{ close: []; open: [id: string]; expired: [] }>()
const dialog = ref<HTMLDialogElement>()
let focus: HTMLElement | null = null
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
</script>
<template><dialog ref="dialog" class="template-library-dialog" aria-labelledby="template-library-title" @cancel.prevent="emit('close')"><header class="template-library-header"><h2 id="template-library-title">模板库</h2><button class="icon-button" aria-label="关闭模板库" @click="emit('close')"><PhX :size="20" /></button></header><div class="template-library-body"><Templates :token="token" :bootstrap="bootstrap" compact @projects="emit('close')" @open="id => emit('open', id)" @expired="emit('expired')" /></div></dialog></template>
