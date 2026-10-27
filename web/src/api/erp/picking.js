import request from '@/utils/request'
export const listPicking = params => request({url:'/erp/picking/list',method:'get',params})
export const getPicking = id => request({url:`/erp/picking/${id}`,method:'get'})
// Domain request keys own retries; generic browser guards must neither retain
// scan bodies nor reject a deliberate retry before it reaches the server.
export const checkPicking = (id,data) => request({url:`/erp/picking/${id}/check`,method:'put',data,headers:{repeatSubmit:false}})
export const shipPicking = (id,data) => request({url:`/erp/picking/${id}/ship`,method:'put',data,headers:{repeatSubmit:false}})
