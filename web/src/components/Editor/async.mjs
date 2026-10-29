import { defineAsyncComponent, h } from 'vue'

const statusStyle = { padding: '16px', minHeight: '100px', color: 'var(--el-text-color-secondary)', border: '1px solid var(--el-border-color)' }
const Loading = { name: 'EditorLoading', render: () => h('div', { role: 'status', 'aria-live': 'polite', style: statusStyle }, '编辑器加载中…') }
const Failed = { name: 'EditorLoadError', render: () => h('div', { role: 'alert', style: statusStyle }, '编辑器加载失败，请检查网络后刷新页面；刷新前请保留其他未保存内容。') }

export function createAsyncEditor(loader) {
  return defineAsyncComponent({
    loader,
    loadingComponent: Loading,
    errorComponent: Failed,
    delay: 200,
    timeout: 30000,
    onError(error, retry, fail, attempts) { if (attempts === 1) retry(); else fail() }
  })
}

// Registration does not download Quill. The loader runs only when an Editor
// instance is actually rendered (including lazy dialog bodies).
export default createAsyncEditor(() => import('./index.vue'))
