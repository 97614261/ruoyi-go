import { resetERPQuery } from '../src/utils/erp-query.mjs'
import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {randomUUID} from 'node:crypto'
import {ref,reactive,computed,watch} from 'vue'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as purchasing from '../src/utils/purchasing.mjs'
import * as purchaseReturn from '../src/utils/purchase-return.mjs'
import * as salesReturn from '../src/utils/sales-return.mjs'
import * as finance from '../src/utils/finance.mjs'
import * as salesFinance from '../src/utils/salesFinance.mjs'
import * as warehouse from '../src/utils/warehouse-operations.mjs'
import * as catalog from '../src/utils/catalog.mjs'
import * as logistics from '../src/utils/logistics.mjs'
import * as erp from '../src/utils/erp.mjs'

const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
const empty=async()=>({rows:[],total:0,data:{lines:[],version:1}})
async function setup(name,overrides={},props={}){
 const path=name.includes('/')?name:`components/${name}`
 const source=await readFile(new URL(`../src/views/erp/${path}.vue`,import.meta.url),'utf8')
 const parsed=parse(source)
 assert.equal(parsed.errors.length,0,`${name}: invalid SFC template`)
 const code=compileScript(parsed.descriptor,{id:name}).content.replace(/^import\s[^\n]*\n/gm,'').replaceAll('import.meta.env.VITE_APP_BASE_API',"'/dev-api'").replace('export default','const component=')
 const hooks={leave:[],unmount:[],activate:[]},messages=[],emitted=[],confirmations=[]
 const proxy={$modal:{msgSuccess:m=>messages.push(m),confirm:async m=>confirmations.push(m),...overrides.modal},download(){},useDict:()=>({sys_normal_disable:ref([])}),resetForm(){}}
 const api={}
 for(const n of ['listPurchases','getPurchase','purchaseEvents','listReceivables','getReceivable','listPurchaseReceipts','getPurchaseReceipt','listReturnableReceipts','getReturnableReceipt','listPurchaseReturns','getPurchaseReturn','listReturnablePackages','getReturnablePackage','listSalesReturns','getSalesReturn','financeList','financeGet','financeReturns','salesFinanceList','salesFinanceGet','salesFinanceReturns','listTransfers','getTransfer','catalogList','catalogGet','listSuppliers','getSupplier','logisticsList','logisticsGet','listErpSku','getErpSku','listStock','listLedger','getStockAlert','reconcileStock','stockReservations','listDocuments','getDocument','listShopItems','getShopItem','listErpShop','getErpShop','listUser','getWarehouseAccess','getShopAccess','listErpWarehouse','getErpWarehouse','listChannels','listImportJobs','getImportJob','importResults'])api[n]=empty
 const stubs={ ERPQueryBar:{}, resetERPQuery,ref,reactive,computed,watch,onMounted(){},onBeforeUnmount:f=>hooks.unmount.push(f),onDeactivated:f=>hooks.leave.push(f),onActivated:f=>hooks.activate.push(f),useRoute:()=>({query:{}}),useRouter:()=>({push(){}}),useUserStore:()=>({id:1,identity:{tenantAdmin:true}}),getCurrentInstance:()=>({proxy}),checkPermi:()=>true,hasPermi:()=>true,crypto:{randomUUID},FormData,saveAs(){},download(){},ElMessage:{success:m=>messages.push(m),warning:m=>messages.push(m)},ElMessageBox:{confirm:async m=>confirmations.push(m)},ReferenceSelect:{},PurchasingReferenceSelect:{},WarehouseAccess:{},ShopAccess:{},OrderFulfillmentDialog:{},FulfillmentBatchDialog:{},OrderReadinessPanel:{},listOrders:empty,orderWorkbench:empty,orderReadiness:empty,...purchasing,...purchaseReturn,...salesReturn,...finance,...salesFinance,...warehouse,...catalog,...logistics,...erp,...api,...overrides}
 const c=vm.runInNewContext(code+'\ncomponent',stubs)
 const state=c.setup(props,{expose(){},emit:(...v)=>emitted.push(v)})
 await Promise.resolve();await Promise.resolve()
 return{state,messages,emitted,confirmations,leave:()=>hooks.leave.forEach(f=>f()),unmount:()=>hooks.unmount.forEach(f=>f()),activate:async()=>{for(const f of hooks.activate)await f()}}
}

