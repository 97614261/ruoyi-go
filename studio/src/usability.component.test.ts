// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import ModelSelect from './ModelSelect.vue'
import { executionQuote } from './execution-quote'
import OfficialTemplates from './OfficialTemplates.vue'
import Runs from './Runs.vue'
import type { CreatorModel, Run } from './contracts'

let view: VueWrapper | undefined
const response = (data: unknown) => new Response(JSON.stringify({code:200,data}),{headers:{'Content-Type':'application/json'}})
const button = (text: string) => view!.findAll('button').find(b => b.text() === text)!
afterEach(()=>{view?.unmount();view=undefined;sessionStorage.clear();document.body.innerHTML='';vi.unstubAllGlobals();vi.restoreAllMocks()})

it('shows selected model points and immutable version rather than the default model price',()=>{
  const models = [{modelId:'1',modelVersionId:'10',name:'Default',officialModelId:'default-id',kind:'image',default:true,pointRule:{perImage:'10'}},{modelId:'2',modelVersionId:'21',name:'Selected',officialModelId:'selected-id',kind:'image',default:false,pointRule:{perImage:'25'}}] as CreatorModel[]
  view=mount(ModelSelect,{props:{models,kind:'image',value:'2'}})
  expect(view.find('.model-rate').text()).toContain('每张 25 点');expect(view.find('.model-rate').text()).toContain('版本 #21')
  expect(view.emitted('change')).toBeUndefined()
})
it('keeps frozen image quantity and model prices in the server quote without a dialog',async()=>{
  const fetch=vi.fn(async()=>response({quoteId:'11111111-1111-4111-8111-111111111111',kind:'run',state:'ready',points:'50',expiresAt:new Date(Date.now()+60000).toISOString(),lines:[{itemNo:0,nodeId:'B',name:'Frozen model',officialModelId:'old-image-id',kind:'image',images:2,modelVersionId:'21',points:'50'}]}));vi.stubGlobal('fetch',fetch)
  const quote=await executionQuote('local',{kind:'run',run:{projectId:'1'}})
  expect(quote.lines[0]).toMatchObject({images:2,officialModelId:'old-image-id',points:'50'})
  expect(fetch).toHaveBeenCalledOnce()
})
it('requires a personal image before copying a starter and never executes its graph',async()=>{
  const posts: {path:string;body:Record<string,unknown>}[]=[]
  vi.stubGlobal('fetch',vi.fn(async(path:string,options:RequestInit={})=>{
    if(options.method==='POST'){posts.push({path,body:JSON.parse(String(options.body))});return response({state:'completed',projectId:'88'})}
    if(path.includes('?'))return new Response(JSON.stringify({code:200,rows:[{templateId:'3',name:'参考图改图',description:'1. 选择自己的图片。'}],total:1}))
    return response({snapshot:{versionId:'12',version:'1',graph:{schemaVersion:1,nodes:[],edges:[]},slots:[{nodeId:'reference',type:'imageInput',label:'参考图片',assetIds:[]}]}})
  }))
  const Picker=defineComponent({name:'AssetPicker',emits:['confirm'],template:'<button @click="$emit(\'confirm\',[\'51\'])">选择测试图片</button>'})
  view=mount(OfficialTemplates,{props:{token:'local',scope:'1:2',limits:{maxUploadBytes:1000,maxPixels:10000,maxDimension:100,maxImagesPerNode:8}},global:{stubs:{AssetPicker:Picker}},attachTo:document.body});await flushPromises()
  await button('查看并使用').trigger('click');await flushPromises()
  expect(view.text()).toContain('请先选择个人图片');expect(button('复制并打开画布').attributes('disabled')).toBeDefined()
  await button('从我的素材选择').trigger('click');await button('选择测试图片').trigger('click');await flushPromises()
  expect(button('复制并打开画布').attributes('disabled')).toBeUndefined()
  await button('复制并打开画布').trigger('click');await flushPromises()
  expect(posts).toHaveLength(1);expect(posts[0]!.path).toContain('/use');expect(posts[0]!.body).toMatchObject({version:'12',bindings:[{nodeId:'reference',assetIds:['51']}]})
  expect(view.emitted('open')).toEqual([['88']])
})
it('offers editing instead of retrying rejected input and does not create a new run',async()=>{
  const run={runId:'10',projectId:'1',projectName:'Input issue',state:'failed',attempt:1,testOnly:false,billing:{scheme:'managedPointsV1',held:'0',spent:'0',released:'10'},nodes:[{nodeId:'B',nodeType:'imageGenerate',state:'failed',errorCode:'providerInputRejected',errorMessage:'供应商拒绝输入',output:{}}]} as Run
  const fetch=vi.fn(async(path:string)=>path.includes('/runs?')?new Response(JSON.stringify({code:200,rows:[run],total:1})):path.includes('/credits?')?new Response(JSON.stringify({code:200,account:{available:'10',held:'0',spent:'0'},rows:[],total:0})):response(run));vi.stubGlobal('fetch',fetch)
  view=mount(Runs,{props:{token:'local'},attachTo:document.body});await flushPromises();await view.find('.run-row').trigger('click');await flushPromises()
  expect(view.text()).toContain('修改输入后重新制作');expect(view.findAll('button').some(b=>b.text()==='恢复失败步骤')).toBe(false)
  await button('返回画布修改').trigger('click');expect(view.emitted('project')).toEqual([['1']]);expect(fetch.mock.calls.every(c=>!c[0].endsWith('/retry'))).toBe(true)
})
