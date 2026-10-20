import test from 'node:test'
import assert from 'node:assert/strict'
import {documentActions,validateDocumentLines} from '../src/utils/erp.mjs'
test('posted/fulfilled documents have no direct edit/cancel action',()=>{
 assert.deepEqual(documentActions('IN','POSTED'),[])
 assert.deepEqual(documentActions('SALE','SHIPPED'),[])
 assert.deepEqual(documentActions('SALE','RESERVED'),['ship','cancel'])
 assert.deepEqual(documentActions('SALE','CANCELLED'),[])
})
test('document lines reject decimals, missing values, duplicate SKUs and oversized batches',()=>{
 assert.equal(validateDocumentLines([{skuId:1,quantity:100}]),'')
 for(const lines of [[],[{skuId:1,quantity:0}],[{skuId:1,quantity:1.5}],[{skuId:1,quantity:1000000001}],[{skuId:1,quantity:1},{skuId:1,quantity:2}],Array.from({length:101},(_,i)=>({skuId:i+1,quantity:1}))])assert.notEqual(validateDocumentLines(lines),'')
})
