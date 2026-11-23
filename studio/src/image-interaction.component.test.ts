// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import CropEditor from './CropEditor.vue'
import ProductLayout from './ProductLayout.vue'
import ImageSettings from './ImageSettings.vue'
import ResultImages from './ResultImages.vue'
import ImageCompareDialog from './ImageCompareDialog.vue'
import type { RunNode } from './contracts'

let wrapper: VueWrapper | undefined
const meta = (id: string) => ({ assetId: id, parentAssetId: id === '9' ? '7' : '0', width: id === '9' ? 300 : 600, height: id === '9' ? 400 : 800, originalName: `${id}.png` })
const json = (data: unknown, code = 200) => new Response(JSON.stringify({ code, data, msg: '无权读取图片' }))
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path.includes('/content') ? new Response(new Blob(['png']), { headers: { 'Content-Type': 'image/png' } }) : json(meta(path.split('/').pop()!))))
  vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:${Math.random()}`)
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks() })
function surface(w: VueWrapper, width = 300, height = 400) {
  const el = w.find('svg').element as SVGSVGElement
  Object.assign(el, { getBoundingClientRect: () => ({ x: 0, y: 0, left: 0, top: 0, width, height }), setPointerCapture: vi.fn(), hasPointerCapture: () => true, releasePointerCapture: vi.fn() })
  return w.find('svg')
}
async function crop(config = {}) {
  wrapper = mount(CropEditor, { props: { assetId: '7', token: 'private-token', config } }); await flushPromises(); return wrapper
}
it('commits a crop only on release, once, and preserves the private header', async () => {
  const w = await crop(), svg = surface(w)
  await svg.trigger('pointerdown', { pointerId: 1, button: 0, clientX: 30, clientY: 40 })
  await svg.trigger('pointermove', { pointerId: 1, clientX: 180, clientY: 280 })
  expect(w.emitted('patch')).toBeUndefined()
  await svg.trigger('pointerup', { pointerId: 1, clientX: 180, clientY: 280 })
  expect(w.emitted('patch')).toEqual([[{ cropX: .1, cropY: .1, cropWidth: .5, cropHeight: .6 }]])
  expect(w.emitted('gesture')).toEqual([[true], [false]])
  expect(new Headers(vi.mocked(fetch).mock.calls[1]![1]?.headers).get('Authorization')).toBe('Bearer private-token')
  expect(String(vi.mocked(fetch).mock.calls[1]![0])).not.toContain('private-token')
  const shortcut = vi.fn(); w.element.addEventListener('keydown', shortcut)
  await svg.trigger('keydown', { key: 's', ctrlKey: true }); await svg.trigger('keydown', { key: 'z', ctrlKey: true })
  expect(shortcut).toHaveBeenCalledTimes(2)
})
it('cancels on Escape, source rotation, disable and lost capture without saving a half crop', async () => {
  const w = await crop(), svg = surface(w)
  const begin = async () => { await svg.trigger('pointerdown', { pointerId: 1, button: 0, clientX: 120, clientY: 160 }); await svg.trigger('pointermove', { pointerId: 1, clientX: 180, clientY: 280 }) }
  await begin(); await svg.trigger('keydown', { key: 'Escape' })
  await begin(); await w.setProps({ config: { rotate: 90 } })
  await begin(); await w.setProps({ disabled: true })
  await w.setProps({ disabled: false }); await begin(); await svg.trigger('lostpointercapture')
  expect(w.emitted('patch')).toBeUndefined()
  expect(w.emitted('gesture')?.filter(([active]) => active === false)).toHaveLength(4)
})
it('moves and resizes existing crop with bounds and supplies keyboard and reset alternatives', async () => {
  const config = { cropX: .2, cropY: .2, cropWidth: .5, cropHeight: .5 }, w = await crop(config), svg = surface(w)
  await w.find('[data-crop-body]').trigger('pointerdown', { pointerId: 2, button: 0, clientX: 90, clientY: 120 })
  await svg.trigger('pointerup', { pointerId: 2, clientX: 900, clientY: 900 })
  expect(w.emitted('patch')?.at(-1)).toEqual([{ ...config, cropX: .5, cropY: .5 }])
  await w.find('[data-corner=se]').trigger('pointerdown', { pointerId: 2, button: 0, clientX: 210, clientY: 280 })
  await svg.trigger('pointerup', { pointerId: 2, clientX: 270, clientY: 360 })
  expect(w.emitted('patch')?.at(-1)).toEqual([{ ...config, cropWidth: .7, cropHeight: .7 }])
  await svg.trigger('keydown', { key: 'ArrowRight', shiftKey: true })
  expect(w.emitted('patch')?.at(-1)).toEqual([{ ...config, cropX: .3 }])
  await w.findAll('button')[1]!.trigger('click')
  expect(w.emitted('patch')?.at(-1)).toEqual([{ cropX: 0, cropY: 0, cropWidth: 1, cropHeight: 1 }])
})
it('rejects invalid numeric regions and accepts one complete valid patch', async () => {
  wrapper = mount(ImageSettings, { props: { type: 'imageProcess', token: 'test', config: { operation: 'crop' } } })
  const inputs = wrapper.findAll('input[type=number]')
  await inputs[0]!.setValue(.5)
  expect(wrapper.text()).toContain('起点加保留尺寸不能超过 1')
  expect(wrapper.emitted('patch')).toBeUndefined()
  await inputs[2]!.setValue(.5)
  expect(wrapper.emitted('patch')).toEqual([[{ cropX: 0, cropY: 0, cropWidth: .5, cropHeight: 1 }]])
})
it('labels status as the previous snapshot and identifies CPU work from that snapshot', async () => {
  const runNode: RunNode = { nodeId: 'process', nodeType: 'imageProcess', state: 'running', attempt: 1, output: { testOnly: false }, errorCode: '', errorMessage: '' }
  wrapper = mount(ImageSettings, { props: { type: 'imageProcess', token: 'test', config: { operation: 'removeBackground', method: 'basic' }, runRevision: '3', runMethod: 'ai', runNode } })
  expect(wrapper.text()).toContain('CPU 图片任务仍在处理')
  expect(wrapper.text()).toContain('来源版本 v3')
  expect(wrapper.text()).toContain('参数修改后需重新执行')
  await wrapper.setProps({ runNode: { ...runNode, state: 'failed', errorMessage: '模型服务暂时不可用' } })
  expect(wrapper.text()).toContain('模型服务暂时不可用')
  expect(wrapper.text()).not.toContain('CPU 图片任务仍在处理')
})
it('centers and resets only placement, and emits one patch for a completed print drag', async () => {
  wrapper = mount(ProductLayout, { props: { assetId: '7', token: 'test', config: { side: 'back', color: '#123456' } } }); await flushPromises()
  await wrapper.findAll('button')[2]!.trigger('click'); expect(wrapper.emitted('patch')?.at(-1)).toEqual([{ printX: .5, printY: .5 }])
  await wrapper.findAll('button')[3]!.trigger('click'); expect(wrapper.emitted('patch')?.at(-1)).toEqual([{ printX: .5, printY: .4, printScale: .6 }])
  expect(wrapper.findAll('path')[0]!.attributes('fill')).toBe('#123456')
  expect(wrapper.findAll('path')[1]!.attributes('d')).toContain('Q500 145')
  const svg = surface(wrapper, 1000, 1200)
  await svg.trigger('pointerdown', { pointerId: 1, clientX: 400, clientY: 420, button: 0 })
  await svg.trigger('pointermove', { pointerId: 1, clientX: 800, clientY: 900 })
  expect(wrapper.emitted('patch')).toHaveLength(2)
  await svg.trigger('pointerup', { pointerId: 1, clientX: 800, clientY: 900 })
  expect(wrapper.emitted('patch')?.at(-1)).toEqual([{ printX: 1, printY: 1 }])
  const shortcut = vi.fn(); wrapper.element.addEventListener('keydown', shortcut)
  await svg.trigger('keydown', { key: 's', ctrlKey: true }); expect(shortcut).toHaveBeenCalledTimes(1)
})
it('compares the actual parent and displays intrinsic pixel sizes; releases both URLs', async () => {
  wrapper = mount(ImageCompareDialog, { props: { result: meta('9') as never, token: 'test' } }); await flushPromises()
  expect(vi.mocked(fetch).mock.calls.map(([path]) => path)).toContain('/api/pod/assets/7/content?variant=original')
  expect(wrapper.findAll('.compare-panel')).toHaveLength(2)
  expect(wrapper.text()).toContain('600 × 800'); expect(wrapper.text()).toContain('300 × 400')
  await wrapper.find('select').setValue('2')
  expect(wrapper.findAll('img')[0]!.attributes('style')).toContain('width: 1200px')
  expect(wrapper.findAll('img')[1]!.attributes('style')).toContain('width: 600px')
  await wrapper.findAll('.image-tool-actions button')[2]!.trigger('click')
  expect(wrapper.findAll('.compare-panel')).toHaveLength(1)
  wrapper.unmount(); wrapper = undefined; expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2)
})
it('offers comparison only for processing results and handles missing parent or expired access', async () => {
  wrapper = mount(ResultImages, { props: { ids: ['9'], token: 'test' }, global: { stubs: { PrivateImage: true, ImageCompareDialog: true } } })
  expect(wrapper.find('button[aria-label="对比处理结果 1"]').exists()).toBe(false)
  await wrapper.setProps({ compare: true }); await wrapper.find('button[aria-label="对比处理结果 1"]').trigger('click'); await flushPromises()
  expect(wrapper.findComponent(ImageCompareDialog).props('result').parentAssetId).toBe('7')
  await wrapper.setProps({ ids: ['7'] }); await wrapper.find('button[aria-label="对比处理结果 1"]').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('没有可读取的直接输入')
  vi.mocked(fetch).mockResolvedValue(json(null, 401))
  await wrapper.find('button[aria-label="对比处理结果 1"]').trigger('click'); await flushPromises()
  expect(wrapper.emitted('expired')).toHaveLength(1)
})
it('does not reopen comparison after the source has changed while loading', async () => {
  let release!: (value: Response) => void
  vi.mocked(fetch).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
  wrapper = mount(ResultImages, { props: { ids: ['9'], token: 'test', compare: true }, global: { stubs: { PrivateImage: true, ImageCompareDialog: true } } })
  await wrapper.find('button[aria-label="对比处理结果 1"]').trigger('click')
  await wrapper.setProps({ ids: ['8'] }); release(json(meta('9'))); await flushPromises()
  expect(wrapper.findComponent(ImageCompareDialog).exists()).toBe(false)
})
