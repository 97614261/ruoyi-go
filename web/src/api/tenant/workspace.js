import request from '@/utils/request'
export const getWorkspace = () => request({ url: '/tenant/workspace', method: 'get' })
export const requestCancellation = data => request({ url: '/tenant/cancellation', method: 'post', headers: { repeatSubmit: false }, data })
