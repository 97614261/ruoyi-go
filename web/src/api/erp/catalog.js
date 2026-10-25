import request from '@/utils/request'
export const catalogList=(category,params)=>request({url:category?'/erp/productCategory/list':'/erp/product/list',method:'get',params})
export const catalogGet=id=>request({url:'/erp/product/'+id,method:'get'})
export const catalogWrite=(category,id,data)=>request({url:(category?'/erp/productCategory':'/erp/product')+(id?'/'+id:''),method:id?'put':'post',data,headers:{repeatSubmit:false},timeout:30000})
export const catalogUpload=file=>{const data=new FormData();data.append('file',file);return request({url:'/erp/product/image',method:'post',data,headers:{'Content-Type':'multipart/form-data',repeatSubmit:false},timeout:30000})}
export const catalogImport=(file,key)=>{const data=new FormData();data.append('file',file);data.append('requestKey',key);return request({url:'/erp/product/import',method:'post',data,headers:{'Content-Type':'multipart/form-data',repeatSubmit:false},timeout:60000})}
export const catalogTemplate=()=>request({url:'/erp/product/template',method:'get',responseType:'blob'})
