// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import ImageSettings from './ImageSettings.vue'
import ProductLayout from './ProductLayout.vue'
import ResultImages from './ResultImages.vue'
let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async (path: string) => String(path).includes('/content') ? new Response(new Blob(['png'], { type: 'image/png' }), { headers: { 'Content-Type': 'image/png' } }) : new Response(JSON.stringify({ code: 200, data: { assetId: '7', width: 600, height: 800, originalName: 'print.png' } }))))
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:test-image'); vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks() })
it('distinguishes basic removal from AI and disables unavailable model', async () => {
  wrapper = mount(ImageSettings, { props: { type: 'imageProcess', config: { operation: 'removeBackground' }, token: 'test' } })
  expect(wrapper.text()).toContain('仅移除从边缘连通')
  expect(wrapper.find('option[value="ai"]').attributes('disabled')).toBeDefined()
  await wrapper.setProps({ aiEnabled: true }); expect(wrapper.find('option[value="ai"]').attributes('disabled')).toBeUndefined()
  const selects = wrapper.findAll('select'); await selects[1]!.setValue('ai')
  expect(wrapper.emitted('config')).toContainEqual(['method', 'ai'])
  await wrapper.setProps({ config: { operation: 'upscale', method: 'basic' } }); expect(wrapper.text()).toContain('普通放大不会恢复真实细节')
})
it('offers PNG and white-background JPEG with physical print dimensions', async () => {
  wrapper = mount(ImageSettings, { props: { type: 'export', config: {}, token: 'test' } })
  expect(wrapper.find('option[value="webp"]').exists()).toBe(false)
  expect(wrapper.text()).toContain('PNG · 保留透明'); expect(wrapper.text()).toContain('JPEG · 白底')
  expect(wrapper.findAll('input[type="number"]')).toHaveLength(2)
  await wrapper.findAll('input[type="number"]')[0]!.setValue(250)
  expect(wrapper.emitted('config')).toContainEqual(['printWidthMM', 250])
})
it('loads private artwork with authorization and bounds keyboard placement', async () => {
  wrapper = mount(ProductLayout, { props: { token: 'private-token', assetId: '7', config: { printX: 1, printY: 0, printScale: 1 } } })
  await flushPromises()
  expect(wrapper.find('image').attributes('href')).toBe('blob:test-image')
  expect(wrapper.find('image').attributes('width')).toBe('350')
  await wrapper.find('svg').trigger('keydown', { key: 'ArrowRight' }); expect(wrapper.emitted('config')).toContainEqual(['printX', 1])
  await wrapper.find('svg').trigger('keydown', { key: 'ArrowUp' }); expect(wrapper.emitted('config')).toContainEqual(['printY', 0])
  await wrapper.setProps({ config: { side: 'back' } }); expect(wrapper.findAll('path')[1]!.attributes('d')).toContain('Q500 145')
  await wrapper.setProps({ assetId: '' }); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test-image')
  expect(String(vi.mocked(fetch).mock.calls[1]?.[0])).not.toContain('private-token')
})
it('keeps DPI warning and full-resolution preview/download actions on each output', async () => {
  wrapper = mount(ResultImages, { props: { token: 'test', ids: ['7'], info: [{ assetId: '7', width: 600, height: 800, format: 'png', kind: 'print', effectiveDpi: 50, warning: '清晰度不足' }] }, global: { stubs: { PrivateImage: true, AssetPreview: true } } })
  expect(wrapper.text()).toContain('50 DPI'); expect(wrapper.text()).toContain('清晰度不足')
  expect(wrapper.find('button[aria-label="查看结果原图 1"]').exists()).toBe(true)
  await wrapper.find('button[aria-label="查看结果原图 1"]').trigger('click'); await flushPromises()
  expect(wrapper.findComponent({ name: 'AssetPreview' }).exists()).toBe(true)
})
