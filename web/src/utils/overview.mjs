export const overviewSections=[
 {key:'orders',name:'订单待办',permission:'erp:order:list'},
 {key:'shipments',name:'本地发货',permission:'erp:shipment:list'},
 {key:'stocks',name:'库存提醒',permission:'erp:stock:list'},
 {key:'purchases',name:'采购待办',permission:'erp:purchase:list'},
 {key:'payables',name:'采购待付',permission:'erp:payable:list',money:true},
 {key:'receivables',name:'销售待收',permission:'erp:receivable:list',money:true}
]
export function overviewCount(n){return Number.isSafeInteger(n)&&n>=0?String(n):'—'}
function beforeBoundary(value){
 if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2} 00:00:00$/.test(value))throw Error('发货时间边界无效，请刷新')
 const date=new Date(value.replace(' ','T')+'Z')
 if(!Number.isFinite(date.getTime())||date.toISOString().slice(0,19).replace('T',' ')!==value)throw Error('发货时间边界无效，请刷新')
 return new Date(date.getTime()-1000).toISOString().slice(0,19).replace('T',' ')
}
export function overviewTarget(kind,metric,data={},currency){
 if(kind==='orders'&&['pendingReview','readyToShip'].includes(metric))return{path:'/erp/order',query:{status:metric==='pendingReview'?'PENDING_REVIEW':'READY_TO_SHIP'}}
 if(kind==='stocks'&&['withStock','withReserved','low'].includes(metric))return{path:'/erp/stock',query:{stockState:{withStock:'HAS_STOCK',withReserved:'HAS_RESERVED',low:'LOW'}[metric]}}
 if(kind==='purchases')return{path:'/erp/purchase',query:{awaitingReceipt:'true'}}
 if(kind==='shipments'&&['today','sevenDays'].includes(metric)){
  const from=metric==='today'?data.todayFrom:data.sevenDaysFrom
  if(typeof from!=='string'||!/^\d{4}-\d{2}-\d{2} 00:00:00$/.test(from))throw Error('发货时间边界无效，请刷新')
  beforeBoundary(from)
  return{path:'/erp/shipment',query:{shippedFrom:from,shippedTo:beforeBoundary(data.through)}}
 }
 if(['payables','receivables'].includes(kind)&&typeof currency==='string'&&/^[A-Z]{3}$/.test(currency))return{path:kind==='payables'?'/erp/payable':'/erp/receivable',query:{currency}}
 throw Error('概览入口无效，请刷新')
}
