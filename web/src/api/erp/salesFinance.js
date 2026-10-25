import request from '@/utils/request'
const paths={account:'accounts',source:'sources',receivable:'receivables',collection:'collections',statement:'statements'}
export function salesFinanceList(kind,params){return request({url:(kind==='account'?'/erp/finance/':'/erp/salesFinance/')+paths[kind],method:'get',params})}
export function salesFinanceGet(kind,id){return request({url:`/erp/salesFinance/${paths[kind]}/${id}`,method:'get'})}
export function salesFinanceReturns(id,params){return request({url:`/erp/salesFinance/receivables/${id}/returns`,method:'get',params})}
export function salesFinanceWrite(command){
 const {action,id,payload}=command
 const routes={accountCreate:['post','accounts'],accountEdit:['put',`accounts/${id}`],receivableCreate:['post','receivables'],adjust:['post',`receivables/${id}/adjust`],collectionCreate:['post','collections'],reverse:['post',`collections/${id}/reverse`]}
 const route=routes[action];if(!route)throw Error('财务动作不合法')
 return request({url:'/erp/salesFinance/'+route[1],method:route[0],data:payload,headers:{repeatSubmit:false}})
}
