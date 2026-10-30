# 门户管理后台

基于本项目参考RuoYi-Vue3 3.9.2建立的自有副本，保留LICENSE（MIT）及上游文件声明。
前端源自 ../RuoYi-Vue3-master，参考目录保持只读；此副本不包含ERP源码或私有配置。

开发：npm ci 后 npm run dev，入口 http://127.0.0.1:5175/admin/，API代理到独立Go端口8085。
构建：npm run build:prod，产物在 ../portal/admin-dist，再构建Go；运行服务器不需要Node.js。
该目录目前只有原后台，CMS菜单与内容模块后续实现；没有创建门户数据库，登录链路尚未验收。
