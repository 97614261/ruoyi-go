import axios from 'axios'
import { getToken } from '@/utils/auth'

// Management requests never copy bodies into a repeat-submit cache or log errors.
const client = axios.create({ baseURL: import.meta.env.VITE_APP_BASE_API, timeout: 10000 })
client.interceptors.request.use(config => {
  const token = getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})
client.interceptors.response.use(response => {
  if (response.data.code === 200) return response.data
  const error = new Error(response.data.msg || '管理请求失败')
  error.code = response.data.code
  throw error
}, () => { throw new Error('管理接口暂不可用，请检查服务后重试') })

export const getAdminBootstrap = () => client.get('/pod-admin/bootstrap')
export const getAdminOverview = params => client.get('/pod-admin/overview', { params })
export const getAdminAudits = params => client.get('/pod-admin/audits', { params })
export const getAdminAudit = id => client.get(`/pod-admin/audits/${encodeURIComponent(id)}`)
export const catalogList = (kind, params) => client.get(`/pod-admin/${kind}`, { params })
export const catalogGet = (kind, id) => client.get(`/pod-admin/${kind}/${encodeURIComponent(id)}`)
export const catalogVersions = (id, params) => client.get(`/pod-admin/models/${encodeURIComponent(id)}/versions`, { params })
export const catalogDefaults = () => client.get('/pod-admin/model-defaults')
export const catalogWrite = (method, path, data) => client.request({ method, url: `/pod-admin/${path}`, data })
export const catalogAction = requestId => client.get(`/pod-admin/actions/${encodeURIComponent(requestId)}`)
export const catalogCheck = id => client.post(`/pod-admin/models/${encodeURIComponent(id)}/check-config`)
export const adminList = (path, params) => client.get(`/pod-admin/${path}`, { params })
export const adminGet = path => client.get(`/pod-admin/${path}`)
export async function adminFile(path) {
  const token = getToken()
  const response = await axios.get(`${import.meta.env.VITE_APP_BASE_API}/pod-admin/${path}`, { headers: token ? { Authorization: `Bearer ${token}` } : {}, responseType: 'blob', timeout: 10000 })
  if (!response.data.type.startsWith('image/')) {
    const body = JSON.parse(await response.data.text())
    throw new Error(body.msg || '图片读取失败')
  }
  return response.data
}
export const officialUpload = (requestId, licenseNote, file) => {
  const data = new FormData(); data.append('file', file)
  return client.post(`/pod-admin/catalog/assets?${new URLSearchParams({ requestId, licenseNote })}`, data, { timeout: 30000 })
}
