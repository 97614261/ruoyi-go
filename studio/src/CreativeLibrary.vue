<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { PhBookmarkSimple, PhCopy, PhImage, PhLightbulb, PhMagnifyingGlass, PhPlus, PhSquaresFour, PhX } from '@phosphor-icons/vue'
import type { Graph } from './contracts'
import { matchesLibrary, prompts, resolvePrompt, templates } from './creative-library'
import PresetGallery from './PresetGallery.vue'

const props = defineProps<{ initialTab: 'templates' | 'prompts'; disabled?: boolean; scope?: string }>()
const emit = defineEmits<{ close: []; prompt: [text: string]; template: [graph: Graph] }>()
const tab = ref(props.initialTab), query = ref(''), category = ref('全部'), dialog = ref<HTMLDialogElement>()
const selection = ref(''), onlySaved = ref(false), saved = ref<string[]>([]), notice = ref(''), preview = ref<HTMLElement>()
const edits = reactive<Record<string, Record<string, string>>>({})
const promptText = ref<HTMLTextAreaElement>()
const key = 'pod-studio.prompt-favorites:' + (props.scope ?? 'local')
const categories = ['全部', ...new Set(prompts.map(item => item.category))]
const filtered = computed(() => prompts.filter(item => (category.value === '全部' || item.category === category.value) && (!onlySaved.value || saved.value.includes(item.id)) && matchesLibrary(query.value, [item.title, item.category, ...item.tags, item.text, item.requiresReference ? '需要参考图' : ''])))
const active = computed(() => filtered.value.find(item => item.id === selection.value) ?? filtered.value[0])
const composed = computed(() => active.value ? resolvePrompt(active.value, edits[active.value.id] || {}) : { text: '', ready: false })
watch(() => [active.value?.id, composed.value.text], () => { notice.value = '' })
let before: HTMLElement | undefined
onMounted(() => {
  try { const stored: unknown = JSON.parse(localStorage.getItem(key) || '[]'); if (Array.isArray(stored)) saved.value = prompts.filter(item => stored.includes(item.id)).map(item => item.id) } catch { /* Browsing works without local storage. */ }
  before = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  dialog.value?.showModal()
})
onBeforeUnmount(() => { dialog.value?.close(); if (before?.isConnected) before.focus() })
function pick(id: string) {
  selection.value = id
  if (window.matchMedia('(max-width: 720px)').matches) void nextTick(() => preview.value?.scrollIntoView({ block: 'nearest' }))
}
function toggleSaved(id: string) {
  saved.value = saved.value.includes(id) ? saved.value.filter(item => item !== id) : [...saved.value, id]
  try { localStorage.setItem(key, JSON.stringify(saved.value)) } catch { notice.value = '收藏只保留在当前页面。' }
}
async function copy() {
  const item = active.value
  const { text, ready } = composed.value
  if (!item || !ready) return
  try {
    if (!navigator.clipboard) throw new Error('Clipboard unavailable')
    await navigator.clipboard.writeText(text)
    if (active.value?.id === item.id && composed.value.text === text) notice.value = '提示词已复制。'
  } catch {
    if (active.value?.id !== item.id || composed.value.text !== text) return
    promptText.value?.focus(); promptText.value?.select()
    notice.value = '浏览器未允许复制，已选中文字，可以手动复制。'
  }
}
function updateField(id: string, key: string, event: Event) {
  const field = event.target as HTMLInputElement
  edits[id] = { ...edits[id], [key]: field.value }
}
function usePrompt() {
  if (!props.disabled && composed.value.ready) emit('prompt', composed.value.text)
}
</script>

