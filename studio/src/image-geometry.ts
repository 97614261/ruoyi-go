export interface Point { x: number; y: number }
export interface Crop { x: number; y: number; width: number; height: number }
export type Corner = 'nw' | 'ne' | 'sw' | 'se'
export const clamp = (n: number, min = 0, max = 1) => Math.max(min, Math.min(max, n))
export const fraction = (n: number) => Math.round(n * 1e6) / 1e6
const number = (value: unknown, fallback: number) => typeof value === 'number' && Number.isFinite(value) ? value : fallback
export function cropConfig(config: Record<string, unknown>): Crop {
  return { x: number(config.cropX, 0), y: number(config.cropY, 0), width: number(config.cropWidth, 1), height: number(config.cropHeight, 1) }
}
export function cropPatch(r: Crop): Record<string, number> {
  return { cropX: r.x, cropY: r.y, cropWidth: r.width, cropHeight: r.height }
}
export function validCrop(r: Crop) {
  return [r.x, r.y, r.width, r.height].every(Number.isFinite) && r.x >= 0 && r.y >= 0 && r.width > 0 && r.height > 0 && r.x + r.width <= 1.000001 && r.y + r.height <= 1.000001
}
export function drawnCrop(a: Point, b: Point): Crop {
  const left = fraction(clamp(Math.min(a.x, b.x))), top = fraction(clamp(Math.min(a.y, b.y)))
  return { x: left, y: top, width: fraction(clamp(Math.max(a.x, b.x)) - left), height: fraction(clamp(Math.max(a.y, b.y)) - top) }
}
export function movedCrop(r: Crop, delta: Point): Crop {
  return { ...r, x: fraction(clamp(r.x + delta.x, 0, 1 - r.width)), y: fraction(clamp(r.y + delta.y, 0, 1 - r.height)) }
}
export function resizedCrop(r: Crop, corner: Corner, p: Point, minX: number, minY: number): Crop {
  const left = corner.endsWith('w') ? clamp(p.x, 0, r.x + r.width - minX) : r.x
  const top = corner.startsWith('n') ? clamp(p.y, 0, r.y + r.height - minY) : r.y
  const right = corner.endsWith('e') ? clamp(p.x, left + minX, 1) : r.x + r.width
  const bottom = corner.startsWith('s') ? clamp(p.y, top + minY, 1) : r.y + r.height
  return { x: fraction(left), y: fraction(top), width: fraction(right - left), height: fraction(bottom - top) }
}
export function cropPixels(r: Crop, width: number, height: number) {
  return { width: Math.trunc((r.x + r.width) * width) - Math.trunc(r.x * width), height: Math.trunc((r.y + r.height) * height) - Math.trunc(r.y * height) }
}
export function rotatedSize(width: number, height: number, rotate: number) {
  return [90, 270].includes(rotate) ? { width: height, height: width } : { width, height }
}
export function rotationTransform(width: number, height: number, rotate: number) {
  return ({ 90: `translate(${height} 0) rotate(90)`, 180: `translate(${width} ${height}) rotate(180)`, 270: `translate(0 ${width}) rotate(270)` } as Record<number, string>)[rotate] || ''
}
export function printGeometry(width: number, height: number, config: Record<string, unknown>) {
  const scale = Math.min(350 / width, 550 / height) * clamp(number(config.printScale, .6), .05, 1)
  const w = Math.max(1, Math.round(width * scale)), h = Math.max(1, Math.round(height * scale))
  return { width: w, height: h, x: 325 + Math.round((350 - w) * clamp(number(config.printX, .5))), y: 300 + Math.round((550 - h) * clamp(number(config.printY, .4))) }
}
