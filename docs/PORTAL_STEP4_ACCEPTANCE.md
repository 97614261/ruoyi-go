# 门户第4步：产品与正式门户页面

日期：2026-10-30。范围：portal分支，本地独立 ruoyi_go_portal；这是本次验收快照。

## 功能与位置

- 后台「门户管理 → 产品展示」：新增/编辑、编号、参数（每行一项的纯文本）、封面、最多16张图集、富文本、SEO、草稿预览、历史恢复、独立发布/下架和删除。
- 产品使用PRODUCT栏目，与文章/单页共享不可变修订内核；图集、编号、参数随修订保存，改稿不改线上。正文/封面/图集合计最多32个不同素材，引用事务保护历史图片。
- 门户首页 /：最新产品、最新动态和已发布介绍页；空站点有真实空状态，不生成虚假内容。
- 新闻 /news、/news/c/:slug、/news/:slug；产品 /products、/products/c/:slug、/products/:slug；单页 /p/:slug。
- /search：只搜索公开标题/摘要，关键词最多80字符，12条/页，页码1—1000；SQL参数化并转义LIKE通配符，不扫正文。栏目页只展示直接属于该栏目的内容。
- 首套business模板升级v2：响应式布局、正文跳转、可见键盘焦点、图集、参数、真实404/500错误页。访客正文由Go输出，不依赖JavaScript请求后再拼页面。
- 每页标题/描述/canonical，搜索noindex；/robots.txt；/sitemap.xml索引和/sitemap/N.xml分片，动态过滤有效发布内容与栏目。分片1000条，最多50000篇内容，超限返回503提示运维扩展，禁止静默输出不完整结果。

## 部署配置

入口 http://127.0.0.1:8085/；后台 http://127.0.0.1:8085/admin/ 。本地账号仍在Git忽略文件 logs/portal-accounts.local.json，不在文档记录密码。
portal.publicURL是canonical与sitemap的固定源地址，默认 http://127.0.0.1:8085 。生产须在私有配置中改为实际域名，例如 https://www.example.com；不从请求Host或转发头生成网址。
仍然只运行一个Go服务；Node仅编译自有admin-web。Windows实际运行，Linux本轮仅交叉编译，不宣称运行或性能验收。

## 数据与权限

升级前备份 logs/portal-step4/pre-v4.local.sql；验收后备份 logs/portal-step4/accepted-v4.local.sql。
显式v3→v4迁移仅扩展cms_content_revision三个产品字段并添加6040—6045菜单，表数仍26，版本为4；原sys表结构、ERP库与参考Vue源码未改。
新装工具安装到当前版本；升级工具默认预览，停门户并备份后显式 -confirm；启动只读核验，不自动改表。不能重放初装覆盖现有库。
产品权限cms:product:list/query/add/edit/publish/remove；新增菜单不自动授权给普通角色。文章编辑员不能读取产品管理接口。
所有公开入口统一检查published指针、未删除状态及栏目祖先启停，媒体在304响应前检查公开引用。草稿/历史引用保护素材，但不让图片公开。

## 已验证

1. 既有栏目/素材、文章/单页、撤权、CAS、登录/退出、结构漂移负向及恢复用例继续通过。
2. 产品编号/参数/图集发布、编辑草稿不影响线上、历史恢复保留图集、素材引用不可删、产品独立权限、栏目停用及主动下架。
3. 首页/产品页/分类/搜索/站点地图统一隐藏停用产品，图集失去最后公开引用后404；搜索注入与页码/关键词上限拒绝。
4. 浏览器实际查看首页/产品详情；390像素手机宽度无横向溢出、图集图片加载成功；后台产品表单编辑保存、私有图集和参数预览均通过。验证码恢复true，测试会话退出。
5. Vite生产构建、Go单元测试、完整本地接口回归、全量go vet ./...与go build ./...；Windows/Linux部署构建。

回归：PORTAL_INTEGRATION=1 GOWORK=off go test -count=1 -v ./test/portal。
本轮日志 D:/WorkSpace/CodeX/.portal-step4-20261030/：integration-final.log、unit-final.log、vet.log、build.log、frontend-final.log。
页面证据：home-desktop.png、product-mobile.png、product-admin-preview.png。
首次搜索隐藏断言把搜索框自身的关键词回显误判为数据泄漏，改成检测产品公开URL后重跑；失败日志保留，不计作通过。

## 清理与后续

测试产品已软删除、测试栏目已停用；历史与私有引用按修订机制保留。没有自动提交/推送Git，没有远程部署。
第5步再做第二套模板、站点设置、导航轮播、区块选择/排序以及外观预览/发布/恢复；本轮不把这些算作完成。
第6步再集中做部署恢复与规模/容量验收。本轮不承诺生产并发上限。
