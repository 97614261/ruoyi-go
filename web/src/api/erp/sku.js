import request from '@/utils/request'

export function listErpSku(query) { return request({ url: "/erp/sku" + '/list', method: 'get', params: query }) }
export function getErpSku(id) { return request({ url: "/erp/sku" + '/' + id, method: 'get' }) }
export function addErpSku(data) { return request({ url: "/erp/sku", method: 'post', data }) }
export function updateErpSku(data) { return request({ url: "/erp/sku", method: 'put', data }) }
export function delErpSku(ids) { return request({ url: "/erp/sku" + '/' + ids, method: 'delete' }) }
