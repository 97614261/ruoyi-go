<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { APIError, request } from './api'
import type { CreatorEligibility, FormalPoints } from './contracts'
const props = defineProps<{ token: string }>()
const emit = defineEmits<{ expired: []; change: [value: CreatorEligibility] }>()
const eligibility=ref<CreatorEligibility | null>(null), points=ref<FormalPoints | null>(null), loading=ref(false), error=ref(''), page=ref(1)
let sequence=0
const bytes=(s:string)=>{try{const n=BigInt(s);return `${n / 1048576n}.${(n % 1048576n) * 10n / 1048576n} MiB`}catch{return '待核对'}}
const date=(v:string | null)=>v?new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai',dateStyle:'short',timeStyle:'medium'}).format(new Date(v)):'长期有效'
async function load(){const seq=++sequence;loading.value=true;error.value='';eligibility.value=null;points.value=null;try{const [e,p]=await Promise.all([request<{data:CreatorEligibility}>('/pod/eligibility',{},props.token),request<{data:FormalPoints}>(`/pod/points?pageNum=${page.value}&pageSize=10`,{},props.token)]);if(seq!==sequence)return;eligibility.value=e.data;points.value=p.data;emit('change',e.data)}catch(e){if(seq===sequence){error.value=(e as Error).message;if(e instanceof APIError && e.code===401)emit('expired')}}finally{if(seq===sequence)loading.value=false}}
function turn(n:number){page.value=n;void load()}
watch(()=>props.token,()=>{page.value=1;void load()})
onMounted(load);onBeforeUnmount(()=>{++sequence})
</script>
<template>
  <section class="eligibility">
    <div class="page-heading"><div><h1>我的资格与点数</h1><p>查看账号有效期、创作限制和正式点数流水。</p></div><button class="secondary-button" :disabled="loading" @click="load">刷新资格</button></div>
    <p v-if="loading" role="status" class="muted">正在读取…</p><p v-if="error" role="alert" class="error-message">{{ error }}</p>
    <template v-if="eligibility && points">
      <p class="qualification" :class="{warning:eligibility.creator.status !== 'active' || eligibility.creator.expired}">{{ eligibility.creator.status === 'suspended' ? '创作资格已停用，请联系管理员。私有作品暂不可访问。' : eligibility.creator.expired ? '创作资格已到期，可查看和下载已有内容；续期请联系管理员。' : '创作资格已开通' }}</p>
      <dl class="eligibility-grid"><div><dt>有效期（北京时间）</dt><dd>{{ date(eligibility.creator.validUntil) }}</dd></div><div><dt>今日提交（{{ eligibility.businessDate }}）</dt><dd>{{ eligibility.dailySubmissions }} / {{ eligibility.creator.maxDailySubmissions }}</dd></div><div><dt>并发 / 待执行上限</dt><dd>{{ eligibility.creator.maxConcurrentNodes }} / {{ eligibility.creator.maxPendingRuns }}</dd></div><div><dt>已用 / 预留空间</dt><dd>{{ bytes(eligibility.storage.usedBytes) }} / {{ bytes(eligibility.storage.heldBytes) }}</dd></div><div><dt>存储上限</dt><dd>{{ bytes(eligibility.creator.maxStorageBytes) }}</dd></div><div><dt>已授权模型数</dt><dd>{{ eligibility.modelIds.length }}</dd></div></dl>
      <h2>正式点数</h2><dl class="eligibility-grid"><div><dt>可用</dt><dd>{{ points.account.available }}</dd></div><div><dt>预占</dt><dd>{{ points.account.held }}</dd></div><div><dt>已使用</dt><dd>{{ points.account.spent }}</dd></div></dl>
      <p class="muted">{{ points.billingEnabled ? '正式点数按服务端报价预占，按确认用量结算。' : '当前执行仍使用旧模式，未启用正式扣点。' }}与原测试额度和模型费用分开。失败、取消不返还当日提交次数；回收站、缩略图和派生图也占存储空间。</p>
      <h2>点数流水</h2><p v-if="!points.rows.length" class="muted">暂无正式点数流水。</p>
      <div v-else class="points-list"><article v-for="row in points.rows" :key="row.eventId"><div><strong>{{ ({ grant: '发放', deduct: '扣减', hold: '预占', settle: '结算', release: '释放', transfer: '派发转移' } as Record<string,string>)[row.kind] || row.kind }} {{ row.amount }} 点</strong><span>操作后可用 {{ row.availableAfter }} 点</span></div><p>{{ row.reason }}</p><time>{{ date(row.createTime) }}</time></article></div>
      <div v-if="points.total > 10" class="eligibility-pager"><button class="secondary-button" :disabled="page <= 1 || loading" @click="turn(page-1)">上一页</button><span>第 {{ page }} 页 · {{ points.total }} 条</span><button class="secondary-button" :disabled="page*10 >= points.total || loading" @click="turn(page+1)">下一页</button></div>
    </template>
  </section>
</template>
<style scoped>
.eligibility{max-width:1100px;margin:0 auto}.qualification{padding:14px 18px;border:1px solid var(--line,#d9e3df);border-radius:12px;background:var(--surface,#fff)}.warning{color:#8a4d10;background:#fff6e8}.eligibility-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.eligibility-grid>div{padding:18px;border:1px solid var(--line,#d9e3df);border-radius:12px;background:var(--surface,#fff)}dt{font-size:13px;color:var(--muted,#63736b)}dd{margin:10px 0 0;font-size:20px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}.points-list article{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:14px 0;border-bottom:1px solid var(--line,#d9e3df)}.points-list article>div{display:grid;gap:5px}.points-list p{flex:1;overflow-wrap:anywhere}.points-list span,time{font-size:13px;color:var(--muted,#63736b)}.eligibility-pager{display:flex;align-items:center;justify-content:flex-end;gap:14px;margin-top:18px}@media(max-width:700px){.eligibility-grid{grid-template-columns:1fr 1fr}.points-list article{align-items:flex-start;flex-direction:column;gap:5px}.points-list p{margin:5px 0}dd{font-size:17px}}@media(max-width:420px){.eligibility-grid{grid-template-columns:1fr}}
</style>
