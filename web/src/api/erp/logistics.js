import request from '@/utils/request'
const masters = method => '/erp/logistics/' + (method ? 'shippingMethods' : 'carriers')
const transient = { repeatSubmit: false }
export const logisticsList = (method, params) => request({ url: masters(method), params })
export const logisticsGet = (method, id) => request({ url: masters(method) + '/' + id })
export const logisticsWrite = (method, id, data) => request({ url: masters(method) + (id ? '/' + id : ''), method: id ? 'put' : 'post', data, headers: transient })
export const logisticsOptions = params => request({ url: '/erp/logistics/options', params })
export const packageShipping = id => request({ url: '/erp/package/' + id + '/shipping' })
export const savePackageShipping = (id, data) => request({ url: '/erp/package/' + id + '/shipping', method: 'put', data, headers: transient })
export const checkTracking = data => request({ url: '/erp/logistics/check-tracking', method: 'post', data, headers: transient })
export const logisticsShipments = params => request({ url: '/erp/logistics/shipments', params })
