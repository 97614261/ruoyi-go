<script setup lang="ts">
import type { ManagedBilling } from './contracts'
defineProps<{ billing: ManagedBilling }>()
</script>
<template><dl class="billing-summary" aria-label="本次正式点数"><div><dt>预占</dt><dd>{{ billing.held }} 点</dd></div><div><dt>已用</dt><dd>{{ billing.spent }} 点</dd></div><div><dt>已释放</dt><dd>{{ billing.released }} 点</dd></div></dl></template>
<style scoped>.billing-summary{display:flex;flex-wrap:wrap;gap:12px 24px;padding:12px 0;margin:0}.billing-summary>div{display:flex;gap:8px;align-items:baseline}dt{font-size:12px;color:var(--muted)}dd{margin:0;font-variant-numeric:tabular-nums;font-weight:650}</style>
