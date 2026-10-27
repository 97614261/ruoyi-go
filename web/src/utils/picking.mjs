export const pickingKey = () => globalThis.crypto.randomUUID()
export function pickingID(raw){return typeof raw==='string'&&/^[1-9]\d*$/.test(raw)&&Number.isSafeInteger(Number(raw))?Number(raw):null}
export function recordPickingScan(sheet,counts,raw,quantity=1){
 if(!sheet?.scannable||sheet.checked||sheet.status!=='READY_TO_SHIP')throw Error('请重新读取未复核的待发货包裹')
 if(!Number.isSafeInteger(quantity)||quantity<1||quantity>1000000000)throw Error('每次数量须为正整数')
 const code=String(raw??'').trim(),matches=sheet.items.filter(r=>r.scanCode===code)
 if(!code||matches.length!==1)throw Error(matches.length>1?'扫描编码重复，请联系管理员':'错货：此编码不在当前包裹中')
 const item=matches[0],next=(counts[item.skuId]||0)+quantity
 if(next>item.quantity)throw Error('该商品已超出包裹应发数量')
 return{...counts,[item.skuId]:next}
}
export function pickingCommand(kind,sheet,counts={},requestKey=pickingKey()){
 if(!sheet||!Number.isSafeInteger(sheet.packageId)||sheet.packageId<1||!Number.isSafeInteger(sheet.version)||sheet.version<1||sheet.status!=='READY_TO_SHIP'||!sheet.scannable)throw Error('包裹状态或版本不允许操作')
 const payload={version:sheet.version,requestKey}
 if(kind==='CHECK'){
  if(sheet.checked)throw Error('当前版本已复核，请刷新')
  if(!Array.isArray(sheet.items)||sheet.items.length<1||sheet.items.length>100||sheet.items.some(r=>counts[r.skuId]!==r.quantity))throw Error('还有商品未扫齐，请逐项核对')
  payload.scans=sheet.items.map(r=>({scanCode:r.scanCode,quantity:counts[r.skuId]})).sort((a,b)=>a.scanCode.localeCompare(b.scanCode))
 }else if(kind==='SHIP'){if(!sheet.checked||!sheet.hasLogistics)throw Error('先保存运单，再完成当前包裹复核，才能扫码发货')}
 else throw Error('未知拣货动作')
 return JSON.parse(JSON.stringify({kind,id:sheet.packageId,payload}))
}
