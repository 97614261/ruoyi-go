import request from '@/utils/request'
export const listReturnablePackages=params=>request({url:'/erp/salesReturnOrder/returnable/list',method:'get',params})
export const getReturnablePackage=id=>request({url:`/erp/package/${id}/returnable`,method:'get'})
export const postSalesReturn=(id,data)=>request({url:`/erp/package/${id}/returns`,method:'post',headers:{repeatSubmit:false},data})
export const listSalesReturns=params=>request({url:'/erp/salesReturnOrder/list',method:'get',params})
export const getSalesReturn=id=>request({url:`/erp/salesReturnOrder/${id}`,method:'get'})
