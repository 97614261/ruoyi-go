import request from '@/utils/request'
export const listReturnableReceipts=params=>request({url:'/erp/purchaseReturnOrder/returnable/list',method:'get',params})
export const getReturnableReceipt=id=>request({url:`/erp/purchaseReceipt/${id}/returnable`,method:'get'})
export const postPurchaseReturn=(id,data)=>request({url:`/erp/purchaseReceipt/${id}/returns`,method:'post',headers:{repeatSubmit:false},data})
export const listPurchaseReturns=params=>request({url:'/erp/purchaseReturnOrder/list',method:'get',params})
export const getPurchaseReturn=id=>request({url:`/erp/purchaseReturnOrder/${id}`,method:'get'})
