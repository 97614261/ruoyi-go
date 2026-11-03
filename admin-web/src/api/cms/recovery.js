// Quiet, session-bound requests: never put private draft text into repeat-submit storage or URLs.
import axios from 'axios'
import { getToken } from '@/utils/auth'
export function recoveryAPI(kind,id){
 const identity=getToken(),cancel=new AbortController(),client=axios.create({baseURL:import.meta.env.VITE_APP_BASE_API,timeout:10000})
 async function call(method,action='',data){
  if(!identity||getToken()!==identity){const e=new Error('登录会话已变化，请复制当前文字后重新打开编辑');e.code=403;throw e}
  try{const r=await client.request({url:`/cms/${kind}/${id}/autosave${action}`,method,data,signal:cancel.signal,headers:{Authorization:'Bearer '+identity,'Content-Type':'application/json'}});if(r.data.code!==200){const e=new Error(r.data.msg||'恢复副本请求失败');e.code=r.data.code;throw e};return r.data.data}
  catch(raw){const e=new Error(raw.code&&typeof raw.code==='number'?raw.message:raw.response?.data?.msg||'自动保存连接失败，当前文字仍留在表单，请重试');e.code=typeof raw.code==='number'?raw.code:raw.response?.status||0;throw e}
 }
 return {get:()=>call('get'),put:data=>call('put','',data),clear:data=>call('delete','',data),commit:data=>call('post','/commit',data),dispose:()=>cancel.abort()}
}
