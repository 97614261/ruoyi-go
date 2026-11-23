// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ProductStudio from './ProductStudio.vue'
import type { Bootstrap } from './contracts'

const bootstrap = {
  userId: '8', workspace: { workspaceId: '12', name: '测试工作空间' },
  capabilities: { projectEditing: true, assets: true, execution: true },
  execution: { mode: 'managed' }, assetLimits: {},
} as Bootstrap
const pack = { packId: '19', productName: '马克杯', sku: 'MUG-8', sellingPoints: '哑光', protectedDetails: '保留杯柄', referenceAssetId: '78' }
const base = { project: { projectId: '42', name: '马克杯 · 商品图', revision: '1' }, pack, quickRun: true }
const ids = ['product-main', 'product-scene']
const offerID = '11111111-1111-4111-8111-111111111111'
const line = (nodeId: string) => ({ nodeId, kind: 'image', images: 1, points: '2', name: '参考图模型' })
const success = { runId: '7', projectRevision: '1', runState: 'failed', state: 'succeeded', output: { assetIds: ['80'] } }
const failure = { runId: '7', projectRevision: '1', runState: 'failed', state: 'failed', errorMessage: '场景图生成失败', output: {} }
const pending = { ...failure, errorMessage: '', state: 'awaitingReconciliation', runState: 'awaitingReconciliation', errorCode: 'archivePending' }
const json = (body: object) => new Response(JSON.stringify({ code: 200, ...body }), { headers: { 'Content-Type': 'application/json' } })

let wrapper: VueWrapper | undefined
function server(options: { slots?: object[]; points?: string; loseResponse?: boolean; loseDraft?: boolean; quoteNodes?: string[] } = {}) {
  let lost = false, lostDraft = false
  const fetch = vi.fn(async (path: string, init?: RequestInit) => {
    const body = init?.body ? JSON.parse(String(init.body)) : null
    if (path.startsWith('/api/pod/models')) return json({ data: [
      { modelId: '1', name: '只支持纯文字', default: true, kind: 'image', capabilities: { ratios: ['1:1'] } },
      { modelId: '20', name: '参考图模型', kind: 'image', capabilities: { maxReferenceImages: 1, ratios: ['3:4'] } },
    ] })
    if (path.startsWith('/api/pod/product-packs?')) return json({ rows: [pack], total: 1 })
    if (path.startsWith('/api/pod/product-jobs?')) return json({ rows: [base.project], total: 1 })
    if (path === '/api/pod/product-jobs' && init?.method === 'POST') {
      if (options.loseDraft && !lostDraft) { lostDraft = true; throw new Error('lost response') }
      return json({ data: { ...base, workKind: body.workKind } })
    }
    if (path.includes('/product-jobs/42/download')) return new Response(new Blob(['zip']), { headers: {
      'Content-Type': 'application/zip', 'Content-Disposition': "attachment; filename*=UTF-8''MUG-8-final.zip",
    } })
    if (path.endsWith('/product-jobs/42')) return json({ data: { ...base, slots: options.slots || ids.map(nodeId => ({ nodeId })) } })
    if (path.endsWith('/execution-quotes')) {
      const lines = body.run.nodeId ? [line(body.run.nodeId)] : (options.quoteNodes || ids).map(line)
      return json({ data: { quoteId: offerID, kind: 'run', state: 'ready', points: String(2 * lines.length), lines, expiresAt: new Date(Date.now() + 60000).toISOString() } })
    }
    if (path.startsWith('/api/pod/points?')) return json({ data: { account: { available: options.points || '10' } } })
    if (path.endsWith('/runs') && init?.method === 'POST') {
      if (options.loseResponse && !lost) { lost = true; throw new Error('lost response') }
      return json({ data: { ...body, runId: '15' } })
    }
    throw new Error('Unexpected request ' + path)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}
const button = (label: string) => wrapper!.findAll('button').find(item => item.text() === label)!
async function mountPage(projectId = '42') {
  wrapper = mount(ProductStudio, { props: { projectId, bootstrap, token: 'test-only' }, attachTo: document.body, global: { stubs: { PrivateImage: true, ResultImages: true, AssetPicker: true } } })
  await flushPromises()
}
afterEach(() => { wrapper?.unmount(); wrapper = undefined; sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('keeps the selected image visible when a newer candidate exists and downloads the named finals', async () => {
  const final = { runId: '7', assetId: '80', projectRevision: '1' }
  const latest = { ...success, runId: '17', output: { assetIds: ['90'] } }
  const fetch = server({ slots: [
    { nodeId: ids[0], latest, result: latest, final },
    { nodeId: ids[1], latest, result: latest, final: { ...final, assetId: '81' } },
  ] })
  const downloads: string[] = []
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { downloads.push(this.download) })
  await mountPage()
  expect(wrapper!.findAll('result-images-stub').map(image => image.attributes('ids'))).toEqual(['80', '81'])
  expect(wrapper!.text()).toContain('已有其他候选图')
  await button('下载定稿 ZIP').trigger('click'); await flushPromises()
  expect(downloads).toEqual(['MUG-8-final.zip'])
  expect(fetch.mock.calls.some(([path]) => path.endsWith('/download?selection=final'))).toBe(true)
})

it('reuses saved fields, filters out models without reference inputs and keeps the same create ID after an unknown response', async () => {
  const fetch = server({ loseDraft: true })
  await mountPage('')
  expect(wrapper!.text()).not.toContain('只支持纯文字')
  await wrapper!.find('select').setValue('19')
  expect((wrapper!.get('input[required]').element as HTMLInputElement).value).toBe('马克杯')
  await wrapper!.find('form').trigger('submit')
  await flushPromises()
  expect(wrapper!.text()).toContain('开工结果待确认')
  await wrapper!.find('form').trigger('submit')
  await flushPromises()
  const submits = fetch.mock.calls.filter(([path, init]) => path.endsWith('/product-jobs') && init?.method === 'POST')
  expect(submits).toHaveLength(2)
  expect(submits[0]?.[1]?.body).toBe(submits[1]?.[1]?.body)
  expect(JSON.parse(String(submits[0]?.[1]?.body))).toMatchObject({ productName: '马克杯', sku: 'MUG-8', referenceAssetId: '78', modelId: '20', ratio: '3:4' })
  expect(wrapper!.emitted('open')).toEqual([['42']])
  expect(fetch.mock.calls.some(([path]) => path.endsWith('/runs'))).toBe(false)
})

it('creates a local T-shirt canvas using the saved pack even without formal image models', async () => {
  const fetch = server()
  wrapper = mount(ProductStudio, { props: { projectId: '', bootstrap: { ...bootstrap, execution: undefined }, token: 'test-only' } })
  await flushPromises()
  await wrapper.get('input[type=radio][value=mockup]').setValue(true)
  await wrapper.get('select').setValue('19')
  await wrapper.get('form').trigger('submit'); await flushPromises()
  const call = fetch.mock.calls.find(([path, init]) => path.endsWith('/product-jobs') && init?.method === 'POST')
  expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ workKind: 'mockup', productName: '马克杯', referenceAssetId: '78', modelId: '', ratio: '' })
  expect(wrapper.emitted('canvas')).toEqual([['42']])
  expect(fetch.mock.calls.some(([path]) => path.includes('/models') || path.endsWith('/runs'))).toBe(false)
})

it('quotes only the selected image types and labels the individual costs', async () => {
  const selected = ['product-main', 'product-angle', 'product-detail']
  const fetch = server({ slots: selected.map(nodeId => ({ nodeId })), quoteNodes: selected })
  await mountPage()
  await button('查看 3 张图的报价').trigger('click'); await flushPromises()
  const text = wrapper!.get('.product-quote').text()
  expect(text).toContain('45° 展示图 · 参考图模型 · 2 点')
  expect(text).toContain('细节特写 · 参考图模型 · 2 点')
  expect(text).not.toContain('场景图')
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))).toHaveLength(0)
})

