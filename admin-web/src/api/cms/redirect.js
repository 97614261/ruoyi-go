// Generated API scaffold, specialized: no deletion; routing changes require explicit publish permission.
import request from '@/utils/request'
export const listRedirects=params=>request({url:'/cms/redirect/list',params})
export const getRedirect=id=>request({url:`/cms/redirect/${id}`})
export const saveRedirect=data=>request({url:'/cms/redirect',method:data.redirectId?'put':'post',data})
export const setRedirectStatus=(row,status)=>request({url:`/cms/redirect/${row.redirectId}/status`,method:'post',data:{version:row.version,status}})
