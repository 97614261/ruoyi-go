<template>
 <el-select :model-value="modelValue" @update:model-value="emit('update:modelValue',$event)" filterable remote clearable :remote-method="load" :loading="loading" placeholder="搜索配送方式名称（最多100条）" @visible-change="visible=>visible&&load('')" style="width:100%">
  <el-option v-if="modelValue&&!options.some(v=>v.id===modelValue)" :value="modelValue" :label="initialLabel||('方式 #'+modelValue)" disabled/>
  <el-option v-for="v in options" :key="v.id" :value="v.id" :label="v.carrierName+' · '+v.name+' ('+v.code+')'"/>
 </el-select>
</template>
<script setup>
import {ref,onBeforeUnmount} from 'vue'
import {logisticsOptions} from '@/api/erp/logistics'
defineProps({modelValue:Number,initialLabel:String})
const emit=defineEmits(['update:modelValue']),options=ref([]),loading=ref(false)
let sequence=0
async function load(name){const ticket=++sequence;loading.value=true;try{const r=await logisticsOptions({name,pageSize:100});if(ticket===sequence)options.value=r.rows}catch{if(ticket===sequence)options.value=[]}finally{if(ticket===sequence)loading.value=false}}
onBeforeUnmount(()=>{++sequence;options.value=[]})
</script>
