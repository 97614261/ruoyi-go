<template>
 <div class="app-container">
  <header class="packagebar"><h2>包裹发货</h2><el-button size="small" @click="router.push('/erp/order')" v-hasPermi="['erp:order:list']">返回订单工作台</el-button><span>按订单定位包裹；查看收件资料仍须订单详情权限。</span></header>
  <el-alert title="一期一个订单对应一个仓库、一个包裹。填写物流与运单后手工确认发货，表示本地库存出库，不代表平台回传、揽收或妥投。取消请从订单页操作。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search"><el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" style="width:200px"/></el-form-item><el-form-item label="订单ID"><el-input v-model="orderText" placeholder="正整数，可选" style="width:140px"/></el-form-item><el-form-item label="状态"><el-select v-model="query.status" clearable style="width:130px"><el-option v-for="(label,key) in statuses" :key="key" :label="label" :value="key"/></el-select></el-form-item><el-form-item><el-button @click="search">查询 / 刷新</el-button><el-button @click="reset">重置</el-button></el-form-item></el-form>
  <div class="packagebar"><span>本页已选 {{selected.length}} 条（最多20）</span><el-button size="small" @click="batchRef.show(selected,'ship')" :disabled="busy||!selected.length" v-hasPermi="['erp:package:ship']">批量确认发货</el-button></div>
  <el-table :data="rows" row-key="packageId" size="small" v-loading="loading" @selection-change="value=>selected=value"><el-table-column type="selection" width="42"/><el-table-column prop="orderId" label="订单ID" width="95"/><el-table-column label="状态" width="100"><template #default="{row}">{{statuses[row.status]||row.status}}</template></el-table-column><el-table-column prop="packageNo" label="包裹号" min-width="235" show-overflow-tooltip/><el-table-column prop="shopId" label="店铺ID" width="90"/><el-table-column prop="warehouseId" label="仓库ID" width="90"/><el-table-column prop="shippedAt" label="发货时间" width="170"/><el-table-column label="操作" width="295" fixed="right"><template #default="{row}"><el-button link type="primary" @click="show(row,false)" :disabled="busy" v-hasPermi="['erp:package:query']">详情</el-button><el-button link type="primary" @click="goOrder(row)" v-if="checkPermi(['erp:order:query'])&&checkPermi(['erp:order:list'])">订单</el-button><el-button v-if="row.status==='READY_TO_SHIP'&&checkPermi(['erp:package:edit'])&&checkPermi(['erp:package:query'])" link type="primary" @click="show(row,true)" :disabled="busy">填运单</el-button><el-button v-if="row.status==='READY_TO_SHIP'" link type="warning" @click="ship(row)" :disabled="busy" v-hasPermi="['erp:package:ship']">确认发货</el-button></template></el-table-column></el-table>
  <p v-if="!loading&&!rows.length" class="empty-hint">当前范围没有包裹。订单审核成功后才会创建包裹；请核对店铺和仓库范围。</p>
  <FulfillmentBatchDialog ref="batchRef" @completed="load"/>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="editing?'保存包裹物流':'包裹详情'" class="erp-dialog" width="min(900px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving" @close="closed">
   <div v-if="detail"><p>{{detail.packageNo}} · 订单 #{{detail.orderId}} · {{statuses[detail.status]}} · 包裹 v{{detail.version}}</p><el-alert title="本页不提供订单收件资料。查看收件人/地址须另有订单详情权限和店铺范围。修改物流也会推进订单版本，取消前请刷新订单。" type="info" :closable="false"/>
    <el-form label-width="105px" size="small" class="mt8" :disabled="saving||!editing"><el-form-item label="物流代码"><el-input v-model="form.carrierCode" maxlength="64" placeholder="例如 MANUAL / SF / EMS（仅手工标签）"/></el-form-item><el-form-item label="运单号"><el-input v-model="form.trackingNo" maxlength="128" placeholder="文本保留前导零"/></el-form-item></el-form>
    <el-table :data="detail.lines" size="small"><el-table-column prop="orderLineId" label="订单行" width="90"/><el-table-column prop="skuCode" label="内部SKU" width="160"/><el-table-column prop="productName" label="商品快照"/><el-table-column prop="unit" label="单位" width="80"/><el-table-column prop="quantity" label="数量" width="100"/></el-table>
   </div><template #footer><el-button v-if="detail&&checkPermi(['erp:order:list'])&&checkPermi(['erp:order:query'])" @click="goOrder(detail)" :disabled="saving">查看订单详情</el-button><el-button @click="open=false" :disabled="saving">关闭</el-button><el-button v-if="editing" type="primary" @click="save" :loading="saving">保存物流信息</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted,watch,onBeforeUnmount } from 'vue'
