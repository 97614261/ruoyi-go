<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowClockwise, PhArrowCounterClockwise, PhArrowRight, PhBezierCurve, PhClockCounterClockwise, PhDotsThree, PhFolderSimple, PhImages, PhPencilSimple, PhPlus, PhTrash } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { Bootstrap, Project, ProjectSummary } from './contracts'
import ProjectNameDialog from './ProjectNameDialog.vue'
import CreationLaunchpad from './CreationLaunchpad.vue'
import type { Section } from './navigation'

const props = defineProps<{ token: string; bootstrap: Bootstrap; home?: boolean; startCreate?: boolean }>()
const emit = defineEmits<{ open: [id: string]; expired: []; navigate: [section: Section]; createEntryConsumed: [] }>()
const rows = ref<ProjectSummary[]>([]), total = ref(0), page = ref(1), deleted = ref(false)
const loading = ref(false), busy = ref(false), error = ref(''), dialogError = ref('')
const dialog = ref<'create' | 'rename' | null>(null), target = ref<ProjectSummary | null>(null)
const recent = computed(() => rows.value.slice(0, 4))
const shortcuts = [{ label: '任务与结果', section: 'runs', icon: PhClockCounterClockwise }, { label: '我的素材', section: 'assets', icon: PhImages }] as const
let loadSequence = 0
function showError(e: unknown) { if (e instanceof APIError && e.code === 401) emit('expired'); error.value = (e as Error).message }
async function load() {
  const sequence = ++loadSequence; loading.value = true; error.value = ''
  try {
    const result = await request<{ rows: ProjectSummary[]; total: number }>(`/pod/projects?pageNum=${page.value}&pageSize=12&deleted=${deleted.value}`, {}, props.token)
    if (sequence !== loadSequence) return
    if (!result.rows.length && page.value > 1) { page.value--; await load(); return }
    rows.value = result.rows; total.value = result.total
  } catch (e) { if (sequence === loadSequence) showError(e) }
  finally { if (sequence === loadSequence) loading.value = false }
}
async function changeTab(value: boolean) { deleted.value = value; page.value = 1; rows.value = []; await load() }
function openDialog(kind: 'create' | 'rename', row: ProjectSummary | null = null) { dialog.value = kind; target.value = row; dialogError.value = '' }
async function confirmName(name: string) {
  if (busy.value) return
  busy.value = true; dialogError.value = ''
  try {
    if (dialog.value === 'create') {
      const result = await request<{ data: Project }>('/pod/projects', { method: 'POST', body: JSON.stringify({ name }) }, props.token)
      dialog.value = null; emit('open', result.data.projectId)
    } else if (target.value) {
      await request(`/pod/projects/${target.value.projectId}`, { method: 'PUT', body: JSON.stringify({ name, revision: target.value.revision }) }, props.token)
      dialog.value = null; await load()
    }
  } catch (e) { dialogError.value = (e as Error).message; if (e instanceof APIError && e.code === 401) emit('expired') }
  finally { busy.value = false }
}
async function changeDeletion(row: ProjectSummary) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    await request(`/pod/projects/${row.projectId}${deleted.value ? '/restore' : ''}`, { method: deleted.value ? 'POST' : 'DELETE', body: JSON.stringify({ revision: row.revision }) }, props.token)
    await load()
  } catch (e) { showError(e) } finally { busy.value = false }
}
onMounted(() => {
  if (props.startCreate) {
    if (props.bootstrap.capabilities.projectEditing) openDialog('create')
    emit('createEntryConsumed')
  }
  void load()
})
onBeforeUnmount(() => ++loadSequence)
</script>

