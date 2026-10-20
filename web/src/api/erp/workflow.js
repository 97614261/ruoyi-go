import request from '@/utils/request'
export const listDocuments=(type,params)=>request({url:`/erp/${type}/list`,method:'get',params})
export const getDocument=(type,id)=>request({url:`/erp/${type}/${id}`,method:'get'})
export const createDocument=(type,data)=>request({url:`/erp/${type}`,method:'post',headers:{repeatSubmit:false},data})
export const actOnDocument=(type,id,action)=>request({url:`/erp/${type}/${id}/${action}`,method:'put',headers:{repeatSubmit:false}})
export const listStock=params=>request({url:'/erp/stock/list',method:'get',params})
export const listLedger=params=>request({url:'/erp/ledger/list',method:'get',params})
