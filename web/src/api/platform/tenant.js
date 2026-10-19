import request from '@/utils/request'
export const listTenants = params => request({ url: '/platform/tenant/list', method: 'get', params })
export const getTenant = id => request({ url: `/platform/tenant/${id}`, method: 'get' })
export const getTenantQuota = id => request({ url: `/platform/tenant/${id}/quota`, method: 'get' })
export const saveTenantQuota = (id, data) => request({ url: `/platform/tenant/${id}/quota`, method: 'put', headers: { repeatSubmit: false }, data })
export const changeTenantLifecycle = (id, data) => request({ url: `/platform/tenant/${id}/lifecycle`, method: 'put', headers: { repeatSubmit: false }, data })
// Do not let the generic repeat-submit interceptor persist onboarding/reset
// credentials in sessionStorage. Version checks protect these updates instead.
export const saveTenant = data => request({ url: '/platform/tenant', method: data.tenantId ? 'put' : 'post', headers: { repeatSubmit: false }, data })
export const getTenantMenus = id => request({ url: `/platform/tenant/${id}/menus`, method: 'get' })
export const saveTenantMenus = (id, data) => request({ url: `/platform/tenant/${id}/menus`, method: 'put', headers: { repeatSubmit: false }, data })
export const resetTenantOwnerPassword = (id, data) => request({ url: `/platform/tenant/${id}/resetOwnerPwd`, method: 'put', headers: { repeatSubmit: false }, data })
