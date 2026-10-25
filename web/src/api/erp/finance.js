import request from '@/utils/request'
const paths={account:'accounts',source:'sources',payable:'payables',payment:'payments',statement:'statements'}
export function financeList(kind,params){return request({url:'/erp/finance/'+paths[kind],method:'get',params})}
export function financeGet(kind,id){return request({url:`/erp/finance/${paths[kind]}/${id}`,method:'get'})}
export function financeReturns(id,params){return request({url:`/erp/finance/payables/${id}/returns`,method:'get',params})}
export function financeWrite(command){
 const {action,id,payload}=command
 const routes={accountCreate:['post','accounts'],accountEdit:['put',`accounts/${id}`],payableCreate:['post','payables'],adjust:['post',`payables/${id}/adjust`],paymentCreate:['post','payments'],reverse:['post',`payments/${id}/reverse`]}
 const route=routes[action];if(!route)throw Error('财务动作不合法')
 return request({url:'/erp/finance/'+route[1],method:route[0],data:payload,headers:{repeatSubmit:false}})
}
