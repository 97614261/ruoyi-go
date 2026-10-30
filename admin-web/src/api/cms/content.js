// Channel/asset API scaffold generated from cms_channel/cms_asset, specialized for private storage and CAS.
import request from '@/utils/request'

export const listChannels = params => request({ url: '/cms/channel/list', params })
export const getChannel = id => request({ url: `/cms/channel/${id}` })
export const saveChannel = data => request({ url: '/cms/channel', method: data.channelId ? 'put' : 'post', data })
export const deleteChannel = row => request({ url: `/cms/channel/${row.channelId}`, method: 'delete', data: { version: row.version } })
export const listAssets = params => request({ url: '/cms/asset/list', params })
export const getAsset = id => request({ url: `/cms/asset/${id}` })
export const saveAsset = (id, data) => request({ url: `/cms/asset/${id}`, method: 'put', data })
export const deleteAsset = row => request({ url: `/cms/asset/${row.assetId}`, method: 'delete', data: { version: row.version } })
export const listAssetReferences = (id, params) => request({ url: `/cms/asset/${id}/references`, params })
export const uploadAsset = file => {
  const data = new FormData(); data.append('file', file)
  return request({ url: '/cms/asset/upload', method: 'post', data, timeout: 60000, headers: { 'Content-Type': 'multipart/form-data', repeatSubmit: false } })
}
export async function privateImage(id) {
  const blob = await request({ url: `/cms/asset/${id}/file`, responseType: 'blob' })
  if (!blob.type.startsWith('image/')) {
    let message = '素材预览失败'
    try { message = JSON.parse(await blob.text()).msg || message } catch {}
    throw new Error(message)
  }
  return blob
}

export const listContent = (kind, params) => request({ url: `/cms/${kind}/list`, params })
export const getContent = (kind, id) => request({ url: `/cms/${kind}/${id}` })
export const saveContent = (kind, data) => request({ url: `/cms/${kind}`, method: data.contentId ? 'put' : 'post', data })
export const contentHistory = (kind, id, params) => request({ url: `/cms/${kind}/${id}/history`, params })
export const contentAction = (kind, id, action, data) => request({ url: `/cms/${kind}/${id}` + (action === 'remove' ? '' : `/${action}`), method: action === 'remove' ? 'delete' : 'post', data })

export const listSchedules = (kind, params) => request({url:`/cms/${kind}/schedules`,params})
export const createSchedule = (kind, data) => request({url:`/cms/${kind}/schedules`,method:"post",data})
export const cancelSchedule = (kind, row) => request({url:`/cms/${kind}/schedules/${row.scheduleId}/cancel`,method:"post",data:{version:row.version}})
