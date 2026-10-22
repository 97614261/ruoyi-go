import request from '@/utils/request'

export const listChannels = params => request({ url: '/erp/channel/list', method: 'get', params })
export const listErpShop = params => request({ url: '/erp/shop/list', method: 'get', params })
export const getErpShop = id => request({ url: `/erp/shop/${id}`, method: 'get' })
export const addErpShop = data => request({ url: '/erp/shop', method: 'post', data })
export const updateErpShop = (id, data) => request({ url: `/erp/shop/${id}`, method: 'put', data })
export const getShopAccess = id => request({ url: `/erp/shop/access/${id}`, method: 'get' })
export const setShopAccess = (id, data) => request({ url: `/erp/shop/access/${id}`, method: 'put', data })
