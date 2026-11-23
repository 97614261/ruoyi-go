<script setup lang="ts">
import { computed } from 'vue'
import type { Run } from './contracts'
import { runRecovery } from './execution-impact'
const props = defineProps<{ run: Run }>()
const emit = defineEmits<{ refresh: [] }>()
const advice = computed(() => runRecovery(props.run))
</script>
<template>
  <aside v-if="advice" class="run-recovery" role="status" aria-label="任务处理建议">
    <strong>{{ advice.title }}</strong><p>{{ advice.detail }}</p>
    <p class="recovery-reference">运行 #{{ run.runId }} · 联系管理员时提供此编号</p>
    <button v-if="advice.refresh" class="text-button" @click="emit('refresh')">刷新原任务状态</button>
  </aside>
</template>
<style scoped>
.run-recovery{padding:14px;border:1px solid var(--line);border-left:3px solid var(--accent);border-radius:8px;background:var(--paper);margin:12px 0;overflow-wrap:anywhere}.run-recovery strong{font-size:13px}.run-recovery p{font-size:12px;color:var(--muted);line-height:1.7;margin-top:7px}.run-recovery .recovery-reference{font-size:11px}.run-recovery button{margin-top:8px}
</style>
