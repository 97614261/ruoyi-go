<template>
 <el-select :model-value="modelValue" @update:model-value="emit('update:modelValue',$event)" filterable remote clearable :remote-method="load" :loading="loading" :placeholder="kind==='sku'?'搜索商品名称（最多显示100条）':'搜索仓库名称'" @visible-change="open=>open&&load('')" style="width:100%">
  <el-option v-for="row in options" :key="row[id]" :value="row[id]" :label="kind==='sku'?`${row.skuCode} · ${row.productName}`:`${row.warehouseCode} · ${row.warehouseName}`" />
 </el-select>
</template>
<script setup>
import {ref,computed} from 'vue'
import {listErpSku} from '@/api/erp/sku'
import {listErpWarehouse} from '@/api/erp/warehouse'
const props=defineProps({modelValue:Number,kind:{type:String,default:'sku'}})
const emit=defineEmits(['update:modelValue'])
const options=ref([]),loading=ref(false),id=computed(()=>props.kind==='sku'?'skuId':'warehouseId')
let sequence=0
async function load(keyword){
 const ticket=++sequence;loading.value=true
 try{const res=await(props.kind==='sku'?listErpSku({pageSize:100,productName:keyword}):listErpWarehouse({pageSize:100,warehouseName:keyword}));if(ticket===sequence){const selected=options.value.find(row=>row[id.value]===props.modelValue);options.value=res.rows.filter(row=>row.status==='0');if(selected&&!options.value.some(row=>row[id.value]===selected[id.value]))options.value.unshift(selected)}}finally{if(ticket===sequence)loading.value=false}
}
</script>
