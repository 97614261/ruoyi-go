<script setup lang="ts">
import { onMounted, ref } from 'vue'
const props = defineProps<{ title: string; initial: string; pending: boolean; error: string }>()
const emit = defineEmits<{ confirm: [name: string]; cancel: [] }>()
const dialog = ref<HTMLDialogElement>(), input = ref<HTMLInputElement>(), name = ref(props.initial)
onMounted(() => { dialog.value?.showModal(); input.value?.focus(); input.value?.select() })
</script>

<template>
  <dialog ref="dialog" class="studio-dialog" aria-labelledby="project-dialog-title" @cancel.prevent="!pending && emit('cancel')">
    <form @submit.prevent="emit('confirm', name)">
      <h2 id="project-dialog-title">{{ title }}</h2>
      <div class="field"><label for="project-name">项目名称</label><input id="project-name" ref="input" v-model="name" required maxlength="100" :disabled="pending" autocomplete="off" /></div>
      <p v-if="error" class="error-message" role="alert">{{ error }}</p>
      <div class="dialog-actions"><button type="button" class="secondary-button" :disabled="pending" @click="emit('cancel')">取消</button><button class="primary-button" :disabled="pending || !name.trim()">{{ pending ? '正在处理…' : '确定' }}</button></div>
    </form>
  </dialog>
</template>
