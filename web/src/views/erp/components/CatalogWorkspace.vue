<template>
 <div class="app-container catalog-workspace">
  <el-alert :title="category?'平级商品分类；编码建立后保留，停用不删除历史。':'商品档案：一个商品管理多个SKU。库存与订单沿用原SKU；已有引用的编码、规格和单位受保护。'" type="info" :closable="false" class="mb8"/>
  <el-form inline size="small" @submit.prevent="search">
   <template v-if="category"><el-form-item label="分类编码"><el-input v-model="query.categoryCode" clearable/></el-form-item><el-form-item label="分类名称"><el-input v-model="query.categoryName" clearable/></el-form-item></template>
   <template v-else><el-form-item label="商品编码"><el-input v-model="query.productCode" clearable/></el-form-item><el-form-item label="商品名称"><el-input v-model="query.productName" clearable/></el-form-item><el-form-item label="SKU编码"><el-input v-model="query.skuCode" clearable/></el-form-item><el-form-item label="条码"><el-input v-model="query.barcode" clearable/></el-form-item></template>
   <el-form-item><el-button @click="search">查询</el-button><el-button v-if="can('add')" type="primary" :disabled="busy||opening" @click="start">新增{{category?'分类':'商品'}}</el-button><template v-if="!category"><el-button v-if="can('import')" :disabled="busy" @click="startImport">批量导入</el-button><el-button v-if="can('export')" @click="exportRows">导出筛选结果</el-button></template></el-form-item>
  </el-form>
  <el-alert v-if="error&&!open&&!importOpen" :title="error" type="error" :closable="false"/>
  <el-table :data="rows" size="small" v-loading="loading" :row-key="category?'categoryId':'productId'">
   <template v-if="category"><el-table-column prop="categoryCode" label="分类编码" min-width="150"/><el-table-column prop="categoryName" label="名称" min-width="170"/><el-table-column label="状态" width="90"><template #default="{row}">{{row.status==='0'?'启用':'停用'}}</template></el-table-column></template>
   <template v-else><el-table-column label="图片" width="68"><template #default="{row}"><img v-if="row.images?.length" :src="imageURL(row.images[0])" class="thumbnail" alt="商品图"/><span v-else>—</span></template></el-table-column><el-table-column prop="productCode" label="商品编码" min-width="150"/><el-table-column prop="productName" label="商品名称" min-width="200"/><el-table-column prop="categoryName" label="分类" min-width="130"/></template>
   <el-table-column prop="version" label="版本" width="70"/><el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip/>
   <el-table-column label="操作" width="150" fixed="right"><template #default="{row}"><el-button v-if="!category&&can('query')" link @click="show(row,false)">详情</el-button><el-button v-if="can('edit')&&(category||can('query'))" link type="primary" @click="show(row,true)">编辑</el-button></template></el-table-column>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
  <el-dialog v-model="open" :title="(editable?(form[category?'categoryId':'productId']?'编辑':'新增'):'查看')+(category?'分类':'商品档案')" width="min(1100px, calc(100vw - 24px))" append-to-body :close-on-click-modal="false" :before-close="closeDialog" class="erp-dialog">
   <el-alert v-if="error" :title="error" type="error" :closable="false" class="mb8"/>
   <el-alert v-if="pending" title="已锁定原请求。可重试确认结果；不要改内容后复用同一请求。" type="warning" :closable="false" class="mb8"/>
   <el-form :disabled="!editable||busy||!!pending" label-width="85px" size="small">
    <div class="catalog-head">
     <el-form-item :label="category?'分类编码':'商品编码'"><el-input v-if="category" v-model="form.categoryCode" maxlength="64" :disabled="!!form.categoryId"/><el-input v-else v-model="form.productCode" maxlength="64" :disabled="!!form.productId"/></el-form-item>
     <el-form-item label="名称"><el-input v-if="category" v-model="form.categoryName" maxlength="120"/><el-input v-else v-model="form.productName" maxlength="120"/></el-form-item>
     <el-form-item v-if="category" label="状态"><el-select v-model="form.status"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></el-form-item>
     <el-form-item v-else label="分类"><el-select v-model="form.categoryId" clearable filterable remote :remote-method="findCategories" :loading="categoryLoading" @visible-change="v=>v&&findCategories('')" placeholder="搜索分类名称（可留空）"><el-option v-for="c in categories" :key="c.categoryId" :value="c.categoryId" :label="c.categoryName"/></el-select></el-form-item>
    </div>
    <el-form-item label="备注"><el-input v-model="form.remark" maxlength="500"/></el-form-item>
    <template v-if="!category">
     <div class="catalog-toolbar"><strong>SKU规格（{{form.variants.length}} / 50）</strong><el-button v-if="editable" size="small" :disabled="form.variants.length>=50||busy||!!pending" @click="form.variants.push(blankVariant())">添加新SKU</el-button><el-select v-if="editable&&hasPermi(['erp:sku:list'])" filterable remote clearable :remote-method="findUnassigned" :loading="skuLoading" placeholder="搜索并归入已有SKU" @visible-change="v=>v&&findUnassigned('')" @change="attachSKU" :model-value="null" style="width:220px"><el-option v-for="s in skuOptions" :key="s.skuId" :value="s.skuId" :label="s.skuCode+' · '+s.productName"/></el-select></div>
     <p class="catalog-note">已有SKU不可移除；已被业务引用的编码、规格、单位不能变。条码按文本保存，未知重量/尺寸留空。</p>
     <div class="catalog-scroll"><el-table :data="form.variants" size="small" style="min-width:970px">
      <el-table-column label="SKU编码" min-width="150"><template #default="{row}"><el-input v-model="row.skuCode" :disabled="!editable||busy||!!pending||row.referenced" maxlength="64"/></template></el-table-column>
      <el-table-column label="规格" min-width="145"><template #default="{row}"><el-input v-model="row.specification" :disabled="!editable||busy||!!pending||row.referenced" maxlength="120"/></template></el-table-column>
      <el-table-column label="单位" width="90"><template #default="{row}"><el-input v-model="row.unit" :disabled="!editable||busy||!!pending||row.referenced" maxlength="20"/></template></el-table-column>
      <el-table-column label="条码" min-width="145"><template #default="{row}"><el-input v-model="row.barcode" maxlength="64" placeholder="文本，可空"/></template></el-table-column>
      <el-table-column label="重量/克" width="105"><template #default="{row}"><el-input v-model="row.weightGrams" inputmode="numeric"/></template></el-table-column>
      <el-table-column v-for="d in dimensions" :key="d.key" :label="d.name+'/毫米'" width="105"><template #default="{row}"><el-input v-model="row[d.key]" inputmode="numeric"/></template></el-table-column>
      <el-table-column label="状态" width="100"><template #default="{row}"><el-select v-model="row.status"><el-option label="启用" value="0"/><el-option label="停用" value="1"/></el-select></template></el-table-column>
      <el-table-column label="保留" width="75"><template #default="{row,$index}"><el-button v-if="!row.skuId&&editable" link type="danger" @click="form.variants.splice($index,1)">移除</el-button><span v-else>{{row.referenced?'已引用':'已建'}}</span></template></el-table-column>
     </el-table></div>
     <div class="catalog-images"><div v-for="(key,i) in form.images" :key="key" class="catalog-image"><img :src="imageURL(key)" alt="商品图片"/><el-button v-if="editable" link type="danger" @click="form.images.splice(i,1)">移除关联</el-button></div><label v-if="editable&&can('image')&&form.images.length<5" class="catalog-upload">＋ 添加图片<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" :disabled="busy||!!pending" @change="uploadImage"/></label></div>
     <p class="catalog-note">最多5张，每张8MB、2000万像素。图片沿用企业共享附件权限；移除关联不删除附件。</p>
    </template>
   </el-form>
   <template #footer><el-button :disabled="busy" @click="closeDialog()">关闭</el-button><el-button v-if="editable" type="primary" :loading="busy" @click="submit">{{pending?'重试原请求':'保存'}}</el-button></template>
  </el-dialog>
  <el-dialog v-model="importOpen" title="批量新增商品" width="min(640px, calc(100vw - 24px))" append-to-body :close-on-click-modal="false" :before-close="closeImport" class="erp-dialog">
   <el-alert title="只新增，不覆盖已有商品。每行一个SKU，同商品编码合并；最多100行，整批成功或回滚。编码/条码使用文本格式保留前导零；图片另行上传。" type="info" :closable="false" class="mb8"/>
   <el-alert v-if="error" :title="error" type="error" :closable="false" class="mb8"/>
   <el-button @click="templateFile" :disabled="busy">下载XLSX模板</el-button><input type="file" accept=".xlsx" :disabled="busy||!!importPending" @change="selectImport"/>
   <p>{{importFile?.name||'尚未选择文件'}}</p><p v-if="importPending">原文件和请求已锁定，可重试确认结果。</p>
   <template #footer><el-button :disabled="busy" @click="closeImport()">关闭</el-button><el-button type="primary" :disabled="!importFile" :loading="busy" @click="submitImport">{{importPending?'重试原文件':'整批导入'}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,onBeforeUnmount,onMounted,onActivated,onDeactivated,getCurrentInstance} from 'vue'