it('requires explicit execution after quote and recovers an unknown submission after remount with the same request', async () => {
  const fetch = server({ loseResponse: true })
  await mountPage()
  await button('查看两张图的报价').trigger('click')
  await flushPromises()
  const requests = () => fetch.mock.calls.filter(([path]) => path.endsWith('/runs'))
  expect(requests()).toHaveLength(0)
  expect(wrapper!.text()).toContain('白底主图 + 场景图 · 4 点')
  await button('确认并生成').trigger('click'); await flushPromises()
  expect(wrapper!.text()).toContain('上次提交未确认')
  wrapper!.unmount()
  await mountPage()
  await button('核对原提交').trigger('click'); await flushPromises()
  expect(requests()).toHaveLength(2)
  expect(requests()[0]?.[1]?.body).toBe(requests()[1]?.[1]?.body)
  expect(JSON.parse(String(requests()[0]?.[1]?.body))).toMatchObject({ mode: 'workflow', quoteId: offerID })
})

it('keeps the successful main image while quoting and submitting only the scene branch', async () => {
  const fetch = server({ slots: [{ nodeId: ids[0], latest: success, result: success }, { nodeId: ids[1], latest: failure }] })
  await mountPage()
  expect(wrapper!.find('result-images-stub').attributes('ids')).toBe('80')
  const scene = wrapper!.findAll('.product-output')[1]!
  await scene.get('button').trigger('click'); await flushPromises()
  expect(wrapper!.text()).toContain('场景图 · 2 点')
  await button('确认并生成').trigger('click'); await flushPromises()
  expect(JSON.parse(String(fetch.mock.calls.find(([path]) => path.endsWith('/runs'))?.[1]?.body))).toMatchObject({ mode: 'branch', nodeId: ids[1] })
  expect(wrapper!.find('result-images-stub').attributes('ids')).toBe('80')
})

it('blocks confirmation when points are insufficient and keeps archive-pending images from resubmission', async () => {
  const fetch = server({ points: '1' })
  await mountPage()
  await button('查看两张图的报价').trigger('click'); await flushPromises()
  expect(button('确认并生成').attributes()).toHaveProperty('disabled')
  expect(wrapper!.text()).toContain('当前可用 1 点')
  expect(fetch.mock.calls.some(([path]) => path.endsWith('/runs'))).toBe(false)
  wrapper!.unmount()
  server({ slots: [{ nodeId: ids[0], latest: success, result: success }, { nodeId: ids[1], latest: pending }] })
  await mountPage()
  expect(wrapper!.text()).toContain('正在保存图片')
  expect(wrapper!.findAll('.product-output')[1]!.find('button').exists()).toBe(false)
})

it('downloads persisted results via authenticated read and does not generate or quote on opening them', async () => {
  const scene = { ...success, runId: '13', output: { assetIds: ['83'] } }
  const fetch = server({ slots: [{ nodeId: ids[0], latest: success, result: success }, { nodeId: ids[1], latest: scene, result: scene }] })
  const download = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  await mountPage()
  expect(wrapper!.findAll('result-images-stub')).toHaveLength(2)
  await button('下载当前结果 ZIP').trigger('click'); await flushPromises()
  expect(download).toHaveBeenCalledOnce()
  expect(fetch.mock.calls.find(([path]) => path.endsWith('/download'))?.[1]?.headers).toEqual({ Authorization: 'Bearer test-only' })
  expect(fetch.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
})
