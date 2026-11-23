<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { APIError, request } from './api'
import type { PersonalTemplate } from './contracts'
import { pendingTemplateRequest, retryTemplateRequest, submitTemplateRequest } from './template-requests'
const props = defineProps<{ row: PersonalTemplate; copyVersion?: string; token: string; scope: string }>()
const emit = defineEmits<{ close: []; changed: []; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), name = ref(props.copyVersion ? `${props.row.name.slice(0, 96)} 副本` : props.row.name), description = ref(props.row.description), busy = ref(false), error = ref(''), pending = ref(false)
let focus: HTMLElement | null = null
function refreshPending() { try { pending.value = !!pendingTemplateRequest(props.scope) } catch (e) { pending.value = true; error.value = (e as Error).message } }
async function submit() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    if (props.copyVersion) {
      if (pending.value) { if (!pendingTemplateRequest(props.scope)?.path.endsWith('/copy')) throw new Error('请先在模板页面处理上一项待确认操作。'); await retryTemplateRequest(props.scope, props.token) }
      else await submitTemplateRequest(props.scope, `/pod/templates/${props.row.templateId}/copy`, { version: props.copyVersion, name: name.value }, props.token)
    } else await request(`/pod/templates/${props.row.templateId}`, { method: 'PUT', body: JSON.stringify({ name: name.value, description: description.value, revision: props.row.revision }) }, props.token)
    emit('changed')
  } catch (e) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
  finally { busy.value = false; refreshPending() }
}
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal(); refreshPending() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="studio-dialog template-meta-dialog" aria-labelledby="template-meta-title" @cancel.prevent="!busy && emit('close')"><form @submit.prevent="submit"><h2 id="template-meta-title">{{ copyVersion ? `复制模板 v${copyVersion}` : '修改模板信息' }}</h2><fieldset :disabled="busy || (!!copyVersion && pending)"><label class="field">模板名称<input v-model="name" maxlength="100" required /></label><label v-if="!copyVersion" class="field">说明<textarea v-model="description" maxlength="500" rows="3" /></label></fieldset><p class="template-note">{{ copyVersion ? '复制为独立的个人模板，从 v1 开始，不会执行模型。' : '修改名称和说明不会改动历史流程版本。' }}</p><p v-if="error" class="error-message" role="alert">{{ error }}</p><div class="dialog-actions"><button type="button" class="secondary-button" :disabled="busy" @click="emit('close')">取消</button><button class="primary-button" :disabled="busy || !name.trim()">{{ busy ? '正在保存…' : copyVersion && pending ? '重试原复制' : copyVersion ? '复制模板' : '保存信息' }}</button></div></form></dialog>
</template>
