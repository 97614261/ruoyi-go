export const catalogRequestKey=()=>globalThis.crypto.randomUUID()
export const blankVariant=()=>({skuId:0,skuCode:'',specification:'',unit:'件',status:'0',barcode:'',weightGrams:'',lengthMm:'',widthMm:'',heightMm:'',referenced:false})
const code=/^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$/
const text=(v,max)=>typeof v==='string'&&[...v].length<=max&&!/[\0\r\n]/.test(v)
const dimension=v=>{if(v===''||v==null)return null;if(!/^[0-9]{1,10}$/.test(String(v)))throw Error('重量和尺寸只能填写整数');const n=Number(v);if(n<1||n>1e9)throw Error('重量或尺寸应为1—1000000000，未知留空');return n}
export function catalogPayload(form,category=false){
 const out={requestKey:form.requestKey,version:form.version||0,remark:(form.remark||'').trim()}
 if(category){Object.assign(out,{categoryCode:form.categoryCode?.trim(),categoryName:form.categoryName?.trim(),status:form.status});if(!code.test(out.categoryCode)||!out.categoryName||!text(out.categoryName,120)||!['0','1'].includes(out.status))throw Error('请填写有效的分类编码、名称和状态')}
 else{
  Object.assign(out,{productCode:form.productCode?.trim(),productName:form.productName?.trim(),categoryId:form.categoryId||null,images:[...(form.images||[])]})
  if(!code.test(out.productCode)||!out.productName||!text(out.productName,120))throw Error('请填写商品编码和名称')
  if(!form.variants?.length||form.variants.length>50||out.images.length>5)throw Error('商品需要1—50个SKU，最多5张图片')
  const codes=new Set(),bars=new Set(),ids=new Set()
  out.variants=form.variants.map(v=>{
   const r={skuId:v.skuId||0,skuCode:v.skuCode?.trim(),specification:(v.specification||'').trim(),unit:v.unit?.trim(),status:v.status,barcode:v.barcode?.trim()||null}
   if(!code.test(r.skuCode)||codes.has(r.skuCode.toLowerCase())||!r.unit||!text(r.unit,20)||!text(r.specification,120)||!['0','1'].includes(r.status)||r.skuId&&ids.has(r.skuId))throw Error('SKU编码不能重复，规格、单位和状态需合法')
   codes.add(r.skuCode.toLowerCase());ids.add(r.skuId)
   if(r.barcode){if(!code.test(r.barcode)||bars.has(r.barcode))throw Error('条码不合法或重复');bars.add(r.barcode)}
   for(const k of ['weightGrams','lengthMm','widthMm','heightMm'])r[k]=dimension(v[k])
   return r
  })
 }
 if(!text(out.remark,500))throw Error('备注不超过500字且不含换行')
 return out
}
export function catalogCommand(category,id,payload){return JSON.parse(JSON.stringify({category,id:id||0,payload}))}
export function catalogImageURL(key,base=''){if(typeof key!=='string'||!/^\/profile\/tenants\/[0-9]+\/catalog\//.test(key)||/[?#\\]/.test(key))return '';return base+key}
