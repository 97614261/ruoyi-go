<template>
 <div class="app-container">
  <el-alert title="一行代表一个店铺可售变种，可配对一个内部SKU。这里只维护商品配对，不会拉取平台商品或产生订单、库存。停用和解绑保留历史。" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <el-form-item label="店铺"><el-select v-model="query.shopId" clearable filterable remote :remote-method="findShops" @visible-change="v=>v&&findShops('')" placeholder="搜索已授权店铺" style="width:190px"><el-option v-for="s in shops" :key="s.shopId" :value="s.shopId" :label="s.shopCode+' · '+s.shopName"/></el-select></el-form-item>
   <el-form-item label="店铺SKU"><el-input v-model="query.shopItemCode" clearable maxlength="128" @keyup.enter="search"/></el-form-item>
   <el-form-item label="标题"><el-input v-model="query.title" clearable maxlength="200" @keyup.enter="search"/></el-form-item>
   <el-form-item label="配对"><el-select v-model="query.mappingState" clearable style="width:110px"><el-option label="已配对" value="MAPPED"/><el-option label="未配对" value="UNMAPPED"/></el-select></el-form-item>
   <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:100px"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
   <el-form-item><el-button @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item>
  </el-form>
  <el-button type="primary" size="small" class="mb8" @click="start" :disabled="busy" v-hasPermi="['erp:shopItem:add']">新增店铺商品</el-button>
  <el-table :data="rows" size="small" v-loading="loading">
   <el-table-column label="店铺" min-width="130" show-overflow-tooltip><template #default="{row}">{{row.shopCode}} · {{row.shopName}}<el-tag v-if="row.shopStatus==='1'" type="info" size="small">店铺停用</el-tag></template></el-table-column>
   <el-table-column label="店铺SKU" prop="shopItemCode" min-width="130" show-overflow-tooltip/>
   <el-table-column label="商品标题" prop="title" min-width="170" show-overflow-tooltip/>
   <el-table-column label="内部SKU" min-width="150" show-overflow-tooltip><template #default="{row}"><span v-if="row.mappingStatus==='0'">{{row.skuCode}} · {{row.productName}} <el-tag v-if="row.skuStatus==='1'" type="danger" size="small">SKU停用</el-tag></span><span v-else>未配对{{row.mappingId?'（已解绑）':''}}</span></template></el-table-column>
   <el-table-column label="状态" width="80"><template #default="{row}">{{row.status==='0'?'启用':'停用'}}</template></el-table-column>
   <el-table-column label="版本" prop="version" width="65"/>
   <el-table-column label="操作" width="240"><template #default="{row}">
    <el-button link type="primary" @click="openItem(row,true)" :disabled="busy" v-hasPermi="['erp:shopItem:query']">详情</el-button>
    <el-button link type="primary" @click="openItem(row,false)" :disabled="busy||row.shopStatus!=='0'" v-hasPermi="['erp:shopItem:edit']">修改</el-button>
    <el-button link type="primary" @click="openMapping(row)" :disabled="busy||row.shopStatus!=='0'||row.status!=='0'" v-hasPermi="['erp:shopItem:map']">配对</el-button>
    <el-button v-if="row.mappingStatus==='0'" link type="warning" @click="unmap(row)" :disabled="busy" v-hasPermi="['erp:shopItem:map']">解绑</el-button>
   </template></el-table-column>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="readOnly?'店铺商品详情':form.itemId?'修改店铺商品':'新增店铺商品'" class="erp-dialog" width="min(660px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving" @close="closeItem">
   <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" size="small" :disabled="saving||readOnly||!!itemPending">
    <el-form-item label="店铺" prop="shopId"><el-select v-model="form.shopId" :disabled="!!form.itemId" filterable remote :remote-method="findShops" @visible-change="v=>v&&findShops('')" style="width:100%"><el-option v-if="form.itemId" :key="form.shopId" :value="form.shopId" :label="form.shopCode+' · '+form.shopName"/><template v-else><el-option v-for="s in shops" :key="s.shopId" :value="s.shopId" :label="s.shopCode+' · '+s.shopName" :disabled="s.status!=='0'"/></template></el-select></el-form-item>
    <el-form-item label="店铺SKU编码" prop="shopItemCode"><el-input v-model="form.shopItemCode" maxlength="128" placeholder="作为文本保留前导零，不含首尾空白"/></el-form-item>
    <el-form-item label="外部商品ID"><el-input v-model="form.externalProductId" maxlength="128" placeholder="可选，未接平台API"/></el-form-item>
    <el-form-item label="外部变种ID"><el-input v-model="form.externalVariantId" maxlength="128" placeholder="可选，同店铺内唯一"/></el-form-item>
    <el-form-item label="商品标题" prop="title"><el-input v-model="form.title" maxlength="200"/></el-form-item>
    <el-form-item label="状态" prop="status"><el-radio-group v-model="form.status"><el-radio value="0">启用</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item>
    <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" maxlength="500" show-word-limit/></el-form-item>
   </el-form>
   <el-alert v-if="form.itemId" :title="`商品版本 v${form.version}。有配对历史或订单引用后，SKU编码和外部标识不可改；不支持转移店铺或删除。`" type="info" :closable="false"/>
   <template #footer><el-button @click="open=false" :disabled="saving">关闭</el-button><el-button v-if="!readOnly" type="primary" @click="saveItem" :loading="saving">保存</el-button></template>
  </el-dialog>
  <el-alert v-if="mappingPending||itemPending" title="上次结果未确认，重试固定原目标/内容/版本；请先查记录，版本冲突不能视为未提交。" type="warning" :closable="false"/><el-dialog v-model="mappingOpen" title="配对内部 SKU" class="erp-dialog" width="min(600px, calc(100vw - 24px))" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving" @close="closeMapping">
   <el-alert title="商品版本控制编辑、配对和解绑。版本冲突请关闭后重新打开；未结束订单引用会阻止改配对，已解绑行不物理删除。" type="warning" :closable="false"/>
   <p>{{mapping.item?.shopName}} / {{mapping.item?.shopItemCode}} · 商品版本 v{{mapping.item?.version}}，配对版本 v{{mapping.item?.mappingVersion||'—'}}</p>
   <el-form label-width="100px" size="small" :disabled="saving">
    <el-form-item label="内部SKU"><el-select v-model="mapping.skuId" filterable remote :remote-method="findSkus" @visible-change="v=>v&&findSkus('')" placeholder="搜索内部SKU商品名称（最多100条）" style="width:100%"><el-option v-for="s in skus" :key="s.skuId" :value="s.skuId" :label="s.skuCode+' · '+s.productName+' / '+s.unit" :disabled="s.status!=='0'"/></el-select></el-form-item>
    <el-form-item label="配对备注"><el-input v-model="mapping.remark" maxlength="500" type="textarea"/></el-form-item>
   </el-form>
   <template #footer><el-button @click="mappingOpen=false" :disabled="saving">取消</el-button><el-button type="primary" @click="saveMapping" :loading="saving">保存配对</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted,watch,onBeforeUnmount,onDeactivated,onActivated } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { checkPermi } from '@/utils/permission'
