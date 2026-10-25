import {price,safeID} from './purchasing.mjs'
export const financeTitles={account:'资金账户',payable:'采购应付',payment:'采购付款',statement:'供应商对账'}
export const financeStatuses={POSTED:'已登记',REVERSED:'已冲销','0':'启用','1':'停用'}
const max=99999999999999999999n
const units=v=>BigInt(price(v).replace('.',''))
const fixed=n=>{if(n<0n||n>max)throw Error('金额超出DECIMAL(20,4)范围');const s=n.toString().padStart(5,'0');return s.slice(0,-4)+'.'+s.slice(-4)}
export function financeSum(rows){let n=0n;for(const r of rows){n+=units(r.amount);fixed(n)}return fixed(n)}
function token(form,version=false){if(!/^[A-Za-z0-9_:-]{8,64}$/.test(form.requestKey||'')||version&&!safeID(form.version))throw Error('请刷新核对版本或请求标识');return version?{version:form.version,requestKey:form.requestKey}:{requestKey:form.requestKey}}
function text(v,max,required=false){if(typeof v!=='string'||[...v].length>max||v!==v.trim()||/[\x00-\x1f\x7f]/.test(v)||required&&!v)throw Error('请核对文字字段长度及首尾空格');return v}
export function financeAccountPayload(f,edit){const out={...token(f,edit),accountCode:text(f.accountCode,64,true),accountName:text(f.accountName,120,true),accountType:f.accountType,currency:String(f.currency||'').toUpperCase(),status:f.status,remark:text(f.remark||'',500)};if(!['BANK','CASH','OTHER'].includes(out.accountType)||!['0','1'].includes(out.status)||!/^[A-Z]{3}$/.test(out.currency))throw Error('账户类型、币种或状态不合法');return out}
export function financePaymentPayload(f){
 const out={...token(f),accountId:safeID(f.accountId),supplierId:safeID(f.supplierId),currency:String(f.currency||'').toUpperCase(),remark:text(f.remark||'',500)}
 if(!out.accountId||!out.supplierId||!/^[A-Z]{3}$/.test(out.currency)||!Array.isArray(f.allocations)||!f.allocations.length||f.allocations.length>100)throw Error('请选择账户及同供应商同币种的1—100张应付')
 const seen=new Set();out.allocations=f.allocations.map(l=>{if(!safeID(l.payableId)||!safeID(l.version)||seen.has(l.payableId))throw Error('核销应付须唯一且有当前版本');seen.add(l.payableId);const amount=price(l.amount);if(amount==='0.0000')throw Error('核销金额须大于0');return{payableId:l.payableId,version:l.version,amount}}).sort((a,b)=>a.payableId-b.payableId)
 out.amount=financeSum(out.allocations);return out
}
export function financeAdjustPayload(f,selected){const ids=selected.map(r=>r.returnId);if(!ids.length||ids.length>100||ids.some(id=>!safeID(id))||new Set(ids).size!==ids.length||selected.some(r=>r.registered))throw Error('请选择未登记的1—100笔原批退货');return{...token(f,true),returnIds:ids.sort((a,b)=>a-b)}}
export function financeReversePayload(f){return{...token(f,true),reason:text(f.reason,500,true)}}
export function financeSourcePayload(f){if(!safeID(f.receiptId))throw Error('请选择已过账收货批次');return{...token(f),receiptId:f.receiptId}}
