// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import CreativeLibrary from './CreativeLibrary.vue'
import PresetGallery from './PresetGallery.vue'
import { prompts, templates } from './creative-library'
import { graphError } from './graph'
import type { NodeDefinition, NodeType, ValueType } from './contracts'

let view: VueWrapper | undefined
beforeEach(() => localStorage.clear())
afterEach(() => { view?.unmount(); view = undefined; vi.restoreAllMocks() })

it('keeps the original content and validates each added graph against the node and port contracts', () => {
  const node = (type: NodeType, inputs: ValueType[], outputs: ValueType[]): NodeDefinition => ({ type, label: type, version: 1, description: '', executable: true, inputs: inputs.map(valueType => ({ valueType, key: valueType, required: true })), outputs: outputs.map(valueType => ({ valueType, key: valueType, required: true })) })
  const types = [node('textInput', [], ['text']), node('imageInput', [], ['images']), node('textGenerate', ['text', 'images'], ['text']), node('imageGenerate', ['text', 'images'], ['images']), node('imageProcess', ['images'], ['images']), node('productPreview', ['images'], ['images']), node('export', ['images'], ['files'])]
  expect(prompts).toHaveLength(24)
  expect(new Set(prompts.map(row => row.id)).size).toBe(24)
  expect(templates).toHaveLength(10)
  expect(templates.every(item => !graphError(item.graph, types, { maxNodes: 100, maxEdges: 300, maxGraphBytes: 1048576 }))).toBe(true)
})
it('combines category and search without changing the template index used for creation', async () => {
  view = mount(PresetGallery)
  await view.findAll('.shelf-filters button').find(button => button.text() === '素材处理')!.trigger('click')
  await view.get('input').setValue('透明')
  expect(view.findAll('.shelf-preset')).toHaveLength(1)
  await view.get('.workflow-preset').trigger('click')
  expect(view.emitted('use')).toEqual([[templates.findIndex(item => item.kind === 'solid-cutout')]])
  await view.get('input').setValue('不存在')
  expect(view.get('.shelf-empty').text()).toContain('没找到')
  await view.get('.shelf-empty button').trigger('click')
  expect(view.findAll('.shelf-preset')).toHaveLength(10)
})
it('copies or inserts only the active original prompt and keeps favorites scoped to their owner', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.spyOn(navigator, 'clipboard', 'get').mockReturnValue({ writeText } as unknown as Clipboard)
  view = mount(CreativeLibrary, { props: { initialTab: 'prompts', scope: 'alice:workspace-a' }, attachTo: document.body })
  await view.get('input').setValue('花瓣 无缝')
  expect(view.findAll('.inspiration-index button')).toHaveLength(1)
  const chosen = prompts.find(item => item.title === '花瓣无缝花纹')!
  expect(view.get('textarea').element.value).toBe(chosen.text)
  await view.get('button[aria-label="收藏此提示词"]').trigger('click')
  await view.get('.inspiration-actions .secondary-button').trigger('click'); await flushPromises()
  expect(writeText).toHaveBeenCalledWith(chosen.text)
  await view.get('.inspiration-actions .primary-button').trigger('click')
  expect(view.emitted('prompt')).toEqual([[chosen.text]])
  view.unmount()
  view = mount(CreativeLibrary, { props: { initialTab: 'prompts', scope: 'bob:workspace-b' }, attachTo: document.body })
  await view.get('.shelf-favorites').trigger('click')
  expect(view.get('.shelf-empty').text()).toContain('没有匹配的收藏')
  view.unmount()
  view = mount(CreativeLibrary, { props: { initialTab: 'prompts', scope: 'alice:workspace-a' }, attachTo: document.body })
  await view.get('.shelf-favorites').trigger('click')
  expect(view.get('textarea').element.value).toBe(chosen.text)
})

it('shows prepared inputs and nodes only in preset details, without using the graph on open', async () => {
  view = mount(PresetGallery, { attachTo: document.body })
  expect(view.findAll('.shelf-cover img')).toHaveLength(10)
  expect(view.find('.template-graph').exists()).toBe(false)
  const cover = view.get('button[aria-label="查看预设详情：参考图创作"]')
  await cover.trigger('click'); await flushPromises()
  const dialog = view.get('.shelf-preset-detail')
  expect(dialog.text()).toContain('参考图与改造要求')
  expect(dialog.text()).toContain('参考图变体')
  expect(dialog.find('.template-graph').exists()).toBe(true)
  expect(view.emitted('use')).toBeUndefined()
  await dialog.get('footer button').trigger('click')
  expect(view.emitted('use')).toEqual([[templates.findIndex(item => item.kind === 'reference')]])
})

it('renders the three product prompts from labeled parameters and emits exactly the previewed text', async () => {
  view = mount(CreativeLibrary, { props: { initialTab: 'prompts' }, attachTo: document.body })
  const search = view.get('input[aria-label="搜索提示词"]')
  await search.setValue('需要参考图')
  expect(view.findAll('.inspiration-index button')).toHaveLength(4)
  for (const [keyword, detail, value] of [
    ['白底 电商', '拍摄角度', '四十五度俯拍'],
    ['木台 侧光', '使用场景', '窗边的咖啡桌'],
    ['帆布袋 近景', '细节重点', '拉链和缝线'],
  ]) {
    await search.setValue(keyword!)
    await flushPromises()
    const use = view.get('.inspiration-actions .primary-button')
    expect(use.attributes()).toHaveProperty('disabled')
    const fields = view.findAll('.inspiration-parameters label')
    await fields.find(field => field.text().includes('商品名'))!.get('input').setValue('  手工帆布包  ')
    await fields.find(field => field.text().includes(detail!))!.get('input').setValue(value!)
    const text = view.get('textarea').element.value
    expect(text).toContain('手工帆布包')
    expect(text).toContain(value)
    await use.trigger('click')
    expect(view.emitted('prompt')?.at(-1)).toEqual([text])
  }
})
