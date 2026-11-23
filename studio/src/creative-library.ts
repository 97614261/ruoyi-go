import type { Graph, NodeType } from './contracts'

const classicPrompts = [
  { title: '蓝色野花', category: '植物花卉', text: '白色背景上的蓝色野花印花，简洁平面插画，花朵居中，细长绿茎，少量叶片，轮廓清晰，适合 T 恤印刷，无文字。' },
  { title: '复古植物标本', category: '植物花卉', text: '复古植物图谱风格的蕨类印花，深绿色与浅米色，精细但清晰的叶片线条，主体独立，四周留白，无文字，无水印。' },
  { title: '周末露营', category: '户外生活', text: '复古露营徽章图案，松林、山峰和小帐篷，圆形紧凑构图，墨绿、赭红、奶白三色，丝网印刷质感，无文字，白色背景。' },
  { title: '滑板猫咪', category: '萌宠插画', text: '一只橘猫站在滑板上的可爱插画，略带夸张的神态，粗细均匀的轮廓，橘色与海蓝色搭配，主体完整，白色背景，无文字。' },
  { title: '冬日小屋', category: '节日主题', text: '冬日节日主题的温馨小屋印花，雪松、窗灯、雪花，红绿配色中加入暖黄色，居中构图，边缘整洁，白色背景，无文字。' },
  { title: '海边日落', category: '户外生活', text: '复古海边日落印花，太阳、海浪与两棵棕榈树，几何色块，橙红、金黄、深蓝三色，清晰分层，白色背景，无文字。' },
]
const newPrompts = [
  ['夜航飞蛾', '植物花卉', '深蓝,线稿,印花', '夜行飞蛾停在两枝银叶之间，翅纹用细密但不断裂的线条表现，墨蓝与冷银双色，对称稳定，构图边缘留白，纯色背景，无文字、无水印。'],
  ['橙花枝条', '植物花卉', '暖橙,植物,手绘', '一枝橙花和两枚小果实，铅笔线稿叠加低饱和色铅笔，花瓣奶白、枝叶橄榄绿、果皮暖橙，单独图案，留出干净外轮廓。'],
  ['高山路线徽章', '户外生活', '山地,徽章,两色', '以山脊剪影和连续路线作一枚无字登山徽章，圆角菱形边框，深墨绿与象牙白两色，减少碎线，适合缩小印刷。'],
  ['破晓冲浪', '户外生活', '海浪,运动,复古', '冲浪板划过破晓海浪，主浪是大块弧线，浪尖点缀小面积米白，海军蓝与浅橘两色，低位地平线，独立印花，不画衣服。'],
  ['花园兔邮票', '萌宠插画', '兔子,邮票,童趣', '一只垂耳兔端着一束小雏菊，做成有齿边的无字邮票插画，淡绿、浅棕和米白三色，线条可爱但不过细，背景干净。'],
  ['黑猫月相', '萌宠插画', '猫咪,月亮,线稿', '墨色黑猫靠在弯月旁，胡须和月相少量留白，保留猫背的清晰剪影，黑白为主并加一点冷金，不添加星空照片或文字。'],
  ['秋收南瓜', '节日主题', '秋季,南瓜,徽章', '南瓜、橡果和秋叶组成紧凑三角构图，仿手工丝网印刷，南瓜橙和林绿两种主色，粗实描边，无祝福文字，可印在布袋或服装。'],
  ['彩灯小集市', '节日主题', '冬季,灯串,节日', '冬夜节日集市的微缩插图，深蓝小摊配一串琥珀色灯光，三层结构清晰，不画招牌文字或品牌；保留干净四周边界。'],
  ['蓝白海鸟', '印花风格', '木刻,单色,丝网', '一只俯冲海鸟掠过两道浪纹，蓝白木刻风格，粗短雕刻线优先，不做细碎渐变；主体不裁切，适合单色印刷。'],
  ['复古公路标牌', '印花风格', '丝网,几何,公路', '直路穿过两侧荒漠巨石和落日，做成无字公路徽章，用砖红、沙棕、墨蓝三块平涂色表现，几何化、对比明显，无背景环境。'],
  ['花瓣无缝花纹', '印花风格', '无缝,织物,图案', '俯视分散排列的小山茶花瓣与针叶，做成正方形重复单元，上下和左右边界严格对齐，大小节奏有变化，留出布面呼吸感，无边框无文字。'],
  ['双面队服标记', '印花风格', '队服,双面,轮廓', '设计一组无字队标：小型胸标仅保留几何鹰首，大型背标加展开羽翼，两种构图用同一组钴蓝与骨白色，线条粗实、轮廓完整。'],
  ['干净白底主图', '商品视觉', '主图,白底,电商', '以输入图的商品为准，保留外轮廓、颜色、标识与比例。去掉杂物，放在干净浅白背景，接触处有很淡的自然阴影，主体居中，不生成额外部件或文字。'],
  ['木台侧光展示', '商品视觉', '场景图,自然光,摄影', '保留参考商品的形状、比例、标记与材质。商品放在浅橡木台面，左侧窗光，背景简洁且适度模糊，视角略俯，不加人物、价格或宣传文字。'],
  ['三色同款陈列', '商品视觉', '配色,系列,陈列', '保留同一商品的剪裁、印花位置和尺度，将配色限定为暗绿、暖棕或深蓝之一。视角和光照保持一致，适合做并排比较，不捏造品牌或新结构。'],
  ['帆布袋近景', '商品视觉', '布料,纹理,局部', '帆布袋的局部展示，画面保留原有印花与布纹，柔和侧光强调缝线和纤维，不增加装饰品；近景但主体图案不要被裁断。'],
  ['蓝橙丝网海报', '版式与海报', '海报,双色,留白', '用钴蓝平面形和橘红圆面组成复古双色海报胚图，右下主体，上半部留排字空间，三层色块有清楚边界，不生成文字或伪字。'],
  ['旅行贴纸套组', '版式与海报', '贴纸,刀线,套组', '生成四枚彼此不接触的旅行贴纸：行李箱、山峰、日出和小船。统一白色外轮廓和配色，四格保持等间距，保留裁切空隙，不加文字。'],
] as const

