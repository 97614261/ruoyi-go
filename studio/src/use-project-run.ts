import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { APIError, request } from './api'
import type { Run } from './contracts'
import { terminalRun } from './run-state'

export function useProjectRun(token: () => string, projectId: string, scope: string, paused: () => boolean, expired: () => void) {
  const activeRun = ref<Run | null>(null), runError = ref(''), selectedRunId = ref('')
  const key = `pod-studio.selected-run:${scope}:${projectId}`
  let sequence = 0, disposed = false, timer: ReturnType<typeof setTimeout> | undefined
  function remember() {
    try { if (selectedRunId.value) sessionStorage.setItem(key, selectedRunId.value); else sessionStorage.removeItem(key) }
    catch { runError.value = '浏览器无法保留运行编号；当前页面仍会刷新状态。' }
  }
  function schedule() {
    clearTimeout(timer)
    if (!disposed && !paused() && document.visibilityState !== 'hidden' && navigator.onLine !== false && selectedRunId.value && (!activeRun.value || !terminalRun(activeRun.value.state))) timer = setTimeout(refreshRun, 2000)
  }
  function selectRun(run: Run) {
    if (disposed || run.mode === 'polish' || run.projectId !== projectId) return
    ++sequence; selectedRunId.value = run.runId; activeRun.value = run; runError.value = ''; remember(); schedule()
  }
  function forgetRun(id: string) {
    if (id !== selectedRunId.value || disposed) return
    ++sequence; clearTimeout(timer); selectedRunId.value = ''; activeRun.value = null
    runError.value = '上次运行已不可访问，请从执行记录重新选择。'; remember()
  }
  async function refreshRun() {
    if (!selectedRunId.value || disposed) return
    clearTimeout(timer)
    const current = ++sequence, id = selectedRunId.value
    try {
      const { data } = await request<{ data: Run }>(`/pod/runs/${encodeURIComponent(id)}`, {}, token())
      if (disposed || current !== sequence) return
      if (data.projectId !== projectId || data.runId !== id || data.mode === 'polish') throw new APIError('运行记录与当前项目不匹配', 404)
      activeRun.value = data; runError.value = ''
    } catch (e) {
      if (disposed || current !== sequence) return
      runError.value = e instanceof Error ? e.message : '运行状态读取失败'
      if (e instanceof APIError && [401, 403, 404].includes(e.code)) { selectedRunId.value = ''; activeRun.value = null; remember(); if (e.code === 401) expired() }
    } finally { if (!disposed && current === sequence) schedule() }
  }
  watch(paused, value => { ++sequence; clearTimeout(timer); if (!value) schedule() })
  function resume() {
    clearTimeout(timer)
    if (!disposed && !paused() && document.visibilityState !== 'hidden' && navigator.onLine !== false && selectedRunId.value) void refreshRun()
  }
  onMounted(() => {
    document.addEventListener('visibilitychange', resume); window.addEventListener('online', resume)
    try { const stored = sessionStorage.getItem(key); if (stored && /^[1-9]\d{0,19}$/.test(stored)) selectedRunId.value = stored; else if (stored) sessionStorage.removeItem(key) }
    catch { runError.value = '浏览器无法读取上次运行编号，请从执行记录选择。' }
    if (selectedRunId.value) void refreshRun()
  })
  onBeforeUnmount(() => { disposed = true; ++sequence; clearTimeout(timer); document.removeEventListener('visibilitychange', resume); window.removeEventListener('online', resume) })
  return { activeRun, selectedRunId, runError, selectRun, refreshRun, forgetRun }
}
