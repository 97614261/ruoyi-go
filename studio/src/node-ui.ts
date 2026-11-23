import { PhCrop, PhDownloadSimple, PhImage, PhMagicWand, PhTShirt, PhTextT } from '@phosphor-icons/vue'
import type { NodeType } from './contracts'

export const nodeIcons = { textInput: PhTextT, imageInput: PhImage, textGenerate: PhMagicWand, imageGenerate: PhImage, imageProcess: PhCrop, productPreview: PhTShirt, export: PhDownloadSimple }
export const nodeCategories: Record<NodeType, string> = { textInput: '输入', imageInput: '输入', textGenerate: '生成', imageGenerate: '生成', imageProcess: '处理', productPreview: '预览', export: '输出' }
