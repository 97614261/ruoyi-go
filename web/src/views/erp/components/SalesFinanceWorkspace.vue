<template>
 <div class="app-container">
  <el-alert title="销售财务登记账：不执行银行转账，不代表银行余额或完整会计账。应收按已发货订单人工登记；退货需另登记抵减，超收不等于已退款。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item v-if="kind!=='statement'" label="完整编码/单号"><el-input v-model="query.code" clearable/></el-form-item>
   <el-form-item v-if="can('erp:shop:list')" label="店铺"><el-select v-model="query.shopId" clearable filterable remote :remote-method="findShops" @visible-change="v=>v&&findShops('')" style="width:190px" placeholder="搜索授权店铺"><el-option v-for="s in shopOptions" :key="s.shopId" :value="s.shopId" :label="s.shopCode+' · '+s.shopName"/></el-select></el-form-item><el-form-item label="币种"><el-input v-model="query.currency" maxlength="3" placeholder="如 CNY" style="width:90px" clearable/></el-form-item>
   <el-form-item v-if="kind!=='account'" label="仓库"><ReferenceSelect v-if="kind!=='collection'" kind="warehouse" v-model="query.warehouseId" style="width:190px"/><span v-else>仅展示有全部仓库范围的收款</span></el-form-item>
   <el-form-item v-if="kind==='account'||kind==='collection'" label="状态"><el-select v-model="query.status" clearable style="width:120px"><template v-if="kind==='account'"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></template><template v-else><el-option label="已登记" value="POSTED"/><el-option label="已冲销" value="REVERSED"/></template></el-select></el-form-item>
   <el-form-item><el-button @click="search">查询</el-button><el-button v-if="kind==='account'&&can('erp:financeAccount:add')" type="primary" :disabled="busy||opening||closing" @click="startAccount">新增账户</el-button><el-button v-if="kind==='receivable'&&can('erp:receivable:add')" type="primary" :disabled="busy||opening||closing" @click="startSources">从已发货订单登记应收</el-button><el-button v-if="kind==='receivable'&&can('erp:collection:add')" :disabled="busy||opening||closing||!selected.length" type="success" @click="startCollection">收款登记（本页已选{{selected.length}}）</el-button><el-button v-if="kind==='collection'&&can('erp:receivable:list')" @click="router.push('/erp/receivable')">去应收页选择核销并收款</el-button><el-button v-if="['receivable','statement'].includes(kind)&&can('erp:'+(kind==='receivable'?'receivable':'shopStatement')+':export')" @click="exportRows">导出当前筛选</el-button></el-form-item>
  </el-form>
  <el-alert v-if="kind==='statement'" title="仅汇总当前仓库范围内已登记的应收、抵减和有效核销；未登记退货不在抵减中。待收与超收分别逐单汇总，不自动跨单抵账，也不是全企业/银行余额。" type="warning" :closable="false" class="mb8"/>
  <el-alert v-if="listError" title="读取失败，旧价格及列表已清空，请重新查询。" type="error" :closable="false"/>
  <el-alert v-if="error&&!open" :title="error" type="warning" :closable="false"/>
  <el-alert v-if="lastResult" :title="`已确认 ${lastResult.kind} #${lastResult.id} · v${lastResult.version} · ${lastResult.status}。重放显示原结果，列表另行刷新。`" type="success" closable @close="lastResult=null"/>
  <el-table :data="rows" size="small" v-loading="loading" :row-key="r=>kind==='statement'?r.shopId+'-'+r.currency:r[{account:'accountId',receivable:'receivableId',collection:'collectionId'}[kind]]" @selection-change="selected=$event">
   <el-table-column v-if="kind==='receivable'&&can('erp:collection:add')" type="selection" width="42" :selectable="r=>r.dueAmount!=='0.0000'&&r.pendingReturnCount===0"/>
   <template v-if="kind==='account'"><el-table-column prop="accountCode" label="内部账户编码" min-width="150"/><el-table-column prop="accountName" label="名称" min-width="150"/><el-table-column prop="accountType" label="类型" width="90"/></template>
   <template v-else><el-table-column v-if="kind!=='statement'" :prop="kind==='receivable'?'receivableNo':'collectionNo'" label="单号" min-width="310"/><el-table-column prop="shopName" label="店铺快照" min-width="150"/><el-table-column v-if="kind==='receivable'" prop="orderNo" label="原订单" min-width="290"/><el-table-column v-if="kind==='collection'" prop="accountName" label="收款账户快照" min-width="140"/></template>
   <el-table-column prop="currency" label="币种" width="75"/>
   <template v-if="kind==='receivable'||kind==='statement'"><el-table-column prop="grossAmount" label="毛应收" min-width="125"/><el-table-column prop="creditAmount" label="已登记抵减" min-width="125"/><el-table-column prop="receivedAmount" label="有效已收" min-width="125"/><el-table-column prop="dueAmount" label="待收" min-width="125"/><el-table-column prop="overreceivedAmount" label="超收待处理" min-width="125"/></template>
   <el-table-column v-if="kind==='receivable'" prop="pendingReturnCount" label="待登记退货笔数" width="130"/>
   <el-table-column v-if="kind==='collection'" prop="amount" label="收款记账金额" min-width="145"/>
   <el-table-column v-if="kind==='account'||kind==='collection'" label="状态" width="95"><template #default="{row}">{{salesFinanceStatuses[row.status]||row.status}}</template></el-table-column>
   <el-table-column v-if="kind!=='statement'" label="操作" width="195" fixed="right"><template #default="{row}"><el-button v-if="can('erp:'+permissionKey+':query')" link @click="show(row,'detail')">详情</el-button><el-button v-if="kind==='account'&&can('erp:financeAccount:query')&&can('erp:financeAccount:edit')" link @click="show(row,'edit')">修改</el-button><el-button v-if="kind==='collection'&&row.status==='POSTED'&&can('erp:collection:query')&&can('erp:collection:reverse')" link type="danger" @click="show(row,'reverse')">记账冲销</el-button></template></el-table-column>
  </el-table>
  <pagination v-show="!listError&&total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="dialogTitle" class="erp-dialog" width="min(1100px, calc(100vw - 24px))" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" :before-close="beforeClose" @close="closed" @closed="closing=false">
   <el-alert v-if="error" :title="error" type="warning" :closable="false" class="mb8"/>
   <el-alert v-if="pending" title="请求内容已固定，价格明细已隐藏。结果未确认时只可沿用原请求重试；关闭后先查记录，不能换键重复收款。" type="warning" :closable="false"/>
   <template v-if="!pending">
    <template v-if="mode==='sources'">
     <p>只列未登记应收且当前有仓库范围的已发货订单。历史数据不会自动入账。</p>
     <el-form inline size="small" @submit.prevent="loadSources"><el-form-item label="完整订单号"><el-input v-model="sourceQuery.code" clearable/></el-form-item><el-form-item><el-button @click="sourceQuery.pageNum=1;loadSources()">查询</el-button></el-form-item></el-form>
     <el-table :data="sources" size="small" v-loading="opening"><el-table-column prop="orderNo" label="已发货订单" min-width="300"/><el-table-column prop="shopName" label="店铺" min-width="140"/><el-table-column prop="currency" label="币种" width="80"/><el-table-column label="操作" width="110"><template #default="{row}"><el-button link type="primary" @click="previewSource(row.orderId)">核对原总额</el-button></template></el-table-column></el-table>
     <pagination v-show="sourceTotal>0" :total="sourceTotal" v-model:page="sourceQuery.pageNum" v-model:limit="sourceQuery.pageSize" @pagination="loadSources"/>
    </template>
    <el-form v-if="mode.startsWith('account')" label-width="100px" size="small" :disabled="busy||mode==='accountDetail'">
     <el-form-item label="内部编码"><el-input v-model="form.accountCode" maxlength="64" :disabled="mode!=='accountCreate'"/></el-form-item><el-form-item label="名称"><el-input v-model="form.accountName" maxlength="120"/></el-form-item><el-form-item label="类型"><el-select v-model="form.accountType" :disabled="mode!=='accountCreate'"><el-option label="银行记账" value="BANK"/><el-option label="现金记账" value="CASH"/><el-option label="其他" value="OTHER"/></el-select></el-form-item><el-form-item label="币种"><el-input v-model="form.currency" maxlength="3" :disabled="mode!=='accountCreate'"/></el-form-item><el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item><el-form-item label="备注"><el-input v-model="form.remark" maxlength="500" type="textarea"/></el-form-item>
    </el-form>
    <template v-if="mode==='source'||mode==='receivableDetail'">
     <p>{{form.orderNo}} · {{form.shopName}} · {{form.currency}} · 原批毛应收 {{form.grossAmount}}</p>
     <p v-if="mode==='receivableDetail'">净应收 {{form.netAmount}} ／ 有效已收 {{form.receivedAmount}} ／ 待收 {{form.dueAmount}} ／ 超收 {{form.overreceivedAmount}} ／ v{{form.version}}</p>
     <el-table :data="form.lines||[]" size="small"><el-table-column prop="skuCode" label="原SKU编码" min-width="140"/><el-table-column prop="productName" label="原商品" min-width="140"/><el-table-column prop="unit" label="单位" width="70"/><el-table-column prop="quantity" label="原发货量" width="100"/></el-table>
     <template v-if="mode==='receivableDetail'&&can('erp:receivable:adjust')">
      <h4>原订单销售退货抵减（每次选一笔）</h4><el-checkbox v-model="pendingOnly" @change="returnQuery.pageNum=1;loadReturns()">只看待登记</el-checkbox><span> 已收款后的退货可能形成超收，需后续处理退款。</span>
      <el-table :data="returns" size="small" @selection-change="returnSelection=$event"><el-table-column type="selection" width="42" :selectable="r=>!r.registered"/><el-table-column prop="returnNo" label="原销售退货单" min-width="300"/><el-table-column label="已登记金额" min-width="140"><template #default="{row}">{{row.registered?row.amount:'待人工确认'}}</template></el-table-column><el-table-column label="状态" width="100"><template #default="{row}">{{row.registered?'已登记':'待登记'}}</template></el-table-column></el-table>
      <el-alert title="订单没有明细售价，不按退货数量推算金额。请每次选择一笔，明确填写本次抵减金额及原因；零额也需确认，不能超过剩余未抵减毛额。" type="warning" :closable="false"/>
      <el-form inline size="small" class="mt8"><el-form-item label="人工抵减金额"><el-input v-model="form.adjustAmount" maxlength="21" inputmode="decimal" placeholder="明确金额，如0.0000"/></el-form-item><el-form-item label="核对原因"><el-input v-model="form.adjustReason" maxlength="500" placeholder="必填核对说明"/></el-form-item></el-form>
      <pagination v-show="returnTotal>0" :total="returnTotal" v-model:page="returnQuery.pageNum" v-model:limit="returnQuery.pageSize" @pagination="loadReturns"/>
     </template>
    </template>
    <template v-if="mode==='collectionCreate'">
     <p>{{form.shopName}} · {{form.currency}} · 合计 {{collectionTotal}}。以下每行金额为本次核销，可分次收；每行仍须通过服务端当前版本与待收检查。</p>
     <el-form label-width="100px" size="small"><el-form-item label="记账账户"><el-select v-model="form.accountId" filterable remote :remote-method="loadAccounts" @visible-change="v=>v&&loadAccounts('')" placeholder="最多100条，可输入完整账户编码检索" style="width:100%"><el-option v-for="a in accounts" :key="a.accountId" :value="a.accountId" :label="a.accountCode+' · '+a.accountName+' · '+a.currency"/></el-select></el-form-item><el-form-item label="记账备注"><el-input v-model="form.remark" maxlength="500"/></el-form-item></el-form>
     <el-table :data="form.allocations" size="small"><el-table-column prop="receivableNo" label="应收单号" min-width="300"/><el-table-column prop="dueAmount" label="读取时待收" min-width="130"/><el-table-column label="本次核销金额" min-width="160"><template #default="{row}"><el-input v-model="row.amount" maxlength="21" inputmode="decimal" placeholder="精确十进制字符串"/></template></el-table-column></el-table>
    </template>
    <template v-if="mode==='collectionDetail'||mode==='collectionReverse'">
     <p>{{form.collectionNo}} · {{form.shopName}} · {{form.accountName}} · {{form.currency}} {{form.amount}} · {{salesFinanceStatuses[form.status]}} · v{{form.version}}</p>
     <el-table :data="form.allocations||[]" size="small"><el-table-column prop="receivableId" label="应收ID" width="130"/><el-table-column prop="amount" label="原核销金额" min-width="160"/></el-table>
     <p>备注：{{form.remark||'—'}}</p><p v-if="form.reverseReason">冲销原因：{{form.reverseReason}}</p>
     <template v-if="mode==='collectionReverse'"><el-alert title="仅撤销本系统的整笔收款记账及核销，不会撤销真实转账、发起退款或修改订单库存。" type="warning" :closable="false"/><el-input class="mt8" v-model="form.reason" type="textarea" maxlength="500" placeholder="必填冲销原因"/></template>
    </template>
   </template>
   <template #footer><el-button :disabled="busy" @click="close">{{pending?'关闭，之后先核对记录':'关闭'}}</el-button><el-button v-if="pending||canSubmit" type="primary" :loading="busy" @click="submit">{{pending?'沿用原请求重试':submitTitle}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,computed,watch,onBeforeUnmount,onDeactivated,onActivated} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {ElMessage} from 'element-plus'