import { listErpShop } from '@/api/erp/shop'
import { listErpSku } from '@/api/erp/sku'
import { listShopItems, getShopItem, addShopItem, editShopItem, setShopItemMapping, cancelShopItemMapping } from '@/api/erp/shopItem'
const rows=ref([]),total=ref(0),loading=ref(false),detailLoading=ref(false),saving=ref(false),open=ref(false),mappingOpen=ref(false),readOnly=ref(false),form=ref({}),formRef=ref(),shops=ref([]),skus=ref([]),mapping=ref({})
const busy=computed(()=>saving.value||detailLoading.value)
const query=reactive({pageNum:1,pageSize:10,shopId:undefined,shopItemCode:'',title:'',status:'',mappingState:''})
const route=useRoute()
const rules={shopId:[{required:true,message:'请选择店铺',trigger:'change'}],shopItemCode:[{required:true,message:'请输入店铺SKU编码',trigger:'blur'}],title:[{required:true,message:'请输入商品标题',trigger:'blur'}],status:[{required:true,message:'请选择状态',trigger:'change'}]}
const mappingPending=ref(null),itemPending=ref(null)
let listSequence=0,detailSequence=0,shopSequence=0,skuSequence=0,view=0,operation=0,alive=true,wasInactive=false
async function load(){if(!alive)return;const ticket=++listSequence;loading.value=true;try{const r=await listShopItems({...query});if(alive&&ticket===listSequence){rows.value=r.rows;total.value=r.total}}catch(e){if(alive&&ticket===listSequence){rows.value=[];total.value=0;throw e}}finally{if(ticket===listSequence)loading.value=false}}
function search(){query.pageNum=1;return load()}
function reset(){Object.assign(query,{shopId:undefined,shopItemCode:'',title:'',status:'',mappingState:''});return search()}
async function findShops(shopName){if(!alive)return;if(!checkPermi(['erp:shop:list'])){ElMessage.warning('店铺选择器需要店铺列表权限');return}const ticket=++shopSequence;const r=await listErpShop({shopName,pageNum:1,pageSize:100});if(ticket===shopSequence)shops.value=r.rows}
async function findSkus(productName){if(!alive)return;if(!checkPermi(['erp:sku:list'])){ElMessage.warning('SKU选择器需要SKU列表权限');return}const ticket=++skuSequence;const r=await listErpSku({productName,pageNum:1,pageSize:100});if(ticket===skuSequence&&mappingOpen.value){const current=mapping.value.item;skus.value=r.rows;if(current?.skuId&&!skus.value.some(s=>s.skuId===current.skuId))skus.value.unshift({skuId:current.skuId,skuCode:current.skuCode,productName:current.productName,unit:current.unit,status:current.skuStatus})}}
function start(){if(!alive||busy.value||itemPending.value&&open.value)return;itemPending.value=null;if(busy.value)return;++detailSequence;form.value={shopId:query.shopId,shopItemCode:'',externalProductId:'',externalVariantId:'',title:'',status:'0',remark:''};readOnly.value=false;open.value=true;formRef.value?.clearValidate()}
async function fetchDetail(row,apply){if(!alive)return;if(busy.value)return;const ticket=++detailSequence;detailLoading.value=true;try{const r=await getShopItem(row.itemId);if(ticket===detailSequence)apply(r.data)}finally{if(ticket===detailSequence)detailLoading.value=false}}
function openItem(row,readonly){return fetchDetail(row,d=>{form.value={...d};readOnly.value=readonly;open.value=true;formRef.value?.clearValidate()})}
function openMapping(row){return fetchDetail(row,d=>{++skuSequence;skus.value=d.skuId?[{skuId:d.skuId,skuCode:d.skuCode,productName:d.productName,unit:d.unit,status:d.skuStatus}]:[];mapping.value={item:d,skuId:d.mappingStatus==='0'?d.skuId:undefined,remark:''};mappingOpen.value=true})}
function closeItem(){if(open.value)return;itemPending.value=null;form.value={};++detailSequence;detailLoading.value=false;formRef.value?.clearValidate()}
function closeMapping(){if(mappingOpen.value)return;mappingPending.value=null;mapping.value={};++detailSequence;++skuSequence;detailLoading.value=false}
async function saveItem(){if(!alive||saving.value||readOnly.value||!open.value)return;const owner=view,op=++operation;saving.value=true;try{let c=itemPending.value;if(!c){await formRef.value.validate();if(!alive||owner!==view||op!==operation||!open.value)return;c=JSON.parse(JSON.stringify({id:form.value.itemId,payload:Object.fromEntries(['shopId','shopItemCode','externalProductId','externalVariantId','title','status','version','remark'].map(k=>[k,form.value[k]]))}));itemPending.value=c};if(c.id)await editShopItem(c.id,c.payload);else await addShopItem(c.payload);if(!alive||owner!==view||op!==operation)return;open.value=false;itemPending.value=null;ElMessage.success('店铺商品已保存');await load()}finally{if(op===operation)saving.value=false}}
async function saveMapping(){if(!alive||saving.value||!mappingOpen.value)return;if(!mappingPending.value&&!mapping.value.skuId){ElMessage.warning('请选择内部SKU');return}const c=mappingPending.value||JSON.parse(JSON.stringify({id:mapping.value.item.itemId,payload:{version:mapping.value.item.version,skuId:mapping.value.skuId,remark:mapping.value.remark||''}})),owner=view,op=++operation;saving.value=true;try{try{await ElMessageBox.confirm('确认替换该商品的内部SKU配对？历史引用不会被物理删除。','确认配对',{type:'warning'})}catch{return};if(!alive||owner!==view||op!==operation)return;mappingPending.value=c;await setShopItemMapping(c.id,c.payload);if(!alive||owner!==view||op!==operation)return;mappingOpen.value=false;mappingPending.value=null;ElMessage.success('配对已保存');await load()}finally{if(op===operation)saving.value=false}}
async function unmap(row){if(!alive||busy.value)return;const id=row.itemId,version=row.version,owner=view,op=++operation;saving.value=true;try{try{await ElMessageBox.confirm('确认解绑该商品？配对历史保留，未结束订单引用时会拒绝。','确认解绑',{type:'warning'})}catch{return};if(!alive||owner!==view||op!==operation)return;await cancelShopItemMapping(id,version);if(!alive||owner!==view||op!==operation)return;ElMessage.success('配对已停用');await load()}finally{if(op===operation)saving.value=false}}
function followShopLink(){if(!alive)return;if(saving.value)return;++detailSequence;detailLoading.value=false;open.value=false;mappingOpen.value=false;const raw=route.query.shopId;if(raw!==undefined&&(typeof raw!=='string'||!/^[1-9]\d*$/.test(raw)||!Number.isSafeInteger(Number(raw)))){++listSequence;rows.value=[];total.value=0;ElMessage.warning('店铺链接编号无效');return}query.shopId=raw?Number(raw):undefined;query.pageNum=1;return load()}
watch(()=>route.query.shopId,followShopLink)

onMounted(followShopLink)
function leave(){mappingPending.value=null;itemPending.value=null;alive=false;++view;++operation;++listSequence;++detailSequence;++shopSequence;++skuSequence;saving.value=false;loading.value=false;detailLoading.value=false;open.value=false;mappingOpen.value=false;rows.value=[];total.value=0;form.value={};mapping.value={};shops.value=[];skus.value=[]}
onBeforeUnmount(leave);onDeactivated(()=>{wasInactive=true;leave()});onActivated(()=>{if(wasInactive){wasInactive=false;alive=true;return followShopLink()}})
</script>
