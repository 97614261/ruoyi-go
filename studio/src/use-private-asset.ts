import { onBeforeUnmount, ref, watch } from 'vue'
import { APIError, request, requestImage } from './api'
import type { Asset } from './contracts'

export function usePrivateAsset(id: () => string, token: () => string, expired: () => void) {
  const asset = ref<Asset | null>(null), src = ref(''), loading = ref(false), error = ref('')
  let sequence = 0
  function revoke() { if (src.value) URL.revokeObjectURL(src.value); src.value = '' }
  function decodeError() { error.value = '图片无法显示，请重新读取。'; revoke() }
  async function load() {
    const current = ++sequence, assetId = id(); revoke(); asset.value = null; error.value = ''; loading.value = !!assetId
    if (!assetId) return
    try {
      const [meta, image] = await Promise.all([request<{ data: Asset }>(`/pod/assets/${assetId}`, {}, token()), requestImage(`/pod/assets/${assetId}/content?variant=original`, token())])
      if (current !== sequence) return
      if (!(meta.data.width > 0 && meta.data.height > 0)) throw new Error('图片尺寸无法读取')
      asset.value = meta.data; src.value = URL.createObjectURL(image)
    } catch (e) {
      if (current === sequence) { error.value = e instanceof Error ? e.message : '无法读取图片'; if (e instanceof APIError && e.code === 401) expired() }
    } finally { if (current === sequence) loading.value = false }
  }
  watch([id, token], load, { immediate: true })
  onBeforeUnmount(() => { ++sequence; revoke() })
  return { asset, src, loading, error, load, decodeError }
}
