# 自动保存v12发布包（2026-11-02）

> 本文为v12历史快照；当前v13内容运营、统计、SEO和首页组件见 [功能增强验收](./PORTAL_GROWTH_ACCEPTANCE.md) 与 [现行发布包](./PORTAL_GROWTH_ARTIFACTS.md)。v12包不适用于当前库。
以下两份包匹配v12/32表。历史v11及更早包不能连接当前库；先备份、停服务，再显式使用portal-migrate，不能初装覆盖现库。

| ZIP（dist目录） | SHA256 | 清单文件数 |
|---|---|---|
| portal-windows-20261102-235023-420.zip | 0d3305cacb5950ddbdd0965caa3b41a9f96b53efa4a804fe5b8364678d3715dc | 22 |
| portal-linux-20261102-235026-105.zip | 511e1a3eb1aaeed073c83e3f04fcf529661ca2d2fb4421ebc594ecedf9bc8362 | 22 |

逐条验证包内SHA256SUMS与文件集合一致；未发现私有配置/账号/日志/备份/上传素材路径，以及本地管理员密码和配置密钥样本。每包含四个程序、配置示例、使用/部署/验收文档和许可证；服务器运行不需要Node。SHA256仅检查传输一致性，不是发布者签名。
Windows真实API、并发、页面、全量回归和v12恢复演练已通过，见PORTAL_AUTOSAVE_ACCEPTANCE。Linux仅交叉编译和包内容核验，没有Linux运行或生产容量结论。第三阶段1—4与第四阶段本地功能已收口；新机v12完整初装仍待验。

## 包内程序与本地运行核验

Windows包内三工具已在ZONEINFO/GOROOT均指向空目录时实际执行：初装预览报告v12；升级确认在只读校验当前结构后拒绝重复执行；恢复确认在配置加载后因18085仍运行而安全拒绝。没有借此执行新机初装或再次迁移。专项go test -trimpath三包及go vet通过。
本地18085已切换到本包portal.exe的逐字节副本dist/portal-windows-amd64.exe；运行文件SHA256：83c6880934e779160e9093933042555ac0ca77215bf278065e84e93b7cf6b40c。重启后只读结构守卫、验证码开启登录、管理员菜单、匿名拒绝和退出撤销通过，浏览器已退出并确认验证码恢复。详细运行记录保存在本轮私有staging/running-package.json，未入包。
