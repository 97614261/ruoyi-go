<template>
 <div class="app-container">
  <el-alert title="这里只是渠道目录，不是已接入的平台。当前仅登记店铺资料；手工录单和文件导入将在后续阶段实现。" type="info" :closable="false" class="mb8"/>
  <el-table :data="rows" v-loading="loading">
   <el-table-column label="渠道编码" prop="channelCode" width="160"/>
   <el-table-column label="渠道名称" prop="channelName" width="160"/>
   <el-table-column label="接入方式" width="180"><template #default>手工 / 文件（规划）</template></el-table-column>
   <el-table-column label="平台API" width="110"><template #default><el-tag type="info">未接入</el-tag></template></el-table-column>
   <el-table-column label="说明" prop="description"/>
  </el-table>
  <pagination v-show="total>0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load"/>
 </div>
</template>
<script setup name="ERPChannel">
import { ref, reactive } from 'vue'
import { listChannels } from '@/api/erp/shop'
const rows=ref([]),total=ref(0),loading=ref(false),query=reactive({pageNum:1,pageSize:10})
let sequence=0
async function load(){const ticket=++sequence;loading.value=true;try{const r=await listChannels({...query});if(ticket===sequence){rows.value=r.rows;total.value=r.total}}finally{if(ticket===sequence)loading.value=false}}
load()
</script>
