import request from '@/utils/request'
export const listShopItems = params => request({ url: '/erp/shopItem/list', method: 'get', params })
export const getShopItem = id => request({ url: `/erp/shopItem/${id}`, method: 'get' })
export const addShopItem = data => request({ url: '/erp/shopItem', method: 'post', data })
export const editShopItem = (id, data) => request({ url: `/erp/shopItem/${id}`, method: 'put', data })
export const setShopItemMapping = (id, data) => request({ url: `/erp/shopItem/${id}/mapping`, method: 'put', data })
export const cancelShopItemMapping = (id, version) => request({ url: `/erp/shopItem/${id}/mapping`, method: 'delete', params: { version } })
