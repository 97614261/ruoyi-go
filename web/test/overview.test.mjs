import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref,reactive,computed} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as overview from '../src/utils/overview.mjs'
const deferred=()=>{let resolve,reject;const promise=new Promise((ok,fail)=>{resolve=ok;reject=fail});return{promise,resolve,reject}}
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/OverviewWorkspace.vue',import.meta.url),'utf8')
 const code=compileScript(parse(source).descriptor,{id:'overview-state'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component=')
 const leave=[],enter=[],calls=[],pushes=[],stubs={ref,reactive,computed,onMounted(){},onBeforeUnmount:f=>leave.push(f),onDeactivated:f=>leave.push(f),onActivated:f=>enter.push(f),useRouter:()=>({push:r=>pushes.push(r)}),ElMessage:{warning(){}},checkPermi:()=>true,overviewSection:async(kind,params)=>{calls.push({kind,params});return{data:{available:true,checkedAt:'2026-10-25 08:00:00',counts:{pendingReview:0},rows:[],total:0}}},...overview,...overrides}
 const component=vm.runInNewContext(code+'\ncomponent',stubs)
 return{state:component.setup({},{expose(){}}),leave,enter,calls,pushes}
}
test('overview distinguishes failures, unavailable modules and actual zero, sections remain independent',async()=>{
 const a=await setup({overviewSection:async k=>{if(k==='orders')throw Error('offline');return{data:{available:k!=='shipments',counts:{withStock:0}}}}});await a.state.refresh();assert.equal(a.state.states.orders.error,true);assert.equal(a.state.states.orders.data,null);assert.equal(a.state.states.shipments.data.available,false);assert.equal(a.state.states.stocks.data.counts.withStock,0);assert.equal(a.state.states.stocks.error,false)
})
test('overview permission never grants hidden sections, pages request bounded money groups',async()=>{
 const a=await setup({checkPermi:p=>['erp:overview:list','erp:order:list'].includes(p[0])});await a.state.refresh();assert.deepEqual(a.calls.map(c=>c.kind),['orders']);assert.equal(a.state.visible.value.length,1)
 const b=await setup({checkPermi:p=>p[0]!=='erp:overview:list'});await b.state.refresh();assert.equal(b.calls.length,0)
 const c=await setup();c.state.pages.payables=3;await c.state.loadOne('payables');assert.equal(c.calls[0].params.pageNum,3);assert.equal(c.calls[0].params.pageSize,10)
})
test('latest section refresh wins and leaving prevents stale response or financial data from reappearing',async()=>{
 const old=deferred();let n=0;const a=await setup({overviewSection:()=>++n===1?old.promise:Promise.resolve({data:{available:true,counts:{pendingReview:2}}})});const first=a.state.loadOne('orders');await a.state.loadOne('orders');old.resolve({data:{available:true,counts:{pendingReview:99}}});await first;assert.equal(a.state.states.orders.data.counts.pendingReview,2)
 const pending=deferred(),b=await setup({overviewSection:()=>pending.promise});const load=b.state.loadOne('payables');b.leave.forEach(f=>f());pending.resolve({data:{available:true,rows:[{currency:'CNY',dueAmount:'900.0000'}]}});await load;assert.equal(b.state.states.payables.data,null);assert.equal(b.state.states.payables.loading,false)
})
test('revoked section loses prior numbers and cannot navigate',async()=>{
 let allowed=true;const a=await setup({checkPermi:()=>allowed});await a.state.loadOne('orders');allowed=false;await a.state.loadOne('orders');assert.equal(a.state.states.orders.data,null);a.state.go('orders','pendingReview');assert.equal(a.pushes.length,0)
})
test('overview drilldowns whitelist routes, local natural-day boundaries and currency text',()=>{
 assert.equal(overview.overviewTarget('orders','pendingReview').query.status,'PENDING_REVIEW')
 assert.equal(overview.overviewTarget('stocks','withReserved').query.stockState,'HAS_RESERVED')
 assert.equal(overview.overviewTarget('purchases').query.awaitingReceipt,'true')
 const t=overview.overviewTarget('shipments','sevenDays',{sevenDaysFrom:'2025-12-27 00:00:00',through:'2026-01-03 00:00:00'});assert.equal(t.query.shippedTo,'2026-01-02 23:59:59')
 assert.equal(overview.overviewTarget('receivables','money',{},'CNY').path,'/erp/receivable')
 for(const value of ['2026-02-30 00:00:00','garbage','2026-01-02 12:00:00'])assert.throws(()=>overview.overviewTarget('shipments','today',{todayFrom:value,through:'2026-01-03 00:00:00'}))
 assert.throws(()=>overview.overviewTarget('payables','money',{},'CNY&tenantId=3'));assert.throws(()=>overview.overviewTarget('javascript:alert(1)','today'))
 assert.equal(overview.overviewCount(0),'0');assert.equal(overview.overviewCount(Number.MAX_SAFE_INTEGER+1),'—');assert.equal(overview.overviewCount(null),'—')
})
