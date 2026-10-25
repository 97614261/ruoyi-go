const code = /^[A-Za-z0-9_-]{1,64}$/
const key = /^[A-Za-z0-9][A-Za-z0-9_:-]{7,63}$/
const id = v => Number.isSafeInteger(v) && v > 0
const text = (v,max,required=false) => typeof v==='string' && v===v.trim() && [...v].length<=max && !/[\x00-\x1f\x7f-\x9f]/.test(v) && (!required || v!=='')
export const logisticsKey = () => globalThis.crypto.randomUUID()
export const logisticsRefused = error => typeof error === 'string'
export function logisticsMasterPayload(form, method=false) {
 const out={requestKey:form.requestKey,version:form.version||0,code:String(form.code||'').trim(),name:String(form.name||'').trim(),status:form.status,remark:String(form.remark||'').trim(),carrierId:method?form.carrierId:0}
 if(!key.test(out.requestKey||'')||!code.test(out.code)||!text(out.name,120,true)||!text(out.remark,500)||!['0','1'].includes(out.status)||!Number.isSafeInteger(out.version)||out.version<0||(form.id?!id(out.version):out.version!==0)||method&&!id(out.carrierId)) throw Error('请核对编码、名称、状态、物流公司及版本')
 return out
}
export function packageLogisticsCommand(detail, form) {
 if(!id(detail?.packageId)||!id(detail?.version)||!key.test(form.requestKey||'')||!text(form.trackingNo,128,true))throw Error('请核对包裹、版本及文本运单号')
 const payload={version:detail.version,requestKey:form.requestKey,trackingNo:form.trackingNo}
 const mode=form.mode
 if(!['CATALOG','MANUAL'].includes(mode))throw Error('请选择物流录入方式')
 if(mode==='CATALOG'){if(!id(form.shippingMethodId))throw Error('请选择启用的配送方式');payload.shippingMethodId=form.shippingMethodId}
 else{if(!code.test(form.carrierCode||''))throw Error('物流代码只能为1—64位字母数字、_或-');payload.carrierCode=form.carrierCode}
 return logisticsCommand(mode,detail.packageId,payload)
}
export function logisticsCommand(kind,target,payload){return JSON.parse(JSON.stringify({kind,id:target||0,payload}))}
