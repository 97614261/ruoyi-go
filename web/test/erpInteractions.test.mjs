import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { randomUUID } from 'node:crypto'
import { ref, reactive, computed,watch } from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as erp from '../src/utils/erp.mjs'

const deferred = () => {
  let resolve, reject
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail })
  return { promise, resolve, reject }
}

// Execute the real SFC setup logic with injected API promises. This is a
// component-state test, NOT a browser, DOM, rendering or click acceptance test.
async function component(name, overrides = {}, props = {}) {
  const file = new URL(`../src/views/erp/components/${name}.vue`, import.meta.url)
  const { descriptor } = parse(await readFile(file, 'utf8'))
  const compiled = compileScript(descriptor, { id: `test-${name}` }).content
    .replace(/^import\s[^\n]*\n/gm, '').replace('export default', 'const component =')
  const messages = [], prompts = []
  const stubs = {
    ref, reactive, computed,watch, onMounted() {},onBeforeUnmount(){}, ReferenceSelect: {},
    useRouter: () => ({ push() {} }),useRoute:()=>({query:{}}), saveAs() {}, download: async()=>{}, FormData,
    FulfillmentBatchDialog:{},OrderReadinessPanel:{},orderWorkbench:async()=>({data:{}}),orderReadiness:async()=>({data:{ready:true,version:1,issues:[],stocks:[]}}),
    OrderFulfillmentDialog:{},reviewOrder:async()=>{},cancelOrder:async()=>{},listPackages:async()=>({rows:[],total:0}),getPackage:async()=>({data:{lines:[]}}),savePackageLogistics:async()=>{},shipPackage:async()=>{},
    listOrders:async()=>({rows:[],total:0}),getOrder:async()=>({data:{lines:[]}}),addOrder:async()=>{},editOrder:async()=>{},importOrders:async()=>({data:{jobId:1}}),orderTemplate:async()=>{},
    listImportJobs:async()=>({rows:[],total:0}),getImportJob:async()=>({data:{}}),importResults:async()=>({rows:[],total:0}),retryImportJob:async()=>{},importSourceFile:async()=>{},
    crypto: { randomUUID }, ...erp,
    useUserStore: () => ({ id: 1, identity: { tenantAdmin: true } }),
    ElMessage: { warning: text => messages.push(['warning', text]), success: text => messages.push(['success', text]) },
    ElMessageBox: { confirm: async text => { prompts.push(text) } },
    listDocuments: async () => ({ rows: [], total: 0 }),
    listErpWarehouse: async () => ({ rows: [], total: 0 }),
    listErpShop: async () => ({ rows: [], total: 0 }),
    getShopAccess: async () => ({ data: { shopIds: [], version: 1 } }),
    setShopAccess: async () => {},
    checkPermi: () => true, ShopAccess: {},
    listChannels: async () => ({ rows: [] }), getErpShop: async () => ({ data: {} }),
    addErpShop: async () => {}, updateErpShop: async () => {},
    listShopItems: async () => ({ rows: [], total: 0 }), getShopItem: async () => ({ data: {} }),
    addShopItem: async () => {}, editShopItem: async () => {},
    setShopItemMapping: async () => {}, cancelShopItemMapping: async () => {},
    listErpSku: async () => ({ rows: [] }),
    listUser: async () => ({ rows: [] }),
    getWarehouseAccess: async () => ({ data: { warehouseIds: [], version: 1 } }),
    setWarehouseAccess: async () => {}, createDocument: async () => {},
    editDocument: async () => {}, actOnDocument: async () => {}, getDocument: async () => ({ data: {} }),
    ...overrides
  }
  const definition = vm.runInNewContext(`${compiled}\ncomponent`, stubs, { filename: file.pathname })
  const emitted=[]
  const state = definition.setup(props, { expose() {}, emit:(...args)=>emitted.push(args) })
  return { state, messages, prompts, emitted }
}

