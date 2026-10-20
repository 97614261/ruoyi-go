<template>
 <div class="app-container">
  <el-alert :title="ledger?'流水只读，记录每次入库、预占、释放和出库；不能直接修改或删除。':'可用库存 = 实物库存 − 预占。库存只能通过单据变化，不提供手工覆盖余额入口。'" type="info" :closable="false" class="mb12"/>
  <el-form inline size="small"><el-form-item label="仓库"><ReferenceSelect kind="warehouse" v-model="query.warehouseId" style="width:220px"/></el-form-item><el-form-item label="商品"><ReferenceSelect v-model="query.skuId" style="width:280px"/></el-form-item><el-form-item><el-button type="primary" @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item></el-form>
  <el-table :data="rows" v-loading="loading" size="small">
   <template v-if="!ledger"><el-table-column label="仓库" prop="warehouseName"/><el-table-column label="SKU编码" prop="skuCode"/><el-table-column label="商品" prop="productName"/><el-table-column label="单位" prop="unit" width="80"/><el-table-column label="实物库存" prop="onHand"/><el-table-column label="预占" prop="reserved"/><el-table-column label="可用" prop="available"/></template>
   <template v-else><el-table-column label="来源单号" prop="documentNo" min-width="300"/><el-table-column label="仓库ID" prop="warehouseId" width="90"/><el-table-column label="SKU ID" prop="skuId" width="90"/><el-table-column label="事件" width="100"><template #default="{row}">{{eventLabels[row.event]}}</template></el-table-column><el-table-column label="实物变化" prop="onHandDelta"/><el-table-column label="预占变化" prop="reservedDelta"/><el-table-column label="实物结余" prop="onHandAfter"/><el-table-column label="预占结余" prop="reservedAfter"/><el-table-column label="操作人" prop="createBy"/><el-table-column label="时间" prop="createTime" width="170"/></template>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
 </div>
</template>
<script setup>
import {ref,reactive,onMounted} from 'vue'
import {listStock,listLedger} from '@/api/erp/workflow'
import {eventLabels} from '@/utils/erp.mjs'
import ReferenceSelect from './ReferenceSelect.vue'
const props=defineProps({ledger:Boolean}),query=reactive({pageNum:1,pageSize:10}),rows=ref([]),total=ref(0),loading=ref(false)
async function load(){loading.value=true;try{const res=await(props.ledger?listLedger(query):listStock(query));rows.value=res.rows;total.value=res.total}finally{loading.value=false}}
function search(){query.pageNum=1;load()}
function reset(){query.warehouseId=undefined;query.skuId=undefined;search()}
onMounted(load)
</script>
<style scoped>.mb12{margin-bottom:12px}</style>
