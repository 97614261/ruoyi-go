import test from 'node:test'
import assert from 'node:assert/strict'
import { catalogFields, catalogPayload, pendingMetadata, readPending, rateLabel, providerProfiles, yuanToMicros, microsToYuan, priceFieldErrors, configurationState, providerProfile, modelExamplesFor, applyModelPreset, modelPresets, providerSupportsImages } from '../src/utils/pod-catalog.js'

test('provider profiles bind official origin, region and protocol without accepting proxy input',()=>{
  for(const [profile,p] of Object.entries(providerProfiles)){
    const r=catalogPayload('provider.create',{providerProfile:profile,code:'local-model',name:'model',endpoint:'https://proxy.invalid'})
    assert.equal(r.data.adapterKind,p.adapterKind)
    assert.equal(r.data.region,p.region)
    if(profile!=='qwen'&&profile!=='relay')assert.equal(r.data.endpoint,p.endpoint)
  }
  assert.throws(()=>catalogPayload('provider.create',{providerProfile:'unknown'}))
  const fields=catalogFields('model.version',{kind:'text'})
  assert.equal(fields.find(f=>f.key==='officialModelId').maxlength,80)
})

test('credential pending metadata and recovery never retain submitted secrets',()=>{
  const f={id:'9',revision:'2',kind:'',reason:'rotate',sourceType:'encrypted',secret:'fake-unit-secret'}
  const w=catalogPayload('provider.credential',f),p=pendingMetadata('provider.credential',w,f,'轮换凭据')
  assert.equal(w.data.secret,f.secret)
  assert.ok(!JSON.stringify(p).includes(f.secret))
  const recovered=readPending(JSON.stringify({...p,secret:f.secret,body:w.data,envName:'DO_NOT_SAVE'}))
  assert.deepEqual(recovered,p)
})
test('pending recovery rejects cross-operation paths, invalid revisions and injected types',()=>{
  const f={id:'7',revision:'2',kind:'text',status:'disabled'},w=catalogPayload('model.status',f),p=pendingMetadata('model.status',w,f,'停用')
  assert.deepEqual(readPending(JSON.stringify(p)),p)
  for(const patch of [{path:'providers/7/status'},{method:'DELETE'},{id:{}},{revision:'0'},{kind:{}},{status:'unknown'},{path:'models/7/../../providers'}])assert.equal(readPending(JSON.stringify({...p,...patch})),null)
})
test('model requests use modelId and preserve integer prices without Number conversion',()=>{
  const f={id:'29',providerId:'3',revision:'5',kind:'text',reason:'version',officialModelId:'qwen-plus',timeoutSeconds:120,maxInputBytes:1000,maxOutputTokens:200,costInputYuan:'1000000',costOutputYuan:'0.000012',pointInput:'800',pointOutput:'24'}
  const w=catalogPayload('model.version',f)
  assert.equal(w.path,'models/29/versions')
  assert.equal(w.data.costRule.inputPerMillion,'1000000000000')
  assert.equal(w.data.costRule.perImage,'0')
  assert.equal(w.data.pointRule.currency,undefined)
  assert.ok(rateLabel(w.data.costRule).includes('输入 1000000'))
  assert.throws(()=>catalogPayload('model.version',{...f,costInputYuan:'1.0000001'}))
  assert.throws(()=>catalogPayload('invalid',f))
})
test('image rates are exclusive and defaults explicitly clear absent selections',()=>{
  const w=catalogPayload('model.version',{id:'2',kind:'image',costImageYuan:'0.00002',pointImage:'2',costInputYuan:'should-not-leak',maxImages:4,ratios:['1:1']})
  assert.equal(w.data.costRule.inputPerMillion,'0')
  assert.equal(w.data.costRule.perImage,'20')
  assert.deepEqual(catalogPayload('defaults.update',{}).data,{reason:undefined,revision:undefined,textModelId:'',imageModelId:''})
})


test('CNY fields round trip exact decimal money and reject ambiguous or out of range values',()=>{
  for(const [yuan,micros] of [['0','0'],['0.15','150000'],['0.000001','1'],['0.10','100000'],['1000000','1000000000000']]){
    assert.equal(yuanToMicros(yuan),micros)
    assert.equal(yuanToMicros(microsToYuan(micros)),micros)
  }
  for(const value of ['',null,0,'-1','1e2','1,000','0.0000001','NaN','Infinity','1000000.000001'])assert.throws(()=>yuanToMicros(value))
  assert.throws(()=>yuanToMicros('1000.000001','1000000000'))
  assert.equal(microsToYuan('9007199254740993'),'9007199254.740993')
  const errors=priceFieldErrors({kind:'text',costInputYuan:'bad',costOutputYuan:'',pointInput:'2.2',pointOutput:'3'})
  assert.deepEqual(Object.keys(errors),['costInputYuan','pointInput','costOutputYuan'])
})
test('configuration states explain all missing pieces without asserting real connectivity',()=>{
  const row={providerCredentialConfigured:false,currentVersion:'0',providerStatus:'disabled',status:'disabled',visible:true}
  assert.equal(configurationState(row).label,'缺 API Key')
  assert.match(configurationState(row).detail,/定价/)
  assert.match(configurationState(row).detail,/供应商停用/)
  assert.match(configurationState({...row,providerCredentialConfigured:true,currentVersion:'1',providerStatus:'enabled',status:'enabled'}).detail,/未验证真实联通/)
  assert.equal(configurationState({credentialConfigured:false,status:'disabled'},true).label,'缺 API Key')
  assert.equal(providerProfile('https://api.deepseek.com'),'deepseek')
  assert.equal(providerProfile('https://proxy.invalid'),'')
  assert.deepEqual(modelExamplesFor('deepseek','image'),[])
  assert.equal(catalogFields('model.create',{providerProfile:'deepseek'}).find(f=>f.key==='kind').options.length,1)
})

