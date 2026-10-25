import {price,safeID} from './purchasing.mjs'
export const salesFinanceTitles={account:'资金账户',receivable:'采购应收',collection:'采购收款',statement:'店铺对账'}
export const salesFinanceStatuses={POSTED:'已登记',REVERSED:'已冲销','0':'启用','1':'停用'}
const max=99999999999999999999n
const units=v=>BigInt(price(v).replace('.',''))
const fixed=n=>{if(n<0n||n>max)throw Error('金额超出DECIMAL(20,4)范围');const s=n.toString().padStart(5,'0');return s.slice(0,-4)+'.'+s.slice(-4)}
export function salesFinanceSum(rows){let n=0n;for(const r of rows){n+=units(r.amount);fixed(n)}return fixed(n)}
function token(form,version=false){if(!/^[A-Za-z0-9_:-]{8,64}$/.test(form.requestKey||'')||version&&!safeID(form.version))throw Error('请刷新核对版本或请求标识');return version?{version:form.version,requestKey:form.requestKey}:{requestKey:form.requestKey}}
function text(v,max,required=false){if(typeof v!=='string'||[...v].length>max||v!==v.trim()||/[\x00-\x1f\x7f]/.test(v)||required&&!v)throw Error('请核对文字字段长度及首尾空格');return v}
export function financeAccountPayload(f,edit){const out={...token(f,edit),accountCode:text(f.accountCode,64,true),accountName:text(f.accountName,120,true),accountType:f.accountType,currency:String(f.currency||'').toUpperCase(),status:f.status,remark:text(f.remark||'',500)};if(!['BANK','CASH','OTHER'].includes(out.accountType)||!['0','1'].includes(out.status)||!/^[A-Z]{3}$/.test(out.currency))throw Error('账户类型、币种或状态不合法');return out}
export function salesFinanceCollectionPayload(f){
 const out={...token(f),accountId:safeID(f.accountId),shopId:safeID(f.shopId),currency:String(f.currency||'').toUpperCase(),remark:text(f.remark||'',500)}
 if(!out.accountId||!out.shopId||!/^[A-Z]{3}$/.test(out.currency)||!Array.isArray(f.allocations)||!f.allocations.length||f.allocations.length>100)throw Error('请选择账户及同店铺同币种的1—100张应收')
 const seen=new Set();out.allocations=f.allocations.map(l=>{if(!safeID(l.receivableId)||!safeID(l.version)||seen.has(l.receivableId))throw Error('核销应收须唯一且有当前版本');seen.add(l.receivableId);const amount=price(l.amount);if(amount==='0.0000')throw Error('核销金额须大于0');return{receivableId:l.receivableId,version:l.version,amount}}).sort((a,b)=>a.receivableId-b.receivableId)
 out.amount=salesFinanceSum(out.allocations);return out
}
export function salesFinanceAdjustPayload(f,selected){if(selected.length!==1||!safeID(selected[0].returnId)||selected[0].registered)throw Error('每次选择一笔尚未登记的原订单退货');return{...token(f,true),returnId:selected[0].returnId,amount:price(f.adjustAmount),reason:text(f.adjustReason,500,true)}}
export function salesFinanceReversePayload(f){return{...token(f,true),reason:text(f.reason,500,true)}}
export function salesFinanceSourcePayload(f){if(!safeID(f.orderId))throw Error('请选择已过账已发货订单');return{...token(f),orderId:f.orderId}}
