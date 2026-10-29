import { resetERPQuery } from '../src/utils/erp-query.mjs'
import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref,reactive,computed} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import {purchaseReturnPayload} from '../src/utils/purchase-return.mjs'
import {safeID,immutablePayload} from '../src/utils/purchasing.mjs'
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
const receipt=()=>({receiptId:1,version:2,lines:[{lineId:9,quantity:10,returnedQuantity:2,netReceivedQuantity:8}]})
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/PurchaseReturnWorkspace.vue',import.meta.url),'utf8')
 const{descriptor}=parse(source),code=compileScript(descriptor,{id:'return'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component =')
 const hooks=[],activated=[],messages=[]
 const stubs={ ERPQueryBar:{}, resetERPQuery,ref,reactive,computed,watch(){},onBeforeUnmount:f=>hooks.push(f),onDeactivated:f=>hooks.push(f),onActivated:f=>activated.push(f),useRoute:()=>({query:{}}),checkPermi:()=>true,ElMessage:{success:x=>messages.push(x)},crypto:{randomUUID:()=> 'return-fixed-key'},safeID,immutablePayload,purchaseReturnPayload,listReturnableReceipts:async()=>({rows:[],total:0}),listPurchaseReturns:async()=>({rows:[],total:0}),getReturnableReceipt:async()=>({data:receipt()}),...overrides}
 const component=vm.runInNewContext(`${code}\ncomponent`,stubs);return{state:component.setup({},{expose(){},emit(){}}),hooks,activated,messages,source}
}
test('return payload whitelists fields and checks original batch remainder',()=>{
 const f={version:2,requestKey:'key',reason:'  破损  ',tenantId:999,lines:[{lineId:9,netReceivedQuantity:8,returnQuantity:2,unitPrice:'99'},{lineId:10,netReceivedQuantity:1,returnQuantity:0}]}
 assert.deepEqual(purchaseReturnPayload(f),{version:2,requestKey:'key',reason:'破损',lines:[{receiptLineId:9,quantity:2}]})
 for(const q of [9,1.5,-1,null,'1',undefined])assert.throws(()=>purchaseReturnPayload({...f,lines:[{lineId:9,netReceivedQuantity:8,returnQuantity:q}]}))
 for(const reason of ['', '  ', 'x\ny','x'.repeat(501)])assert.throws(()=>purchaseReturnPayload({...f,reason}))
 assert.throws(()=>purchaseReturnPayload({...f,lines:[f.lines[0],f.lines[0]]}))
 assert.throws(()=>purchaseReturnPayload({...f,lines:[f.lines[1]]}))
})
test('return retry preserves original key version quantity reason and single submission',async()=>{
 const wait=deferred(),calls=[];const{state}=await setup({postPurchaseReturn:async(id,p)=>{calls.push(p);return wait.promise}})
 await state.start(1);state.form.value.reason='破损';state.form.value.lines[0].returnQuantity=2
 const first=state.submit();await state.submit();assert.equal(calls.length,1);wait.reject(Error('reply lost'));await first
 const pending=state.pending.value;assert.ok(pending);assert.ok(state.error.value)
 state.form.value.version=999;state.form.value.reason='变更';state.form.value.lines[0].returnQuantity=8;await state.submit()
 assert.equal(calls.length,2);assert.equal(calls[1].version,2);assert.equal(calls[1].reason,'破损');assert.equal(calls[1].lines[0].quantity,2);assert.equal(state.pending.value,pending)
})
test('detail and return permissions do not imply private purchase query',async()=>{
 const{state}=await setup({checkPermi:ps=>ps[0]==='erp:purchaseReturnOrder:list',getReturnableReceipt:()=>assert.fail('return permission bypass'),getPurchaseReturn:()=>assert.fail('query permission bypass')})
 await state.start(1);await state.show(1);assert.equal(state.open.value,false)
})
test('late detail and submitted response do not repopulate deactivated workspace',async()=>{
 const detail=deferred();const a=await setup({getReturnableReceipt:()=>detail.promise});const opening=a.state.start(1);a.hooks.forEach(f=>f());detail.resolve({data:receipt()});await opening;assert.equal(a.state.open.value,false);assert.equal(a.state.form.value.lines.length,0)
 const post=deferred();const b=await setup({postPurchaseReturn:()=>post.promise});await b.state.start(1);b.state.form.value.reason='破损';b.state.form.value.lines[0].returnQuantity=1;const writing=b.state.submit();b.hooks.forEach(f=>f());post.resolve({data:{returnId:1}});await writing;assert.equal(b.state.open.value,false);assert.equal(b.state.lastResult.value,null);assert.equal(b.messages.length,0)
})
test('tab switch invalidates previous list result and list errors are explicit',async()=>{
 const wait=deferred();const{state}=await setup({listReturnableReceipts:()=>wait.promise,listPurchaseReturns:async()=>({rows:[{returnId:5}],total:1})});state.tab.value='returns';await state.switchTab();wait.resolve({rows:[{receiptId:99}],total:1});await Promise.resolve();assert.equal(state.rows.value[0].returnId,5)
 const b=await setup({listReturnableReceipts:async()=>{throw Error('denied')}});await b.state.load();assert.equal(b.state.listError.value,true);assert.equal(b.state.total.value,0)
})
test('return page reuses dialog bounds and never persists sensitive retry body',async()=>{
 const{source}=await setup();assert.match(source,/class="erp-dialog"/);assert.match(source,/netReceivedQuantity/);assert.match(source,/:disabled="busy\|\|!!pending"/);assert.doesNotMatch(source,/localStorage|sessionStorage|getPurchase\(/)
 const api=await readFile(new URL('../src/api/erp/purchaseReturn.js',import.meta.url),'utf8');assert.match(api,/repeatSubmit:false/);assert.doesNotMatch(api,/method:'delete'/)
})