const categoryPalettes: Record<string, string[]> = {
  '植物花卉': ['#e6e4ca', '#738b77', '#263d46'], '户外生活': ['#d8aa71', '#86553f', '#284c4a'], '萌宠插画': ['#f6e5ba', '#d58b58', '#665b8c'], '节日主题': ['#f1dca9', '#ae5d55', '#45655c'], '印花风格': ['#d9d6cb', '#5472a1', '#202e49'], '商品视觉': ['#f1e6d9', '#a9a89c', '#4f554b'], '版式与海报': ['#f2deaa', '#d26850', '#385b90'],
}
type PromptField = { key: string; label: string; placeholder: string; initial?: string; required?: boolean }
type ProductPrompt = { fields: PromptField[]; compose: (values: Record<string, string>) => string }
const productName: PromptField = { key: 'product', label: '商品名', placeholder: '例如 陶瓷马克杯', required: true }
const productPrompts: Record<string, ProductPrompt> = {
  'idea-19': {
    fields: [productName,
      { key: 'angle', label: '拍摄角度', placeholder: '例如 正面平视', initial: '正面平视' },
      { key: 'shadow', label: '阴影', placeholder: '例如 很淡的接触阴影', initial: '很淡的自然接触阴影' }],
    compose: ({ product, angle, shadow }) => `以参考图里的${product}为商品主体，保留它的外轮廓、颜色、标识和比例。采用${angle}，商品完整居中，背景为纯白，只保留${shadow}。不增加道具、文字、水印或原图没有的部件。`,
  },
  'idea-20': {
    fields: [productName,
      { key: 'scene', label: '使用场景', placeholder: '例如 清晨的木桌', initial: '浅橡木台面与简洁背景' },
      { key: 'angle', label: '拍摄角度', placeholder: '例如 三分之四侧视', initial: '略微俯拍' },
      { key: 'light', label: '光线', placeholder: '例如 左侧自然窗光', initial: '左侧柔和窗光' }],
    compose: ({ product, scene, angle, light }) => `用参考图里的${product}做场景图，保留它的形状、比例、标记和材质。场景设为${scene}，用${angle}展示商品，以${light}突出主体，背景适度虚化。不添加价格、宣传文字或未经提供的商品信息。`,
  },
  'idea-22': {
    fields: [productName,
      { key: 'detail', label: '细节重点', placeholder: '例如 缝线和布料纹理', initial: '缝线和织物纹理' },
      { key: 'angle', label: '拍摄角度', placeholder: '例如 贴近主体的斜侧视', initial: '斜侧视近景' }],
    compose: ({ product, detail, angle }) => `以参考图里的${product}为准，用${angle}拍摄局部特写，重点展示${detail}。柔和侧光表现实际质感，保留原有结构、图案与色彩。只展示参考图能确认的细节，不添加图外的零件或装饰。`,
  },
}
const referenceIdeas = new Set([...Object.keys(productPrompts), 'idea-21'])
export const prompts = [...classicPrompts.map(item => ({ ...item, tags: [item.category, 'POD 印花'] })), ...newPrompts.map(([title, category, tags, text]) => ({ title, category, tags: tags.split(','), text }))].map((item, index) => {
  const id = `idea-${index + 1}`
  return { ...item, id, colors: categoryPalettes[item.category] || categoryPalettes['印花风格']!, parameters: productPrompts[id], requiresReference: referenceIdeas.has(id) }
})

