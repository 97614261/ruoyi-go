import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {ref,reactive,computed} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as helpers from '../src/utils/picking.mjs'
const sheet=()=>({packageId:1,version:2,status:'READY_TO_SHIP',scannable:true,checked:false,hasLogistics:true,items:[{skuId:9,scanCode:'00001',quantity:2},{skuId:10,scanCode:'__proto__',quantity:1}]})
const deferred=()=>{let resolve,reject;const promise=new Promise((ok,no)=>{resolve=ok;reject=no});return{promise,resolve,reject}}
async function setup(overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/PickingWorkspace.vue',import.meta.url),'utf8'),code=compileScript(parse(source).descriptor,{id:'picking'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component=')
 const leave=[],prints=[],writes=[],route={query:{}},stubs={ref,reactive,computed,ReferenceSelect:{},watch(){},nextTick:async()=>{},onMounted(){},onActivated(){},onDeactivated:f=>leave.push(f),onBeforeUnmount:f=>leave.push(f),useRouter:()=>({push(){}}),useRoute:()=>route,checkPermi:()=>true,ElMessage:{warning(){},success(){}},ElMessageBox:{confirm:async()=>{}},listPicking:async()=>({rows:[],total:0}),getPicking:async()=>({data:sheet()}),checkPicking:async(id,payload)=>{writes.push({id,payload})},shipPicking:async(id,payload)=>{writes.push({id,payload})},window:{print:()=>prints.push(1)},...helpers,pickingKey:()=> '12345678',pickingCommand:(kind,d,c)=>helpers.pickingCommand(kind,d,c,'12345678'),...overrides}
 const component=vm.runInNewContext(code+'\ncomponent',stubs);return{state:component.setup({},{expose(){}}),leave,prints,writes,route}
}
test('scanner preserves leading zeroes and hostile-looking text; rejects wrong/over/missing quantities',()=>{
 const d=sheet();let counts=helpers.recordPickingScan(d,{},'00001',1);counts=helpers.recordPickingScan(d,counts,'00001',1);counts=helpers.recordPickingScan(d,counts,'__proto__',1);assert.equal(counts[9],2);assert.equal(counts[10],1);assert.equal(Object.getPrototypeOf(counts),Object.prototype)
 assert.throws(()=>helpers.recordPickingScan(d,counts,'00001'));assert.throws(()=>helpers.recordPickingScan(d,{},'1'));assert.throws(()=>helpers.pickingCommand('CHECK',d,{9:1},'12345678'))
 assert.equal(helpers.pickingCommand('CHECK',d,counts,'12345678').payload.scans.find(x=>x.scanCode==='00001').quantity,2);assert.throws(()=>helpers.pickingCommand('SHIP',d,{},'12345678'))
})
test('packing failure keeps exact unknown request, while explicit rejection and leaving clear old data',async()=>{
 const a=await setup({checkPicking:async()=>{throw Error('connection lost')}});await a.state.openPackage(1);a.state.counts.value={9:2,10:1};await a.state.confirmCheck();const original=JSON.stringify(a.state.pending.value);a.state.counts.value={9:999};await a.state.retry();assert.equal(JSON.stringify(a.state.pending.value),original);a.leave.forEach(f=>f());assert.equal(a.state.pending.value,null);assert.equal(a.state.detail.value,null)
 const b=await setup({checkPicking:async()=>{throw 'version refused'}});await b.state.openPackage(1);b.state.counts.value={9:2,10:1};await b.state.confirmCheck();assert.equal(b.state.detail.value,null);assert.equal(b.state.pending.value,null);assert.equal(b.state.saving.value,false)
})
test('late read cannot reopen a left page and failed permission recheck cannot print',async()=>{
 const d=deferred(),a=await setup({getPicking:()=>d.promise});const loading=a.state.openPackage(1);a.leave.forEach(f=>f());d.resolve({data:sheet()});await loading;assert.equal(a.state.detail.value,null)
 let n=0;const b=await setup({getPicking:async()=>{if(++n===2)throw 'forbidden';return{data:sheet()}}});await b.state.openPackage(1);await b.state.printSheet();assert.equal(b.prints.length,0);assert.equal(b.state.detail.value,null)
})

