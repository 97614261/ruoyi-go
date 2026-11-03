// Plain state machine; dependency-free so timing/concurrency tests can use Node's built-in runner.
export const recoveryFields=['productCode','productParams','galleryIds','slug','channelId','title','summary','bodyHtml','author','coverAssetId','coverAltText','seoTitle','seoDescription','remark']
export function recoveryDraft(form){return Object.fromEntries(recoveryFields.map(k=>[k,Array.isArray(form[k])?[...form[k]]:form[k]]))}
export function createRecoverySession({api,id,baseVersion,read,apply,ready,notify=()=>{},delay=3000,maxDelay=30000,clock={now:()=>Date.now(),set:(f,n)=>setTimeout(f,n),clear:n=>clearTimeout(n)}}){
 let closed=false,loaded=false,manual=false,timer=null,flight=null,firstDirty=0,last=JSON.stringify(read())
 let view={slotId:0,version:0,found:false,baseMatches:true,contentVersion:baseVersion}
 const original=last
 let phase='loading',message='正在核对个人恢复副本'
 const token=()=>({slotId:view.slotId,version:view.version})
 const dirty=()=>JSON.stringify(read())!==last
 function emit(p=phase,m=message){if(closed)return;phase=p;message=m;notify({phase,message,view:{...view},dirty:dirty()})}
 function stop(){if(timer!==null)clock.clear(timer);timer=null}
 function blocked(){return !loaded||['review','conflict','error'].includes(phase)}
 function fail(e){stop();emit(e.code===409?'conflict':'error',e.message||'恢复副本操作失败，当前文字仍在表单')}
 function changed(){if(closed)return;emit();if(!dirty()){stop();firstDirty=0;return};if(blocked()||manual)return;queue()}
 function queue(){if(closed||manual||blocked())return;stop();if(!firstDirty)firstDirty=clock.now();const wait=Math.max(0,Math.min(delay,maxDelay-(clock.now()-firstDirty)));timer=clock.set(()=>{timer=null;flush().catch(()=>{})},wait);emit('pending','编辑停止3秒后自动保存个人副本；连续编辑最多30秒保存一次')}
 async function open(){
  stop();emit('loading','正在核对个人恢复副本')
  try{const v=await api.get();if(closed)return;view=v;loaded=true
   if(v.contentVersion!==baseVersion){emit('conflict','服务端内容已变化，请保留当前文字并重新打开编辑');return}
   if(v.found){emit('review',v.baseMatches?'发现未提交的个人副本，请先恢复或清除后继续':'发现旧版本副本，只能查看，请先核对服务端新稿');return}
   emit('idle','个人自动保存已就绪；副本保留7天，不提交草稿、不发布');if(dirty())queue()
  }catch(e){if(!closed)fail(e);throw e}
 }
 async function flush(){
  if(closed)return;if(flight)return flight;if(blocked()){const e=new Error(message);throw e};if(!dirty())return
  stop();if(!ready()){const e=new Error('正文或图片尚未完整加载，自动保存已暂停；加载完成后点重新核对');fail(e);throw e}
  const draft=read(),sent=JSON.stringify(draft);emit('saving','正在保存个人恢复副本')
  const work=(async()=>{try{const v=await api.put({...token(),baseVersion,draft});if(closed)return;view=v;last=sent;firstDirty=0;emit('saved','个人副本已保存，尚未提交正式草稿')}catch(e){if(!closed)fail(e);throw e}finally{flight=null;if(!closed&&!manual&&!blocked()&&dirty())queue()}})();flight=work;return work
 }
 async function reload(){if(closed)return;stop();if(flight){try{await flight}catch{}};if(closed)return;loaded=false;return open()}
 function restore(){if(closed||!view.found||!view.baseMatches||view.contentVersion!==baseVersion)return false;apply(view.draft);last=JSON.stringify(read());firstDirty=0;emit('saved','个人副本已恢复到表单，尚未提交正式草稿');return true}
 async function clear(){if(closed||!loaded)return;stop();if(flight){try{await flight}catch{}};if(closed)return;last=original;try{view=await api.clear(token());if(closed)return;last=original;firstDirty=0
  if(view.contentVersion!==baseVersion){emit('conflict','副本已清除，但服务端已变化，请复制当前文字后重新打开编辑');return}
  // Retain current form but permit a subsequent edit to create a fresh recovery.
  emit('idle','个人副本已清除；表单有改动时会继续自动保存');if(dirty())queue()
 }catch(e){if(!closed)fail(e);throw e}}
 async function commit(){
  if(closed)throw new Error('编辑已关闭');stop();manual=true
  try{if(flight)await flight;if(closed)throw new Error('编辑已关闭');if(blocked())throw new Error(message);if(!ready())throw new Error('正文尚未完整加载')
   const result=await api.commit({...token(),baseVersion,draft:{...read(),contentId:id,version:baseVersion}});return result
  }catch(e){if(!closed)fail(e);throw e}finally{manual=false}
 }
 function close(){stop();closed=true;api.dispose?.()}
 return {open,changed,flush,reload,restore,clear,commit,close,get dirty(){return dirty()},get state(){return {phase,message,view}}}
}
