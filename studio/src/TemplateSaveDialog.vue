<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhX } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { PersonalTemplate } from './contracts'
import { pendingTemplateRequest, retryTemplateRequest, submitTemplateRequest } from './template-requests'
const props = defineProps<{ token: string; scope: string; projectId: string; projectName: string; prepare: () => Promise<string> }>()
const emit = defineEmits<{ close: []; saved: [row: PersonalTemplate]; expired: [] }>()
const dialog = ref<HTMLDialogElement>(), mode = ref('new'), name = ref(props.projectName), description = ref(''), targetId = ref('')
const rows = ref<PersonalTemplate[]>([]), page = ref(1), total = ref(0), loading = ref(false), busy = ref(false), error = ref('')
const pending = ref(false), target = computed(() => rows.value.find(row => row.templateId === targetId.value))
let focus: HTMLElement | null = null
function refreshPending() { try { pending.value = !!pendingTemplateRequest(props.scope) } catch (e) { pending.value = true; error.value = (e as Error).message } }
async function load() {
  loading.value = true
  try { const result = await request<{ rows: PersonalTemplate[]; total: number }>(`/pod/templates?pageNum=${page.value}&pageSize=12`, {}, props.token); rows.value = result.rows; total.value = result.total }
  catch (e) { showError(e) } finally { loading.value = false }
}
function showError(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
function choose() { if (target.value) { name.value = target.value.name; description.value = target.value.description } }
function changeMode() { targetId.value = ''; name.value = props.projectName; description.value = ''; error.value = ''; if (mode.value === 'version') void load() }
async function submit(retry = false) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    let row: PersonalTemplate
    if (retry) {
      const intent = pendingTemplateRequest(props.scope)
      if (!intent || intent.path.endsWith('/use') || intent.path.endsWith('/copy')) throw new Error('请到模板页面处理上一项待确认操作。')
      row = (await retryTemplateRequest<PersonalTemplate>(props.scope, props.token)).data
    } else {
      if (mode.value === 'version' && !target.value) throw new Error('请选择要保存新版本的模板')
      const projectRevision = await props.prepare()
      const body = { name: name.value, description: description.value, projectId: props.projectId, projectRevision, ...(mode.value === 'version' ? { revision: target.value!.revision } : {}) }
      row = (await submitTemplateRequest<PersonalTemplate>(props.scope, mode.value === 'version' ? `/pod/templates/${targetId.value}/versions` : '/pod/templates', body, props.token)).data
    }
    emit('saved', row)
  } catch (e) { showError(e) } finally { busy.value = false; refreshPending() }
}
onMounted(() => { focus = document.activeElement as HTMLElement; dialog.value?.showModal(); refreshPending() })
onBeforeUnmount(() => { dialog.value?.close(); focus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="studio-dialog template-save-dialog" aria-labelledby="template-save-title" @cancel.prevent="!busy && emit('close')">
    <form @submit.prevent="submit(pending)">
      <header class="template-dialog-header"><div><h2 id="template-save-title">保存为个人模板</h2></div><button type="button" class="icon-button" aria-label="关闭模板保存" :disabled="busy" @click="emit('close')"><PhX :size="20" /></button></header>
      <fieldset :disabled="busy || pending"><label class="field">保存方式<select v-model="mode" @change="changeMode"><option value="new">新建个人模板</option><option value="version">为已有模板保存新版本</option></select></label>
        <template v-if="mode === 'version'"><label class="field">已有模板<select v-model="targetId" :disabled="loading" required @change="choose"><option value="" disabled>请选择模板</option><option v-for="row in rows" :key="row.templateId" :value="row.templateId">{{ row.name }} · 当前 v{{ row.currentVersion }}</option></select></label><div v-if="total > 12" class="template-pagination"><button type="button" class="text-button" :disabled="loading || page === 1" @click="page--; load()">上一页</button><span>第 {{ page }} 页</span><button type="button" class="text-button" :disabled="loading || page * 12 >= total" @click="page++; load()">下一页</button></div></template>
        <label class="field">模板名称<input v-model="name" maxlength="100" required autocomplete="off" /></label><label class="field">说明<textarea v-model="description" maxlength="500" rows="3" placeholder="例如：适合透明印花，使用前换一张图片。" /></label>
      </fieldset>
      <p class="template-note">保存流程和默认输入，保留图片引用；不包含历史结果。</p>
      <p v-if="pending" class="editor-alert" role="status">上次提交结果待确认，重试会沿用原请求，不重复创建。</p><p v-if="error" class="error-message" role="alert">{{ error }}</p>
      <div class="dialog-actions"><button type="button" class="secondary-button" :disabled="busy" @click="emit('close')">取消</button><button class="primary-button" :disabled="busy || (!pending && (!name.trim() || (mode === 'version' && !target)))">{{ busy ? '正在保存…' : pending ? '重试原提交' : mode === 'version' ? '保存新版本' : '保存模板' }}</button></div>
    </form>
  </dialog>
</template>