const account=id=>({requestKey:`account-key-${id}`,accountCode:`A${id}`,accountName:'test',accountType:'BANK',currency:'CNY',status:'0',remark:''})
const cases=[
 {name:'PurchaseWorkspace',api:'actPurchase',set:(s,id)=>{s.mode.value='confirm';s.form.value={purchaseId:id,version:2,requestKey:`purchase-key-${id}`}}},
 {name:'PurchaseReceiveWorkspace',api:'receivePurchase',set:(s,id)=>{s.form.value={purchaseId:id,version:2,requestKey:`receive-key-${id}`,lines:[{lineId:1,quantity:1,orderedQuantity:3,receivedQuantity:0}]}}},
 {name:'PurchaseReturnWorkspace',api:'postPurchaseReturn',set:(s,id)=>{s.mode.value='return';s.form.value={receiptId:id,version:2,requestKey:`return-key-${id}`,reason:'reason',lines:[{lineId:1,returnQuantity:1,netReceivedQuantity:3}]}}},
 {name:'SalesReturnWorkspace',api:'postSalesReturn',set:(s,id)=>{s.mode.value='return';s.form.value={packageId:id,version:2,requestKey:`return-key-${id}`,reason:'reason',lines:[{lineId:1,returnQuantity:1,netShippedQuantity:3}]}}},
 {name:'FinanceWorkspace',props:{kind:'account'},api:'financeWrite',set:(s,id)=>{s.mode.value='accountCreate';s.form.value=account(id)}},
 {name:'SalesFinanceWorkspace',props:{kind:'account'},api:'salesFinanceWrite',set:(s,id)=>{s.mode.value='accountCreate';s.form.value=account(id)}},
 {name:'TransferWorkspace',api:'actTransfer',set:(s,id)=>{s.mode.value='dispatch';s.form.value={transferId:id,version:2,requestKey:`transfer-key-${id}`}}},
 {name:'CatalogWorkspace',props:{category:true},api:'catalogWrite',set:(s,id)=>{s.editable.value=true;s.form.value={categoryCode:`C${id}`,categoryName:'test',status:'0',version:0,requestKey:`category-key-${id}`}}},
 {name:'LogisticsWorkspace',api:'logisticsWrite',flag:'saving',set:(s,id)=>{s.editable.value=true;s.form.value={id:0,code:`C${id}`,name:'test',status:'0',version:0,requestKey:`logistics-key-${id}`}}},
 {name:'SupplierWorkspace',api:'addSupplier',flag:'saving',method:'save',set:(s,id)=>{s.readonly.value=false;s.formRef.value={validate:async()=>{},clearValidate(){}};s.form.value={supplierCode:`S${id}`,supplierName:'test',status:'0'}}},
 {name:'StockWorkspace',api:'setStockAlert',method:'saveAlert',set:(s,id)=>{s.alertOpen.value=true;s.alert.value={warehouseId:id,skuId:1,version:1,enabled:true,thresholdQuantity:0}}}
]
function prepare(s,c,id){if(s.open)s.open.value=true;if(s.closing)s.closing.value=false;c.set(s,id)}
for(const c of cases)for(const outcome of ['success','failure'])test(`${c.name}: leaving releases its lock; old ${outcome} cannot unlock a newer write`,async()=>{
 const old=deferred(),latest=deferred(),started=deferred(),newStarted=deferred(),calls=[]
 const a=await setup(c.name,{[c.api]:(...v)=>{calls.push(v);if(calls.length===1){started.resolve();return old.promise}newStarted.resolve();return latest.promise}},c.props)
 prepare(a.state,c,1);const first=a.state[c.method||'submit']();await Promise.race([started.promise,first.then(()=>{throw Error('test input did not reach write')})])
 a.leave();assert.equal(a.state[c.flag||'busy'].value,false);await a.activate()
 prepare(a.state,c,9);const current=a.state[c.method||'submit']();await Promise.race([newStarted.promise,current.then(()=>{throw Error('new test input did not reach write')})])
 const beforeMessages=a.messages.length
 if(outcome==='success')old.resolve({data:{rows:1}});else old.reject(Error('lost reply'))
 await first
 assert.equal(a.state[c.flag||'busy'].value,true);assert.equal(a.messages.length,beforeMessages)
 latest.resolve({data:{rows:1}});await current
 assert.equal(a.state[c.flag||'busy'].value,false);assert.equal(calls.length,2)
})

for(const c of cases.filter(c=>!['SupplierWorkspace','StockWorkspace'].includes(c.name)))test(`${c.name}: an unknown write retries its immutable original command`,async()=>{
 const calls=[]
 const a=await setup(c.name,{[c.api]:async(...v)=>{calls.push(JSON.stringify(v));throw Error('lost reply')}},c.props)
 prepare(a.state,c,1);await a.state[c.method||'submit']()
 const original=a.state.pending.value;assert.ok(original)
 // Deliberate state mutation simulates a stale binding or later form edit.
 c.set(a.state,9);await a.state[c.method||'submit']()
 assert.equal(calls.length,2);assert.equal(calls[0],calls[1]);assert.equal(a.state.pending.value,original)
})

