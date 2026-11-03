import request from '@/utils/request'
export const workflow=(path,method='get',data,params)=>request({url:'/cms/'+path,method,data,params})
export const reviewLabels={PENDING:'待审核',APPROVED:'通过待发布',REJECTED:'已退回',WITHDRAWN:'已撤回',STALE:'已失效',PUBLISHED:'批准已使用'}
export const kindLabels={ARTICLE:'文章',PRODUCT:'产品',PAGE:'单页',SITE:'站点外观'}
// Only server-provided resource enums and positive numeric IDs become local router targets.
export function notificationTarget(n){const id=Number(n.resourceId);if(!Number.isSafeInteger(id)||id<1)return '/index';if(n.resourceType==='REVIEW')return {path:'/cms/review',query:{reviewId:id}};if(n.resourceType==='INQUIRY')return {path:'/cms/inquiry',query:{inquiryId:id}};if(n.resourceType==='SCHEDULE'){const cid=Number(n.targetContentId),kind=String(n.kind).toLowerCase();if(['article','product','page'].includes(kind)&&Number.isSafeInteger(cid)&&cid>0)return {path:'/cms/'+kind,query:{scheduleContentId:cid}};return '/index'};return '/index'}
