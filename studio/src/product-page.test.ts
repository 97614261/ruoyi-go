import { expect, it } from 'vitest'
import { pageDefaults, pageFilename, readPageDraft } from './product-page'
import { drawProductPage, wrapPageText } from './product-page-canvas'

const pack = { productName: '马克杯', sku: 'MUG/8', sellingPoints: '哑光釉面', referenceAssetId: '78' }

it('uses final photos or saved results before falling back to the reference, excluding test results', () => {
  const draft = pageDefaults(pack, [
    { nodeId: 'product-main', final: { assetId: '90' }, result: { output: { assetIds: ['92'] } } },
    { nodeId: 'product-scene', final: { assetId: '91', testOnly: true }, result: { output: { assetIds: ['93'] } } },
    { nodeId: 'product-detail', result: { output: { assetIds: ['94'], testOnly: true } } },
    { nodeId: 'product-angle', result: { output: { assetIds: ['95'] } } },
  ])
  expect(draft).toMatchObject({
    title: '马克杯', specs: 'SKU MUG/8', points: '哑光釉面', images: { hero: '90', scene: '93', detail: '95' },
  })
  expect(pageDefaults(pack, []).images.hero).toBe('78')
})

it('preserves removed photos in a saved draft and protects saved selections from invalid values', () => {
  const defaults = pageDefaults(pack, [])
  const stored = { ...defaults, template: 'gallery', title: '新标题', images: { hero: '', scene: '82', detail: 8 } }
  expect(readPageDraft(JSON.stringify(stored), defaults)).toMatchObject({
    template: 'gallery', title: '新标题', images: { hero: '', scene: '82', detail: '' },
  })
  expect(readPageDraft(JSON.stringify({ ...stored, template: 'constructor' }), defaults)).toEqual(defaults)
  expect(pageFilename(pack.sku, stored.title, 'gallery')).toBe('MUG-8-多图详情.png')
})

it('wraps long words and Chinese text without dropping content or splitting an emoji', () => {
  const source = '超长商品标题 👨‍👩‍👧‍👦\nSKU-AB-123-456 耐用'
  const lines = wrapPageText(source, line => Array.from(line).length <= 8)
  expect(lines.join('').replace(/\s/g, '')).toBe(source.replace(/\s/g, ''))
  expect(lines.some(line => line.includes('👨‍👩‍👧‍👦'))).toBe(true)
  expect(lines.every(line => Array.from(line).length <= 8)).toBe(true)
})

it('refuses an oversized layout before overwriting the previous preview canvas', () => {
  const context = { measureText: (value: string) => ({ width: value.length * 30 }) }
  const canvas = { width: 300, height: 150, getContext: () => context } as unknown as HTMLCanvasElement
  const draft = { ...pageDefaults(pack, []), points: ('一\n').repeat(198) }
  expect(() => drawProductPage(canvas, draft, {})).toThrow('内容太长')
  expect([canvas.width, canvas.height]).toEqual([300, 150])
})
