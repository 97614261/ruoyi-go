import test from 'node:test'
import assert from 'node:assert/strict'
import {createRecoverySession} from '../src/utils/cmsRecovery.js'
const empty={slotId:0,version:0,found:false,baseMatches:true,contentVersion:4}
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
function setup(overrides={}){
 let draft={title:'server',bodyHtml:'<p>base</p>'},now=1,next=0;const jobs=new Map(),calls=[],states=[]
 const api={get:async()=>({...empty}),put:async d=>{calls.push(d);return{...empty,slotId:10,version:1,found:true,draft:d.draft}},clear:async()=>({...empty,slotId:10,version:2}),commit:async d=>{calls.push({commit:d});return{contentId:7,version:5}},dispose:()=>{}}
 Object.assign(api,overrides)
 const clock={now:()=>now,set:(fn,ms)=>{const id=++next;jobs.set(id,{at:now+ms,fn});return id},clear:id=>jobs.delete(id)}
 const s=createRecoverySession({api,id:7,baseVersion:4,read:()=>({...draft}),apply:d=>{draft={...d}},ready:()=>true,notify:x=>states.push(x),clock})
 return{s,api,calls,states,edit:d=>{draft={...draft,...d};s.changed()},read:()=>draft,async tick(ms){now+=ms;for(const[id,j]of [...jobs])if(j.at<=now){jobs.delete(id);j.fn()};await Promise.resolve();await Promise.resolve()}}
}
test('idle editor does not write; typing debounces and never updates current form from an old response',async()=>{
 const gate=deferred(),x=setup({put:async d=>{x.calls.push(d);return gate.promise}});await x.s.open();await x.tick(5000);assert.equal(x.calls.length,0)
 x.edit({title:'first'});await x.tick(1000);x.edit({title:'second'});await x.tick(2999);assert.equal(x.calls.length,0);await x.tick(1);assert.equal(x.calls.length,1)
 x.edit({title:'third'});gate.resolve({...empty,slotId:10,version:1,found:true,draft:{title:'second'}});await Promise.resolve();await Promise.resolve();assert.equal(x.read().title,'third');assert.equal(x.s.dirty,true);x.s.close()
})
test('saved recovery needs an explicit choice; only matching base can restore',async()=>{
 const x=setup({get:async()=>({...empty,slotId:4,version:3,found:true,draft:{title:'recovered'}})});await x.s.open();x.edit({title:'local'});await x.tick(6000);assert.equal(x.calls.length,0);assert.equal(x.s.state.phase,'review');assert.equal(x.s.restore(),true);assert.equal(x.read().title,'recovered');x.s.close()
 const stale=setup({get:async()=>({...empty,slotId:4,version:3,found:true,baseMatches:false,contentVersion:5,draft:{title:'old'}})});await stale.s.open();assert.equal(stale.s.restore(),false);assert.equal(stale.read().title,'server');await assert.rejects(stale.s.commit());stale.s.close()
})
test('conflict stops future automatic writes and retains local words',async()=>{
 let n=0;const x=setup({put:async()=>{n++;const e=new Error('conflict');e.code=409;throw e}});await x.s.open();x.edit({title:'keep me'});await assert.rejects(x.s.flush());x.edit({title:'still local'});await x.tick(90000);assert.equal(n,1);assert.equal(x.s.state.phase,'conflict');assert.equal(x.read().title,'still local');await assert.rejects(x.s.commit());x.s.close()
})
test('clear is CAS-bound and does not pretend unsaved local changes were persisted',async()=>{
 const x=setup();await x.s.open();x.edit({title:'not saved'});await x.s.clear();assert.equal(x.s.dirty,true);await x.tick(3000);assert.equal(x.calls.length,1);assert.equal(x.calls[0].slotId,10);assert.equal(x.calls[0].version,2);x.s.close()
})
test('manual save waits for automatic request and uses its returned slot version',async()=>{
 const gate=deferred(),x=setup({put:async()=>gate.promise});await x.s.open();x.edit({title:'automatic'});const p=x.s.flush();x.edit({title:'latest manual'});const saving=x.s.commit();await Promise.resolve();assert.equal(x.calls.length,0);gate.resolve({...empty,slotId:8,version:9,found:true});await p;await saving;assert.equal(x.calls[0].commit.slotId,8);assert.equal(x.calls[0].commit.version,9);assert.equal(x.calls[0].commit.draft.title,'latest manual');x.s.close()
})
test('closed or replaced editor ignores late reads and never submits after a delayed request',async()=>{
 const read=deferred(),a=setup({get:()=>read.promise});const loading=a.s.open();a.s.close();read.resolve({...empty,found:true,draft:{title:'old'}});await loading;assert.equal(a.read().title,'server')
 const write=deferred(),b=setup({put:()=>write.promise});await b.s.open();b.edit({title:'last'});const pending=b.s.flush();const saving=b.s.commit();b.s.close();write.resolve({...empty,slotId:1,version:1});await pending;await assert.rejects(saving);assert.equal(b.calls.length,0)
})
test('continuous typing is bounded by the 30 second maximum',async()=>{
 const x=setup();await x.s.open();for(let i=0;i<15;i++){x.edit({title:'typing '+i});await x.tick(2000)};assert.equal(x.calls.length,1);x.s.close()
})

test('a lost clear response keeps a warning for unsaved local words',async()=>{
 const x=setup({clear:async()=>{throw new Error('network')}});await x.s.open();x.edit({title:'keep local'});await x.s.flush();assert.equal(x.s.dirty,false);await assert.rejects(x.s.clear());assert.equal(x.s.dirty,true);assert.equal(x.s.state.phase,'error');x.s.close()
})