<template>
  <dialog ref="dialog" class="creative-library-dialog library-redesign" aria-labelledby="creative-library-title" @cancel.prevent="emit('close')">
    <header><h2 id="creative-library-title">创作素材</h2><nav aria-label="创作分类"><button type="button" :aria-pressed="tab === 'templates'" @click="tab = 'templates'"><PhSquaresFour :size="18" />工作流预设</button><button type="button" :aria-pressed="tab === 'prompts'" @click="tab = 'prompts'"><PhLightbulb :size="18" />提示词灵感</button></nav><button type="button" class="icon-button" aria-label="关闭创作素材" @click="emit('close')"><PhX :size="20" /></button></header>
    <PresetGallery v-if="tab === 'templates'" :disabled="disabled" @use="index => { const item = templates[index]; if (item) emit('template', item.graph) }" />
    <section v-else class="creative-shelf inspiration-shelf" aria-label="提示词灵感目录">
      <div class="shelf-intro"><h2>提示词灵感</h2><label class="shelf-search"><PhMagnifyingGlass :size="18" /><input v-model="query" type="search" aria-label="搜索提示词" placeholder="搜索题材、风格或用途" /></label></div>
      <div class="shelf-filters" role="group" aria-label="提示词分类"><button v-for="item in categories" :key="item" type="button" :aria-pressed="category === item" @click="category = item">{{ item }}</button><button type="button" class="shelf-favorites" :aria-pressed="onlySaved" @click="onlySaved = !onlySaved"><PhBookmarkSimple :size="16" />已收藏</button></div>
      <div v-if="active" class="inspiration-layout">
        <div class="inspiration-index" role="group" aria-label="灵感列表">
          <button v-for="item in filtered" :key="item.id" type="button" :aria-pressed="active.id === item.id" @click="pick(item.id)">
            <span class="inspiration-swatch" aria-hidden="true"><i v-for="color in item.colors" :key="color" :style="{backgroundColor: color}" /></span>
            <span><strong>{{ item.title }}</strong><small>{{ item.requiresReference ? '需要参考图' : item.tags.join(' / ') }}</small></span>
            <PhBookmarkSimple v-if="saved.includes(item.id)" :size="15" weight="fill" aria-hidden="true" />
          </button>
        </div>
        <article ref="preview" class="inspiration-preview"><div class="inspiration-copy"><div class="inspiration-title"><div><small>{{ active.category }}</small><h3>{{ active.title }}</h3></div><button type="button" class="icon-button" :aria-label="saved.includes(active.id) ? '取消收藏此提示词' : '收藏此提示词'" :aria-pressed="saved.includes(active.id)" @click="toggleSaved(active.id)"><PhBookmarkSimple :size="21" :weight="saved.includes(active.id) ? 'fill' : 'regular'" /></button></div>
          <p v-if="active.requiresReference" class="inspiration-reference"><PhImage :size="17" aria-hidden="true" />需要参考图，生图前请把商品原图连到生图节点。</p>
          <div v-if="active.parameters" :key="active.id" class="inspiration-parameters" role="group" aria-label="提示词参数">
            <label v-for="field in active.parameters.fields" :key="field.key">
              <span>{{ field.label }} <small v-if="field.required">必填</small></span>
              <input :value="edits[active.id]?.[field.key] ?? field.initial ?? ''" :required="field.required" :placeholder="field.placeholder" maxlength="80" @input="updateField(active.id, field.key, $event)" />
            </label>
          </div>
          <label class="inspiration-full"><span>完整提示词</span><textarea ref="promptText" readonly :value="composed.text" rows="4" /></label>
          <div class="inspiration-actions"><button type="button" class="secondary-button" :disabled="!composed.ready" @click="copy"><PhCopy :size="17" />复制提示词</button><button type="button" class="primary-button" :disabled="disabled || !composed.ready" @click="usePrompt"><PhPlus :size="17" />放入画布</button></div><p v-if="notice" role="status" class="shelf-notice">{{ notice }}</p>
        </div></article>
      </div>
      <div v-else class="shelf-empty" role="status"><p>{{ onlySaved ? '这里没有匹配的收藏。' : '没找到匹配的灵感。' }}</p><button type="button" class="text-button" @click="onlySaved = false; category = '全部'; query = ''">查看全部灵感</button></div>
    </section>
  </dialog>
</template>
