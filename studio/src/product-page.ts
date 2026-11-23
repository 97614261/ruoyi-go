export const pageTemplates = {
  highlights: { label: '卖点图', images: ['hero'] },
  detail: { label: '图文详情', images: ['hero', 'scene', 'detail'] },
  gallery: { label: '多图详情', images: ['hero', 'scene', 'detail'] },
} as const

export const pageImageLabels = { hero: '主图', scene: '场景图', detail: '细节图' } as const
export const pageTextLimits = { title: 100, specs: 180, points: 400 } as const
export type PageTemplate = keyof typeof pageTemplates
export type PageImageSlot = keyof typeof pageImageLabels
export type PageDraft = {
  template: PageTemplate
  title: string
  specs: string
  points: string
  images: Record<PageImageSlot, string>
}

type ProductPageSlot = {
  nodeId: string
  final?: { assetId: string; testOnly?: boolean }
  result?: { testOnly?: boolean; output: { assetIds?: string[]; testOnly?: boolean } }
}

function assetID(value: unknown): string {
  return typeof value === 'string' && /^[1-9][0-9]*$/.test(value) ? value : ''
}

export function pageDefaults(pack: { productName: string; sku: string; sellingPoints: string; referenceAssetId: string }, slots: ProductPageSlot[]): PageDraft {
  function fromSlot(nodeId: string) {
    const slot = slots.find(item => item.nodeId === nodeId)
    const final = slot?.final?.testOnly ? '' : assetID(slot?.final?.assetId)
    const result = slot?.result
    return final || (result?.testOnly || result?.output.testOnly ? '' : assetID(result?.output.assetIds?.[0]))
  }
  return {
    template: 'highlights', title: pack.productName, specs: pack.sku ? `SKU ${pack.sku}` : '', points: pack.sellingPoints,
    images: { hero: fromSlot('product-main') || assetID(pack.referenceAssetId), scene: fromSlot('product-scene'),
      detail: fromSlot('product-detail') || fromSlot('product-angle') || fromSlot('product-side') },
  }
}

export function readPageDraft(serialized: string | null, defaults: PageDraft): PageDraft {
  if (!serialized) return defaults
  try {
    const value = JSON.parse(serialized) as Partial<PageDraft> | null
    if (!value || !value.template || !Object.hasOwn(pageTemplates, value.template) || typeof value.title !== 'string' ||
      typeof value.specs !== 'string' || typeof value.points !== 'string' || !value.images) return defaults
    return {
      template: value.template, title: value.title.slice(0, pageTextLimits.title), specs: value.specs.slice(0, pageTextLimits.specs),
      points: value.points.slice(0, pageTextLimits.points),
      images: { hero: assetID(value.images.hero), scene: assetID(value.images.scene), detail: assetID(value.images.detail) },
    }
  } catch { return defaults }
}

export function pageFilename(sku: string, title: string, template: PageTemplate): string {
  const name = (sku.trim() || title.trim()).replace(/[<>:"/\\|?*\p{Cc}]/gu, '-').replace(/[. ]+$/g, '')
  return `${Array.from(name).slice(0, 60).join('') || '商品'}-${pageTemplates[template].label}.png`
}
