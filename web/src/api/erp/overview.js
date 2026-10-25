import request from '@/utils/request'
export function overviewSection(kind, params) {
  const allowed=['orders','shipments','stocks','purchases','payables','receivables']
  if(!allowed.includes(kind))throw Error('无效的概览区域')
  return request({url:'/erp/overview/'+kind,method:'get',params})
}
