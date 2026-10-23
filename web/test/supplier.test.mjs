import test from 'node:test'
import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFile } from 'node:fs/promises'
import { ref, reactive, computed } from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/SupplierWorkspace.vue',import.meta.url),'utf8')
 const {descriptor}=parse(source),code=compileScript(descriptor,{id:'supplier'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component =')
 const hooks=[],messages=[],activated=[]
 const stubs={ref,reactive,computed,onBeforeUnmount:f=>hooks.push(f),onDeactivated:f=>hooks.push(f),onActivated:f=>activated.push(f),checkPermi:()=>true,ElMessage:{success:x=>messages.push(x)},listSuppliers:async()=>({rows:[],total:0}),getSupplier:async()=>({data:{supplierId:1,version:1,supplierCode:'A',supplierName:'test',phone:'private'}}),addSupplier:async()=>{},editSupplier:async()=>{},...overrides}
 const definition=vm.runInNewContext(`${code}\ncomponent`,stubs)
 const state=definition.setup({}, {expose(){}});return {state,hooks,messages,source,activated}
}
test('supplier save busy includes validation, sends one whitelist payload and clears private form',async()=>{
 const wait=deferred(),calls=[];const {state}=await setup({addSupplier:async v=>calls.push(v)});state.start();state.formRef.value={validate:()=>wait.promise,clearValidate(){}};state.form.value.supplierCode='A';state.form.value.supplierName='test';state.form.value.tenantId=999
 const first=state.save();await state.save();assert.equal(state.saving.value,true);wait.resolve();await first;assert.equal(calls.length,1);assert.equal(calls[0].tenantId,undefined);assert.equal(state.open.value,false);assert.equal(state.form.value.phone,undefined)
})
test('supplier failed save preserves contacts and version; retry never silently changes version',async()=>{
 const calls=[];const {state}=await setup({editSupplier:async(id,v)=>{calls.push({id,...v});throw Error('version conflict')}});await state.edit({supplierId:1},false);state.formRef.value={validate:async()=>{},clearValidate(){}};await state.save();assert.equal(state.open.value,true);assert.equal(state.form.value.phone,'private');assert.equal(state.form.value.version,1);assert.ok(state.saveError.value);await state.save();assert.equal(calls[1].version,1)
})
test('supplier list/read/edit permissions stay separate',async()=>{
 const {state}=await setup({checkPermi:perms=>perms[0]==='erp:supplier:list'||perms[0]==='erp:supplier:edit',getSupplier:()=>assert.fail('query permission bypass')});assert.equal(state.canEdit.value,false);await state.edit({supplierId:1},true);await state.edit({supplierId:1},false);assert.equal(state.open.value,false)
})
test('supplier close and deactivation clear contacts and invalidate delayed replies',async()=>{
 const wait=deferred();const {state,hooks}=await setup({getSupplier:()=>wait.promise});const pending=state.edit({supplierId:1},true);hooks[0]();wait.resolve({data:{phone:'old private'}});await pending;assert.equal(state.form.value.phone,undefined);assert.equal(state.open.value,false)
 state.form.value={phone:'private'};state.open.value=false;state.closed();assert.equal(state.form.value.phone,undefined)
})
test('supplier list failure is explicit, not a successful empty result',async()=>{
 const {state}=await setup({listSuppliers:async()=>{throw Error('offline')}});await state.load();assert.equal(state.listError.value,true);assert.equal(state.loading.value,false)
})
test('supplier API avoids contact repeat cache and UI exposes no delete/export',async()=>{
 const {source}=await setup();const api=await readFile(new URL('../src/api/erp/supplier.js',import.meta.url),'utf8');assert.equal((api.match(/repeatSubmit: false/g)||[]).length,2);assert.doesNotMatch(source,/handleDelete|handleExport/);assert.match(source,/class="erp-dialog"/);assert.match(source,/onDeactivated\(/)
})
test('supplier cached page reloads its list after deactivation without restoring contacts',async()=>{
 let calls=0;const {state,hooks,activated}=await setup({listSuppliers:async()=>{calls++;return{rows:[{supplierId:1}],total:1}}});await state.load();const before=calls;hooks[1]();assert.equal(state.rows.value.length,0);await activated[0]();assert.equal(calls,before+1);assert.equal(state.rows.value.length,1);assert.equal(state.form.value.phone,undefined)
})

test('supplier close transition blocks reopen until closed; stale close cannot cancel a new fetch',async()=>{
 let reads=0;const {state,source}=await setup({getSupplier:async()=>{reads++;return{data:{supplierId:1,version:1,phone:'private'}}}})
 await state.edit({supplierId:1},true);state.closeDialog();assert.equal(state.closing.value,true);assert.equal(state.form.value.phone,undefined)
 state.start();await state.edit({supplierId:1},false);assert.equal(reads,1);assert.equal(state.open.value,false)
 state.closed();state.closing.value=false;await state.edit({supplierId:1},false);assert.equal(reads,2);assert.equal(state.open.value,true);assert.equal(state.form.value.phone,'private');assert.match(source,/@closed="closing=false"/)
 state.saving.value=true;state.closeDialog();assert.equal(state.open.value,true)
})
