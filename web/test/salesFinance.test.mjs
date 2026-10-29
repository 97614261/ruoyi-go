import { resetERPQuery } from '../src/utils/erp-query.mjs'
import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {parse,compileScript} from '@vue/compiler-sfc'
import {ref,reactive,computed} from 'vue'
import * as salesFinance from '../src/utils/salesFinance.mjs'
import {safeID,immutablePayload} from '../src/utils/purchasing.mjs'
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
async function setup(kind='account',overrides={}){
 const source=await readFile(new URL('../src/views/erp/components/SalesFinanceWorkspace.vue',import.meta.url),'utf8'),{descriptor}=parse(source),code=compileScript(descriptor,{id:'salesFinance'}).content.replace(/^import\s[^\n]*\n/gm,'').replace('export default','const component=')
 const hooks=[],messages=[],stubs={ ERPQueryBar:{}, resetERPQuery,ref,reactive,computed,onBeforeUnmount:f=>hooks.push(f),onDeactivated:f=>hooks.push(f),onActivated(){},watch(){},useRoute:()=>({query:{}}),useRouter:()=>({push(){}}),checkPermi:()=>true,crypto:{randomUUID:()=> 'salesFinance-fixed-key'},ElMessage:{success:x=>messages.push(x)},ReferenceSelect:{},salesFinanceList:async()=>({rows:[],total:0}),salesFinanceGet:async()=>({data:{}}),salesFinanceReturns:async()=>({rows:[],total:0}),salesFinanceWrite:async()=>({data:{id:1,kind:'account',version:1,status:'0'}}),...salesFinance,safeID,immutablePayload,...overrides}
 const c=vm.runInNewContext(`${code}\ncomponent`,stubs);return{state:c.setup({kind},{expose(){},emit(){}}),hooks,messages,source}
}
const account=()=>({requestKey:'account-test-key',accountCode:'A',accountName:'记账账户',accountType:'BANK',currency:'cny',status:'0',remark:''})
test('salesFinance money sums are exact; rejects numbers, overflows and invalid precision',()=>{assert.equal(salesFinance.salesFinanceSum([{amount:'0.1'},{amount:'0.2'}]),'0.3000');for(const v of [0.1,'1e2','1.00001','01','-1'])assert.throws(()=>salesFinance.salesFinanceSum([{amount:v}]));assert.throws(()=>salesFinance.salesFinanceSum([{amount:'9999999999999999.9999'},{amount:'0.0001'}]))})
test('salesFinance collection canonical whitelist, row versions, positive allocations and exact total',()=>{const p=salesFinance.salesFinanceCollectionPayload({accountId:1,shopId:2,currency:'usd',requestKey:'collection-test-key',tenantId:999,amount:'999',status:'REVERSED',allocations:[{receivableId:4,version:1,amount:'0.2'},{receivableId:3,version:2,amount:'0.1'}]});assert.equal(p.amount,'0.3000');assert.equal(p.tenantId,undefined);assert.equal(p.status,undefined);assert.equal(p.allocations[0].receivableId,3);for(const a of ['0',3,'-1'])assert.throws(()=>salesFinance.salesFinanceCollectionPayload({...p,allocations:[{receivableId:3,version:1,amount:a}]}))})
test('salesFinance account and adjustment payloads do not accept server financial state',()=>{const p=salesFinance.financeAccountPayload({...account(),accountId:99,tenantId:999,version:1},true);assert.equal(p.tenantId,undefined);assert.equal(p.accountId,undefined);assert.equal(p.currency,'CNY');assert.throws(()=>salesFinance.salesFinanceAdjustPayload({version:1,requestKey:'adjust-test-key'},[{returnId:1,registered:true}]));assert.deepEqual(salesFinance.salesFinanceSourcePayload({orderId:1,requestKey:'source-test-key',grossAmount:'9'}),{orderId:1,requestKey:'source-test-key'})})
test('salesFinance request retries original immutable content after lost reply, blocks double click and hides old prices',async()=>{const wait=deferred(),calls=[];const a=await setup('account',{salesFinanceWrite:p=>{calls.push(p);return wait.promise}});a.state.startAccount();Object.assign(a.state.form.value,account());const first=a.state.submit();await a.state.submit();assert.equal(calls.length,1);wait.reject(Error('lost reply'));await first;const original=a.state.pending.value;assert.ok(original);assert.equal(Object.keys(a.state.form.value).length,0);assert.equal(a.state.rows.value.length,0);a.state.form.value.accountName='changed';await a.state.submit();assert.equal(calls[1].payload.accountName,'记账账户');assert.equal(a.state.pending.value,original)})
test('late salesFinance detail and list responses cannot restore closed or inactive data',async()=>{const wait=deferred();const a=await setup('receivable',{salesFinanceGet:()=>wait.promise});const f=a.state.show({receivableId:1},'detail');a.hooks.forEach(x=>x());wait.resolve({data:{receivableId:1,grossAmount:'123.4500'}});await f;assert.equal(a.state.open.value,false);assert.equal(a.state.form.value.grossAmount,undefined);const late=deferred();let n=0;const b=await setup('account',{salesFinanceList:async()=>++n===1?late.promise:{rows:[{accountId:2}],total:1}});await b.state.load();late.resolve({rows:[{accountId:1}],total:1});await Promise.resolve();assert.equal(b.state.rows.value[0].accountId,2)})
test('salesFinance detail read failure allows later reopen, closing dialog guards transitions',async()=>{const a=await setup('account',{salesFinanceGet:async()=>{throw Error('revoked')}});await a.state.show({accountId:1},'detail');assert.equal(a.state.closing.value,false);a.state.startAccount();assert.equal(a.state.open.value,true);a.state.close();a.state.startAccount();assert.equal(a.state.open.value,false);assert.equal(Object.keys(a.state.form.value).length,0)})
test('salesFinance UI and API enforce privacy boundaries and show financial limitations',async()=>{const a=await setup();assert.match(a.source,/不执行银行转账/);assert.match(a.source,/当前仓库范围/);assert.doesNotMatch(a.source,/localStorage|sessionStorage/);const api=await readFile(new URL('../src/api/erp/salesFinance.js',import.meta.url),'utf8');assert.match(api,/repeatSubmit:false/);assert.doesNotMatch(api,/method:'delete'/)})
test('sales credit requires one explicit return, nonnegative exact amount and reason; never estimates quantity prices',()=>{const base={version:1,requestKey:'sales-credit-key',adjustAmount:'0',adjustReason:'确认无金额抵减'};assert.deepEqual(salesFinance.salesFinanceAdjustPayload(base,[{returnId:8,quantity:999}]),{version:1,requestKey:'sales-credit-key',returnId:8,amount:'0.0000',reason:'确认无金额抵减'});for(const bad of [{...base,adjustAmount:undefined},{...base,adjustAmount:'-1'},{...base,adjustAmount:3},{...base,adjustReason:''}])assert.throws(()=>salesFinance.salesFinanceAdjustPayload(bad,[{returnId:8}]));assert.throws(()=>salesFinance.salesFinanceAdjustPayload(base,[{returnId:8},{returnId:9}]))})
test('sales finance shop search uses scoped shop API, not warehouse fallback; failures discard stale choices',async()=>{let good=true;const a=await setup('receivable',{listErpShop:async()=>{if(!good)throw Error('revoked');return{rows:[{shopId:77,shopCode:'S',shopName:'店铺'}]}}});await a.state.findShops('店铺');assert.equal(a.state.shopOptions.value[0].shopId,77);good=false;await a.state.findShops('店铺');assert.equal(a.state.shopOptions.value.length,0);assert.doesNotMatch(a.source,/<ReferenceSelect kind="shop"/);assert.match(a.source,/搜索授权店铺/)})

