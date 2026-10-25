export const statusLabels={DRAFT:'草稿',POSTED:'已入库',RESERVED:'已预占',SHIPPED:'已出库',CANCELLED:'已取消'}
export const actionLabels={confirm:'确认',ship:'确认出库',cancel:'取消单据'}
export const eventLabels={IN:'采购入库',RESERVE:'销售预占',RELEASE:'取消释放',OUT:'销售出库',SR:'销售退回',PR:'采购退回',COUNT:'盘点调整',TOUT:'调拨出库',TIN:'调拨入库'}
export const documentKinds={receipt:'IN',sales:'SALE',salesReturn:'SR',purchaseReturn:'PR',count:'COUNT'}
export function documentActionPrompt(kind,action){
 const postingLabels={IN:'采购入库',SALE:'预占库存',SR:'销售退货入库',PR:'采购退货出库',COUNT:'盘点过账'}
 const label=action==='confirm'?postingLabels[kind]:{ship:'销售出库',cancel:'取消单据'}[action]
 if(!label)throw new Error('不支持的单据动作')
 return `确定${label}吗？库存、流水和单据状态将在同一事务处理。`
}
export function documentStatusLabel(kind,status){return status==='POSTED'&&kind!=='IN'?'已过账':statusLabels[status]||status}
export function documentActions(kind,status){
 if(status==='DRAFT')return ['confirm','cancel']
 if(kind==='SALE'&&status==='RESERVED')return ['ship','cancel']
 return []
}
export function validateDocumentLines(lines,kind){
 if(!Array.isArray(lines)||lines.length<1||lines.length>100)return '请添加1—100行商品'
 const ids=new Set()
 for(const line of lines){
  if(!Number.isSafeInteger(line.skuId)||line.skuId<1)return '请选择商品'
  const quantity=kind==='COUNT'?line.countedQuantity:line.quantity
  if(!Number.isSafeInteger(quantity)||quantity<(kind==='COUNT'?0:1)||quantity>1000000000)return kind==='COUNT'?'实盘须为0—十亿的整数':'数量须为1—十亿的整数'
  if(ids.has(line.skuId))return '同一单据不能重复选择SKU'
  ids.add(line.skuId)
 }
 return ''
}