test('printing rechecks authority without discarding scans or typed input for an unchanged sheet',async()=>{
 let reads=0
 const a=await setup({getPicking:async()=>{reads++;return{data:sheet()}}})
 await a.state.openPackage(1)
 a.state.counts.value={9:1};a.state.scanCode.value='00001';a.state.scanQty.value=2
 await a.state.printSheet()
 assert.equal(reads,2);assert.equal(a.prints.length,1)
 assert.equal(a.state.counts.value[9],1);assert.equal(a.state.scanCode.value,'00001');assert.equal(a.state.scanQty.value,2)
})

test('printing a changed current barcode discards old counts; an authority refusal clears everything',async()=>{
 let reads=0
 const a=await setup({getPicking:async()=>{const value=sheet();if(++reads===2)value.items[0].scanCode='00002';return{data:value}}})
 await a.state.openPackage(1);a.state.counts.value={9:1};a.state.scanCode.value='00001';await a.state.printSheet()
 assert.equal(a.prints.length,1);assert.equal(a.state.detail.value.items[0].scanCode,'00002');assert.equal(Object.keys(a.state.counts.value).length,0);assert.equal(a.state.scanCode.value,'')
 let deniedReads=0
 const b=await setup({getPicking:async()=>{if(++deniedReads===2)throw 'forbidden';return{data:sheet()}}})
 await b.state.openPackage(1);b.state.counts.value={9:1};b.state.scanCode.value='00001';await b.state.printSheet()
 assert.equal(b.prints.length,0);assert.equal(b.state.detail.value,null);assert.equal(Object.keys(b.state.counts.value).length,0);assert.equal(b.state.scanCode.value,'')
})

test('leaving during the final print flush cancels the pending browser print',async()=>{
 const ready=deferred(),flush=deferred();let ticks=0
 const a=await setup({nextTick:()=>{if(++ticks===3){ready.resolve();return flush.promise}return Promise.resolve()}})
 await a.state.openPackage(1)
 const printing=a.state.printSheet();await ready.promise
 a.leave.forEach(f=>f());flush.resolve();await printing
 assert.equal(a.prints.length,0);assert.equal(a.state.detail.value,null)
})

test('a newer read of the same package cancels an older pending print',async()=>{
 const ready=deferred(),flush=deferred();let ticks=0,reads=0
 const a=await setup({nextTick:()=>{if(++ticks===3){ready.resolve();return flush.promise}return Promise.resolve()},getPicking:async()=>{const value=sheet();if(++reads===3)value.version=3;return{data:value}}})
 await a.state.openPackage(1)
 const printing=a.state.printSheet();await ready.promise
 await a.state.openPackage(1);flush.resolve();await printing
 assert.equal(a.prints.length,0);assert.equal(a.state.detail.value.version,3)
})

test('domain retry transport bypasses the generic body-retaining duplicate guard',async()=>{
 const source=await readFile(new URL('../src/api/erp/picking.js',import.meta.url),'utf8'),calls=[]
 const api=vm.runInNewContext(source.replace(/^import[^\n]*\n/gm,'').replace(/export const /g,'const ')+'\n;({checkPicking,shipPicking})',{request:config=>{calls.push(config);return Promise.resolve()}})
 const payload={version:3,requestKey:'12345678',scans:[{scanCode:'00001',quantity:2}]};await api.checkPicking(1,payload);await api.checkPicking(1,payload);await api.shipPicking(1,{version:4,requestKey:'abcdefgh'})
 assert.equal(calls.length,3);assert.equal(calls[0].data,payload);assert.equal(calls[1].data,payload);for(const c of calls)assert.equal(c.headers.repeatSubmit,false)
})
