<template>
 <div style="width:100%"><el-select :model-value="modelValue" @update:model-value="emit('update:modelValue',$event)" :disabled="disabled" remote filterable clearable :remote-method="load" :loading="loading" @visible-change="v=>v&&load('')" :placeholder="'搜索'+names[kind]+'名称，最多100条'" style="width:100%"><el-option v-for="r in choices" :key="r[id]" :value="r[id]" :label="r[label]||('#'+r[id])"/></el-select><small v-if="error" style="color:var(--el-color-danger)">读取失败，请重新展开重试</small></div>
</template>
<script setup>
import {ref,computed,onBeforeUnmount,onDeactivated} from 'vue'
import {listSuppliers} from '@/api/erp/supplier'
import {listErpSku} from '@/api/erp/sku'
import {listErpWarehouse} from '@/api/erp/warehouse'
const props=defineProps({modelValue:Number,kind:{type:String,default:'sku'},disabled:Boolean,seeds:{type:Array,default:()=>[]}}),emit=defineEmits(['update:modelValue'])
const names={sku:'商品',supplier:'供应商',warehouse:'仓库'},id=computed(()=>({sku:'skuId',supplier:'supplierId',warehouse:'warehouseId'}[props.kind])),label=computed(()=>props.kind==='sku'?'productName':props.kind+'Name')
const options=ref([...props.seeds]),loading=ref(false),error=ref(false);let sequence=0
// A newly selected ID has no snapshot label yet: do not let that placeholder
// hide the actual remote name. Existing frozen labels still take precedence.
const choices=computed(()=>{const items=new Map(options.value.map(r=>[r[id.value],r]));for(const seed of props.seeds){const current=items.get(seed[id.value])||{};items.set(seed[id.value],seed[label.value]?{...current,...seed}:{...seed,...current})}return[...items.values()]})
async function load(keyword){const ticket=++sequence;loading.value=true;error.value=false;try{const fn={sku:listErpSku,supplier:listSuppliers,warehouse:listErpWarehouse}[props.kind];const r=await fn({pageNum:1,pageSize:100,[label.value]:keyword});if(ticket!==sequence)return;const selected=[...options.value,...props.seeds].find(r=>r[id.value]===props.modelValue);options.value=r.rows.filter(r=>r.status==='0');if(selected&&!options.value.some(r=>r[id.value]===selected[id.value]))options.value.unshift(selected)}catch{if(ticket===sequence){options.value=[...props.seeds];error.value=true}}finally{if(ticket===sequence)loading.value=false}}
function clear(){++sequence;options.value=[];loading.value=false}onBeforeUnmount(clear);onDeactivated(clear)
</script>
