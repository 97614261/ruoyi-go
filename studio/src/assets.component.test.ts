// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import Assets from './Assets.vue'
import AssetPicker from './AssetPicker.vue'
import PrivateImage from './PrivateImage.vue'
import type { Asset } from './contracts'

const limits = { maxUploadBytes: 1024, maxPixels: 24000000, maxDimension: 12000, maxImagesPerNode: 8 }
const asset: Asset = { assetId: '1', workspaceId: '20', parentAssetId: '0', source: 'upload', originalName: '印花.png', mediaType: 'image/png', byteSize: 200, width: 640, height: 320, revision: '1', deleted: false, referenceCount: 0, createTime: '2026-11-11 14:00:00' }
let wrapper: VueWrapper | null = null
beforeEach(() => { vi.stubGlobal('confirm', () => true) })
afterEach(() => { wrapper?.unmount(); wrapper = null; document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks() })
const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
function open(props = {}) { wrapper = mount(Assets, { props: { token: 'test-session', limits, ...props }, global: { stubs: { PrivateImage: true, AssetPreview: true } } }); return wrapper }

it('lists image metadata and selects an asset without modifying it', async () => {
  const fetch = vi.fn().mockResolvedValue(json({ code: 200, rows: [asset], total: 1 })); vi.stubGlobal('fetch', fetch)
  const view = open({ picker: true }); await flushPromises()
  expect(view.text()).toContain('640 × 320')
  await view.get('[aria-label="选择 印花.png"]').trigger('click')
  expect(view.emitted('select')?.[0]).toEqual([asset])
  expect(fetch.mock.calls).toHaveLength(1)
})
it('uploads a file using FormData and refuses client oversize files', async () => {
  const fetch = vi.fn(async (_path: string, options: RequestInit = {}) => options.method === 'POST' ? json({ code: 200, data: asset }) : json({ code: 200, rows: [asset], total: 1 })); vi.stubGlobal('fetch', fetch)
  const view = open({ picker: true }); await flushPromises()
  const input = view.get('input[type="file"]'); Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['png'], 'print.png', { type: 'image/png' })] })
  await input.trigger('change'); await flushPromises()
  const upload = fetch.mock.calls.find(([, options]) => options?.method === 'POST')
  expect(upload?.[1]?.body).toBeInstanceOf(FormData)
  expect(view.emitted('select')?.[0]).toEqual([asset])
  Object.defineProperty(input.element, 'files', { configurable: true, value: [new File([new Uint8Array(1025)], 'large.png', { type: 'image/png' })] })
  await input.trigger('change'); await flushPromises()
  expect(view.text()).toContain('最大')
  expect(fetch.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
})
it('shows reference protection instead of removing a rejected image', async () => {
  vi.stubGlobal('fetch', vi.fn(async (_path: string, options: RequestInit = {}) => options.method === 'DELETE' ? json({ code: 409, msg: '素材仍被项目引用' }) : json({ code: 200, rows: [asset], total: 1 })))
  const view = open(); await flushPromises()
  await view.get('[aria-label="移入回收站 印花.png"]').trigger('click'); await flushPromises()
  expect(view.text()).toContain('素材仍被项目引用')
  expect(view.findAll('.asset-item')).toHaveLength(1)
})
it('enforces picker bounds and confirms explicit selected IDs', async () => {
  wrapper = mount(AssetPicker, { props: { token: 'test-session', limits: { ...limits, maxImagesPerNode: 1 }, initial: ['1'] }, attachTo: document.body, global: { stubs: { Assets: true } } })
  const library = wrapper.findComponent(Assets)
  library.vm.$emit('select', { ...asset, assetId: '2' }); await flushPromises()
  expect(wrapper.text()).toContain('最多选择 1')
  library.vm.$emit('select', asset); library.vm.$emit('select', { ...asset, assetId: '2' }); await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === '加入节点')!.trigger('click')
  expect(wrapper.emitted('confirm')?.[0]).toEqual([['2']])
})
it('revokes private preview object URLs when the account changes or the image unmounts', async () => {
  const create = vi.spyOn(URL, 'createObjectURL').mockReturnValueOnce('blob:first').mockReturnValueOnce('blob:second'), revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => new Response(new Blob(['png']), { headers: { 'Content-Type': 'image/png' } })))
  wrapper = mount(PrivateImage, { props: { token: 'first-session', assetId: '1', alt: '素材' } }); await flushPromises()
  expect(create).toHaveBeenCalledTimes(1)
  await wrapper.setProps({ token: 'second-session' }); await flushPromises()
  expect(revoke).toHaveBeenCalledWith('blob:first')
  wrapper.unmount(); wrapper = null; expect(revoke).toHaveBeenCalledWith('blob:second')
})
