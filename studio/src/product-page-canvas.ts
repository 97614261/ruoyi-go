import type { PageDraft, PageImageSlot } from './product-page'

const width = 1080
const maxHeight = 4096
const margin = 76
const contentWidth = width - 2 * margin
const fonts = '"Manrope Variable", "PingFang SC", "Microsoft YaHei", sans-serif'
const colors = { paper: '#f4f2ea', ink: '#182720', muted: '#4c6056', green: '#263f32', white: '#f8faf3', media: '#ffffff', accent: '#c3d3a4' }
const segmenter = typeof Intl.Segmenter === 'function' ? new Intl.Segmenter(undefined, { granularity: 'grapheme' }) : null

export function wrapPageText(value: string, fits: (text: string) => boolean): string[] {
  return value.trim().split(/\r?\n/u).map(line => line.trim()).filter(Boolean).flatMap(paragraph => {
    const units = paragraph.match(/\s+|[A-Za-z0-9]+(?:[._/-][A-Za-z0-9]+)*|[^\x00-\x7f]+|./gu) || []
    const lines: string[] = []
    let line = ''
    function append(unit: string) {
      if (line && !fits(line + unit)) {
        lines.push(line.trimEnd())
        line = ''
      }
      line += line ? unit : unit.trimStart()
    }
    for (const unit of units) {
      if (fits(unit)) append(unit)
      else {
        const characters = segmenter ? Array.from(segmenter.segment(unit), item => item.segment) : Array.from(unit)
        characters.forEach(append)
      }
    }
    if (line.trim()) lines.push(line.trimEnd())
    return lines
  })
}

type SourceImage = Pick<HTMLImageElement, 'naturalWidth' | 'naturalHeight'> & CanvasImageSource
type TextStyle = { size: number; lineHeight: number; weight: number; color: string }
type Block = (context: CanvasRenderingContext2D) => void

export function drawProductPage(canvas: HTMLCanvasElement, draft: PageDraft, images: Partial<Record<PageImageSlot, SourceImage>>) {
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器无法生成图片。')
  const measure = context
  const blocks: Block[] = []
  let y = 0
  function rectangle(x: number, top: number, w: number, h: number, color: string) {
    blocks.push(ctx => { ctx.fillStyle = color; ctx.fillRect(x, top, w, h) })
  }
  function paragraph(value: string, x: number, top: number, w: number, style: TextStyle) {
    const font = `${style.weight} ${style.size}px ${fonts}`
    measure.font = font
    const lines = wrapPageText(value, text => measure.measureText(text).width <= w)
    blocks.push(ctx => {
      ctx.font = font
      ctx.fillStyle = style.color
      ctx.textBaseline = 'alphabetic'
      lines.forEach((line, index) => ctx.fillText(line, x, top + style.size + index * style.lineHeight, w))
    })
    return lines.length * style.lineHeight
  }
  function photo(image: SourceImage, x: number, top: number, w: number, h: number) {
    rectangle(x, top, w, h, colors.media)
    const scale = Math.min(w / image.naturalWidth, h / image.naturalHeight)
    const iw = image.naturalWidth * scale, ih = image.naturalHeight * scale
    blocks.push(ctx => ctx.drawImage(image, x + (w - iw) / 2, top + (h - ih) / 2, iw, ih))
  }
  function heading(top: number, dark = false) {
    let end = top + paragraph(draft.title, margin, top, contentWidth, { size: 76, lineHeight: 97, weight: 750, color: dark ? colors.white : colors.ink })
    if (draft.specs.trim()) {
      end += 20 + paragraph(draft.specs, margin, end + 20, contentWidth, { size: 31, lineHeight: 45, weight: 500, color: dark ? colors.accent : colors.muted })
    }
    return end
  }
  function sellingPoints(top: number) {
    if (!draft.points.trim()) return top
    rectangle(margin, top + 16, 34, 6, colors.muted)
    const label = { size: 30, lineHeight: 45, weight: 700, color: colors.muted }
    paragraph('商品卖点', margin + 52, top - 7, contentWidth - 52, label)
    return top + 62 + paragraph(draft.points, margin, top + 62, contentWidth, { size: 40, lineHeight: 59, weight: 500, color: colors.ink })
  }

  const hero = images.hero
  if (draft.template === 'highlights') {
    y = heading(margin) + 42
    if (hero) {
      photo(hero, margin, y, contentWidth, 695)
      y += 753
    }
    y = sellingPoints(y) + margin
  } else if (draft.template === 'detail') {
    if (hero) {
      photo(hero, 0, 0, width, 880)
      y = 880
    }
    const headerIndex = blocks.length
    const headerStart = y
    y = heading(y + 68, true) + 68
    const headerHeight = y - headerStart
    blocks.splice(headerIndex, 0, ctx => {
      ctx.fillStyle = colors.green
      ctx.fillRect(0, headerStart, width, headerHeight)
    })
    y = sellingPoints(y + margin)
    for (const image of [images.scene, images.detail]) {
      if (!image) continue
      photo(image, margin, y + 38, contentWidth, 700)
      y += 738
    }
    y += margin
  } else {
    y = heading(margin) + 42
    if (hero) {
      photo(hero, margin, y, contentWidth, 795)
      y += 817
    }
    const gallery = [images.scene, images.detail].filter(image => image !== undefined)
    if (gallery.length) {
      const gap = 22
      const w = gallery.length === 1 ? contentWidth : (contentWidth - gap) / 2
      for (const [index, image] of gallery.entries()) photo(image, margin + index * (w + gap), y, w, 500)
      y += 558
    }
    y = sellingPoints(y) + margin
  }

  if (y > maxHeight) throw new Error('内容太长，请缩短文字后再导出。')
  canvas.width = width
  canvas.height = Math.max(410, Math.ceil(y))
  context.fillStyle = colors.paper
  context.fillRect(0, 0, canvas.width, canvas.height)
  blocks.forEach(block => block(context))
}
