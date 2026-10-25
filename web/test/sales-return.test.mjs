import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref,reactive,computed} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import {salesReturnPayload} from '../src/utils/sales-return.mjs'
import {safeID,immutablePayload} from '../src/utils/purchasing.mjs'
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
const receipt=()=>({packageId:1,version:2,lines:[{lineId:9,quantity:10,returnedQuantity:2,netShippedQuantity:8}]})
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/SalesReturnWorkspace.vue',import.meta.url),'utf8')
 const{descriptor}=parse(source),code=compileScript(descriptor,{id:'return'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component =')
 const hooks=[],activated=[],messages=[]
 const stubs={ref,reactive,computed,watch(){},onBeforeUnmount:f=>hooks.push(f),onDeactivated:f=>hooks.push(f),onActivated:f=>activated.push(f),useRoute:()=>({query:{}}),checkPermi:()=>true,ElMessage:{success:x=>messages.push(x)},crypto:{randomUUID:()=> 'return-fixed-key'},safeID,immutablePayload,salesReturnPayload,listReturnablePackages:async()=>({rows:[],total:0}),listSalesReturns:async()=>({rows:[],total:0}),getReturnablePackage:async()=>({data:receipt()}),...overrides}
 const component=vm.runInNewContext(`${code}\ncomponent`,stubs);return{state:component.setup({},{expose(){},emit(){}}),hooks,activated,messages,source}
}
test('return payload whitelists fields and checks original batch remainder',()=>{
 const f={version:2,requestKey:'key',reason:'  破损  ',tenantId:999,lines:[{lineId:9,netShippedQuantity:8,returnQuantity:2,unitPrice:'99'},{lineId:10,netShippedQuantity:1,returnQuantity:0}]}
 assert.deepEqual(salesReturnPayload(f),{version:2,requestKey:'key',reason:'破损',lines:[{packageLineId:9,quantity:2}]})
 for(const q of [9,1.5,-1,null,'1',undefined])assert.throws(()=>salesReturnPayload({...f,lines:[{lineId:9,netShippedQuantity:8,returnQuantity:q}]}))
 for(const reason of ['', '  ', 'x\ny','x'.repeat(501)])assert.throws(()=>salesReturnPayload({...f,reason}))
 assert.throws(()=>salesReturnPayload({...f,lines:[f.lines[0],f.lines[0]]}))
 assert.throws(()=>salesReturnPayload({...f,lines:[f.lines[1]]}))
})
test('return retry preserves original key version quantity reason and single submission',async()=>{
 const wait=deferred(),calls=[];const{state}=await setup({postSalesReturn:async(id,p)=>{calls.push(p);return wait.promise}})
 await state.start(1);state.form.value.reason='破损';state.form.value.lines[0].returnQuantity=2
 const first=state.submit();await state.submit();assert.equal(calls.length,1);wait.reject(Error('reply lost'));await first
 const pending=state.pending.value;assert.ok(pending);assert.ok(state.error.value)
 state.form.value.version=999;state.form.value.reason='变更';state.form.value.lines[0].returnQuantity=8;await state.submit()
 assert.equal(calls.length,2);assert.equal(calls[1].version,2);assert.equal(calls[1].reason,'破损');assert.equal(calls[1].lines[0].quantity,2);assert.equal(state.pending.value,pending)
})
test('detail and return permissions do not imply private purchase query',async()=>{
 const{state}=await setup({checkPermi:ps=>ps[0]==='erp:salesReturnOrder:list',getReturnablePackage:()=>assert.fail('return permission bypass'),getSalesReturn:()=>assert.fail('query permission bypass')})
 await state.start(1);await state.show(1);assert.equal(state.open.value,false)
})
test('late detail and submitted response do not repopulate deactivated workspace',async()=>{
 const detail=deferred();const a=await setup({getReturnablePackage:()=>detail.promise});const opening=a.state.start(1);a.hooks.forEach(f=>f());detail.resolve({data:receipt()});await opening;assert.equal(a.state.open.value,false);assert.equal(a.state.form.value.lines.length,0)
 const post=deferred();const b=await setup({postSalesReturn:()=>post.promise});await b.state.start(1);b.state.form.value.reason='破损';b.state.form.value.lines[0].returnQuantity=1;const writing=b.state.submit();b.hooks.forEach(f=>f());post.resolve({data:{returnId:1}});await writing;assert.equal(b.state.open.value,false);assert.equal(b.state.lastResult.value,null);assert.equal(b.messages.length,0)
})
test('tab switch invalidates previous list result and list errors are explicit',async()=>{
 const wait=deferred();const{state}=await setup({listReturnablePackages:()=>wait.promise,listSalesReturns:async()=>({rows:[{returnId:5}],total:1})});state.tab.value='returns';await state.switchTab();wait.resolve({rows:[{packageId:99}],total:1});await Promise.resolve();assert.equal(state.rows.value[0].returnId,5)
 const b=await setup({listReturnablePackages:async()=>{throw Error('denied')}});await b.state.load();assert.equal(b.state.listError.value,true);assert.equal(b.state.total.value,0)
})
test('return page reuses dialog bounds and never persists sensitive retry body',async()=>{
 const{source}=await setup();assert.match(source,/class="erp-dialog"/);assert.match(source,/netShippedQuantity/);assert.match(source,/:disabled="busy\|\|!!pending"/);assert.doesNotMatch(source,/localStorage|sessionStorage|getPurchase\(/)
 const api=await readFile(new URL('../src/api/erp/salesReturn.js',import.meta.url),'utf8');assert.match(api,/repeatSubmit:false/);assert.doesNotMatch(api,/method:'delete'/)
})
test('same SKU original rows keep independent limits and IDs',()=>{
 const f={version:1,requestKey:'same-sku',reason:'验收',lines:[{lineId:1,skuId:7,netShippedQuantity:2,returnQuantity:2},{lineId:2,skuId:7,netShippedQuantity:3,returnQuantity:1}]}
 assert.deepEqual(salesReturnPayload(f).lines,[{packageLineId:1,quantity:2},{packageLineId:2,quantity:1}])
 f.lines[0].returnQuantity=3;assert.throws(()=>salesReturnPayload(f))
})
test('invalid and mismatched deep links never show another original order',async()=>{
 const a=await setup({useRoute:()=>({query:{packageId:'1.5'}}),getReturnablePackage:()=>assert.fail('invalid link fetched')});await a.state.openLink();assert.equal(a.state.listError.value,true);assert.equal(a.state.open.value,false)
 const warnings=[];const b=await setup({useRoute:()=>({query:{orderId:'99'}}),getReturnablePackage:async()=>({data:{...receipt(),orderId:4}}),ElMessage:{success(){},warning:x=>warnings.push(x)}});await b.state.openLink();await b.state.start(1);assert.equal(b.state.open.value,false);assert.equal(warnings.length,1)
})
test('revoked deep link is handled without opening stale detail',async()=>{
 const a=await setup({getReturnablePackage:async()=>{throw Error('scope revoked')}});await a.state.start(1);assert.equal(a.state.open.value,false);assert.equal(a.state.opening.value,false);assert.equal(a.state.form.value.lines.length,0);assert.equal(a.state.error.value,'scope revoked')
})
test('order return navigation is outside the disabled read-only form',async()=>{
 const source=await readFile(new URL('../src/views/erp/components/OrderWorkspace.vue',import.meta.url),'utf8')
 const button=source.indexOf('原包裹退货入库 / 历史');const form=source.indexOf('<el-form ref="formRef"');assert.ok(button>0&&button<form)
})
