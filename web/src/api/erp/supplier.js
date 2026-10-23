import request from '@/utils/request'

export const listSuppliers = params => request({ url: '/erp/supplier/list', method: 'get', params })
export const getSupplier = id => request({ url: `/erp/supplier/${id}`, method: 'get' })
// Do not retain contact payloads in the generic in-memory repeat guard either.
export const addSupplier = data => request({ url: '/erp/supplier', method: 'post', data, headers: { repeatSubmit: false } })
export const editSupplier = (id, data) => request({ url: `/erp/supplier/${id}`, method: 'put', data, headers: { repeatSubmit: false } })
