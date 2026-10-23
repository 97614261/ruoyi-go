import {safeID} from './purchasing.mjs'

export function purchaseReturnPayload(form) {
 if(!safeID(form.version)||typeof form.requestKey!=='string'||!form.requestKey)throw Error('请重新读取原批次版本')
 const reason=typeof form.reason==='string'?form.reason.trim():''
 if(!reason||Array.from(reason).length>500||/\p{Cc}/u.test(reason))throw Error('退货原因必填、最多500字且不可换行')
 if(!Array.isArray(form.lines)||!form.lines.length||form.lines.length>100)throw Error('原批次明细异常')
 const seen=new Set()
 const lines=form.lines.filter(l=>l.returnQuantity!==0).map(l=>{
  if(!safeID(l.lineId)||seen.has(l.lineId)||!Number.isSafeInteger(l.returnQuantity)||l.returnQuantity<1||l.returnQuantity>1000000000||!Number.isSafeInteger(l.netReceivedQuantity)||l.returnQuantity>l.netReceivedQuantity)throw Error('本次退货须为不超过本批可退量的正整数')
  seen.add(l.lineId);return {receiptLineId:l.lineId,quantity:l.returnQuantity}
 })
 if(!lines.length)throw Error('请至少填写一行本次退货数量')
 return {version:form.version,requestKey:form.requestKey,reason,lines}
}
