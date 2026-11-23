import { expect, it } from 'vitest'
import { cropPixels, drawnCrop, movedCrop, printGeometry, resizedCrop, rotatedSize, rotationTransform, validCrop } from './image-geometry'

it('normalizes reverse selection and bounds drawing and movement', () => {
  const rect = drawnCrop({ x: .8, y: .9 }, { x: .2, y: .3 })
  expect(rect).toEqual({ x: .2, y: .3, width: .6, height: .6 })
  expect(movedCrop(rect, { x: 3, y: -3 })).toEqual({ ...rect, x: .4, y: 0 })
  expect(drawnCrop({ x: -.2, y: -.1 }, { x: 2, y: 3 })).toEqual({ x: 0, y: 0, width: 1, height: 1 })
  expect(validCrop({ ...rect, x: .8 })).toBe(false)
  expect(validCrop({ ...rect, width: 0 })).toBe(false)
})
it('resizes all four corners while preserving the opposite corner and minimum size', () => {
  const rect = { x: .2, y: .2, width: .5, height: .5 }
  expect(resizedCrop(rect, 'nw', { x: -.1, y: -.1 }, .01, .01)).toEqual({ x: 0, y: 0, width: .7, height: .7 })
  expect(resizedCrop(rect, 'ne', { x: 1.5, y: 0 }, .01, .01)).toEqual({ x: .2, y: 0, width: .8, height: .7 })
  expect(resizedCrop(rect, 'sw', { x: 0, y: 2 }, .01, .01)).toEqual({ x: 0, y: .2, width: .7, height: .8 })
  expect(resizedCrop(rect, 'se', { x: 0, y: 0 }, .01, .01)).toEqual({ x: .2, y: .2, width: .01, height: .01 })
})
it('reports crop pixels by flooring both edges after rotation', () => {
  const size = rotatedSize(7, 11, 90)
  expect(size).toEqual({ width: 11, height: 7 })
  expect(cropPixels({ x: .15, y: .2, width: .5, height: .5 }, size.width, size.height)).toEqual({ width: 6, height: 3 })
  expect(rotatedSize(7, 11, 270)).toEqual(size)
  expect(rotatedSize(7, 11, 180)).toEqual({ width: 7, height: 11 })
  expect(rotationTransform(7, 11, 90)).toBe('translate(11 0) rotate(90)')
  expect(rotationTransform(7, 11, 270)).toBe('translate(0 7) rotate(270)')
})
it('matches the server default, centered and capped print rectangle', () => {
  expect(printGeometry(600, 800, {})).toEqual({ x: 395, y: 408, width: 210, height: 280 })
  expect(printGeometry(600, 800, { printX: .5, printY: .5 })).toEqual({ x: 395, y: 435, width: 210, height: 280 })
  expect(printGeometry(600, 800, { printX: 1, printY: 1, printScale: 1.5 })).toEqual({ x: 325, y: 383, width: 350, height: 467 })
})
