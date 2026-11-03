import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import * as Vue from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import {replenishmentPayload} from '../src/utils/replenishment.mjs'
import {safeID} from '../src/utils/purchasing.mjs'
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
const row={skuId:3,skuCode:'X',unit:'件',neededQuantity:6,suggestedQuantity:6,basis:'a'.repeat(64)}
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/ReplenishmentPanel.vue',import.meta.url),'utf8')
 const code=compileScript(parse(source).descriptor,{id:'replenishment'}).content.replace(/^import[^\n]*\n/gm,'').replace('export default','const component =')
 const hooks=[],activate=[],events=[],props=Vue.reactive({modelValue:true})
 const env={...Vue,ERPQueryBar:{},PurchasingReferenceSelect:{},safeID,replenishmentPayload,crypto:{randomUUID:()=> 'test-replenish-key'},ElMessage:{success(){}},checkPermi:()=>true,onBeforeUnmount:f=>hooks.push(f),onDeactivated:f=>hooks.push(f),onActivated:f=>activate.push(f),listReplenishments:async()=>({rows:[{...row}],total:1}),createReplenishment:async()=>({data:{purchaseId:7,version:1,status:'DRAFT'}}),...overrides}
 const component=vm.runInNewContext(code+'\ncomponent',env)
 const state=component.setup(props,{expose(){},emit:(...args)=>events.push(args)})
 state.query.warehouseId=2;await Vue.nextTick()
 return{state,props,hooks,activate,events}
}
test('replenishment whitelists purchase inputs and freezes basis with zero prices',()=>{
 const form={warehouseId:2,supplierId:1,currency:'cny',requestKey:'x',tenantId:9,lines:[{...row,quantity:6,unitPrice:'0'}]}
 const p=replenishmentPayload(form);assert.equal(p.tenantId,undefined);assert.equal(p.lines[0].unitPrice,'0.0000');assert.deepEqual(p.basis,[{skuId:3,basis:row.basis}]);assert.ok(Object.isFrozen(p.basis[0]));assert.throws(()=>replenishmentPayload({...form,lines:[{...form.lines[0],basis:'bad'}]}))
})
test('replenishment selection excludes covered rows, admits one submit and retries frozen original content',async()=>{
 const wait=deferred(),calls=[];const{state}=await setup({createReplenishment:async p=>{calls.push(p);return wait.promise}})
 state.choose([row,{...row,skuId:4,neededQuantity:0}]);assert.equal(state.form.value.lines.length,1)
 state.form.value.supplierId=1;state.form.value.lines[0].unitPrice='0';const first=state.submit();await state.submit();assert.equal(calls.length,1)
 wait.reject(Error('response lost'));await first;assert.ok(state.pending.value)
 state.form.value.lines[0].quantity=999;state.query.warehouseId=999;await state.submit();assert.equal(calls.length,2);assert.equal(calls[1].warehouseId,2);assert.equal(calls[1].lines[0].quantity,6);assert.equal(calls[1],calls[0])
})
test('replenishment drops late results across warehouse change and close, failures do not look empty-success',async()=>{
 const wait=deferred();const{state,props}=await setup({listReplenishments:()=>wait.promise})
 const request=state.load();state.query.warehouseId=4;await Vue.nextTick();wait.resolve({rows:[row],total:1});await request;assert.equal(state.rows.value.length,0);assert.equal(state.loading.value,false)
 props.modelValue=false;await Vue.nextTick();assert.equal(state.form.value.lines.length,0)
 const failed=await setup({listReplenishments:async()=>{throw Error('offline')}});await failed.state.load();assert.equal(failed.state.rows.value.length,0);assert.match(failed.state.error.value,/offline/)
})
test('replenishment cannot create without purchase permission; leave discards late success and unlocks only its own view',async()=>{
 const blocked=await setup({checkPermi:ps=>ps[0]!=='erp:purchase:add',createReplenishment:()=>assert.fail('permission bypass')});assert.equal(blocked.state.canCreate.value,false);await blocked.state.submit()
 const wait=deferred(),active=await setup({createReplenishment:()=>wait.promise});active.state.choose([row]);active.state.form.value.supplierId=1;active.state.form.value.lines[0].unitPrice='1';const request=active.state.submit();active.hooks[0]();wait.resolve({data:{purchaseId:1}});await request;assert.equal(active.events.filter(e=>e[0]==='created').length,0);assert.equal(active.state.pending.value,null);assert.equal(active.state.busy.value,false)
})
