<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { PhArrowClockwise, PhArrowCounterClockwise, PhCopy, PhPencilSimple, PhSquaresFour, PhTrash } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { Bootstrap, PersonalTemplate, Project } from './contracts'
import { templates } from './creative-library'
import PresetGallery from './PresetGallery.vue'
import TemplateUseDialog from './TemplateUseDialog.vue'
import TemplateMetadataDialog from './TemplateMetadataDialog.vue'
import OfficialTemplates from './OfficialTemplates.vue'
import { pendingTemplateRequest, retryTemplateRequest } from './template-requests'
const props = defineProps<{ token: string; bootstrap: Bootstrap; compact?: boolean; initialTab?: 'official' | 'mine' }>()
const emit = defineEmits<{ open: [id: string]; expired: []; projects: [] }>()
const scope = computed(() => `${props.bootstrap.userId}:${props.bootstrap.workspace.workspaceId}`)
const tab = ref<'mine' | 'builtin' | 'trash' | 'official'>(props.initialTab || 'mine'), rows = ref<PersonalTemplate[]>([]), total = ref(0), page = ref(1), loading = ref(false), busy = ref(false), error = ref('')
const selected = ref<PersonalTemplate | null>(null), editing = ref<{ row: PersonalTemplate; version?: string } | null>(null), pending = ref(false)
const presetCreated = ref<Project | null>(null), presetIndex = ref(-1), presetUnknown = ref(false)
let sequence = 0
function showError(e: unknown) { error.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
function refreshPending() { try { pending.value = !!pendingTemplateRequest(scope.value) } catch (e) { pending.value = true; showError(e) } }
async function load() {
  const id = ++sequence; loading.value = true; error.value = ''
  try { const result = await request<{ rows: PersonalTemplate[]; total: number }>(`/pod/templates?pageNum=${page.value}&pageSize=12&deleted=${tab.value === 'trash'}`, {}, props.token); if (id !== sequence) return; if (!result.rows.length && page.value > 1) { page.value--; await load(); return }; rows.value = result.rows; total.value = result.total }
  catch (e) { if (id === sequence) showError(e) } finally { if (id === sequence) loading.value = false }
}
function changeTab(value: typeof tab.value) { ++sequence; tab.value = value; rows.value = []; page.value = 1; loading.value = false; error.value = ''; if (value !== 'builtin' && value !== 'official') void load() }
async function deletion(row: PersonalTemplate) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { await request(`/pod/templates/${row.templateId}${tab.value === 'trash' ? '/restore' : ''}`, { method: tab.value === 'trash' ? 'POST' : 'DELETE', body: JSON.stringify({ revision: row.revision }) }, props.token); await load() }
  catch (e) { showError(e) } finally { busy.value = false }
}
async function retry() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { const intent = pendingTemplateRequest(scope.value); const result = await retryTemplateRequest<PersonalTemplate | Project>(scope.value, props.token); if (intent?.path.endsWith('/use')) emit('open', (result.data as Project).projectId); else { tab.value = 'mine'; await load() } }
  catch (e) { showError(e) } finally { busy.value = false; refreshPending() }
}
async function preset(index: number) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    const item = templates[index]!
    if (!presetCreated.value) {
      presetIndex.value = index
      try { presetCreated.value = (await request<{ data: Project }>('/pod/projects', { method: 'POST', body: JSON.stringify({ name: item.title }) }, props.token)).data }
      catch (e) { if (!(e instanceof APIError) || e.code >= 500 || e.code === 408) presetUnknown.value = true; throw e }
    }
    if (presetIndex.value !== index) throw new Error('请先处理上一份预设项目')
    const created = presetCreated.value
    const saved = await request<{ data: Project }>(`/pod/projects/${created.projectId}`, { method: 'PUT', body: JSON.stringify({ name: item.title, revision: created.revision, graph: item.graph }) }, props.token)
    emit('open', saved.data.projectId)
  } catch (e) { showError(e); if (presetCreated.value) error.value += ' 项目已创建，预设保存结果待核对；可以重试保存或打开已创建的项目。'; else if (presetUnknown.value) error.value += ' 创建结果待确认，请先到项目列表核对。' } finally { busy.value = false }
}
function copy(row: PersonalTemplate, version = row.currentVersion) { selected.value = null; editing.value = { row, version } }
async function changed() { editing.value = null; refreshPending(); await load() }
onMounted(() => { refreshPending(); void load() })
</script>
<template>
  <section class="template-library" :class="{ compact }">
    <div v-if="!compact" class="page-heading"><h1>工作流模板</h1></div>
    <div class="project-tabs template-tabs"><div><button :class="{ active: tab === 'mine' }" :aria-pressed="tab === 'mine'" :disabled="busy" @click="changeTab('mine')">我的模板</button><button :class="{ active: tab === 'official' }" :aria-pressed="tab === 'official'" :disabled="busy" @click="changeTab('official')">官方模板</button><button :class="{ active: tab === 'builtin' }" :aria-pressed="tab === 'builtin'" :disabled="busy" @click="changeTab('builtin')">内置预设</button><button :class="{ active: tab === 'trash' }" :aria-pressed="tab === 'trash'" :disabled="busy" @click="changeTab('trash')">回收站</button></div><button v-if="tab !== 'builtin' && tab !== 'official'" class="icon-button" aria-label="刷新模板列表" :disabled="loading || busy" @click="load"><PhArrowClockwise :size="18" /></button></div>
    <div v-if="pending" class="editor-alert"><span>有一项模板操作结果待确认，重试会核对原结果。</span><button class="text-button" :disabled="busy" @click="retry">重试原操作</button></div><p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <div v-if="presetCreated" class="editor-alert"><span>已创建项目「{{ presetCreated.name }}」，可以打开画布核对预设。</span><button class="text-button" :disabled="busy" @click="emit('open', presetCreated.projectId)">打开已创建的项目</button></div>
    <OfficialTemplates v-if="tab === 'official'" :token="token" :scope="scope" :limits="bootstrap.assetLimits" @open="id => emit('open',id)" @expired="emit('expired')"/><PresetGallery v-else-if="tab === 'builtin'" project :disabled="busy || pending || presetUnknown" :locked-index="presetCreated ? presetIndex : undefined" @use="preset" />
    <p v-else-if="loading" class="muted" role="status">正在读取模板…</p>
    <div v-else-if="rows.length" class="personal-template-list"><article v-for="row in rows" :key="row.templateId" class="personal-template-row"><span class="template-symbol"><PhSquaresFour :size="24" weight="duotone" /></span><div class="personal-template-copy"><button v-if="tab !== 'trash'" class="project-title" :disabled="busy || pending" @click="selected = row">{{ row.name }}</button><h2 v-else>{{ row.name }}</h2><p v-if="row.description">{{ row.description }}</p><small>v{{ row.currentVersion }} · {{ row.updateTime }}</small></div><div class="personal-template-actions"><template v-if="tab !== 'trash'"><button class="primary-button" :disabled="busy || pending" @click="selected = row">使用模板</button><button class="icon-button" :aria-label="`修改模板 ${row.name}`" title="修改模板" :disabled="busy || pending" @click="editing = { row }"><PhPencilSimple :size="18" /></button><button class="icon-button" :aria-label="`复制模板 ${row.name}`" title="复制模板" :disabled="busy || pending" @click="copy(row)"><PhCopy :size="18" /></button></template><button class="text-button" :class="{ 'icon-button': tab !== 'trash' }" :aria-label="`${tab === 'trash' ? '恢复模板' : '移入回收站'} ${row.name}`" :title="tab === 'trash' ? '恢复模板' : '移入回收站'" :disabled="busy || pending" @click="deletion(row)"><PhArrowCounterClockwise v-if="tab === 'trash'" :size="16" /><PhTrash v-else :size="16" /><span v-if="tab === 'trash'">恢复模板</span></button></div></article></div>
    <div v-else class="template-empty"><PhSquaresFour :size="36" weight="light" /><h2>{{ tab === 'trash' ? '回收站是空的' : '还没有个人模板' }}</h2><p>{{ tab === 'trash' ? '移入回收站的模板可以恢复。' : '在画布中保存模板，或先选一个现成模板。' }}</p><button v-if="tab !== 'trash'" class="primary-button" @click="changeTab('official')">选择官方入门模板</button></div>
    <div v-if="tab !== 'builtin' && tab !== 'official' && total > 12" class="template-pagination"><button class="secondary-button" :disabled="loading || busy || page === 1" @click="page--; load()">上一页</button><span>第 {{ page }} / {{ Math.ceil(total / 12) }} 页，共 {{ total }} 个模板</span><button class="secondary-button" :disabled="loading || busy || page * 12 >= total" @click="page++; load()">下一页</button></div>
    <p v-if="tab === 'trash'" class="template-note">回收站保留历史版本及图片引用，不释放存储。</p>
    <TemplateUseDialog v-if="selected" :key="selected.templateId" :row="selected" :token="token" :scope="scope" :limits="bootstrap.assetLimits" :execution-mode="bootstrap.execution?.mode" @close="selected = null; refreshPending()" @open="id => emit('open', id)" @copy="version => copy(selected!, version)" @expired="emit('expired')" />
    <TemplateMetadataDialog v-if="editing" :row="editing.row" :copy-version="editing.version" :token="token" :scope="scope" @close="editing = null; refreshPending()" @changed="changed" @expired="emit('expired')" />
  </section>
</template>