test('overview currency link selects the correct financial list and rejects forged values before request',async()=>{
 const calls=[];const a=await setup('receivable',{useRoute:()=>({query:{currency:'USD'}}),salesFinanceList:async(k,q)=>{calls.push({...q});return{rows:[],total:0}}});await Promise.resolve();assert.equal(a.state.query.currency,'USD');assert.equal(calls[0].currency,'USD')
 let reads=0;const b=await setup('receivable',{useRoute:()=>({query:{currency:['USD','CNY']}}),salesFinanceList:async()=>{reads++;return{rows:[],total:0}}});assert.equal(reads,0);assert.equal(b.state.listError.value,true);assert.equal(b.state.rows.value.length,0)
})


test('source search returns to page one; reset clears code and retains page length under write locks',async()=>{
 const calls=[]
 const {state:s}=await setup('receivable',{salesFinanceList:async(kind,params)=>{calls.push({kind,params});return{rows:[],total:0}}})
 await Promise.resolve()
 s.open.value=true;s.mode.value='sources';s.sourceQuery.pageNum=5;s.sourceQuery.pageSize=50;s.sourceQuery.code='OLD'
 await s.searchSources()
 assert.equal(calls.at(-1).kind,'source');assert.equal(calls.at(-1).params.pageNum,1);assert.equal(calls.at(-1).params.code,'OLD')
 await s.resetSources()
 assert.equal(calls.at(-1).params.code,undefined);assert.equal(calls.at(-1).params.pageSize,50)
 s.busy.value=true;s.sourceQuery.pageNum=4;s.sourceQuery.code='LOCKED';const count=calls.length
 s.resetSources();s.searchSources();assert.equal(calls.length,count);assert.equal(s.sourceQuery.code,'LOCKED');assert.equal(s.sourceQuery.pageNum,4)
})
