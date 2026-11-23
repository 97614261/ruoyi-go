import type { NodeType } from './contracts'

export interface ConfigField { key: string; label: string; type: 'text' | 'textarea' | 'select' | 'number' | 'color'; max?: number; min?: number; step?: number; options?: { value: string; label: string }[]; fallback?: string | number }
const options = (...values: string[]) => values.map(value => ({ value, label: value }))
export const nodeFields: Record<NodeType, ConfigField[]> = {
  textInput: [{ key: 'text', label: '设计要求', type: 'textarea', max: 12000 }],
  imageInput: [],
  textGenerate: [{ key: 'instruction', label: '文本处理指令', type: 'textarea', max: 12000 }],
  imageGenerate: [{ key: 'prompt', label: '补充提示词', type: 'textarea', max: 12000 }, { key: 'ratio', label: '画面比例', type: 'select', options: options('1:1', '3:4', '4:3', '9:16', '16:9'), fallback: '1:1' }, { key: 'count', label: '图片数量', type: 'number', min: 1, max: 4, step: 1, fallback: 1 }],
  imageProcess: [{ key: 'operation', label: '处理方式', type: 'select', options: [{ value: 'resize', label: '调整尺寸' }, { value: 'crop', label: '裁剪' }, { value: 'removeBackground', label: '去背景' }, { value: 'upscale', label: '放大' }], fallback: 'resize' }, { key: 'width', label: '目标宽度 / px', type: 'number', min: 64, max: 8192, step: 1, fallback: 2048 }, { key: 'height', label: '目标高度 / px', type: 'number', min: 64, max: 8192, step: 1, fallback: 2048 }, { key: 'scale', label: '放大倍数', type: 'select', options: options('2', '4'), fallback: 2 }],
  productPreview: [{ key: 'color', label: 'T 恤底色', type: 'color', fallback: '#ffffff' }, { key: 'printX', label: '印花横向位置 / 0–1', type: 'number', min: 0, max: 1, step: 0.01, fallback: 0.5 }, { key: 'printY', label: '印花纵向位置 / 0–1', type: 'number', min: 0, max: 1, step: 0.01, fallback: 0.4 }, { key: 'printScale', label: '印花缩放', type: 'number', min: 0.05, max: 1.5, step: 0.05, fallback: 0.6 }],
  export: [{ key: 'format', label: '文件格式', type: 'select', options: options('png', 'jpeg'), fallback: 'png' }, { key: 'fileName', label: '文件名，不含扩展名', type: 'text', max: 80, fallback: '我的印花' }, { key: 'mode', label: '导出内容', type: 'select', options: [{ value: 'print', label: '印刷图案' }, { value: 'mockup', label: '商品展示图' }, { value: 'image', label: '节点原图' }], fallback: 'print' }],
}

export function defaultConfig(type: NodeType): Record<string, unknown> {
  const config: Record<string, unknown> = {}
  for (const field of nodeFields[type]) if (field.fallback !== undefined) config[field.key] = field.fallback
  if (type === 'imageInput') config.assetIds = []
  if (type === 'imageProcess') Object.assign(config, { rotate: 0, cropX: 0, cropY: 0, cropWidth: 1, cropHeight: 1, method: 'basic', background: '#ffffff', tolerance: 30 })
  if (type === 'export') Object.assign(config, { printWidthMM: 300, printHeightMM: 400 })
  if (type === 'productPreview') Object.assign(config, { product: 'tshirt', side: 'front' })
  return config
}
