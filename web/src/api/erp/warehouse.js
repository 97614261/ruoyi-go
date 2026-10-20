import request from '@/utils/request'

export function listErpWarehouse(query) { return request({ url: "/erp/warehouse" + '/list', method: 'get', params: query }) }
export function getErpWarehouse(id) { return request({ url: "/erp/warehouse" + '/' + id, method: 'get' }) }
export function addErpWarehouse(data) { return request({ url: "/erp/warehouse", method: 'post', data }) }
export function updateErpWarehouse(data) { return request({ url: "/erp/warehouse", method: 'put', data }) }
export function delErpWarehouse(ids) { return request({ url: "/erp/warehouse" + '/' + ids, method: 'delete' }) }