<template>
  <section v-if="home" class="workspace-home workspace-home-split" aria-label="创作首页">
    <div class="home-create">
      <CreationLaunchpad :create-disabled="busy || !bootstrap.capabilities.projectEditing" @create="openDialog('create')" @templates="emit('navigate', 'templates')" @product="emit('navigate', 'product')" />
      <div class="home-tools" aria-label="创作快捷入口">
        <button v-for="tool in shortcuts" :key="tool.section" @click="emit('navigate', tool.section)"><component :is="tool.icon" :size="18" /><strong>{{ tool.label }}</strong><PhArrowRight :size="13" /></button>
      </div>
    </div>
    <section class="home-recent" aria-labelledby="recent-projects-heading">
      <div class="section-heading"><h1 id="recent-projects-heading">最近项目</h1><button class="text-button" @click="emit('navigate', 'projects')">查看全部<PhArrowRight :size="16" /></button></div>
      <p v-if="loading" class="muted" role="status">正在读取项目…</p>
      <div v-else-if="recent.length" class="recent-projects">
        <button v-for="row in recent" :key="row.projectId" class="recent-project" :aria-label="`打开项目 ${row.name}`" @click="emit('open', row.projectId)"><span class="recent-project-thumb"><PhBezierCurve :size="22" weight="light" /></span><span class="recent-project-copy"><strong>{{ row.name }}</strong><time>{{ row.updateTime.slice(0, 10) }}</time></span><PhArrowRight :size="16" /></button>
      </div>
      <div v-else-if="!error" class="recent-empty"><PhFolderSimple :size="24" /><span>还没有项目</span></div>
    </section>
  </section>
  <section v-else class="workspace-projects workspace-project-index" aria-label="项目列表">
    <div class="page-heading"><h1>{{ deleted ? '项目回收站' : '全部项目' }}<span class="heading-count">{{ total }}</span></h1><div class="project-list-actions"><button class="secondary-button" :aria-pressed="deleted" :disabled="busy" @click="changeTab(!deleted)"><PhArrowCounterClockwise v-if="deleted" :size="17" /><PhTrash v-else :size="17" />{{ deleted ? '返回项目' : '回收站' }}</button><button v-if="!deleted" class="primary-button" :disabled="busy || !bootstrap.capabilities.projectEditing" @click="openDialog('create')"><PhPlus :size="18" />新建项目</button><button class="icon-button" aria-label="刷新项目列表" :disabled="loading || busy" @click="load"><PhArrowClockwise :size="18" /></button></div></div>
    <p v-if="loading" class="muted" role="status">正在读取项目…</p>
    <div v-else-if="rows.length" class="project-index">
      <article v-for="row in rows" :key="row.projectId" class="project-index-row">
        <span class="project-index-icon" aria-hidden="true"><PhFolderSimple v-if="deleted" :size="20" /><PhBezierCurve v-else :size="20" /></span>
        <button v-if="!deleted" class="project-title" :aria-label="`打开项目 ${row.name}`" @click="emit('open', row.projectId)">{{ row.name }}</button><h2 v-else>{{ row.name }}</h2>
        <span class="project-index-version">v{{ row.revision }}</span><time>{{ row.updateTime.slice(0, 10) }}</time>
        <details class="project-menu"><summary :aria-label="`管理项目 ${row.name}`"><PhDotsThree :size="23" /></summary><div><button v-if="!deleted" :disabled="busy || !bootstrap.capabilities.projectEditing" @click="openDialog('rename', row)"><PhPencilSimple :size="16" />重命名</button><button :disabled="busy" @click="changeDeletion(row)"><PhArrowCounterClockwise v-if="deleted" :size="16" /><PhTrash v-else :size="16" />{{ deleted ? '恢复项目' : '移入回收站' }}</button></div></details>
      </article>
    </div>
    <div v-else-if="!error" class="recent-empty project-index-empty"><PhTrash v-if="deleted" :size="26" /><PhFolderSimple v-else :size="26" /><span>{{ deleted ? '回收站是空的' : '还没有项目' }}</span></div>
    <div v-if="total > 12" class="project-pagination"><button class="secondary-button" :disabled="loading || page === 1" @click="page--; load()">上一页</button><span>第 {{ page }} / {{ Math.ceil(total / 12) }} 页，共 {{ total }} 个项目</span><button class="secondary-button" :disabled="loading || page * 12 >= total" @click="page++; load()">下一页</button></div>
  </section>
  <p v-if="error" class="error-message" role="alert">{{ error }}</p>
  <p v-if="bootstrap.execution?.mode === 'simulation'" class="project-stage-note"><PhDotsThree :size="20" />模拟模式，不调用真实模型。</p>
  <ProjectNameDialog v-if="dialog" :title="dialog === 'create' ? '新建项目' : '重命名项目'" :initial="dialog === 'rename' ? target?.name || '' : '未命名项目'" :pending="busy" :error="dialogError" @cancel="dialog = null" @confirm="confirmName" />
</template>
