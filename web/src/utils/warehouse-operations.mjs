import {validateDocumentLines} from './erp.mjs'
export const transferStatuses={DRAFT:'草稿',IN_TRANSIT:'在途待收',COMPLETED:'已收货',CANCELLED:'已取消'}
export function transferPayload(f,action){
 if(!['create','edit','dispatch','receive','cancel'].includes(action))throw Error('调拨动作不合法')
 if(typeof f.requestKey!=='string'||!/^[A-Za-z0-9_-]{8,64}$/.test(f.requestKey))throw Error('请求标识不合法')
 const out={version:f.version,requestKey:f.requestKey}
 if(action!=='create'&&(!Number.isSafeInteger(f.version)||f.version<1))throw Error('请刷新取得当前版本')
 if(action==='create'||action==='edit'){
  if(!Number.isSafeInteger(f.sourceWarehouseId)||!Number.isSafeInteger(f.targetWarehouseId)||f.sourceWarehouseId<1||f.targetWarehouseId<1||f.sourceWarehouseId===f.targetWarehouseId)throw Error('请选择两个不同仓库')
  const problem=validateDocumentLines(f.lines);if(problem)throw Error(problem)
  const remark=String(f.remark||'').trim();if([...remark].length>500)throw Error('备注过长')
  Object.assign(out,{sourceWarehouseId:f.sourceWarehouseId,targetWarehouseId:f.targetWarehouseId,remark,lines:f.lines.map(l=>({skuId:l.skuId,quantity:l.quantity}))})
 }
 return out
}
export function sourceType(kind){return{IN:'receipt',SALE:'sales',SR:'salesReturn',PR:'purchaseReturn',COUNT:'count'}[kind]}
export function thresholdLabel(value){return value===null||value===undefined?'未配置':String(value)}