import {checkPermi} from '@/utils/permission'
import {download} from '@/utils/request'
import {salesFinanceList,salesFinanceGet,salesFinanceReturns,salesFinanceWrite} from '@/api/erp/salesFinance'
import {salesFinanceTitles,salesFinanceStatuses,salesFinanceSum,financeAccountPayload,salesFinanceCollectionPayload,salesFinanceAdjustPayload,salesFinanceReversePayload,salesFinanceSourcePayload} from '@/utils/salesFinance.mjs'
import {safeID,immutablePayload} from '@/utils/purchasing.mjs'
import ReferenceSelect from './ReferenceSelect.vue'
import {listErpShop} from '@/api/erp/shop'
const shopOptions=ref([]);let shopSeq=0
async function findShops(keyword){const seq=++shopSeq;shopOptions.value=[];try{const r=await listErpShop({pageNum:1,pageSize:100,shopName:keyword});if(seq===shopSeq&&!inactive)shopOptions.value=r.rows}catch{if(seq===shopSeq)shopOptions.value=[]}}
const props=defineProps({kind:{type:String,required:true}}),kind=props.kind,router=useRouter(),route=useRoute(),can=p=>checkPermi([p]),permissionKey={account:'financeAccount',receivable:'receivable',collection:'collection',statement:'shopStatement'}[kind]
const query=reactive({pageNum:1,pageSize:10}),rows=ref([]),selected=ref([]),total=ref(0),loading=ref(false),listError=ref(false),open=ref(false),closing=ref(false),opening=ref(false),mode=ref(''),form=ref({}),pending=ref(null),busy=ref(false),error=ref(''),lastResult=ref(null),sources=ref([]),sourceTotal=ref(0),sourceQuery=reactive({pageNum:1,pageSize:10}),accounts=ref([]),returns=ref([]),returnTotal=ref(0),returnQuery=reactive({pageNum:1,pageSize:10}),pendingOnly=ref(true),returnSelection=ref([])
const dialogTitle=computed(()=>({sources:'选择已过账已发货订单',source:'按原发货及原订单总额登记应收',accountCreate:'新增内部记账账户',accountEdit:'修改账户资料',accountDetail:'账户详情',receivableDetail:'应收与原批退货抵减',collectionCreate:'登记收款并核销',collectionDetail:'收款详情',collectionReverse:'整笔收款记账冲销'}[mode.value]||salesFinanceTitles[kind]))
const collectionTotal=computed(()=>{try{return salesFinanceSum(form.value.allocations||[])}catch{return '金额格式待核对'}})
const canSubmit=computed(()=>['accountCreate','accountEdit','source','collectionCreate','collectionReverse'].includes(mode.value)||mode.value==='receivableDetail'&&can('erp:receivable:adjust')&&returnSelection.value.length===1)
const submitTitle=computed(()=>mode.value==='receivableDetail'?'登记所选退货抵减':mode.value==='collectionReverse'?'确认整笔记账冲销':mode.value==='collectionCreate'?'确认已收款并核销':mode.value==='source'?'确认登记原批应收':'保存账户')
let listSeq=0,detailSeq=0,sourceSeq=0,accountSeq=0,returnSeq=0,viewSeq=0,inactive=false
async function load(){const seq=++listSeq;rows.value=[];selected.value=[];total.value=0;loading.value=true;listError.value=false;try{const v=await salesFinanceList(kind,{...query});if(seq===listSeq&&!inactive){rows.value=v.rows;total.value=v.total}}catch{if(seq===listSeq&&!inactive)listError.value=true}finally{if(seq===listSeq)loading.value=false}}
function search(){query.pageNum=1;load()}
function resetForm(){++detailSeq;++accountSeq;++returnSeq;++sourceSeq;form.value={};returns.value=[];returnSelection.value=[];returnTotal.value=0;sources.value=[];sourceTotal.value=0;accounts.value=[];pending.value=null}
function start(action){if(busy.value||opening.value||closing.value)return false;resetForm();error.value='';mode.value=action;form.value={requestKey:crypto.randomUUID()};open.value=true;return true}
function startAccount(){if(!can('erp:financeAccount:add')||!start('accountCreate'))return;Object.assign(form.value,{accountCode:'',accountName:'',accountType:'BANK',currency:'CNY',status:'0',remark:''})}
function startSources(){if(!can('erp:receivable:add')||!start('sources'))return;sourceQuery.pageNum=1;sourceQuery.code='';loadSources()}
async function loadSources(){const seq=++sourceSeq;sources.value=[];sourceTotal.value=0;opening.value=true;try{const v=await salesFinanceList('source',{...sourceQuery});if(seq===sourceSeq&&!inactive&&open.value){sources.value=v.rows;sourceTotal.value=v.total}}catch(e){if(seq===sourceSeq)readFailure(e,'已发货订单读取失败')}finally{if(seq===sourceSeq)opening.value=false}}
async function previewSource(id){if(opening.value||busy.value||!safeID(id))return;const seq=++detailSeq;opening.value=true;form.value={};try{const v=await salesFinanceGet('source',id);if(seq===detailSeq&&!inactive&&open.value){form.value={...v.data,requestKey:crypto.randomUUID()};mode.value='source'}}catch(e){if(seq===detailSeq)readFailure(e,'原批读取失败')}finally{if(seq===detailSeq)opening.value=false}}
async function show(row,action){if(busy.value||opening.value||closing.value||!can('erp:'+permissionKey+':query'))return;const id=row[{account:'accountId',receivable:'receivableId',collection:'collectionId'}[kind]];if(!safeID(id))return;resetForm();const seq=++detailSeq;opening.value=true;open.value=false;error.value='';try{const v=await salesFinanceGet(kind,id);if(seq!==detailSeq||inactive)return;form.value={...v.data,requestKey:crypto.randomUUID(),reason:''};mode.value=kind==='account'?(action==='edit'?'accountEdit':'accountDetail'):kind==='collection'?(action==='reverse'?'collectionReverse':'collectionDetail'):'receivableDetail';open.value=true;if(mode.value==='receivableDetail'&&can('erp:receivable:adjust')){returnQuery.pageNum=1;pendingOnly.value=true;await loadReturns()}}catch(e){if(seq===detailSeq)readFailure(e,'财务详情读取失败')}finally{if(seq===detailSeq)opening.value=false}}
async function loadReturns(){if(!form.value.receivableId||pending.value)return;const seq=++returnSeq;returns.value=[];returnSelection.value=[];returnTotal.value=0;try{const v=await salesFinanceReturns(form.value.receivableId,{...returnQuery,pending:pendingOnly.value});if(seq===returnSeq&&!inactive&&open.value){returns.value=v.rows;returnTotal.value=v.total}}catch(e){if(seq===returnSeq)readFailure(e,'退货读取失败')}}
function startCollection(){if(!can('erp:collection:add')||!selected.value.length)return;const chosen=selected.value.map(x=>({...x})),first=chosen[0];if(chosen.some(x=>x.shopId!==first.shopId||x.currency!==first.currency||x.pendingReturnCount>0||x.dueAmount==='0.0000')){error.value='请选择同店铺、同币种、无待登记退货且仍有待收款的应收';return};if(!can('erp:financeAccount:list')){error.value='页面选择收款账户还需资金账户列表权限，请由企业管理员配置';return};if(!start('collectionCreate'))return;Object.assign(form.value,{shopId:first.shopId,shopName:first.shopName,currency:first.currency,remark:'',allocations:chosen.map(x=>({receivableId:x.receivableId,receivableNo:x.receivableNo,version:x.version,dueAmount:x.dueAmount,amount:x.dueAmount}))});loadAccounts('')}
async function loadAccounts(code){const seq=++accountSeq;accounts.value=[];try{const v=await salesFinanceList('account',{pageNum:1,pageSize:100,status:'0',currency:form.value.currency,code});if(seq===accountSeq&&!inactive&&open.value)accounts.value=v.rows}catch(e){if(seq===accountSeq)readFailure(e,'账户读取失败')}}
function readFailure(e,fallback){++listSeq;rows.value=[];selected.value=[];total.value=0;listError.value=true;error.value=e.message||fallback;close()}
function command(){const f=form.value;if(mode.value==='accountCreate'||mode.value==='accountEdit')return{action:mode.value,id:f.accountId,payload:financeAccountPayload(f,mode.value==='accountEdit')};if(mode.value==='source')return{action:'receivableCreate',payload:salesFinanceSourcePayload(f)};if(mode.value==='receivableDetail')return{action:'adjust',id:f.receivableId,payload:salesFinanceAdjustPayload(f,returnSelection.value)};if(mode.value==='collectionCreate')return{action:'collectionCreate',payload:salesFinanceCollectionPayload(f)};if(mode.value==='collectionReverse')return{action:'reverse',id:f.collectionId,payload:salesFinanceReversePayload(f)};throw Error('此页面不能提交')}
async function submit(){if(busy.value||!open.value)return;const view=viewSeq;busy.value=true;error.value='';try{if(!pending.value)pending.value=immutablePayload(command());const v=await salesFinanceWrite(pending.value);if(view!==viewSeq||inactive)return;lastResult.value=v.data;open.value=false;closed();ElMessage.success('财务记账操作已确认');await load()}catch(e){if(view===viewSeq&&!inactive){if(pending.value){++listSeq;rows.value=[];selected.value=[];total.value=0;listError.value=true;form.value={};returns.value=[];sources.value=[];accounts.value=[]};error.value=pending.value?'结果未确认，原请求已锁定；价格明细已隐藏。请原请求重试，或关闭后先查记录。':e.message||'请核对财务参数'}}finally{busy.value=false}}
function closed(){if(open.value)return;closing.value=true;resetForm();opening.value=false}
function close(){if(busy.value)return;const wasOpen=open.value;open.value=false;closed();if(!wasOpen)closing.value=false}
function beforeClose(done){if(busy.value)return;close();done()}
function clear(){inactive=true;++shopSeq;shopOptions.value=[];++listSeq;++viewSeq;open.value=false;resetForm();rows.value=[];selected.value=[];total.value=0;lastResult.value=null;error.value='';opening.value=false;loading.value=false;closing.value=false}
function exportRows(){return download(kind==='statement'?'/erp/salesFinance/statements/export':'/erp/salesFinance/receivables/export',{...query},'销售财务登记账.xlsx',{headers:{repeatSubmit:false}})}
function applyCurrencyLink(){if(kind!=='receivable')return true;const raw=route.query.currency;if(raw!==undefined&&(typeof raw!=='string'||!/^[A-Z]{3}$/.test(raw))){++listSeq;rows.value=[];total.value=0;listError.value=true;return false}query.currency=raw;query.pageNum=1;return true}
watch(()=>route.query.currency,()=>{if(!inactive&&applyCurrencyLink())load()})
function activate(){inactive=false;closing.value=false;if(applyCurrencyLink())load()}
onBeforeUnmount(clear);onDeactivated(clear);onActivated(()=>{if(inactive)activate()});activate()
</script>
<style scoped>.mb8{margin-bottom:8px}.mt8{margin-top:8px}h4{margin:16px 0 8px}</style>