test('Bailian examples include hosted text models and exclude image translation from generation',()=>{
  const text=modelExamplesFor('qwen','text')
  for(const id of ['qwen3.8-flash','qwen3.8-27b','qwen3.8-omni-flash','deepseek-v4.1-flash'])assert.ok(text.includes(id))
  assert.ok(modelExamplesFor('qwen','image').includes('qwen-image-2.1-pro'))
  assert.ok(!modelExamplesFor('qwen','image').includes('qwen-mt-image-2.0'))
})


test('custom channels retain their own base and independent protocol options',()=>{
  const w=catalogPayload('provider.create',{providerProfile:'relay',region:'cn',endpoint:'https://relay.example.com/v1',code:'my-channel',name:'My channel'})
  assert.equal(w.data.endpoint,'https://relay.example.com/v1')
  assert.equal(w.data.adapterKind,'openai-compatible-v1')
  assert.equal(w.data.region,'cn')
  assert.equal(providerProfile(w.data.endpoint,w.data.adapterKind),'relay')
  assert.ok(providerSupportsImages('relay'))
  assert.ok(providerSupportsImages('zhipu'))
  const r=catalogPayload('model.version',{id:'7',kind:'text',textApi:'responses',costInputYuan:'0.8',costOutputYuan:'2.8',pointInput:'80',pointOutput:'280',maxInputBytes:512,maxOutputTokens:64})
  assert.equal(r.data.capabilities.textApi,'responses')
  const image=catalogPayload('model.version',{id:'8',kind:'image',costImageYuan:'0.1',pointImage:'10',maxImages:1,ratios:['3:4'],imageQuality:'low',imageResponse:'url','size_3:4':'1024x1536'})
  assert.deepEqual(image.data.capabilities.imageSizes,{'3:4':'1024x1536'})
  assert.equal(image.data.capabilities.imageQuality,'low')
  assert.equal(image.data.capabilities.imageResponse,'url')
  assert.equal(image.data.capabilities.textApi,undefined)
})

test('reviewed presets keep unknown prices blank and free models explicit',()=>{
  const f={kind:'image',providerProfile:'zhipu',maxImages:4,maxReferenceImages:3,ratios:['1:1','16:9'],costImageYuan:'999'}
  applyModelPreset(f,'glm-cogview-3-flash')
  assert.equal(f.costImageYuan,'0')
  assert.equal(f.maxImages,1)
  assert.equal(f.maxReferenceImages,0)
  assert.equal(f.officialModelId,'cogview-3-flash')
  f.providerProfile='openai';applyModelPreset(f,'openai-dalle3')
  assert.equal(f.costImageYuan,'')
  assert.ok(priceFieldErrors(f).costImageYuan)
  assert.ok(modelPresets.every(p=>p.source.startsWith('https://')&&p.checkedAt))
  assert.equal(modelPresets.filter(p=>p.cost===null).length,4)
})

test('Ark has a fixed official endpoint and Seedream single-image pricing',()=>{
  const w=catalogPayload('provider.create',{providerProfile:'volcengine',endpoint:'https://ignored.example.com',code:'volcengine-ark',name:'火山方舟'})
  assert.equal(w.data.endpoint,'https://ark.cn-beijing.volces.com')
  assert.equal(w.data.region,'cn-beijing')
  assert.equal(providerProfile(w.data.endpoint,w.data.adapterKind),'volcengine')
  assert.ok(providerSupportsImages('volcengine'))
  const f={providerProfile:'volcengine',kind:'image'}
  applyModelPreset(f,'volcengine-seedream-5-0-flash')
  assert.equal(f.costImageYuan,'0.12')
  assert.equal(f.pointImage,'12')
  assert.equal(f.maxImages,1)
  assert.equal(f.maxReferenceImages,3)
})

test('real response status requires current version evidence from the server',()=>{
  const row={providerCredentialConfigured:true,currentVersion:'1',providerStatus:'enabled',status:'enabled',visible:true,verifiedAt:'2026-11-17 12:00:00'}
  assert.equal(configurationState(row).label,'真实响应通过')
  assert.match(configurationState(row).detail,/2026-11-17/)
  assert.equal(configurationState({...row,verifiedAt:undefined}).label,'配置就绪')
  assert.equal(configurationState({...row,providerCredentialConfigured:false}).label,'缺 API Key')
})
