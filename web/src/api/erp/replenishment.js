import request from '@/utils/request'
export const listReplenishments = params => request({ url: '/erp/replenishment/list', method: 'get', params })
export const createReplenishment = data => request({ url: '/erp/replenishment', method: 'post', headers: { repeatSubmit: false }, data })
