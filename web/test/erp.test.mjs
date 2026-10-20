import test from 'node:test'
import assert from 'node:assert/strict'
import {documentActions,documentActionPrompt,validateDocumentLines,documentKinds,documentStatusLabel,eventLabels} from '../src/utils/erp.mjs'

test('confirmation prompts name the inventory effect without duplicate confirmation words',()=>{
 const effects={IN:'采购入库',SALE:'预占库存',SR:'销售退货入库',PR:'采购退货出库',COUNT:'盘点过账'}
 for(const [kind,effect] of Object.entries(effects)){
  assert.equal(documentActionPrompt(kind,'confirm'),`确定${effect}吗？库存、流水和单据状态将在同一事务处理。`)
  assert.doesNotMatch(documentActionPrompt(kind,'confirm'),/确认确认/)
 }
 assert.equal(documentActionPrompt('SALE','ship'),'确定销售出库吗？库存、流水和单据状态将在同一事务处理。')
 assert.equal(documentActionPrompt('SALE','cancel'),'确定取消单据吗？库存、流水和单据状态将在同一事务处理。')
 assert.throws(()=>documentActionPrompt('IN','delete'),/不支持的单据动作/)
})
test('posted/fulfilled documents have no direct edit/cancel action',()=>{
 assert.deepEqual(documentActions('IN','POSTED'),[])
 assert.deepEqual(documentActions('SALE','SHIPPED'),[])
 assert.deepEqual(documentActions('SALE','RESERVED'),['ship','cancel'])
 assert.deepEqual(documentActions('SALE','CANCELLED'),[])
})
test('corrections/count use distinct types and only drafts may post',()=>{
 assert.equal(documentKinds.salesReturn,'SR');assert.equal(documentKinds.purchaseReturn,'PR');assert.equal(documentKinds.count,'COUNT')
 for(const kind of ['SR','PR','COUNT']){assert.deepEqual(documentActions(kind,'DRAFT'),['confirm','cancel']);assert.deepEqual(documentActions(kind,'POSTED'),[]);assert.equal(documentStatusLabel(kind,'POSTED'),'已过账');assert.ok(eventLabels[kind])}
})
test('count zero is explicit and ordinary lines stay positive integers',()=>{
 assert.equal(validateDocumentLines([{skuId:1,countedQuantity:0}],'COUNT'),'')
 for(const value of [undefined,null,-1,1.2,1000000001])assert.notEqual(validateDocumentLines([{skuId:1,countedQuantity:value}],'COUNT'),'')
 assert.notEqual(validateDocumentLines([{skuId:1,quantity:0}],'SR'),'')
})
test('document lines reject decimals, missing values, duplicate SKUs and oversized batches',()=>{
 assert.equal(validateDocumentLines([{skuId:1,quantity:100}]),'')
 for(const lines of [[],[{skuId:1,quantity:0}],[{skuId:1,quantity:1.5}],[{skuId:1,quantity:1000000001}],[{skuId:1,quantity:1},{skuId:1,quantity:2}],Array.from({length:101},(_,i)=>({skuId:i+1,quantity:1}))])assert.notEqual(validateDocumentLines(lines),'')
})