export function resolvePrompt(prompt: typeof prompts[number], edits: Record<string, string>) {
  if (!prompt.parameters) return { text: prompt.text, ready: true }
  const { fields, compose } = prompt.parameters
  const values = Object.fromEntries(fields.map(field => [field.key, edits[field.key]?.trim() || field.initial || '参考图中的商品']))
  return { text: compose(values), ready: fields.every(field => !field.required || Boolean(edits[field.key]?.trim())) }
}

export const presetCover = (kind: string) => import.meta.env.BASE_URL + 'preset-examples/' + kind + '.webp'
const textNode = { id: 'text', type: 'textInput' as const, version: 1, position: { x: 0, y: 50 }, config: { text: prompts[0]!.text } }
const imageNode = { id: 'image', type: 'imageGenerate' as const, version: 1, position: { x: 410, y: 50 }, config: { ratio: '1:1', count: 1 } }
const classicTemplates: { title: string; description: string; kind: string; graph: Graph }[] = [

  { title: '一条想法生成印花', description: '输入要求，可先润色，再生成一张图。', kind: 'text-image', graph: { schemaVersion: 1, nodes: [textNode, imageNode], edges: [{ id: 'text-image', source: 'text', sourcePort: 'text', target: 'image', targetPort: 'text' }] } },
  { title: '参考图创作', description: '选择你的参考图，补充要求后生成新图。', kind: 'reference', graph: { schemaVersion: 1, nodes: [textNode, { id: 'reference', type: 'imageInput', version: 1, position: { x: 0, y: 350 }, config: { assetIds: [] } }, imageNode], edges: [{ id: 'text-image', source: 'text', sourcePort: 'text', target: 'image', targetPort: 'text' }, { id: 'ref-image', source: 'reference', sourcePort: 'images', target: 'image', targetPort: 'images' }] } },
  { title: '两种风格做对比', description: '同一主题分两路生成，保留不同风格。', kind: 'compare', graph: { schemaVersion: 1, nodes: [textNode, { ...imageNode, position: { x: 410, y: 0 }, config: { ratio: '1:1', count: 1, prompt: '平面色块，极简构图' } }, { ...imageNode, id: 'image2', position: { x: 410, y: 380 }, config: { ratio: '1:1', count: 1, prompt: '细腻手绘，复古图谱风格' } }], edges: [{ id: 'text-image', source: 'text', sourcePort: 'text', target: 'image', targetPort: 'text' }, { id: 'text-image2', source: 'text', sourcePort: 'text', target: 'image2', targetPort: 'text' }] } },
	{ title: '已有印花做 T 恤', description: '选择图片，去掉纯色背景，分别导出印花与 T 恤展示图。不调用付费模型。', kind: 'print-mockup', graph: { schemaVersion: 1, nodes: [
		{ id: 'input', type: 'imageInput', version: 1, position: { x: 0, y: 100 }, config: { assetIds: [] } },
		{ id: 'process', type: 'imageProcess', version: 1, position: { x: 400, y: 100 }, config: { operation: 'removeBackground', method: 'basic', background: '#ffffff', tolerance: 30 } },
		{ id: 'preview', type: 'productPreview', version: 1, position: { x: 800, y: 100 }, config: { product: 'tshirt', side: 'front', color: '#233446', printX: .5, printY: .4, printScale: .75 } },
		{ id: 'mockup', type: 'export', version: 1, position: { x: 1200, y: 100 }, config: { mode: 'mockup', format: 'jpeg', fileName: 'T恤展示' } },
		{ id: 'print', type: 'export', version: 1, position: { x: 800, y: 550 }, config: { mode: 'print', format: 'png', fileName: '印花', printWidthMM: 300, printHeightMM: 400 } },
	], edges: [
		{ id: 'input-process', source: 'input', sourcePort: 'images', target: 'process', targetPort: 'images' },
		{ id: 'process-preview', source: 'process', sourcePort: 'images', target: 'preview', targetPort: 'images' },
		{ id: 'preview-mockup', source: 'preview', sourcePort: 'images', target: 'mockup', targetPort: 'images' },
		{ id: 'process-print', source: 'process', sourcePort: 'images', target: 'print', targetPort: 'images' },
	] } },
]