const confirmations=[
 {name:'DocumentWorkspace',props:{type:'receipt'},api:'actOnDocument',run:s=>s.act({documentId:1},'confirm')},
 {name:'ShopItemWorkspace',api:'setShopItemMapping',run:s=>{s.mappingOpen.value=true;s.mapping.value={item:{itemId:1,version:2},skuId:3,remark:''};return s.saveMapping()}},
 {name:'WarehouseAccess',api:'setWarehouseAccess',run:s=>{s.open.value=true;s.userId.value=2;s.version.value=1;s.selected.value=[3];return s.save()}},
 {name:'ShopAccess',api:'setShopAccess',run:s=>{s.open.value=true;s.userId.value=2;s.version.value=1;s.selected.value=[3];return s.save()}},
 {name:'warehouse/index',api:'delErpWarehouse',run:s=>s.handleDelete({warehouseId:1})},
 {name:'sku/index',api:'delErpSku',run:s=>s.handleDelete({skuId:1})},
 {name:'importJob/index',api:'retryImportJob',run:s=>s.retry({jobId:1,version:2,status:'FAILED',fileState:'PRESENT'})}
]
for(const c of confirmations)test(`${c.name}: leaving cancels a not-yet-sent confirmed action`,async()=>{
 const prompt=deferred(),writes=[]
 const a=await setup(c.name,{[c.api]:async(...v)=>writes.push(v),modal:{confirm:()=>prompt.promise},ElMessageBox:{confirm:()=>prompt.promise}},c.props)
 const operation=c.run(a.state);a.leave();await a.activate();prompt.resolve();await operation
 assert.equal(writes.length,0);assert.equal(a.messages.length,0)
})

for(const c of [{name:'shop/index',api:'addErpShop',start:'start',save:'save'},{name:'warehouse/index',api:'addErpWarehouse',start:'handleAdd',save:'submitForm'},{name:'sku/index',api:'addErpSku',start:'handleAdd',save:'submitForm'}])test(`${c.name}: validation cannot submit after leaving`,async()=>{
 const validation=deferred(),calls=[]
 const a=await setup(c.name,{[c.api]:async(...v)=>calls.push(v)})
 a.state[c.start]();a.state.formRef.value={validate:()=>validation.promise,clearValidate(){}}
 const operation=a.state[c.save]();a.leave();validation.resolve();await operation
 assert.equal(calls.length,0);assert.equal(a.state.open.value,false)
})

test('single order fulfillment retries original action/version/warehouse after an unknown response',async()=>{
 const calls=[],a=await setup('OrderFulfillmentDialog',{reviewOrder:async(...v)=>{calls.push(JSON.stringify(v));throw Error('lost')}})
 a.state.show({orderId:1,version:2},'review');a.state.form.value.warehouseId=3
 await a.state.submit().catch(()=>{});a.state.target.value.version=99;a.state.form.value.warehouseId=9
 await a.state.submit().catch(()=>{})
 assert.equal(calls.length,2);assert.equal(calls[0],calls[1]);assert.ok(a.state.pending.value)
})

for(const editing of [false,true])test(`order ${editing?'edit':'create'} retries original recipient/version/request key after an unknown response`,async()=>{
 const calls=[],write=async(...v)=>{calls.push(JSON.stringify(v));throw Error('lost')}
 const a=await setup('OrderWorkspace',{addOrder:write,editOrder:write})
 a.state.start();Object.assign(a.state.form.value,{shopId:1,sourceOrderNo:'SOURCE',receiverName:'original',...(editing?{orderId:1,version:2}:{})});a.state.formRef.value={validate:async()=>{},clearValidate(){}}
 await a.state.saveOrder().catch(()=>{});a.state.form.value.receiverName='changed';a.state.form.value.version=99;a.state.form.value.requestKey='changed-key'
 await a.state.saveOrder().catch(()=>{})
 assert.equal(calls.length,2);assert.equal(calls[0],calls[1]);assert.ok(a.state.savePending.value)
})

test('order import retries its original file, shop and request key after an unknown response',async()=>{
 const calls=[],a=await setup('OrderWorkspace',{importOrders:async d=>{calls.push([d.get('shopId'),d.get('requestKey'),await d.get('file').text()]);throw Error('lost')}})
 a.state.openUpload();Object.assign(a.state.upload,{shopId:1,file:new Blob(['original']),requestKey:'original-key'})
 await a.state.submitImport().catch(()=>{});Object.assign(a.state.upload,{shopId:9,file:new Blob(['changed']),requestKey:'changed-key'})
 await a.state.submitImport().catch(()=>{})
 assert.equal(calls.length,2);assert.deepEqual(calls[0],calls[1]);assert.ok(a.state.importPending.value)
})

