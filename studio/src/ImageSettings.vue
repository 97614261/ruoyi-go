<script setup lang="ts">
import { computed, ref } from 'vue'
import type { NodeType, RunNode } from './contracts'
import ProductLayout from './ProductLayout.vue'
import CropEditor from './CropEditor.vue'
import { cropConfig, cropPatch, validCrop } from './image-geometry'
import { runStateLabel } from './run-state'
const props = defineProps<{ type: NodeType; config: Record<string, unknown>; token: string; inputId?: string; disabled?: boolean; aiEnabled?: boolean; runNode?: RunNode; runRevision?: string; runMethod?: string }>()
const emit = defineEmits<{ config: [key: string, value: unknown]; patch: [values: Record<string, number>]; expired: []; gesture: [active: boolean] }>()
const error = ref('')
const op = computed(() => String(props.config.operation || 'resize'))
function value(event: Event) { return (event.target as HTMLInputElement).value }
function number(key: string, event: Event) { const input = event.target as HTMLInputElement; if (!input.value || !input.checkValidity()) { error.value = '请填写范围内的数值，当前设置尚未修改。'; return }; error.value = ''; emit('config', key, Number(value(event))) }
function cropNumber(key: string, event: Event) {
  const input = event.target as HTMLInputElement, config = { ...props.config, [key]: Number(input.value) }, rect = cropConfig(config)
  if (!input.value || !input.checkValidity() || !validCrop(rect)) { error.value = '起点加保留尺寸不能超过 1，且尺寸必须大于 0。当前设置尚未修改。'; input.value = String(props.config[key] ?? (key === 'cropWidth' || key === 'cropHeight' ? 1 : 0)); return }
  error.value = ''; emit('patch', cropPatch(rect))
}
</script>
<template>
  <div class="image-settings">
    <template v-if="type === 'imageProcess'">
      <label>处理方式<select :value="op" :disabled="disabled" @change="emit('config', 'operation', value($event))"><option value="resize">等比调整尺寸</option><option value="crop">裁剪</option><option value="removeBackground">去背景</option><option value="upscale">放大图片</option></select></label>
      <label v-if="['resize', 'crop'].includes(op)">先旋转<select :value="config.rotate || 0" :disabled="disabled" @change="number('rotate', $event)"><option :value="0">不旋转</option><option :value="90">顺时针 90°</option><option :value="180">旋转 180°</option><option :value="270">逆时针 90°</option></select></label>
      <div v-if="op === 'resize'" class="image-field-pair"><label>最大宽度 / px<input type="number" :value="config.width ?? 2048" min="64" max="8192" :disabled="disabled" @change="number('width', $event)" /></label><label>最大高度 / px<input type="number" :value="config.height ?? 2048" min="64" max="8192" :disabled="disabled" @change="number('height', $event)" /></label></div>
      <p v-if="op === 'resize'" class="muted">按原比例放入目标尺寸范围，保留透明背景。</p>
      <template v-if="op === 'crop'"><CropEditor :asset-id="inputId || ''" :token="token" :config="config" :disabled="disabled" @patch="values => { error = ''; emit('patch', values) }" @gesture="active => emit('gesture', active)" @expired="emit('expired')" /><div class="image-field-pair"><label v-for="field in [{ key: 'cropX', label: '左侧起点', fallback: 0 }, { key: 'cropY', label: '顶部起点', fallback: 0 }, { key: 'cropWidth', label: '保留宽度', fallback: 1 }, { key: 'cropHeight', label: '保留高度', fallback: 1 }]" :key="field.key">{{ field.label }}<input type="number" :value="config[field.key] ?? field.fallback" :min="field.fallback ? .000001 : 0" max="1" step="any" :disabled="disabled" @change="cropNumber(field.key, $event)" /></label></div></template>
      <template v-if="['removeBackground', 'upscale'].includes(op)"><label>处理引擎<select :value="config.method || 'basic'" :disabled="disabled" @change="emit('config', 'method', value($event))"><option value="basic">{{ op === 'removeBackground' ? '纯色去背景 · 本地' : '普通插值放大 · 本地' }}</option><option value="ai" :disabled="!aiEnabled">{{ op === 'removeBackground' ? 'AI 抠图 · CPU 模型' : 'AI 高清放大 · CPU 模型' }}{{ aiEnabled ? '' : '（未配置）' }}</option></select></label><p class="muted">{{ config.method === 'ai' ? '使用本机模型，不消耗千问额度；首次执行可能较慢。' : op === 'removeBackground' ? '仅移除从边缘连通的相近颜色，适合纯色背景；复杂背景请选择 AI。' : '普通放大不会恢复真实细节，需要高清修复请选择 AI。' }}</p></template>
      <template v-if="op === 'removeBackground' && config.method !== 'ai'"><label>背景颜色<input type="color" :value="config.background || '#ffffff'" :disabled="disabled" @input="emit('config', 'background', value($event))" /></label><label>颜色容差 / 0—120<input type="number" :value="config.tolerance ?? 30" min="0" max="120" :disabled="disabled" @change="number('tolerance', $event)" /></label></template>
      <label v-if="op === 'upscale'">放大倍数<select :value="config.scale || 2" :disabled="disabled" @change="number('scale', $event)"><option :value="2">2 倍</option><option :value="4">4 倍</option></select></label>
    </template>
    <template v-else-if="type === 'productPreview'">
      <ProductLayout :token="token" :asset-id="inputId || ''" :config="config" :disabled="disabled" @config="(key, val) => emit('config', key, val)" @patch="values => emit('patch', values)" @gesture="active => emit('gesture', active)" @expired="emit('expired')" />
      <label>展示面<select :value="config.side || 'front'" :disabled="disabled" @change="emit('config', 'side', value($event))"><option value="front">正面</option><option value="back">背面</option></select></label>
      <label>T 恤底色<input type="color" :value="config.color || '#ffffff'" :disabled="disabled" @input="emit('config', 'color', value($event))" /></label>
      <label v-for="field in [{ key: 'printX', label: '横向位置', fallback: .5 }, { key: 'printY', label: '纵向位置', fallback: .4 }, { key: 'printScale', label: '区域内大小', fallback: .6 }]" :key="field.key">{{ field.label }}<input type="range" :value="Math.min(1, Number(config[field.key] ?? field.fallback))" :min="field.key === 'printScale' ? .05 : 0" max="1" step=".01" :disabled="disabled" @input="number(field.key, $event)" /></label>
    </template>
    <template v-else-if="type === 'export'">
      <label>导出内容<select :value="config.mode || 'print'" :disabled="disabled" @change="emit('config', 'mode', value($event))"><option value="print">印花原图</option><option value="mockup">商品展示图</option><option value="image">普通图片</option></select></label>
      <label>文件格式<select :value="config.format || 'png'" :disabled="disabled" @change="emit('config', 'format', value($event))"><option value="png">PNG · 保留透明</option><option value="jpeg">JPEG · 白底</option></select></label>
      <label>文件名<input :value="config.fileName || 'POD'" maxlength="80" :disabled="disabled" @change="emit('config', 'fileName', value($event))" /></label>
      <div v-if="!config.mode || config.mode === 'print'" class="image-field-pair"><label>印刷宽度 / mm<input type="number" :value="config.printWidthMM ?? 300" min="1" max="2000" :disabled="disabled" @change="number('printWidthMM', $event)" /></label><label>印刷高度 / mm<input type="number" :value="config.printHeightMM ?? 400" min="1" max="2000" :disabled="disabled" @change="number('printHeightMM', $event)" /></label></div>
      <p class="muted">印花连接原图或处理结果；商品展示连接预览节点。DPI 根据像素和目标毫米尺寸计算，规格以生产商要求为准。</p>
    </template>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <div v-if="runNode" class="image-run-status" role="status"><strong>上次执行 · {{ runStateLabel[runNode.state] || runNode.state }}</strong><span>来源版本 v{{ runRevision }} · 参数修改后需重新执行</span><span v-if="runNode.state === 'running' && runMethod === 'ai'">CPU 图片任务仍在处理，请稍候。</span><span v-if="runNode.errorMessage" class="error-message">{{ runNode.errorMessage }}</span></div>
  </div>
</template>
