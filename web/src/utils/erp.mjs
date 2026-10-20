export const statusLabels={DRAFT:'草稿',POSTED:'已入库',RESERVED:'已预占',SHIPPED:'已出库',CANCELLED:'已取消'}
export const actionLabels={confirm:'确认',ship:'确认出库',cancel:'取消单据'}
export const eventLabels={IN:'采购入库',RESERVE:'销售预占',RELEASE:'取消释放',OUT:'销售出库'}
export function documentActions(kind,status){
 if(status==='DRAFT')return ['confirm','cancel']
 if(kind==='SALE'&&status==='RESERVED')return ['ship','cancel']
 return []
}
export function validateDocumentLines(lines){
 if(!Array.isArray(lines)||lines.length<1||lines.length>100)return '请添加1—100行商品'
 const ids=new Set()
 for(const line of lines){
  if(!Number.isSafeInteger(line.skuId)||line.skuId<1)return '请选择商品'
  if(!Number.isSafeInteger(line.quantity)||line.quantity<1||line.quantity>1000000000)return '数量须为1—十亿的整数'
  if(ids.has(line.skuId))return '同一单据不能重复选择SKU'
  ids.add(line.skuId)
 }
 return ''
}