test('import job requeue retries its original version/key instead of creating a new command',async()=>{
 const calls=[],a=await setup('importJob/index',{retryImportJob:async(...v)=>{calls.push(JSON.stringify(v));throw Error('lost')}})
 const row={jobId:1,version:2,status:'FAILED',fileState:'PRESENT'}
 await a.state.retry(row).catch(()=>{});row.version=99;await a.state.retry(row).catch(()=>{})
 assert.equal(calls.length,2);assert.equal(calls[0],calls[1]);assert.ok(a.state.retryPending.value)
})

const casCases=[
 {...cases.find(c=>c.name==='SupplierWorkspace')},
 {...cases.find(c=>c.name==='StockWorkspace')},
 {name:'DocumentWorkspace',props:{type:'receipt'},api:'createDocument',method:'save',set:(s,id)=>{s.open.value=true;s.form.value={warehouseId:id,requestKey:`document-key-${id}`,lines:[{skuId:1,quantity:1}]}}},
 {name:'ShopItemWorkspace',api:'setShopItemMapping',method:'saveMapping',set:(s,id)=>{s.mappingOpen.value=true;s.mapping.value={item:{itemId:id,version:2},skuId:3,remark:''}}},
 ...['shop','warehouse','sku'].map(dir=>({name:`${dir}/index`,api:{shop:'addErpShop',warehouse:'addErpWarehouse',sku:'addErpSku'}[dir],method:dir==='shop'?'save':'submitForm',set:(s,id)=>{s.open.value=true;s.formRef.value={validate:async()=>{},clearValidate(){}};s.form.value={shopCode:`S${id}`,warehouseCode:`W${id}`,skuCode:`K${id}`,status:'0',unit:'件',version:1}}})),
 ...['Warehouse','Shop'].map(kind=>({name:`${kind}Access`,api:`set${kind}Access`,method:'save',set:(s,id)=>{s.open.value=true;s.userId.value=id;s.version.value=1;s.selected.value=[id+1]}}))
]
for(const c of casCases)test(`${c.name}: legacy CAS/unique-code retry cannot silently change its original target or body`,async()=>{
 const calls=[],a=await setup(c.name,{[c.api]:async(...v)=>{calls.push(JSON.stringify(v));throw Error('unknown')}},c.props)
 prepare(a.state,c,1);await a.state[c.method]().catch(()=>{})
 c.set(a.state,9);await a.state[c.method]().catch(()=>{})
 assert.equal(calls.length,2);assert.equal(calls[0],calls[1])
})

// Cleared remote options must remain visibly loading until the latest request
// settles; this also prevents repeated Element Plus visible-change feedback.
for(const name of ['FinanceWorkspace','SalesFinanceWorkspace']){
 const api=name==='FinanceWorkspace'?'financeList':'salesFinanceList'
 test(`${name}: only latest account lookup may finish loading or restore choices`,async()=>{
  const requests=[],a=await setup(name,{[api]:async kind=>{if(kind!=='account')return empty();const d=deferred();requests.push(d);return d.promise}},{kind:'payable'})
  a.state.open.value=true;a.state.form.value={currency:'CNY'}
  const first=a.state.loadAccounts('OLD'),second=a.state.loadAccounts('NEW')
  assert.equal(a.state.accountLoading.value,true);assert.equal(a.state.accounts.value.length,0)
  requests[0].resolve({rows:[{accountId:1}]});await first
  assert.equal(a.state.accountLoading.value,true);assert.equal(a.state.accounts.value.length,0)
  requests[1].resolve({rows:[{accountId:2}]});await second
  assert.equal(a.state.accountLoading.value,false);assert.equal(a.state.accounts.value[0].accountId,2)
 })
 test(`${name}: rejected account lookup clears loading and choices`,async()=>{
  const a=await setup(name,{[api]:async kind=>{if(kind==='account')throw Error('denied');return empty()}},{kind:'payable'})
  a.state.open.value=true;a.state.accounts.value=[{accountId:1}]
  await a.state.loadAccounts('')
  assert.equal(a.state.accountLoading.value,false);assert.equal(a.state.accounts.value.length,0)
 })
 test(`${name}: leave invalidates an in-flight account selector`,async()=>{
  const d=deferred(),a=await setup(name,{[api]:kind=>kind==='account'?d.promise:empty()},{kind:'payable'})
  a.state.open.value=true;const pending=a.state.loadAccounts('');a.leave()
  assert.equal(a.state.accountLoading.value,false)
  d.resolve({rows:[{accountId:9}]});await pending
  assert.equal(a.state.accountLoading.value,false);assert.equal(a.state.accounts.value.length,0);assert.equal(a.state.open.value,false)
 })
}
