<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowClockwise, PhArrowRight } from '@phosphor-icons/vue'
import { APIError, request } from './api'
import type { Bootstrap, CreatorEligibility, CreatorModel, FormalPoints } from './contracts'

const props = defineProps<{ token: string; bootstrap: Bootstrap; eligibility?: CreatorEligibility | null }>()
const emit = defineEmits<{ navigate: [section: 'templates' | 'assets' | 'eligibility' | 'runs']; expired: [] }>()
const models = ref<CreatorModel[]>([]), points = ref<FormalPoints | null>(null), loading = ref(false), error = ref('')
const managed = computed(() => props.bootstrap.execution?.mode === 'managed')
const textModels = computed(() => models.value.filter(m => m.kind === 'text'))
const imageModels = computed(() => models.value.filter(m => m.kind === 'image'))
let sequence = 0
async function load() {
  const current = ++sequence; loading.value = true; error.value = ''
  try {
    const [catalog, account] = await Promise.all([
      request<{ data: CreatorModel[] }>('/pod/models', {}, props.token),
      request<{ data: FormalPoints }>('/pod/points?pageNum=1&pageSize=1', {}, props.token),
    ])
    if (current !== sequence) return
    models.value = catalog.data; points.value = account.data
  } catch (e) {
    if (current !== sequence) return
    models.value = []; points.value = null
    error.value = '暂时无法核对模型和点数，请重新检查或打开资格与点数。'
    if (e instanceof APIError && e.code === 401) emit('expired')
  } finally { if (current === sequence) loading.value = false }
}
onMounted(() => { if (managed.value) void load() })
onBeforeUnmount(() => { ++sequence })
</script>

<template>
  <section class="getting-started" aria-labelledby="getting-started-title">
    <div class="getting-started-heading"><h2 id="getting-started-title">创作状态</h2><button v-if="managed" class="icon-button" aria-label="重新检查创作条件" :disabled="loading" @click="load"><PhArrowClockwise :size="18" /></button></div>
    <p v-if="eligibility?.creator.expired" class="error-message" role="status">创作资格已到期，可查看和下载已有结果。联系管理员续期后再制作。</p>
    <p v-if="loading" class="muted" role="status">正在核对可选模型与正式点数…</p>
    <p v-else-if="error" class="error-message" role="alert">{{ error }}</p>
    <template v-else-if="managed && points">
      <div class="creation-readiness" aria-label="当前创作条件"><span>正式点数 <strong>{{ points.account.available }}</strong></span><span>文字模型 <strong>{{ textModels.length }}</strong> 个</span><span>图片模型 <strong>{{ imageModels.length }}</strong> 个</span><button class="text-button" @click="emit('navigate', 'eligibility')">查看资格与点数</button></div>
      <p v-if="!imageModels.length" class="creation-hint" role="status">当前没有可选图片模型，暂不能 AI 生图或改图。可上传已有图案，用“印花处理与商品预览”制作；恢复生图请联系管理员。</p>
      <p v-if="points.account.available === '0'" class="creation-hint" role="status">正式点数为 0。AI 创作前请联系管理员分配点数，本地图片处理不扣模型点数。</p>
    </template>
    <details class="creation-guide"><summary>快速上手</summary><ol class="creation-steps">
      <li><span>01</span><div><strong>选择官方模板</strong><p>文字生图、参考图改图，或已有印花处理。</p><button class="text-button" :disabled="!bootstrap.capabilities.projectEditing" @click="emit('navigate', 'templates')">去选模板<PhArrowRight :size="15" /></button></div></li>
      <li><span>02</span><div><strong>填入自己的内容</strong><p>在复制窗口填文字和图片，进入画布再选模型。</p><button class="text-button" :disabled="!bootstrap.capabilities.assets" @click="emit('navigate', 'assets')">准备图片<PhArrowRight :size="15" /></button></div></li>
      <li><span>03</span><div><strong>选好模型，开始制作</strong><p>点击生成或制作；系统自动核价，按用量结算点数。</p></div></li>
      <li><span>04</span><div><strong>查看并下载结果</strong><p>运行详情查看结果图和印花/展示图；离开页面也能找回。</p><button class="text-button" @click="emit('navigate', 'runs')">运行记录<PhArrowRight :size="15" /></button></div></li>
    </ol></details>
  </section>
</template>
