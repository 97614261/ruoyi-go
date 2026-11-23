// @vitest-environment happy-dom
import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import type {VueWrapper} from '@vue/test-utils'
import OfficialTemplates from './OfficialTemplates.vue'
import {pendingOfficialUse,retryOfficialUse,submitOfficialUse} from './official-requests'

let wrapper:VueWrapper|null=null
let calls:{path:string;body:Record<string,unknown>;method:string}[]=[]
let fail=false, conflict=false, actionState='copying'
const respond=(value:unknown)=>new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}})
beforeEach(()=>{
 sessionStorage.clear();calls=[];fail=false;conflict=false;actionState='copying'
 vi.stubGlobal('fetch',vi.fn(async(path:string,options:RequestInit={})=>{
  const body=options.body?JSON.parse(String(options.body)):{};calls.push({path,body,method:options.method||'GET'})
  if(path.includes('/use-actions/'))return respond({code:200,data:{requestId:path.split('/').at(-1),state:actionState,projectId:actionState==='completed'?'88':'0'}})
  if(options.method==='POST'){
   if(fail)throw new Error('response lost')
   if(conflict)return respond({code:409,msg:'source unavailable'})
   return respond({code:200,data:{requestId:body.requestId,state:'completed',projectId:'88'}})
  }
  if(path.includes('?'))return respond({code:200,rows:[{templateId:'7',name:'官方花朵',description:'替换文字和图片\n完整使用步骤：填入自己的设计要求。'}],total:1})
  return respond({code:200,data:{template:{templateId:'7'},snapshot:{versionId:'17',version:'2',graph:{schemaVersion:1,nodes:[],edges:[]},slots:[{nodeId:'text',type:'textInput',text:'a flower',label:'设计要求'}],outputs:[]}}})
 }))
})
afterEach(()=>{wrapper?.unmount();wrapper=null;vi.unstubAllGlobals();vi.restoreAllMocks();document.body.innerHTML=''})

it('does not create or execute until explicitly confirmed; pins the published version',async()=>{
 wrapper=mount(OfficialTemplates,{props:{token:'local-token',scope:'1:2',limits:{maxUploadBytes:100000,maxPixels:100000,maxDimension:1024,maxImagesPerNode:8}},global:{stubs:{TemplateGraph:true}} ,attachTo:document.body})
 await flushPromises()
 expect(wrapper.get('.personal-template-copy p').text()).toBe('替换文字和图片')
 expect(calls.every(c=>c.method==='GET')).toBe(true)
 await wrapper.get('.personal-template-row .primary-button').trigger('click');await flushPromises()
 const instructions=wrapper.findAll('details').find(detail=>detail.text().includes('完整使用步骤'))!
 expect(instructions.attributes('open')).toBeUndefined()
 expect(instructions.get('p').text()).toContain('完整使用步骤：填入自己的设计要求。')
 expect(calls.every(c=>c.method==='GET')).toBe(true)
 await wrapper.get('textarea').setValue('a blue flower')
 await wrapper.get('dialog footer .primary-button').trigger('click');await flushPromises()
 const sent=calls.find(c=>c.method==='POST')!
 expect(sent.body).toMatchObject({version:'17',name:'官方花朵',bindings:[{nodeId:'text',text:'a blue flower'}]})
 expect(wrapper.emitted('open')).toEqual([['88']])
 expect(calls.some(c=>c.path.includes('/runs')||c.path.includes('/quotes'))).toBe(false)
})

it('retries the original request after response loss and keeps ownership scopes separate',async()=>{
 fail=true
 await expect(submitOfficialUse('1:2','token','7',{version:'17',name:'first',bindings:[{nodeId:'text',text:'original'}]})).rejects.toThrow()
 const pending=pendingOfficialUse('1:2')!
 expect(pendingOfficialUse('3:4')).toBeNull()
 await expect(submitOfficialUse('1:2','token','8',{version:'18',name:'different',bindings:[]})).rejects.toThrow('上一项')
 fail=false;await retryOfficialUse('1:2','token')
 const sent=calls.filter(c=>c.method==='POST')
 expect(sent[0]!.body).toEqual(sent[1]!.body)
 expect(sent[1]!.body.requestId).toBe(pending.requestId)
 expect(pendingOfficialUse('1:2')).toBeNull()
})

it('retains a copying plan after 409 and recovers a completed action without new project',async()=>{
 conflict=true
 await expect(submitOfficialUse('1:2','token','7',{version:'17',name:'first',bindings:[]})).rejects.toThrow('source unavailable')
 const original=pendingOfficialUse('1:2')!
 expect(original).not.toBeNull()
 actionState='completed'
 const result=await retryOfficialUse('1:2','token')
 expect(result.projectId).toBe('88')
 expect(pendingOfficialUse('1:2')).toBeNull()
 expect(calls.filter(c=>c.method==='POST').every(c=>c.body.requestId===original.requestId)).toBe(true)
})
