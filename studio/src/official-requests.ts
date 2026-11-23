import { createUUID } from './uuid'
import {APIError,request} from './api'
export interface OfficialUseResult {requestId:string;state:'copying'|'completed';projectId:string}
export interface OfficialIntent {path:string;requestId:string;body:{version:string;name:string;bindings:unknown[]}}
const key=(scope:string)=>`pod-studio.official-use:${scope}`
export function pendingOfficialUse(scope:string):OfficialIntent|null{
 const raw=sessionStorage.getItem(key(scope));if(!raw)return null
 const value=JSON.parse(raw) as OfficialIntent
 if(!/^\/pod\/catalog\/templates\/[1-9][0-9]*\/use$/.test(value.path)||!/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(value.requestId)||!value.body||!/^[1-9][0-9]*$/.test(value.body.version)||typeof value.body.name!=='string'||!Array.isArray(value.body.bindings))throw new Error('官方模板操作记录无效，请先在项目列表核对。')
 return value
}
export async function retryOfficialUse(scope:string,token:string):Promise<OfficialUseResult>{
 const pending=pendingOfficialUse(scope);if(!pending)throw new Error('没有待核对的官方模板操作')
 try{const result=await request<{data:OfficialUseResult}>(pending.path,{method:'POST',body:JSON.stringify({...pending.body,requestId:pending.requestId})},token);if(result.data.state==='completed')sessionStorage.removeItem(key(scope));return result.data}
 catch(error){
  if(error instanceof APIError&&[400,404,409].includes(error.code)){
   try{const action=await request<{data:OfficialUseResult}>(`/pod/catalog/use-actions/${pending.requestId}`,{},token);if(action.data.state==='completed'){sessionStorage.removeItem(key(scope));return action.data}}
   catch(check){if(check instanceof APIError&&check.code===404)sessionStorage.removeItem(key(scope))}
  }
  throw error
 }
}
export async function submitOfficialUse(scope:string,token:string,id:string,body:OfficialIntent['body']):Promise<OfficialUseResult>{
 if(!/^[1-9][0-9]*$/.test(id))throw new Error('模板编号无效')
 if(pendingOfficialUse(scope))throw new Error('请先核对上一项模板复制')
 const intent:OfficialIntent={path:`/pod/catalog/templates/${id}/use`,requestId:createUUID(),body}
 sessionStorage.setItem(key(scope),JSON.stringify(intent))
 return retryOfficialUse(scope,token)
}
