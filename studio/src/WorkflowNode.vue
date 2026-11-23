<script setup lang="ts">
import { computed } from 'vue'
import { Position } from '@vue-flow/core'
import { PhCopy, PhImage, PhPlus, PhSlidersHorizontal, PhSparkle, PhTrash, PhTShirt } from '@phosphor-icons/vue'
import type { NodeDefinition, RunNode } from './contracts'
import { nodeIcons } from './node-ui'
import { runStateLabel } from './run-state'
import PrivateImage from './PrivateImage.vue'
import ResultImages from './ResultImages.vue'
import MagneticHandle from './MagneticHandle.vue'

const props = defineProps<{ id: string; data: { definition: NodeDefinition; config: Record<string, unknown> }; selected?: boolean; token: string; runNode?: RunNode; runRevision?: string; imageResult?: { revision: string; node: RunNode }; currentRevision?: string; draftChanged?: boolean; disabled?: boolean; handleMotion?: boolean; existingConnection?: boolean; quickPort?: string }>()
const emit = defineEmits<{ text: [value: string]; choose: []; expired: []; copy: []; remove: []; inspect: []; quickAdd: [side: 'source' | 'target', key: string, anchor: HTMLElement] }>()
const assets = computed(() => Array.isArray(props.data.config.assetIds) ? props.data.config.assetIds.filter((id): id is string => typeof id === 'string') : [])
const portName = (type: string) => ({ text: '文本', images: '图片', files: '文件' }[type] || type)
const images = computed(() => props.imageResult?.node.output || props.runNode?.output)
const previousImage = computed(() => props.imageResult && (props.draftChanged || props.imageResult.revision !== props.currentRevision || props.runNode !== props.imageResult.node))
const portTop = (index: number, count: number) => `calc(50% + ${index - (count - 1) / 2} * var(--node-port-step, 38px))`
</script>

<template>
  <article class="workflow-node" :class="[{ 'is-selected': selected, 'has-run': !!runNode }, `workflow-${data.definition.type}`]">
    <header class="workflow-node-title">
      <span class="node-icon"><component :is="nodeIcons[data.definition.type]" :size="17" weight="duotone" /></span>
      <strong>{{ data.config.title || data.definition.label }}</strong>
    </header>
    <div class="node-title-actions node-float-tools nodrag nopan" role="group" :aria-label="`${data.config.title || data.definition.label} 快捷操作`" @click.stop><button class="icon-button" :aria-label="`设置节点 ${id}`" title="节点参数" @click="emit('inspect')"><PhSlidersHorizontal :size="15" /><span>参数</span></button><button class="icon-button" :aria-label="`复制节点 ${id}`" title="复制节点" :disabled="disabled" @click="emit('copy')"><PhCopy :size="15" /><span>复制</span></button><button class="icon-button" :aria-label="`删除节点 ${id}`" title="删除节点" :disabled="disabled" @click="emit('remove')"><PhTrash :size="15" /></button></div>
    <div class="workflow-node-content">
      <template v-if="data.definition.type === 'textInput'">
        <textarea :value="String(data.config.text || '')" class="nodrag nowheel" :aria-label="`${data.config.title || '文本输入'} 的设计要求`" placeholder="写下主题、风格与设计要求…" rows="3" maxlength="12000" :disabled="disabled" @click.stop @input="emit('text', ($event.target as HTMLTextAreaElement).value)" />
      </template>
      <div v-else-if="data.definition.type === 'imageInput'" class="node-image-input nodrag nowheel">
        <PrivateImage v-if="assets[0]" :asset-id="assets[0]" :token="token" alt="图片输入节点的第一张图片" @expired="emit('expired')" />
        <div v-else class="node-image-placeholder"><PhImage :size="32" weight="light" /><span>放入你的参考图</span><small>PNG / JPG / WebP</small></div>
        <button class="node-pick-image" :disabled="disabled" @click.stop="emit('choose')"><PhPlus :size="15" />{{ assets.length ? `已选 ${assets.length} 张 · 修改图片` : '选择或上传图片' }}</button>
      </div>
      <template v-else-if="data.definition.type === 'imageGenerate'">
        <ResultImages v-if="images?.assetIds?.length" :ids="images.assetIds" :info="images.images" :token="token" @expired="emit('expired')" />
        <div v-else class="node-image-placeholder node-generation-preview"><PhSparkle :size="31" weight="light" /><span>暂无生成图片</span><small>选中节点，设置参数后生成</small></div>
      </template>
      <template v-else-if="['imageProcess', 'productPreview', 'export'].includes(data.definition.type)">
        <ResultImages :compare="data.definition.type === 'imageProcess'" v-if="images?.assetIds?.length" :ids="images.assetIds" :info="images.images" :token="token" @expired="emit('expired')" />
        <div v-else-if="data.definition.type === 'productPreview'" class="node-product-placeholder"><PhTShirt :size="48" weight="light" /><span>T 恤印花布局</span><small>正反面、底色和印花位置可保存</small></div>
        <p v-else class="node-config-summary">{{ data.definition.type === 'export' ? (data.config.mode === 'mockup' ? '商品展示图' : data.config.mode === 'image' ? '普通图片' : '印花原图') : ({ resize: '等比调整尺寸', crop: '旋转与裁剪', removeBackground: data.config.method === 'ai' ? 'AI 抠图' : '纯色去背景', upscale: data.config.method === 'ai' ? 'AI 高清放大' : '普通插值放大' }[String(data.config.operation || 'resize')] || '图片处理') }}</p>
      </template>
      <template v-else>
        <p v-if="runNode?.output.text && data.definition.type === 'textGenerate'" class="node-run-output nodrag nowheel">{{ runNode.output.text }}</p>
        <p v-else class="node-config-summary">{{ data.config.instruction || data.definition.description }}</p>
      </template>
    </div>
    <MagneticHandle v-for="(port, index) in data.definition.inputs" :key="`in-${port.key}`" :id="port.key" type="target" :position="Position.Left" :top="portTop(index, data.definition.inputs.length)" :value-type="port.valueType" :label="`${data.definition.label} 输入 ${portName(port.valueType)}`" :disabled="disabled" :following="handleMotion" quick :existing-connection="existingConnection" :pressed-menu="quickPort === `target:${port.key}`" @quick="anchor => emit('quickAdd', 'target', port.key, anchor)" />
    <MagneticHandle v-for="(port, index) in data.definition.outputs" :key="`out-${port.key}`" :id="port.key" type="source" :position="Position.Right" :top="portTop(index, data.definition.outputs.length)" :value-type="port.valueType" :label="`${data.definition.label} 输出 ${portName(port.valueType)}`" :disabled="disabled" :following="handleMotion" quick :existing-connection="existingConnection" :pressed-menu="quickPort === `source:${port.key}`" @quick="anchor => emit('quickAdd', 'source', port.key, anchor)" />
    <footer v-if="runNode || previousImage"><span v-if="previousImage" title="这是之前生成的图片，不代表当前设置">历史图片 · v{{ imageResult?.revision }}</span><span v-else-if="runNode" :title="`上次执行的来源版本 v${runRevision}；参数修改后需重新执行`">上次{{ ['imageProcess', 'productPreview', 'export'].includes(data.definition.type) ? '图片处理' : runNode.output.testOnly ? '测试执行' : '模型执行' }} · v{{ runRevision }}</span><span v-if="runNode" class="run-state" :data-state="runNode.state">{{ runNode.errorCode === 'archivePending' ? '图片待保存' : runStateLabel[runNode.state] }}</span></footer>
  </article>
</template>
