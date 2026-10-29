import { resetERPQuery } from '../src/utils/erp-query.mjs'
import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref,reactive,computed,watch} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as util from '../src/utils/logistics.mjs'

const deferred=()=>{let resolve,reject;const promise=new Promise((ok,fail)=>{resolve=ok;reject=fail});return{promise,resolve,reject}}
async function setup(name,overrides={}){
 const source=await readFile(new URL(`../src/views/erp/components/${name}.vue`,import.meta.url),'utf8')
 const code=compileScript(parse(source).descriptor,{id:'logistics-state'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component=')
 const hooks=[],stubs={ ERPQueryBar:{}, resetERPQuery,ref,reactive,computed,watch,onMounted(){},onBeforeUnmount(fn){hooks.push(fn)},onDeactivated(fn){hooks.push(fn)},onActivated(){},getCurrentInstance:()=>({proxy:{download:async()=>{}}}),useRouter:()=>({push(){}}),useRoute:()=>({query:{}}),checkPermi:()=>true,ReferenceSelect:{},FulfillmentBatchDialog:{},LogisticsMethodSelect:{},ElMessage:{warning(){},success(){}},ElMessageBox:{confirm:async()=>{}},listPackages:async()=>({rows:[],total:0}),getPackage:async()=>({data:{packageId:1,orderId:2,version:1,status:'READY_TO_SHIP',lines:[]}}),savePackageLogistics:async()=>{},shipPackage:async()=>{},packageShipping:async()=>({data:{packageVersion:1,available:true,snapshot:null}}),savePackageShipping:async()=>{},checkTracking:async()=>({data:{duplicate:false}}),logisticsList:async()=>({rows:[],total:0}),logisticsGet:async()=>({data:{id:1,code:'C',name:'原名称',status:'0',version:1}}),logisticsWrite:async()=>({data:{id:1,version:1}}),logisticsShipments:async()=>({rows:[],total:0}),...util,logisticsKey:()=> 'fixed-request-key',...overrides}
 const component=vm.runInNewContext(code+'\ncomponent',stubs)
 return{state:component.setup({}, {expose(){}}),hooks,source}
}

test('logistics master and package commands whitelist server-owned fields and preserve tracking text',()=>{
 const p=util.logisticsMasterPayload({tenantId:99,id:3,version:2,requestKey:'request-key',code:'C',name:'名称',status:'0',remark:''})
 assert.equal(p.tenantId,undefined);assert.equal(p.id,undefined)
 const command=util.packageLogisticsCommand({packageId:5,version:2},{mode:'CATALOG',shippingMethodId:7,carrierCode:'INJECTED',trackingNo:'000001',requestKey:'request-key',tenantId:99})
 assert.equal(command.payload.trackingNo,'000001');assert.equal(command.payload.carrierCode,undefined);assert.equal(command.payload.tenantId,undefined)
 for(const mode of ['INVALID',''])assert.throws(()=>util.packageLogisticsCommand({packageId:1,version:1},{mode,carrierCode:'C',trackingNo:'x',requestKey:'request-key'}))
 assert.throws(()=>util.logisticsMasterPayload({id:1,version:0,requestKey:'request-key',code:'C',name:'N',status:'0'}))
})
test('master lost response freezes original payload and blocks double save',async()=>{
 const wait=deferred(),calls=[];const a=await setup('LogisticsWorkspace',{logisticsWrite:(...v)=>{calls.push(v);return wait.promise}})
 a.state.start();Object.assign(a.state.form.value,{code:'C',name:'原名称'});const first=a.state.submit();await a.state.submit();assert.equal(calls.length,1);wait.reject(Error('lost'));await first
 assert.ok(a.state.pending.value);a.state.form.value.name='新名称';await a.state.submit();assert.equal(calls[1][2].name,'原名称');assert.match(a.state.error.value,/未确认/)
})
test('explicit master refusal clears stale form and rows, late detail cannot reopen',async()=>{
 const a=await setup('LogisticsWorkspace',{logisticsWrite:async()=>{throw 'error'}});a.state.start();Object.assign(a.state.form.value,{code:'C',name:'N'});a.state.rows.value=[{id:1}];await a.state.submit();assert.equal(a.state.open.value,false);assert.equal(a.state.rows.value.length,0);assert.equal(a.state.pending.value,null)
 const wait=deferred(),b=await setup('LogisticsWorkspace',{logisticsGet:()=>wait.promise});const show=b.state.show({id:1},true);b.hooks.forEach(h=>h());wait.resolve({data:{id:1}});await show;assert.equal(b.state.open.value,false);assert.equal(b.state.form.value.id,undefined)
})
test('package mismatched snapshot version is never combined with old package detail',async()=>{
 const a=await setup('PackageWorkspace',{packageShipping:async()=>({data:{packageVersion:2,available:true,snapshot:{carrierName:'new'}}})});await a.state.show({packageId:1},true);assert.equal(a.state.open.value,false);assert.equal(a.state.detail.value,null)
})
test('catalog package save keeps original command after loss and skips precheck on retry',async()=>{
 const calls=[];let checks=0;const a=await setup('PackageWorkspace',{savePackageShipping:async(...v)=>{calls.push(v);throw Error('lost')},checkTracking:async()=>{checks++;return{data:{duplicate:false}}}})
 a.state.open.value=true;a.state.editing.value=true;a.state.detail.value={packageId:1,version:3};a.state.form.value={mode:'CATALOG',shippingMethodId:7,trackingNo:'00001',requestKey:'original-key'}
 await a.state.save();a.state.form.value.shippingMethodId=9;a.state.form.value.trackingNo='changed';await a.state.save();assert.equal(checks,1);assert.equal(calls.length,2);assert.equal(calls[1][1].shippingMethodId,7);assert.equal(calls[1][1].trackingNo,'00001');assert.equal(calls[1][1].version,3)
})
test('duplicate tracking cancel makes no write and leaves editable form',async()=>{
 let writes=0;const a=await setup('PackageWorkspace',{checkTracking:async()=>({data:{duplicate:true}}),ElMessageBox:{confirm:async()=>{throw Error('cancel')}},savePackageShipping:async()=>{writes++}})
 a.state.open.value=true;a.state.editing.value=true;a.state.detail.value={packageId:1,version:1};a.state.form.value={mode:'CATALOG',shippingMethodId:7,trackingNo:'00001',requestKey:'original-key'};await a.state.save();assert.equal(writes,0);assert.equal(a.state.savePending.value,null);assert.equal(a.state.saving.value,false)
})
test('single shipment lost response retries original version and clears on explicit refusal',async()=>{
 const calls=[];const a=await setup('PackageWorkspace',{shipPackage:async(...v)=>{calls.push(v);if(calls.length===1)throw Error('lost');throw 'error'}});const row={packageId:1,version:2};await a.state.ship(row);assert.ok(a.state.shipPending.value);row.version=9;await a.state.retryShip();assert.equal(calls[1][1].version,2);assert.equal(calls[1][1].requestKey,calls[0][1].requestKey);assert.equal(a.state.shipPending.value,null)
})
test('shipment list rejects late results and clears tracking filters on leave',async()=>{
 const wait=deferred();let n=0;const a=await setup('ShipmentWorkspace',{logisticsShipments:()=>++n===1?wait.promise:Promise.resolve({rows:[{packageId:2}],total:1})});const first=a.state.load();await a.state.load();wait.resolve({rows:[{packageId:1}],total:1});await first;assert.equal(a.state.rows.value[0].packageId,2);a.state.query.trackingNo='private';a.hooks.forEach(h=>h());assert.equal(a.state.rows.value.length,0);assert.equal(a.state.query.trackingNo,'')
})
