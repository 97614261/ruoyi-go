// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import Eligibility from './Eligibility.vue'
let view:VueWrapper|undefined
afterEach(()=>{view?.unmount();view=undefined;vi.unstubAllGlobals()})
const data={creator:{userId:'1',status:'suspended',expired:false,validUntil:null,maxConcurrentNodes:1,maxPendingRuns:10,maxDailySubmissions:50,maxStorageBytes:'1073741824'},storage:{usedBytes:'1048576',heldBytes:'0'},businessDate:'2026-11-13',dailySubmissions:2,modelIds:[]}
it('shows suspended qualification and independent point history using only own GET endpoints',async()=>{
 vi.stubGlobal('fetch',vi.fn(async(path:string)=>new Response(JSON.stringify({code:200,data:path.includes('/points')?{account:{available:'9007199254740993',held:'0',spent:'0'},rows:[{eventId:'1',kind:'grant',amount:'100',availableAfter:'100',reason:'测试发放',createTime:'2026-11-13T04:00:00Z'}],total:1,billingEnabled:false}:data}))))
 view=mount(Eligibility,{props:{token:'fixture-token'}});await flushPromises()
 expect(view.text()).toContain('创作资格已停用');expect(view.text()).toContain('9007199254740993');expect(view.text()).toContain('未启用正式扣点');expect(view.text()).toContain('测试发放');expect(view.text()).not.toContain('开通个人空间')
 expect(vi.mocked(fetch).mock.calls.every(([path,options])=>['/api/pod/eligibility','/api/pod/points?pageNum=1&pageSize=10'].includes(String(path)) && !options?.method)).toBe(true)
 vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({code:403,msg:'访问已失效'})))
 await view.get('button').trigger('click');await flushPromises();expect(view.text()).not.toContain('9007199254740993');expect(view.text()).toContain('访问已失效')
})
it('ignores previous identity responses and emits session expiry on 401',async()=>{
 let resolve!:(r:Response)=>void
 vi.stubGlobal('fetch',vi.fn(()=>new Promise<Response>(r=>{resolve=r})))
 view=mount(Eligibility,{props:{token:'old'}})
 vi.mocked(fetch).mockImplementation(async()=>new Response(JSON.stringify({code:401,msg:'会话失效'})))
 await view.setProps({token:'new'});await flushPromises();resolve(new Response(JSON.stringify({code:200,data})));await flushPromises()
 expect(view.emitted('expired')).toHaveLength(1);expect(view.text()).toContain('会话失效');expect(view.text()).not.toContain('1048576')
})