import { useRouter,useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { checkPermi } from '@/utils/permission'
import ReferenceSelect from './ReferenceSelect.vue'
import FulfillmentBatchDialog from './FulfillmentBatchDialog.vue'
import { listPackages, getPackage, savePackageLogistics, shipPackage } from '@/api/erp/order'
const rows=ref([]),total=ref(0),loading=ref(false),saving=ref(false),detailLoading=ref(false),open=ref(false),editing=ref(false),detail=ref(null),form=ref({}),orderText=ref('')
const selected=ref([]),batchRef=ref(),router=useRouter(),route=useRoute()
const busy=computed(()=>saving.value||detailLoading.value),query=reactive({pageNum:1,pageSize:10,warehouseId:undefined,orderId:undefined,status:''}),statuses={READY_TO_SHIP:'待发货',SHIPPED:'已发货',CANCELLED:'已取消'}
let listSequence=0,detailSequence=0,linkSequence=0
async function load(){const ticket=++listSequence;loading.value=true;try{const r=await listPackages({...query});if(ticket===listSequence){rows.value=r.rows;total.value=r.total;selected.value=[]}}finally{if(ticket===listSequence)loading.value=false}}
function search(){++linkSequence;if(orderText.value&&(!/^[1-9]\d*$/.test(orderText.value)||!Number.isSafeInteger(Number(orderText.value)))){ElMessage.warning('订单ID须为可精确表示的正整数');return}query.orderId=orderText.value?Number(orderText.value):undefined;query.pageNum=1;return load()}
function reset(){orderText.value='';Object.assign(query,{warehouseId:undefined,orderId:undefined,status:''});return search()}
async function show(row,edit){if(busy.value)return;const ticket=++detailSequence;detailLoading.value=true;try{const r=await getPackage(row.packageId);if(ticket!==detailSequence)return;if(row.orderId&&r.data.orderId!==row.orderId){ElMessage.warning('包裹与链接订单不一致');return}detail.value=r.data;editing.value=edit&&r.data.status==='READY_TO_SHIP';form.value={carrierCode:r.data.carrierCode,trackingNo:r.data.trackingNo,requestKey:crypto.randomUUID()};open.value=true}finally{if(ticket===detailSequence)detailLoading.value=false}}
function goOrder(row){if(checkPermi(['erp:order:list'])&&checkPermi(['erp:order:query']))return router.push({path:'/erp/order',query:{orderId:String(row.orderId)}})}
function safeID(raw){return typeof raw==='string'&&/^[1-9]\d*$/.test(raw)&&Number.isSafeInteger(Number(raw))?Number(raw):null}
async function followPackageLink(){if(saving.value)return;const ticket=++linkSequence;++detailSequence;detailLoading.value=false;open.value=false;detail.value=null;form.value={};const order=route.query.orderId,pack=route.query.packageId;if((order!==undefined&&!safeID(order))||(pack!==undefined&&!safeID(pack))){++listSequence;rows.value=[];total.value=0;ElMessage.warning('包裹链接编号无效');return}orderText.value=order||'';query.orderId=order?safeID(order):undefined;query.pageNum=1;try{await load();if(ticket!==linkSequence||!pack)return;if(!checkPermi(['erp:package:query'])){ElMessage.warning('查看包裹需要包裹详情权限');return}await show({packageId:safeID(pack),orderId:query.orderId},false)}catch{}}
watch(()=>[route.query.orderId,route.query.packageId],followPackageLink)
onBeforeUnmount(()=>{++listSequence;++detailSequence;++linkSequence;detail.value=null;form.value={}})
function closed(){if(open.value)return;++detailSequence;detailLoading.value=false;detail.value=null;form.value={}}
async function save(){if(saving.value||!open.value||!editing.value||!detail.value)return;if(!form.value.carrierCode||!form.value.trackingNo){ElMessage.warning('请填写物流代码和运单号');return}const id=detail.value.packageId,payload={version:detail.value.version,requestKey:form.value.requestKey,carrierCode:form.value.carrierCode,trackingNo:form.value.trackingNo};saving.value=true;try{await savePackageLogistics(id,payload);open.value=false;detail.value=null;form.value={};ElMessage.success('物流已保存，尚未发货');await load()}finally{saving.value=false}}
async function ship(row){if(busy.value)return;const id=row.packageId,payload={version:row.version,requestKey:crypto.randomUUID()};saving.value=true;try{try{await ElMessageBox.confirm('确认包裹已交付出库？成功后原子扣减实物和预占库存，订单不可取消。这里只登记本地发货，不会回传平台。','确认发货',{type:'warning'})}catch{return}await shipPackage(id,payload);ElMessage.success('本地发货出库已完成');await load()}finally{saving.value=false}}
onMounted(followPackageLink)
</script>
<style scoped>
.packagebar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:12px}.packagebar h2{font-size:18px;margin:0 10px 0 0}.packagebar span,.empty-hint{font-size:12px;color:var(--el-text-color-secondary)} :deep(.el-dialog__body){max-height:calc(100dvh - 200px);overflow:auto} :deep(.el-table .cell){line-height:22px}
</style>