type PresetEntry = typeof classicTemplates[number] & { category: string; input: string; output: string }
const classicMeta = [ ['创意起稿', '创意描述', '一张印花初稿'], ['参考图改造', '参考图与改造要求', '参考图变体'], ['创意起稿', '同一个设计主题', '两个风格版本'], ['印花与样机', '已有印花图', '印花原稿和 T 恤展示图'] ] as const
function chain(kind: string, title: string, description: string, category: string, input: string, output: string, steps: [NodeType, Record<string, unknown>][], channels: ('text' | 'images')[]): PresetEntry {
  const nodes = steps.map(([type, config], index) => ({ id: `step-${index + 1}`, type, version: 1, position: { x: index * 390, y: 40 }, config }))
  const edges = channels.map((port, index) => ({ id: `link-${index + 1}`, source: nodes[index]!.id, target: nodes[index + 1]!.id, sourcePort: port, targetPort: port }))
  return { kind, title, description, category, input, output, graph: { schemaVersion: 1, nodes, edges } }
}
const emptyImage = (): [NodeType, Record<string, unknown>] => ['imageInput', { assetIds: [] }]
const imageExport = (fileName: string, format = 'png'): [NodeType, Record<string, unknown>] => ['export', { mode: 'image', fileName, format }]
export const templates: PresetEntry[] = [
  ...classicTemplates.map((item, i) => { const [category, input, output] = classicMeta[i]!; return { ...item, category, input, output } }),
  chain('brief-to-draft', '把一句话变成设计稿', '先把简短需求整理成图像提示词，再生成初稿。', '创意起稿', '一句设计需求', '结构化提示词和一张图', [['textInput', { text: '用黑猫和弯月做一枚两色印花，适合深色帆布袋。' }], ['textGenerate', { instruction: '只输出可直接生图的中文提示词。明确主体、构图、配色、印刷轮廓与禁止元素，保留原主题。' }], ['imageGenerate', { ratio: '1:1', count: 1 }]], ['text', 'text']),
  chain('square-product', '商品图裁切与缩放', '先裁掉多余留白，再按比例缩放到 1600 像素以内。', '商品出图', '一张商品图，先检查裁剪框', '缩放后的商品 PNG', [emptyImage(), ['imageProcess', { operation: 'crop', method: 'basic', rotate: 0, cropX: .1, cropY: .1, cropWidth: .8, cropHeight: .8 }], ['imageProcess', { operation: 'resize', method: 'basic', width: 1600, height: 1600 }], imageExport('square-product')], ['images', 'images', 'images']),
  chain('solid-cutout', '白底素材转透明图', '去掉与背景接近的白色，保留主体并导出透明 PNG。', '素材处理', '白底或近白底素材', '透明底 PNG', [emptyImage(), ['imageProcess', { operation: 'removeBackground', method: 'basic', background: '#ffffff', tolerance: 18 }], imageExport('transparent-cutout')], ['images', 'images']),
  chain('print-ready', '整理印花原图', '用普通插值放大已有素材，导出前检查尺寸和有效 DPI。', '印花与样机', '清晰的印花原图', '印花 PNG', [emptyImage(), ['imageProcess', { operation: 'upscale', method: 'basic', scale: 2 }], ['export', { mode: 'print', format: 'png', fileName: 'print-ready', printWidthMM: 300, printHeightMM: 300 }]], ['images', 'images']),
  chain('back-preview', 'T 恤背面样机', '把已有图案放到背面，调整位置后导出展示图。', '印花与样机', '建议使用透明底印花', 'T 恤背面展示图', [emptyImage(), ['productPreview', { product: 'tshirt', side: 'back', color: '#253641', printX: .5, printY: .4, printScale: .7 }], ['export', { mode: 'mockup', format: 'jpeg', fileName: 'tshirt-back' }]], ['images', 'images']),
  chain('resize-delivery', '缩小图片做发货预览', '限制图片尺寸，再导出便于转发的 JPEG。', '素材处理', '已有商品展示图', 'JPEG 预览图', [emptyImage(), ['imageProcess', { operation: 'resize', method: 'basic', width: 1200, height: 1200 }], imageExport('delivery-preview', 'jpeg')], ['images', 'images']),
]

export function matchesLibrary(query: string, fields: (string | number)[]) {
  const content = fields.join(' ').normalize('NFKC').toLocaleLowerCase()
  return query.normalize('NFKC').toLocaleLowerCase().trim().split(/\s+/).every(word => content.includes(word))
}
