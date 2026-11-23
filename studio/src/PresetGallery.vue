<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { PhArrowRight, PhMagnifyingGlass, PhX } from '@phosphor-icons/vue'
import { matchesLibrary, presetCover, templates } from './creative-library'
import TemplateGraph from './TemplateGraph.vue'
const props = defineProps<{ project?: boolean; disabled?: boolean; lockedIndex?: number }>()
const emit = defineEmits<{ use: [index: number] }>()
const category = ref('全部'), query = ref('')
const categories = ['全部', ...new Set(templates.map(item => item.category))]
const items = templates.map((item, index) => ({ ...item, index }))
const filtered = computed(() => items.filter(item => props.lockedIndex === item.index || ((category.value === '全部' || item.category === category.value) && matchesLibrary(query.value, [item.title, item.description, item.input, item.output, item.category]))))
const selected = ref<typeof items[number]>()
const dialog = ref<HTMLDialogElement>()
let opener: HTMLElement | null = null
async function details(item: typeof items[number], event: MouseEvent) {
  opener = event.currentTarget as HTMLElement
  selected.value = item
  await nextTick()
  dialog.value?.showModal()
}
function closeDetails() {
  dialog.value?.close()
  selected.value = undefined
  if (opener?.isConnected) opener.focus()
}
function use(index: number) {
  if (props.disabled || (props.lockedIndex !== undefined && props.lockedIndex !== index)) return
  if (dialog.value?.open) closeDetails()
  emit('use', index)
}
onBeforeUnmount(() => { dialog.value?.close() })
</script>

<template>
  <section class="creative-shelf preset-shelf" aria-label="工作流预设目录">
    <div class="shelf-intro"><h2>工作流预设</h2><label class="shelf-search"><PhMagnifyingGlass :size="18" /><input v-model="query" type="search" aria-label="搜索工作流预设" placeholder="搜索用途或成品" /></label></div>
    <div class="shelf-filters" role="group" aria-label="预设分类"><button v-for="item in categories" :key="item" type="button" :aria-pressed="category === item" @click="category = item">{{ item }}</button></div>
    <div v-if="filtered.length" class="template-preset-grid shelf-grid"><article v-for="item in filtered" :key="item.kind" class="shelf-preset">
      <button type="button" class="shelf-cover" :aria-label="'查看预设详情：' + item.title" aria-haspopup="dialog" @click="details(item, $event)">
        <img :src="presetCover(item.kind)" alt="" width="640" height="400" loading="lazy" decoding="async" />
        <span>成品示意</span>
      </button>
      <div class="shelf-card-body"><h3>{{ item.title }}</h3><p>{{ item.output }}</p>
        <button type="button" class="workflow-preset secondary-button" :aria-label="(project ? '从预设新建项目：' : '添加到画布：') + item.title" :disabled="disabled || (lockedIndex !== undefined && lockedIndex !== item.index)" @click="use(item.index)">{{ project ? lockedIndex === item.index ? '重试保存预设' : '从预设新建项目' : '添加到画布' }}<PhArrowRight :size="16" /></button>
      </div>
    </article></div>
    <div v-else class="shelf-empty" role="status"><p>没找到匹配的工作流。</p><button type="button" class="text-button" @click="query = ''; category = '全部'">查看全部预设</button></div>
  </section>
  <dialog ref="dialog" class="shelf-preset-detail" aria-labelledby="shelf-preset-title" @cancel.prevent.stop="closeDetails">
    <template v-if="selected">
      <header><h2 id="shelf-preset-title">{{ selected.title }}</h2><button type="button" class="icon-button" aria-label="关闭预设详情" autofocus @click="closeDetails"><PhX :size="20" /></button></header>
      <div class="shelf-detail-body">
        <figure><img :src="presetCover(selected.kind)" :alt="selected.output + '示意图'" width="640" height="400" /><figcaption>成品示意</figcaption></figure>
        <section aria-label="预设说明">
          <dl><div><dt>准备素材</dt><dd>{{ selected.input }}</dd></div><div><dt>成品</dt><dd>{{ selected.output }}</dd></div></dl>
          <p>{{ selected.description }}</p>
          <h3>节点流程</h3><TemplateGraph :graph="selected.graph" />
        </section>
      </div>
      <footer><button type="button" class="primary-button" :disabled="disabled || (lockedIndex !== undefined && lockedIndex !== selected.index)" @click="use(selected.index)">{{ project ? lockedIndex === selected.index ? '重试保存预设' : '从预设新建项目' : '添加到画布' }}</button></footer>
    </template>
  </dialog>
</template>
