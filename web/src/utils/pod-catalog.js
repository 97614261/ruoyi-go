import { createUUID } from './uuid.js'
import modelPresets from './pod-model-presets.json' with { type: 'json' }
export { modelPresets }
const select = (key,label,values) => ({ key,label,type:'select',options:values.map(([value,label])=>({value,label})),required:true })
const input = (key,label,extra={}) => ({key,label,required:true,...extra})
const number = (key,label,min,max) => ({key,label,type:'number',min,max})
export const providerProfiles={
  qwen:{label:'阿里云百炼 / 千问',adapterKind:'qwen-v1',region:'cn-beijing',endpoint:'https://dashscope.aliyuncs.com'},
  deepseek:{label:'DeepSeek',adapterKind:'openai-chat-v1',region:'cn',endpoint:'https://api.deepseek.com'},
  zhipu:{label:'智谱 GLM',adapterKind:'openai-chat-v1',region:'cn',endpoint:'https://open.bigmodel.cn'},
  siliconflow:{label:'硅基流动',adapterKind:'openai-chat-v1',region:'cn',endpoint:'https://api.siliconflow.cn'},
  kimi:{label:'Kimi / Moonshot',adapterKind:'openai-chat-v1',region:'cn',endpoint:'https://api.moonshot.cn'},
  volcengine:{label:'火山引擎 / 火山方舟',adapterKind:'openai-chat-v1',region:'cn-beijing',endpoint:'https://ark.cn-beijing.volces.com'},
  relay:{label:'自定义 / OpenAI 兼容渠道',adapterKind:'openai-compatible-v1',region:'global',endpoint:''},
  openai:{label:'OpenAI',adapterKind:'openai-chat-v1',region:'global',endpoint:'https://api.openai.com'},
}
const qwenExamples={text:['qwen-flash','qwen-plus','qwen3.8-flash','qwen3.8-27b','qwen3.8-omni-flash','deepseek-v4.1-flash'],image:['qwen-image-2.1-turbo','qwen-image-2.1-pro','qwen-image-2.0','qwen-image-2.0-pro']}
export function providerProfile(endpoint,adapterKind){if(adapterKind==='openai-compatible-v1')return 'relay';return Object.entries(providerProfiles).find(([,p])=>p.endpoint && p.endpoint===endpoint)?.[0]||(/^https:\/\/[a-zA-Z0-9-]+\.cn-beijing\.maas\.aliyuncs\.com$/.test(endpoint||'')?'qwen':'')}
export function modelExamplesFor(profile,kind){return profile==='qwen'?qwenExamples[kind]||[]:modelPresets.filter(p=>p.provider===profile&&p.kind===kind).map(p=>p.officialModelId)}
export function providerSupportsImages(profile){return ['qwen','zhipu','siliconflow','volcengine','openai','relay'].includes(profile)}
export function applyModelPreset(f,code){const p=modelPresets.find(p=>p.code===code&&p.provider===f.providerProfile&&p.kind===f.kind);if(!p)return;for(const key of ['maxImages','maxReferenceImages','maxPromptChars','maxInputBytes','maxOutputTokens','ratios'])delete f[key];const points=value=>value===undefined?'0':((BigInt(yuanToMicros(value))+9999n)/10000n).toString();Object.assign(f,p.capabilities,{presetId:p.code,officialModelId:p.officialModelId,textApi:'',imageQuality:'',imageResponse:'',costInputYuan:p.cost?.input||'',costOutputYuan:p.cost?.output||'',costImageYuan:p.cost?.image||'',pointInput:points(p.cost?.input),pointOutput:points(p.cost?.output),pointImage:points(p.cost?.image)});for(const ratio of ['1:1','3:4','4:3','9:16','16:9'])delete f['size_'+ratio]}
export function yuanToMicros(value,max='1000000000000'){
  if(typeof value!=='string'||!/^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$/.test(value.trim()))throw new Error('请填写非负金额，最多六位小数，不使用逗号或科学计数法')
  const [whole,fraction='']=value.trim().split('.'),n=BigInt(whole)*1000000n+BigInt(fraction.padEnd(6,'0'))
  if(n>BigInt(max))throw new Error(`金额超过上限 ${microsToYuan(max)} 元`)
  return n.toString()
}
export function microsToYuan(value){if(!/^[0-9]+$/.test(String(value)))return '待核对';const n=BigInt(value),fraction=(n%1000000n).toString().padStart(6,'0').replace(/0+$/,'');return `${n/1000000n}${fraction?'.'+fraction:''}`}
export function priceFieldErrors(f){const errors={};for(const suffix of f.kind==='image'?['Image']:['Input','Output']){try{yuanToMicros(f['cost'+suffix+'Yuan'],suffix==='Image'?'1000000000':'1000000000000')}catch(e){errors['cost'+suffix+'Yuan']=e.message}if(!/^(0|[1-9][0-9]*)$/.test(f['point'+suffix]||''))errors['point'+suffix]='点数须为非负整数'}return errors}
const catalogCodeRules={
  'provider.create':{pattern:/[^a-z0-9_-]/,hint:'3—64 位，以小写字母开头；仅支持小写字母、数字、下划线 (_) 和半角连字符 (-)。',example:'bian-image'},
  'model.create':{pattern:/[^a-z0-9_.-]/,hint:'3—64 位，以小写字母开头；仅支持小写字母、数字、下划线 (_)、半角连字符 (-) 和点号 (.)。',example:'bbroot-gpt-image-2.6-sunburst'},
}
export function catalogFieldErrors(op,f){
  if(op==='model.version')return priceFieldErrors(f)
  if(op==='provider.update'&&f.canEditEndpoint&&!f.endpoint?.trim())return{endpoint:'请输入 HTTPS Base URL'}
  const rule=catalogCodeRules[op],code=f.code
  return rule&&(typeof code!=='string'||code.length<3||code.length>64||!/^[a-z]/.test(code)||rule.pattern.test(code))?{code:rule.hint}:{}
}
export function configurationState(row,provider=false){
  const reasons=[]
  if((provider?row.credentialConfigured:row.providerCredentialConfigured)===false)reasons.push('缺 API Key')
  if(!provider && (!row.currentVersion || row.currentVersion==='0'))reasons.push('缺能力与定价版本')
  if(!provider && row.providerStatus!=='enabled')reasons.push('供应商停用')
  if(row.status!=='enabled')reasons.push(provider?'供应商停用':'模型停用')
  if(!provider && !row.visible)reasons.push('未向用户展示')
  return{label:reasons[0]||(row.verifiedAt?'真实响应通过':'配置就绪'),detail:reasons.join('；')||(row.verifiedAt?`当前模型版本和凭据已有成功响应：${row.verifiedAt}`:'已配置，可供获授权的用户选择；未验证真实联通'),type:reasons.length?'info':'success'}
}
export function catalogFields(op,f){
  const name=input('name','名称',{maxlength:100}),rule=catalogCodeRules[op],code=input('code','固定编码',{maxlength:64,placeholder:rule?.example,hint:rule&&`${rule.hint}保存后不可修改。`})
  const modelMeta=[{key:'visible',label:'展示标记',type:'switch',hint:'启用、版本和本人模型授权均满足时，创作端才可选择。'},number('sort','排序',0,1000000)]
  if(op==='provider.create')return[code,name,select('providerProfile','厂商 / 渠道',Object.entries(providerProfiles).map(([key,p])=>[key,p.label])),...(f.providerProfile==='relay'?[select('region','部署地域',[['cn','中国大陆'],['global','海外 / 全球']])]:[]),input('endpoint','HTTPS Base URL',{maxlength:200,placeholder:f.providerProfile==='relay'?'https://你的中转域名/v1':'',hint:f.providerProfile==='relay'?'填写服务文档中的 Base URL，包含 /v1 等前缀，不填 /chat/completions；公网 HTTPS 域名，不带密钥、查询参数、端口或末尾斜线。':'固定官方根地址；千问可使用北京工作空间专属根域名。',readonly:!['qwen','relay'].includes(f.providerProfile||'qwen')})]
  if(op==='provider.update')return[name,input('endpoint','HTTPS Base URL',{maxlength:200,readonly:!f.canEditEndpoint,hint:f.canEditEndpoint?'只改原域名下的路径（如补 /v1）。保存会为该供应商的模型生成新版本；换域名请新建供应商。':f.providerProfile==='relay'?'修改地址还需要模型编辑权限。':'官方接口地址固定。'})]
  if(op==='model.create')return[code,name,{key:'providerId',label:'供应商',type:'remote',required:true},select('kind','类型',f.providerProfile&&!providerSupportsImages(f.providerProfile)?[['text','文字']]:[['text','文字'],['image','图片']]),...modelMeta]
  if(op==='model.update')return[name,...modelMeta]
  if(op==='provider.credential')return[select('sourceType','来源',[['encrypted','加密保存'],['env','环境变量引用']]),f.sourceType==='env'?input('envName','环境变量名',{maxlength:64,hint:'只允许部署白名单中的变量名，不能填写变量值。'}):input('secret','API Key',{maxlength:512,hint:'不回显旧值，不保存到浏览器；提交后清空输入。'})]
  if(op==='defaults.update')return[{key:'textModelId',label:'默认文字模型',type:'remote',clearable:true},{key:'imageModelId',label:'默认图片模型',type:'remote',clearable:true}]
  if(op==='model.version'){
    const image=f.kind==='image',examples=modelExamplesFor(f.providerProfile,f.kind),fields=[input('officialModelId','官方模型 ID',{maxlength:80,placeholder:examples[0]||'填写本厂商官方模型 ID',hint:`${providerProfiles[f.providerProfile]?.label||'对应厂商'}${examples.length?'，示例：'+examples.join(' / '):''}。示例不代表本账号已开通；请核对当前型号。${f.providerProfile==='qwen'?'图片型号还需核对参考图与生成数量限制。':f.providerProfile==='relay'?'须核对中转站支持的协议，计费与官方厂商独立。':'按所选型号限制参考图、张数和比例。'}`}),number('timeoutSeconds','超时 / 秒',10,300)]
    const presets=modelPresets.filter(p=>p.provider===f.providerProfile&&p.kind===f.kind)
    if(presets.length)fields.unshift(select('presetId','型号预设',presets.map(p=>[p.code,p.name])))
    const preset=presets.find(p=>p.code===f.presetId)
    if(preset)fields.push({key:'presetNote',label:'核实记录',type:'note',text:`${preset.checkedAt} · ${preset.cost?'已核实人民币报价':'缺人民币换算价格'}。${preset.note||''}`,source:preset.source})
    fields.push(...(image?[number('maxPromptChars','提示词字数上限',1,1200),number('maxImages','最多输出图片',1,4),number('maxReferenceImages','最多参考图片',0,3),{key:'ratios',label:'允许比例',type:'ratios'}]:[number('maxInputBytes','输入字节上限',1,32000),number('maxOutputTokens','输出 Token 上限',1,2048)]))
    if(f.providerProfile==='relay'||f.providerProfile==='openai'){if(!image)fields.push(select('textApi','文字接口',[['','Chat · max_tokens'],['chat-completion-tokens','Chat · max_completion_tokens'],['responses','Responses']]));else if(f.providerProfile==='relay'){fields.push({...select('imageResponse','图片结果',[['','自动识别'],['url','URL 链接'],['b64_json','Base64 · 直接存本地']]),hint:'按渠道文档选择。Base64 无需二次下载，图片由服务器保存；不支持的格式不会自动重试。'});fields.push(select('imageQuality','图片质量',[['','不传 quality'],['auto','auto'],['low','low'],['medium','medium'],['high','high'],['standard','standard'],['hd','hd']]));for(const ratio of f.ratios||[])fields.push(input('size_'+ratio,`尺寸 · ${ratio}`,{required:false,maxlength:9,placeholder:'如 1024x1536',hint:'可选；不填用标准 Images API 尺寸。3:4、4:3 须填写本渠道支持的尺寸。'}))}}
    const rates=image?[['Image','每张图片']]:[['Input','每百万输入 Token'],['Output','每百万输出 Token']]
    for(const [key,label] of rates)fields.push(input(`cost${key}Yuan`,`估算成本 · 元/${key==='Image'?'张':key==='Input'?'百万输入 Token':'百万输出 Token'}`,{decimal:true,hint:'填写人民币金额，最多六位小数；保存时自动转换，免费模型请明确填 0。外币先核实换算口径；图片按当前质量和全部已选尺寸的最高单张价填写，避免低估预占。'}))
    for(const [key,label] of rates)fields.push(input(`point${key}`,`点数 · ${label}`,{numeric:true,hint:'非负整数；型号预设按每元 100 点初始化，可自行调整。与人民币及测试额度独立记账。'}))
    return fields
  }
  return[]
}
export function catalogPayload(op,f){
  if(!operations.has(op))throw new Error('未知管理操作')
  if(!op.endsWith('.create') && op!=='defaults.update' && !/^[1-9][0-9]*$/.test(f.id))throw new Error('目标编号无效')
  const provider=op.startsWith('provider.'),create=op.endsWith('.create')
  let method='POST',path=provider?'providers':'models',data={reason:f.reason}
  if(!create){path+=`/${f.id}`;data.revision=f.revision}
  if(op==='provider.create'){const p=providerProfiles[f.providerProfile||'qwen'];if(!p)throw new Error('供应商厂商无效');Object.assign(data,{code:f.code,name:f.name,adapterKind:p.adapterKind,region:f.providerProfile==='relay'?(f.region||'global'):p.region,endpoint:['qwen','relay'].includes(f.providerProfile||'qwen')?(f.endpoint||p.endpoint):p.endpoint})}
  if(op==='provider.update'){method='PUT';data.name=f.name;if(f.canEditEndpoint&&f.endpoint!==f.originalEndpoint)data.endpoint=f.endpoint?.trim()}
  if(op.endsWith('.status')){path+='/status';data.status=f.status}
  if(op==='provider.credential'){method='PUT';path+='/credential';data.sourceType=f.sourceType;if(f.sourceType==='encrypted')data.secret=f.secret;else data.envName=f.envName}
  if(op==='model.create')Object.assign(data,{code:f.code,name:f.name,providerId:f.providerId,kind:f.kind,visible:f.visible,sort:f.sort})
  if(op==='model.update'){method='PUT';Object.assign(data,{name:f.name,visible:f.visible,sort:f.sort})}
  if(op==='model.version'){
    path+='/versions';const image=f.kind==='image'
    const errors=priceFieldErrors(f);if(Object.keys(errors).length)throw new Error(Object.values(errors)[0])
    const value=(suffix,unit)=>unit==='micros'?yuanToMicros(f['cost'+suffix+'Yuan'],image?'1000000000':'1000000000000'):f['point'+suffix]
    const rate=(_prefix,unit)=>({version:1,...(unit==='micros'?{currency:'CNY'}:{}),unit,inputPerMillion:image?'0':value('Input',unit),outputPerMillion:image?'0':value('Output',unit),perImage:image?value('Image',unit):'0'})
    Object.assign(data,{officialModelId:f.officialModelId,timeoutSeconds:f.timeoutSeconds,capabilities:image?{maxPromptChars:f.maxPromptChars,maxImages:f.maxImages,maxReferenceImages:f.maxReferenceImages,ratios:f.ratios,...(f.imageQuality?{imageQuality:f.imageQuality}:{}),...(f.imageResponse?{imageResponse:f.imageResponse}:{}),...(f.ratios?.some(r=>f['size_'+r])?{imageSizes:Object.fromEntries(f.ratios.filter(r=>f['size_'+r]).map(r=>[r,f['size_'+r]]))}:{})}:{maxInputBytes:f.maxInputBytes,maxOutputTokens:f.maxOutputTokens,...(f.textApi?{textApi:f.textApi}:{})},costRule:rate('cost','micros'),pointRule:rate('point','points')})
  }
  if(op==='defaults.update'){method='PUT';path='model-defaults';Object.assign(data,{textModelId:f.textModelId||'',imageModelId:f.imageModelId||''})}
  return{method,path,data}
}
const operations=new Set(['provider.create','provider.update','provider.credential','provider.status','model.create','model.update','model.status','model.version','defaults.update'])
export function pendingMetadata(op,w,f,label){return{requestId:createUUID(),path:w.path,method:w.method,op,revision:f.revision||'',id:f.id||'',kind:f.kind||'',status:op.endsWith('.status')?f.status:'',label}}
export function readPending(raw){
  try{
    const p=JSON.parse(raw), decimal=s=>typeof s==='string' && /^[1-9][0-9]{0,18}$/.test(s)
    if(!p || !operations.has(p.op) || typeof p.requestId!=='string' || !/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(p.requestId) || p.requestId==='00000000-0000-0000-0000-000000000000')return null
    const create=p.op.endsWith('.create'), defaults=p.op==='defaults.update'
    if((!create && !decimal(p.revision)) || (create && p.revision!=='') || (!create && !defaults && !decimal(p.id)) || ((create || defaults) && p.id!==''))return null
    if(!['','text','image'].includes(p.kind) || (p.op.endsWith('.status') && !['enabled','disabled'].includes(p.status)))return null
    const expected=catalogPayload(p.op,{id:p.id,reason:'',kind:p.kind,costInputYuan:'0',costOutputYuan:'0',pointInput:'0',pointOutput:'0',costImageYuan:'0',pointImage:'0'})
    if(p.path!==expected.path || p.method!==expected.method)return null
    return{requestId:p.requestId,path:p.path,method:p.method,op:p.op,revision:p.revision,id:p.id,kind:p.kind,status:p.op.endsWith('.status')?p.status:'',label:typeof p.label==='string' && p.label.length<=100 && !/[<>\r\n]/.test(p.label)?p.label:'管理操作'}
  }catch{return null}
}
export function capabilityLabel(c){const extra=[c.imageResponse?`图片结果 ${c.imageResponse}`:'',c.textApi?`接口 ${c.textApi}`:'',c.imageQuality?`质量 ${c.imageQuality}`:'',c.imageSizes?Object.entries(c.imageSizes).map(([r,s])=>`${r}=${s}`).join(' / '):''].filter(Boolean).join('；');return ( c.maxInputBytes?`输入 ≤ ${c.maxInputBytes} 字节，输出 ≤ ${c.maxOutputTokens} Token`:`提示词 ≤ ${c.maxPromptChars} 字，输出 ≤ ${c.maxImages} 张，参考 ≤ ${c.maxReferenceImages} 张；比例 ${(c.ratios||[]).join(' / ')}`)+(extra?`；${extra}`:'')}
export function rateLabel(r,kind){const money=r.unit==='micros',unit=money?'元':'点',format=v=>money?microsToYuan(v):v;return kind==='image'||r.perImage!=='0'?`每张图片 ${format(r.perImage)} ${unit}`:`每百万 Token：输入 ${format(r.inputPerMillion)}，输出 ${format(r.outputPerMillion)} ${unit}`}
