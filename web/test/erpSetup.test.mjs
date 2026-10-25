import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as helpers from '../src/utils/erpSetup.mjs'
const response=()=>({data:{checkedAt:'2026-10-25 21:00:00',checks:helpers.setupChecks.map(r=>({key:r.key,state:'MISSING'}))}})
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/SetupWorkspace.vue',import.meta.url),'utf8')
 const code=compileScript(parse(source).descriptor,{id:'erp-setup'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component=')
 const leave=[],pushes=[],calls=[]
 const stubs={ref,onMounted(){},onActivated(){},onDeactivated:f=>leave.push(f),onBeforeUnmount:f=>leave.push(f),useRouter:()=>({push:r=>pushes.push(r)}),useUserStore:()=>({identity:{tenantAdmin:true,platformAdmin:false}}),checkPermi:()=>true,getERPSetup:async()=>{calls.push(1);return response()},...helpers,...overrides}
 const component=vm.runInNewContext(code+'\ncomponent',stubs)
 return{state:component.setup({},{expose(){}}),leave,pushes,calls}
}
test('enablement denies employees/platform and never fetches their business checklist',async()=>{
 for(const identity of [{tenantAdmin:false},{tenantAdmin:true,platformAdmin:true}]){const x=await setup({useUserStore:()=>({identity})});await x.state.refresh();assert.equal(x.calls.length,0);assert.equal(x.state.rows.value.length,0)}
})
test('enablement clears old state on failure and rejects malformed or incomplete status data',async()=>{
 let n=0;const x=await setup({getERPSetup:async()=>{if(++n>1)throw Error('offline');return response()}});await x.state.refresh();assert.equal(x.state.rows.value.length,16);await x.state.refresh();assert.equal(x.state.rows.value.length,0);assert.equal(x.state.error.value,true)
 assert.throws(()=>helpers.setupRows({checks:[]}));assert.throws(()=>helpers.setupRows({checks:[{key:'roles',state:'READY'},{key:'roles',state:'READY'}]}));assert.throws(()=>helpers.setupRows({checks:[{key:'roles',state:'oops'}]}))
})
test('enablement leaving or newer refresh discards late data; unavailable steps cannot navigate',async()=>{
 let finish;const old=new Promise(ok=>{finish=ok});const x=await setup({getERPSetup:()=>old});const load=x.state.refresh();x.leave.forEach(f=>f());finish(response());await load;assert.equal(x.state.rows.value.length,0)
 const y=await setup();await y.state.refresh();const row=y.state.rows.value[0];y.state.go({...row,state:'NOT_ENABLED'});assert.equal(y.pushes.length,0);y.state.go(row);assert.equal(y.pushes[0],'/system/role')
})
test('role draft retains exact IDs and disables parent cascading, without granting scopes or users',()=>{
 const d=helpers.roleTemplateDraft({templateKey:'erp_warehouse',name:'仓库员',menuIds:[6000,7900,7900]});assert.deepEqual(d,{roleName:'仓库员',roleKey:'erp_warehouse',menuIds:[6000,7900],menuCheckStrictly:false});assert.equal(d.userIds,undefined);assert.equal(d.warehouseIds,undefined)
 for(const menuIds of [[],[-1],[1.5],[Number.MAX_SAFE_INTEGER+1]])assert.throws(()=>helpers.roleTemplateDraft({templateKey:'erp_warehouse',menuIds}))
 assert.throws(()=>helpers.roleTemplateDraft({templateKey:'platform_admin',menuIds:[1]}))
})