test('readiness rejects stale/blocked checks and changing warehouse invalidates late responses',async()=>{
 const pending=deferred();let writes=0;const {state}=await component('OrderFulfillmentDialog',{orderReadiness:()=>pending.promise,reviewOrder:async()=>writes++})
 state.show({orderId:1,version:2},'review');state.form.value.warehouseId=3;const first=state.check();state.form.value.warehouseId=4;pending.resolve({data:{ready:false,version:2,warehouseId:3,issues:[{message:'shortage'}]}});await first
 assert.equal(state.checkResult.value,null);assert.equal(state.checking.value,false)
 state.checkResult.value={ready:false,issues:[]};await state.submit();assert.equal(writes,0)
 const stale=await component('OrderFulfillmentDialog',{orderReadiness:async()=>({data:{ready:true,version:99,issues:[]}})});stale.state.show({orderId:1,version:2},'review');stale.state.form.value.warehouseId=3;await stale.state.check();assert.equal(stale.state.checkResult.value.ready,false)
})
test('batch partial outcome preserves original key/payload and never resends successful rows',async()=>{
 let fail=true;const calls=[];const {state,emitted}=await component('FulfillmentBatchDialog',{cancelOrder:async(id,payload)=>{calls.push([id,{...payload}]);if(id===2&&fail)throw Error('lost reply');return {data:{existing:true}}}})
 state.show([{orderId:1,version:2,status:'READY_TO_SHIP'},{orderId:2,version:3,status:'PENDING_REVIEW'}],'cancel');state.form.value.reason='fixed reason';await state.run();assert.equal(state.count('DONE'),1);assert.equal(state.count('UNCONFIRMED'),1);assert.equal(state.open.value,true)
 state.form.value.reason='changed after failure';state.results.value[1].version=99;fail=false;await state.run(true)
 assert.equal(calls.length,3);assert.equal(calls[2][0],2);assert.deepEqual(calls[2][1],calls[1][1]);assert.equal(state.count('DONE'),2);assert.equal(emitted.length,2)
})
test('batch confirmation freezes parameters, blocks repeats and cancellation sends nothing',async()=>{
 const pending=deferred(),calls=[];const {state}=await component('FulfillmentBatchDialog',{ElMessageBox:{confirm:()=>pending.promise},reviewOrder:async(id,v)=>calls.push([id,v])})
 state.show([{orderId:1,version:2,status:'PENDING_REVIEW'}],'review');state.form.value.warehouseId=3;const first=state.run();await state.run();state.form.value.warehouseId=4;pending.resolve();await first;assert.equal(calls.length,1);assert.equal(calls[0][1].warehouseId,3)
 const cancelled=await component('FulfillmentBatchDialog',{ElMessageBox:{confirm:async()=>{throw Error('cancel')}},shipPackage:async()=>assert.fail('unexpected write')});cancelled.state.show([{packageId:1,orderId:2,version:1,status:'READY_TO_SHIP'}],'ship');await cancelled.state.run();assert.equal(cancelled.state.frozen.value,false);assert.equal(cancelled.state.saving.value,false)
})
test('batch limits, duplicate IDs, preview blockers and wrong states are explicit',async()=>{
 let writes=0;const {state}=await component('FulfillmentBatchDialog',{reviewOrder:async()=>writes++})
 state.show(Array.from({length:21},(_,i)=>({orderId:i+1})),'review');assert.equal(state.open.value,false)
 state.show([{orderId:1,version:1,status:'PENDING_REVIEW'},{orderId:1,version:1,status:'PENDING_REVIEW'},{orderId:2,version:1,status:'SHIPPED'}],'review');assert.equal(state.results.value.length,2);state.form.value.warehouseId=3;state.results.value[0].check={ready:false,issues:[{message:'unmapped'}]};await state.run();assert.equal(writes,0);assert.equal(state.count('SKIPPED'),2)
})
test('batch preview is invalidated after warehouse change and late responses cannot return',async()=>{
 const pending=deferred();const {state}=await component('FulfillmentBatchDialog',{orderReadiness:()=>pending.promise});state.show([{orderId:1,version:1,status:'PENDING_REVIEW'}],'review');state.form.value.warehouseId=3;const first=state.checkAll();state.form.value.warehouseId=4;pending.resolve({data:{ready:true,version:1}});await first;assert.equal(state.results.value[0].check,null);assert.equal(state.checking.value,false)
})
test('workbench count failure is unknown rather than zero and page refresh clears selection',async()=>{
 const {state}=await component('OrderWorkspace',{orderWorkbench:async()=>{throw Error('offline')},listOrders:async()=>({rows:[{orderId:1}],total:1})});state.selected.value=[{orderId:9}];await state.load();assert.equal(state.rows.value.length,1);assert.equal(state.counts.value,null);assert.equal(state.selected.value.length,0)
})
test('order deep links reject imprecise IDs and clear old recipient/detail response',async()=>{
 const route=reactive({query:{orderId:'1'}}),pending=deferred();let calls=0;const {state}=await component('OrderWorkspace',{useRoute:()=>route,getOrder:()=>{calls++;return pending.promise}})
 const first=state.followOrderLink();route.query.orderId='9007199254740993';await state.followOrderLink();pending.resolve({data:{orderId:1,receiverName:'stale private recipient',lines:[]}});await first;assert.equal(calls,1);assert.equal(state.form.value.receiverName,undefined);assert.equal(state.open.value,false)
})
test('order/package links use explicit IDs and do not assume hidden query permission',async()=>{
 const calls=[];const {state}=await component('OrderWorkspace',{useRouter:()=>({push:v=>calls.push(v)}),checkPermi:p=>p[0]!=='erp:package:query'});state.goPackage({orderId:12,package:{packageId:34}});assert.equal(calls[0].path,'/erp/package');assert.equal(calls[0].query.orderId,'12');assert.equal(calls[0].query.packageId,undefined)
 const denied=await component('PackageWorkspace',{checkPermi:()=>false,useRouter:()=>({push:()=>assert.fail('permission bypass')})});denied.state.goOrder({orderId:12})
})
test('package deep-link mismatch does not open a different order package',async()=>{
 const route={query:{orderId:'1',packageId:'2'}};const {state}=await component('PackageWorkspace',{useRoute:()=>route,getPackage:async()=>({data:{packageId:2,orderId:99,status:'READY_TO_SHIP',lines:[]}})});await state.followPackageLink();assert.equal(state.query.orderId,1);assert.equal(state.open.value,false);assert.equal(state.detail.value,null)
})
test('fulfillment pages keep actions reachable and dialogs viewport bounded',async()=>{
 for(const name of ['OrderWorkspace','PackageWorkspace']){const source=await readFile(new URL(`../src/views/erp/components/${name}.vue`,import.meta.url),'utf8');assert.match(source,/fixed="right"/);assert.match(source,/width="min\(/);assert.match(source,/type="selection"/)}
})
test('order review confirmation snapshots order version and warehouse and admits one action',async()=>{
 const pending=deferred(),writes=[];const {state,emitted}=await component('OrderFulfillmentDialog',{ElMessageBox:{confirm:()=>pending.promise},reviewOrder:async(...args)=>writes.push(args)})
 const row={orderId:1,version:2};state.show(row,'review');state.form.value.warehouseId=3;const first=state.submit();await state.submit();row.version=99;state.form.value.warehouseId=99;pending.resolve();await first
 assert.equal(writes.length,1);assert.equal(writes[0][0],1);assert.equal(writes[0][1].version,2);assert.equal(writes[0][1].warehouseId,3);assert.equal(state.saving.value,false);assert.equal(state.open.value,false);assert.equal(emitted[0][0],'completed')
})
test('order cancel failure retains reason and retry key without false success',async()=>{
 const {state,messages}=await component('OrderFulfillmentDialog',{cancelOrder:async()=>{throw Error('scope revoked')}})
 state.show({orderId:1,version:2},'cancel');state.form.value.reason='keep reason';const key=state.form.value.requestKey;await assert.rejects(state.submit(),/scope revoked/)
 assert.equal(state.form.value.reason,'keep reason');assert.equal(state.form.value.requestKey,key);assert.equal(state.open.value,true);assert.equal(state.saving.value,false);assert.equal(messages.length,0)
})
test('package logistics double save uses snapshot and retains failed draft',async()=>{
 const pending=deferred(),writes=[];const {state}=await component('PackageWorkspace',{savePackageLogistics:(...args)=>{writes.push(args);return pending.promise}})
 state.open.value=true;state.editing.value=true;state.detail.value={packageId:7,version:3};state.form.value={carrierCode:'MANUAL',trackingNo:'00001',requestKey:'retain-key'}
 const first=state.save();await state.save();pending.reject(Error('offline'));await assert.rejects(first,/offline/)
 assert.equal(writes.length,1);assert.equal(writes[0][1].trackingNo,'00001');assert.equal(writes[0][1].version,3);assert.equal(state.form.value.trackingNo,'00001');assert.equal(state.saving.value,false)
})
test('package close invalidates late detail and clears logistics from memory',async()=>{
 const pending=deferred();const {state}=await component('PackageWorkspace',{getPackage:()=>pending.promise})
 const first=state.show({packageId:1},true);state.open.value=false;state.closed();pending.resolve({data:{packageId:1,version:1,trackingNo:'00001',lines:[]}});await first
 assert.equal(state.detail.value,null);assert.equal(state.form.value.trackingNo,undefined);assert.equal(state.detailLoading.value,false)
})
test('package ship confirmation blocks duplicates and snapshots package CAS',async()=>{
 const pending=deferred(),writes=[];const {state}=await component('PackageWorkspace',{ElMessageBox:{confirm:()=>pending.promise},shipPackage:async(...args)=>writes.push(args)})
 const row={packageId:1,version:2};const first=state.ship(row);await state.ship(row);row.version=9;pending.resolve();await first
 assert.equal(writes.length,1);assert.equal(writes[0][1].version,2);assert.equal(state.saving.value,false)
})
test('package list ignores late pages and rejects malformed order filter',async()=>{
 const pending=deferred();let calls=0;const {state}=await component('PackageWorkspace',{listPackages:()=>++calls===1?pending.promise:Promise.resolve({rows:[{packageId:2}],total:1})})
 const first=state.load();await state.load();pending.resolve({rows:[{packageId:1}],total:1});await first;assert.equal(state.rows.value[0].packageId,2)
 state.orderText.value='1 OR 1';await state.search();assert.equal(calls,2)
})
test('order save includes validation in busy window, sends whitelisted text/null and preserves failed draft',async()=>{
 const pending=deferred(),writes=[]
 const {state}=await component('OrderWorkspace',{addOrder:async data=>{writes.push(data);throw Error('revoked')}})
 state.start();Object.assign(state.form.value,{shopId:1,sourceOrderNo:'00001',receiverName:'test',originalAmount:'',status:'SHIPPED'})
 state.formRef.value={validate:()=>pending.promise};const first=state.saveOrder();await state.saveOrder();pending.resolve();await assert.rejects(first,/revoked/)
 assert.equal(writes.length,1);assert.equal(writes[0].sourceOrderNo,'00001');assert.equal(writes[0].originalAmount,null);assert.equal(writes[0].status,undefined)
 assert.equal(state.open.value,true);assert.equal(state.form.value.receiverName,'test');assert.equal(state.saving.value,false)
})
test('order close clears PII and invalidates stale detail before another form',async()=>{
 const pending=deferred();const {state}=await component('OrderWorkspace',{getOrder:()=>pending.promise})
 const first=state.openOrder({orderId:1},false);state.open.value=false;state.closeOrder();state.start()
 pending.resolve({data:{orderId:1,receiverName:'stale recipient',lines:[]}});await first
 assert.equal(state.form.value.orderId,undefined);assert.equal(state.form.value.receiverName,'')
 state.form.value.receiverName='recipient';state.open.value=false;state.closeOrder();assert.equal(state.form.value.receiverName,undefined)
})
test('order list late response does not replace newer query results',async()=>{
 const one=deferred();let calls=0;const {state}=await component('OrderWorkspace',{listOrders:()=>++calls===1?one.promise:Promise.resolve({rows:[{orderId:2}],total:1})})
 const first=state.load();await state.load();one.resolve({rows:[{orderId:1}],total:1});await first;assert.equal(state.rows.value[0].orderId,2)
})
test('import retry snapshots version before confirmation and blocks duplicate submission',async()=>{
 const pending=deferred(),writes=[];const {state}=await component('../importJob/index',{ElMessageBox:{confirm:()=>pending.promise},retryImportJob:async(...args)=>writes.push(args)})
 const row={jobId:1,version:3};const first=state.retry(row);await state.retry(row);row.version=99;pending.resolve();await first
 assert.equal(writes.length,1);assert.equal(writes[0][0],1);assert.equal(writes[0][1].version,3);assert.equal(state.saving.value,false)
})
test('import detail close invalidates pending result and clears data',async()=>{
 const pending=deferred();const {state}=await component('../importJob/index',{getImportJob:()=>pending.promise})
 const first=state.showJob({jobId:1});state.open.value=false;state.closeDetail();pending.resolve({data:{jobId:1}});await first
 assert.equal(state.job.value,null);assert.equal(state.results.value.length,0);assert.equal(state.detailLoading.value,false)
})
test('order APIs exclude request bodies from generic repeat-submit cache',async()=>{
 const source=await readFile(new URL('../src/api/erp/order.js',import.meta.url),'utf8')
 for(const name of ['addOrder','editOrder','importOrders'])assert.match(source.split('\n').find(l=>l.includes(`const ${name} =`)),/repeatSubmit: false/)
})
test('item mapping confirmation locks one write and snapshots item version and SKU', async () => {
  const pending=deferred(), writes=[]
  const {state}=await component('ShopItemWorkspace',{
    ElMessageBox:{confirm:()=>pending.promise},setShopItemMapping:async(id,payload)=>writes.push([id,payload])
  })
  state.mappingOpen.value=true;state.mapping.value={item:{itemId:1,version:3},skuId:7,remark:'keep'}
  const first=state.saveMapping();await state.saveMapping()
  state.mapping.value.skuId=99;state.mapping.value.item.version=99
  pending.resolve();await first
  assert.equal(writes.length,1);assert.equal(writes[0][0],1);assert.equal(writes[0][1].version,3);assert.equal(writes[0][1].skuId,7)
  assert.equal(state.saving.value,false);assert.equal(state.mappingOpen.value,false)
})

test('item mapping cancel and stale version preserve draft without false success',async()=>{
  for(const cancel of [true,false]){
    const {state,messages}=await component('ShopItemWorkspace',{
      ElMessageBox:{confirm:async()=>{if(cancel)throw Error('cancel')}},setShopItemMapping:async()=>{throw Error('stale version')}
    })
    state.mappingOpen.value=true;state.mapping.value={item:{itemId:1,version:3},skuId:7,remark:'keep'}
    if(cancel)await state.saveMapping();else await assert.rejects(state.saveMapping(),/stale version/)
    assert.equal(state.mappingOpen.value,true);assert.equal(state.mapping.value.remark,'keep');assert.equal(state.saving.value,false);assert.equal(messages.length,0)
  }
})

test('item unmap confirmation snapshots row and blocks duplicate requests',async()=>{
  const pending=deferred(),writes=[]
  const {state}=await component('ShopItemWorkspace',{ElMessageBox:{confirm:()=>pending.promise},cancelShopItemMapping:async(...args)=>writes.push(args)})
  const row={itemId:1,version:3};const a=state.unmap(row);await state.unmap(row);row.version=99;pending.resolve();await a
  assert.equal(writes.length,1);assert.equal(writes[0][1],3)
})

test('item form includes validation in busy window and preserves external ID text on failure',async()=>{
  const pending=deferred();let writes=0
  const {state,messages}=await component('ShopItemWorkspace',{addShopItem:async()=>{writes++;throw Error('permission revoked')}})
  state.start();Object.assign(state.form.value,{shopId:2,shopItemCode:'00001',externalVariantId:'00002',title:'draft'})
  state.formRef.value={validate:()=>pending.promise};const a=state.saveItem();await state.saveItem();pending.resolve();await assert.rejects(a,/permission revoked/)
  assert.equal(writes,1);assert.equal(state.form.value.externalVariantId,'00002');assert.equal(state.open.value,true);assert.equal(messages.length,0)
})

test('item stale detail cannot overwrite a new form and stale SKU search cannot restore options',async()=>{
  const detail=deferred(),skus=deferred()
  const {state}=await component('ShopItemWorkspace',{getShopItem:()=>detail.promise,listErpSku:()=>skus.promise})
  const a=state.openItem({itemId:1},false);state.open.value=false;state.closeItem();state.start()
  detail.resolve({data:{itemId:99}});await a;assert.equal(state.form.value.itemId,undefined);assert.equal(state.detailLoading.value,false)
  state.mappingOpen.value=true;state.mapping.value={item:{itemId:2,version:1}}
  const b=state.findSkus('old');state.mappingOpen.value=false;state.closeMapping();state.mappingOpen.value=true;state.skus.value=[]
  skus.resolve({rows:[{skuId:99}]});await b;assert.equal(state.skus.value.length,0)
})

test('shop scope confirmation snapshots the target and blocks double save', async () => {
  const pending = deferred(); const writes = []
  const { state, prompts } = await component('ShopAccess', {
    ElMessageBox: { confirm: text => { prompts.push(text); return pending.promise } },
    setShopAccess: async (id, body) => { writes.push([id, body]) }
  })
  state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
  const first = state.save(); await state.save()
  state.userId.value = 9; state.selected.value.push(7)
  pending.resolve(); await first
  assert.equal(prompts.length, 1); assert.equal(writes.length, 1)
  assert.equal(writes[0][0], 2); assert.equal(writes[0][1].shopIds.join(','), '3')
  assert.equal(state.saving.value, false)
})

test('shop scope close/reopen and employee changes invalidate old responses', async () => {
  const old = deferred(), newer = deferred()
  const { state } = await component('ShopAccess', { getShopAccess: id => id === 2 ? old.promise : newer.promise })
  state.open.value = true; state.userId.value = 2
  const a = state.chooseUser(2); state.userId.value = 3; const b = state.chooseUser(3)
  newer.resolve({ data: { shopIds: [8], version: 4 } }); await b
  old.resolve({ data: { shopIds: [99], version: 9 } }); await a
  assert.equal(state.selected.value.join(','), '8'); assert.equal(state.version.value, 4)
  const delayed = deferred(); const second = await component('ShopAccess', { getShopAccess: () => delayed.promise })
  second.state.open.value = true; second.state.userId.value = 2
  const request = second.state.chooseUser(2); second.state.open.value = false; second.state.closed(); await second.state.start()
  delayed.resolve({ data: { shopIds: [99], version: 9 } }); await request
  assert.equal(second.state.version.value, undefined); assert.equal(second.state.selected.value.length, 0)
})

test('shop scope failed or canceled save preserves selection and reports no success', async () => {
  for (const canceled of [true, false]) {
    let writes = 0
    const { state, messages } = await component('ShopAccess', {
      ElMessageBox: { confirm: async () => { if (canceled) throw new Error('cancel') } },
      setShopAccess: async () => { writes++; throw new Error('stale version') }
    })
    state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
    if (canceled) await state.save(); else await assert.rejects(state.save(), /stale version/)
    assert.equal(writes, canceled ? 0 : 1); assert.equal(state.open.value, true)
    assert.equal(state.selected.value.join(','), '3'); assert.equal(state.saving.value, false); assert.equal(messages.length, 0)
  }
})

test('shop edits include validation in busy state and preserve draft on conflict', async () => {
  const validation = deferred(); let writes = 0
  const { state, messages } = await component('../shop/index', {
    updateErpShop: async () => { writes++; throw new Error('stale version') }
  })
  state.open.value = true; state.form.value = { shopId: 1, version: 2, shopName: 'keep draft' }
  state.formRef.value = { validate: () => validation.promise }
  const first = state.save(); await state.save(); assert.equal(writes, 0)
  validation.resolve(); await assert.rejects(first, /stale version/)
  assert.equal(writes, 1); assert.equal(state.saving.value, false); assert.equal(state.open.value, true)
  assert.equal(state.form.value.shopName, 'keep draft'); assert.equal(messages.length, 0)
})

test('shop list stale response cannot replace newer page', async () => {
  const old = deferred(); let calls = 0
  const { state } = await component('../shop/index', { listErpShop: async () => ++calls === 1 ? old.promise : ({ rows: [{ shopId: 2 }], total: 1 }) })
  await state.load(); old.resolve({ rows: [{ shopId: 99 }], total: 100 }); await Promise.resolve(); await Promise.resolve()
  assert.equal(state.rows.value[0].shopId, 2); assert.equal(state.total.value, 1)
})

test('shop close invalidation releases detail busy and cannot poison a reopened dialog', async () => {
  const pending = deferred()
  const { state } = await component('../shop/index', { getErpShop: () => pending.promise })
  const old = state.edit({ shopId: 1 }, false)
  assert.equal(state.detailLoading.value, true)
  state.open.value = false; state.closed()
  assert.equal(state.detailLoading.value, false)
  state.start(); state.closed() // a stale animation callback after reopen is harmless
  pending.resolve({ data: { shopId: 99, shopName: 'old' } }); await old
  assert.equal(state.open.value, true); assert.equal(state.form.value.shopId, undefined)
  assert.equal(state.detailLoading.value, false)
})

test('document double submit sends one request and preserves draft on timeout/offline rejection', async () => {
  for (const message of ['Network Error', 'timeout of 10000ms exceeded', 'warehouse access revoked']) {
    const pending = deferred(); let writes = 0
    const { state, messages } = await component('DocumentWorkspace', {
      createDocument: () => { writes++; return pending.promise }
    }, { type: 'receipt' })
    state.start()
    Object.assign(state.form.value, { warehouseId: 1, lines: [{ skuId: 2, quantity: 3 }], remark: 'keep my draft' })
    const requestKey = state.form.value.requestKey
    const first = state.save(); await state.save()
    assert.equal(writes, 1); assert.equal(state.saving.value, true)
    pending.reject(new Error(message)); await assert.rejects(first, new RegExp(message.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
    assert.equal(state.saving.value, false); assert.equal(state.open.value, true)
    assert.equal(state.form.value.remark, 'keep my draft'); assert.equal(state.form.value.requestKey, requestKey)
    assert.equal(messages.filter(([type]) => type === 'success').length, 0)
  }
})

test('successful draft retry reuses the original creation idempotency key', async () => {
  let attempts = 0; const payloads = []
  const { state, messages } = await component('DocumentWorkspace', {
    createDocument: async (_, payload) => { payloads.push(payload); if (++attempts === 1) throw new Error('Network Error') }
  }, { type: 'receipt' })
  state.start(); Object.assign(state.form.value, { warehouseId: 1, lines: [{ skuId: 2, quantity: 1 }] })
  await assert.rejects(state.save(), /Network Error/); await state.save()
  assert.equal(payloads[0].requestKey, payloads[1].requestKey)
  assert.equal(state.open.value, false); assert.equal(messages.filter(([type]) => type === 'success').length, 1)
})

test('document confirmation cancellation and failure release busy state without a success message', async () => {
  let writes = 0
  const { state, messages } = await component('DocumentWorkspace', {
    ElMessageBox: { confirm: async () => { throw new Error('cancel') } },
    actOnDocument: async () => { writes++ }
  }, { type: 'sales' })
  await state.act({ documentId: 1 }, 'confirm')
  assert.equal(writes, 0); assert.equal(state.working.value, false); assert.equal(messages.length, 0)
  const failure = await component('DocumentWorkspace', { actOnDocument: async () => { throw new Error('scope denied') } }, { type: 'sales' })
  await assert.rejects(failure.state.act({ documentId: 1 }, 'ship'), /scope denied/)
  assert.equal(failure.state.working.value, false); assert.equal(failure.messages.length, 0)
})

test('document repeated action while confirmation is pending opens one prompt only', async () => {
  const pending = deferred(); let prompts = 0, writes = 0
  const { state } = await component('DocumentWorkspace', {
    ElMessageBox: { confirm: () => { prompts++; return pending.promise } },
    actOnDocument: async () => { writes++ }
  }, { type: 'sales' })
  const first = state.act({ documentId: 1 }, 'ship'); await state.act({ documentId: 1 }, 'ship')
  assert.equal(prompts, 1); pending.resolve(); await first; assert.equal(writes, 1)
})

test('warehouse authorization blocks repeated save before the confirmation resolves', async () => {
  const pending = deferred(); let prompts = 0, writes = 0
  const { state } = await component('WarehouseAccess', {
    ElMessageBox: { confirm: () => { prompts++; return pending.promise } },
    setWarehouseAccess: async () => { writes++ }
  })
  state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
  const first = state.save(); const second = state.save()
  assert.equal(prompts, 1)
  pending.resolve(); await Promise.all([first, second]); assert.equal(writes, 1); assert.equal(state.saving.value, false)
})

test('warehouse canceled/rejected authorization never reports success or closes the form', async () => {
  for (const cancelPrompt of [true, false]) {
    let writes = 0
    const { state, messages } = await component('WarehouseAccess', {
      ElMessageBox: { confirm: async () => { if (cancelPrompt) throw new Error('cancel') } },
      setWarehouseAccess: async () => { writes++; throw new Error('stale version') }
    })
    state.open.value = true; state.userId.value = 2; state.version.value = 1; state.selected.value = [3]
    if (cancelPrompt) await state.save(); else await assert.rejects(state.save(), /stale version/)
    assert.equal(writes, cancelPrompt ? 0 : 1); assert.equal(state.open.value, true)
    assert.equal(state.saving.value, false); assert.equal(messages.length, 0)
  }
})

test('closed/reopened warehouse form ignores the old employee scope response', async () => {
  const pending = deferred()
  const { state } = await component('WarehouseAccess', { getWarehouseAccess: () => pending.promise })
  state.open.value = true; state.userId.value = 2
  const old = state.chooseUser(2); state.open.value = false; state.start()
  pending.resolve({ data: { version: 9, warehouseIds: [999] } }); await old
  assert.equal(state.userId.value, undefined); assert.equal(state.version.value, undefined)
  assert.equal(state.selected.value.length, 0)
})
