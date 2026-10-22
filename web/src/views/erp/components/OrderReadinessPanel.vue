<template>
 <section v-if="result" class="readiness" aria-label="审核检查结果">
  <el-alert :title="result.ready?'当前检查可审核；未锁定库存，提交时仍会重新校验':'当前不能审核，请先处理下列原因'" :type="result.ready?'success':'warning'" :closable="false"/>
  <p class="stamp">订单 v{{result.version}} · 仓库 #{{result.warehouseId}} · 检查时间 {{result.checkedAt}}</p>
  <ul v-if="result.issues?.length"><li v-for="(issue,i) in result.issues" :key="i">{{issue.message}}</li></ul>
  <el-table v-if="result.stocks?.length" :data="result.stocks" size="small" max-height="260"><el-table-column prop="skuCode" label="内部SKU" min-width="130"/><el-table-column prop="required" label="本单合计" width="100"/><el-table-column prop="available" label="当前可用" width="100"/><el-table-column label="缺口" width="90"><template #default="{row}"><span :class="{'shortage':row.shortage>0}">{{row.shortage}}</span></template></el-table-column><el-table-column prop="unit" label="单位" width="70"/></el-table>
  <p class="stamp">商品未建档、未配对或数量异常时先显示首个阻断原因；修复后重新检查。库存明细按内部SKU汇总，不是全企业缺货统计。</p>
 </section>
</template>
<script setup>
defineProps({result:{type:Object,default:null}})
</script>
<style scoped>
.readiness{margin-top:12px}.stamp{font-size:12px;color:var(--el-text-color-secondary);line-height:1.6}.shortage{color:var(--el-color-danger);font-weight:600}ul{padding-left:20px;font-size:13px}
</style>
