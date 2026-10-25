import request from '@/utils/request'
export const getERPSetup = () => request({url:'/erp/setup',method:'get'})
export const getERPRoleTemplates = () => request({url:'/erp/setup/roleTemplates',method:'get'})