import {saveAs} from 'file-saver'
import {checkPermi as hasPermi} from '@/utils/permission'
import {catalogList,catalogGet,catalogWrite,catalogUpload,catalogImport,catalogTemplate} from '@/api/erp/catalog'
import {listErpSku} from '@/api/erp/sku'
import {blankVariant,catalogPayload,catalogCommand,catalogRequestKey,catalogImageURL} from '@/utils/catalog.mjs'
const props=defineProps({category:Boolean}),{proxy}=getCurrentInstance()
const category=props.category,dimensions=[{key:'lengthMm',name:'长'},{key:'widthMm',name:'宽'},{key:'heightMm',name:'高'}]
const rows=ref([]),total=ref(0),loading=ref(false),error=ref(''),open=ref(false),editable=ref(false),busy=ref(false),opening=ref(false),form=ref({variants:[],images:[]}),pending=ref(null),categories=ref([]),categoryLoading=ref(false),skuOptions=ref([]),skuLoading=ref(false)
const importOpen=ref(false),importFile=ref(null),importPending=ref(null),query=reactive({pageNum:1,pageSize:10})
let listSeq=0,detailSeq=0,categorySeq=0,skuSeq=0,alive=true
const can=action=>hasPermi(['erp:'+(category?'productCategory':'product')+':'+action])
const imageURL=key=>catalogImageURL(key,import.meta.env.VITE_APP_BASE_API)
function blank(){return category?{categoryCode:'',categoryName:'',status:'0',remark:'',version:0,requestKey:catalogRequestKey()}:{productCode:'',productName:'',categoryId:null,remark:'',version:0,requestKey:catalogRequestKey(),variants:[blankVariant()],images:[]}}
async function load(){const seq=++listSeq;loading.value=true;try{const r=await catalogList(category,{...query});if(alive&&seq===listSeq){rows.value=r.rows;total.value=r.total}}catch(e){if(alive&&seq===listSeq){rows.value=[];total.value=0;error.value=e.message||'读取失败'}}finally{if(alive&&seq===listSeq)loading.value=false}}
function search(){query.pageNum=1;error.value='';load()}
function start(){if(busy.value||opening.value)return;++detailSeq;error.value='';pending.value=null;form.value=blank();categories.value=[];editable.value=true;open.value=true}
async function show(row,edit){if(busy.value||opening.value)return;const seq=++detailSeq;opening.value=true;open.value=false;form.value={variants:[],images:[]};pending.value=null;error.value='';try{const r=category?{data:row}:await catalogGet(row.productId);if(!alive||seq!==detailSeq)return;form.value=JSON.parse(JSON.stringify(r.data));form.value.requestKey=catalogRequestKey();categories.value=form.value.categoryId?[{categoryId:form.value.categoryId,categoryName:form.value.categoryName}]:[];editable.value=edit;open.value=true}catch(e){if(alive&&seq===detailSeq){error.value=e.message||'读取失败';rows.value=[];total.value=0}}finally{if(alive&&seq===detailSeq)opening.value=false}}
function closeDialog(done){if(busy.value)return;++detailSeq;++categorySeq;++skuSeq;open.value=false;form.value={variants:[],images:[]};pending.value=null;categories.value=[];skuOptions.value=[];if(done)done();load()}
function discardRejectedState(){++listSeq;++detailSeq;++categorySeq;++skuSeq;rows.value=[];total.value=0;open.value=false;importOpen.value=false;form.value={variants:[],images:[]};pending.value=null;importFile.value=null;importPending.value=null;categories.value=[];skuOptions.value=[];loading.value=false;opening.value=false;categoryLoading.value=false;skuLoading.value=false;error.value='本次操作已被服务端拒绝，请重新查询或联系管理员授权'}
async function submit(){if(busy.value)return;const epoch=detailSeq;try{if(!pending.value){const payload=catalogPayload(form.value,category);pending.value=catalogCommand(category,category?form.value.categoryId:form.value.productId,payload)}busy.value=true;const c=pending.value;const r=await catalogWrite(c.category,c.id,c.payload);if(!alive||epoch!==detailSeq)return;proxy.$modal.msgSuccess('已确认保存：'+r.data.rows+'项');busy.value=false;closeDialog()}catch(e){if(alive&&epoch===detailSeq){if(e==='error')discardRejectedState();else error.value=e.message||'保存结果待确认，请重试原请求'}}finally{busy.value=false}}
async function findCategories(keyword){if(!hasPermi(['erp:productCategory:list']))return;const seq=++categorySeq;categoryLoading.value=true;try{const r=await catalogList(true,{pageNum:1,pageSize:100,categoryName:keyword});if(alive&&seq===categorySeq){const selected=categories.value.find(c=>c.categoryId===form.value.categoryId);categories.value=r.rows.filter(c=>c.status==='0');if(selected&&!categories.value.some(c=>c.categoryId===selected.categoryId))categories.value.unshift(selected)}}catch{if(seq===categorySeq)categories.value=[]}finally{if(seq===categorySeq)categoryLoading.value=false}}
async function findUnassigned(keyword){const seq=++skuSeq;skuLoading.value=true;try{const r=await listErpSku({pageNum:1,pageSize:100,productName:keyword});if(alive&&seq===skuSeq)skuOptions.value=r.rows.filter(s=>!form.value.variants.some(v=>v.skuId===s.skuId))}catch{if(seq===skuSeq)skuOptions.value=[]}finally{if(seq===skuSeq)skuLoading.value=false}}
function attachSKU(id){if(busy.value||pending.value||form.value.variants.length>=50)return;const s=skuOptions.value.find(v=>v.skuId===id);if(!s)return;form.value.variants.push({...blankVariant(),...s});skuOptions.value=[]}
async function uploadImage(event){const file=event.target.files?.[0];event.target.value='';if(!file||busy.value||pending.value)return;if(file.size>8*1024*1024){error.value='图片超过8MB';return}const epoch=detailSeq;busy.value=true;try{const r=await catalogUpload(file);if(alive&&epoch===detailSeq)form.value.images.push(r.data.storageKey)}catch(e){if(alive&&epoch===detailSeq){if(e==='error')discardRejectedState();else error.value=e.message||'上传失败'}}finally{busy.value=false}}
function startImport(){error.value='';importFile.value=null;importPending.value=null;importOpen.value=true}
function selectImport(event){const f=event.target.files?.[0];if(!f)return;if(!/\.xlsx$/i.test(f.name)||f.size>10*1024*1024){error.value='请选择不超过10MB的XLSX';event.target.value='';return}importFile.value=f;error.value=''}
function closeImport(done){if(busy.value)return;importOpen.value=false;importFile.value=null;importPending.value=null;if(done)done();load()}
async function submitImport(){if(busy.value||!importFile.value)return;const epoch=detailSeq;importPending.value||=( {file:importFile.value,key:catalogRequestKey()} );busy.value=true;try{const c=importPending.value,r=await catalogImport(c.file,c.key);if(!alive||epoch!==detailSeq)return;proxy.$modal.msgSuccess('导入已确认：'+r.data.rows+'个SKU');busy.value=false;closeImport()}catch(e){if(alive&&epoch===detailSeq){if(e==='error')discardRejectedState();else error.value=e.message||'导入结果待确认，请重试原文件'}}finally{busy.value=false}}
async function templateFile(){try{const b=await catalogTemplate();if(b.type.includes('application/json'))throw Error('模板读取失败');saveAs(b,'商品导入模板.xlsx')}catch(e){error.value=e.message||'模板读取失败'}}
function exportRows(){proxy.download('erp/product/export',{...query},'商品档案.xlsx')}
onMounted(load)
function invalidate(){alive=false;++listSeq;++detailSeq;++categorySeq;++skuSeq;rows.value=[];total.value=0;open.value=false;importOpen.value=false;form.value={variants:[],images:[]};pending.value=null;importFile.value=null;importPending.value=null;categories.value=[];skuOptions.value=[]}
onDeactivated(invalidate)
onActivated(()=>{if(!alive){alive=true;load()}})
onBeforeUnmount(invalidate)
</script>
<style scoped>
.catalog-head{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0 10px}.catalog-toolbar{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin:10px 0 4px}.catalog-scroll{max-width:100%;overflow:auto}.catalog-note{font-size:12px;color:var(--el-text-color-secondary);line-height:1.6}.catalog-images{display:flex;gap:10px;flex-wrap:wrap;margin-top:16px}.catalog-image{display:flex;flex-direction:column;gap:4px;align-items:center}.catalog-image img{width:90px;height:90px;object-fit:contain;border:1px solid var(--el-border-color)}.thumbnail{width:38px;height:38px;object-fit:contain}.catalog-upload{position:relative;display:flex;align-items:center;justify-content:center;width:120px;min-height:90px;border:1px dashed var(--el-border-color);cursor:pointer}.catalog-upload input{position:absolute;inset:0;opacity:0;width:100%;cursor:pointer}.catalog-workspace :deep(.el-table .cell){line-height:22px}.catalog-workspace :deep(.el-table td.el-table__cell){padding:5px 0}@media(max-width:768px){.catalog-head{grid-template-columns:1fr}.catalog-toolbar{gap:8px}}
</style>
