# ERP 一期步骤3：渠道目录、店铺与员工店铺范围

> 后续导航：步骤4已实现[店铺商品和SKU配对](./ERP_PHASE1_ITEMS.md)。用户随后允许测试时临时关闭专用企业验证码，本轮补验了店铺新增/编辑及商品配对页面；下文“等待验证码”的描述保留步骤3收尾当时状态，不代表后续仍然卡登录，也不外推员工授权弹窗全量点测已通过。

日期：2026-10-21，America/New_York。适用erp分支、本地独立ruoyi_go_mt。
本阶段只做渠道目录、店铺资料和店铺范围，**不是已实现电商订单闭环**。本地前后端为验收启动；原库、参考Vue、已有企业权限及密码不动，不自动Git提交/推送。

## 1. 已实现的范围

| 能力 | 行为 |
|---|---|
| 渠道目录 | 6个服务端分类标签，只读、分页；supportsApi=false，不保存凭据、不调用平台、不伪造授权成功 |
| 店铺资料 | 列表、详情、新增、编辑、启用/停用；没有DELETE或导出入口 |
| 权限 | 企业开通∩员工功能权限∩企业身份；所有者维护资料，员工仅能读当前明确授权的店铺 |
| 员工范围 | 所有者GET/PUT，显式shopIds整体替换；默认无、[]撤回、缺失/null拒绝，上限200，正整数且不能重复 |
| 并发 | 店铺和范围均有持久version；同版本并发仅一个成功，旧版本不得覆盖 |
| 审计 | 范围替换、版本和完整成功审计同事务；失败全部回滚；200个ID分成有part/parts的审计行，不静默截断 |
| 页面 | 自有web内渠道/店铺页、员工店铺弹窗；分页、详情、启停、提交busy、冲突保留内容、旧异步响应失效 |

目录编码MANUAL/AMAZON/SHOPEE/LAZADA/TIKTOK/ALIEXPRESS只是本系统标签。sources中的MANUAL/IMPORT描述一期计划的录入来源，步骤3还没有商业订单或文件导入接口；页面明确提示后续实现。站点由企业填写归类编码，不保证是平台有效站点；币种只做三位字母格式和大写归一，不代表支持换汇/结算。

## 2. 接口与校验

| 接口 | 权限 | 额外边界 |
|---|---|---|
| GET /erp/channel/list | erp:channel:list | 当前企业身份，只读分页 |
| GET /erp/shop/list | erp:shop:list | 实时店铺范围；pageSize≤100，白名单排序+shop_id兜底 |
| GET /erp/shop/:shopId | erp:shop:query | 当前店铺范围；跨企业/未授权拒绝 |
| POST /erp/shop | erp:shop:add | 当前所有者；服务端赋企业/主键/审计，version强制1 |
| PUT /erp/shop/:shopId | erp:shop:edit | 当前所有者；路径ID+当前version，成功版本+1 |
| GET/PUT /erp/shop/access/:userId | erp:shop:grant | 当前所有者，目标本企业未删除员工；不给所有者自己存“全部”授权 |

店铺编码1—64位ASCII字母数字及._-，以字母数字开头，企业内大小写敏感唯一；名称trim后非空且≤120字；站点1—32位字母数字及_-并大写；币种3位字母并大写；状态只接受0/1；备注≤500字。请求DTO不接受tenantId、shopId、createBy等作为控制值，更新version必须正整数。

已被店铺商品、订单或导入任务引用后，编码/渠道/站点/币种不能更改；仍可改名称/备注或启停。停用不删除授权或历史。一期订单操作如何受停用限制仍需步骤5—6实现，不能把店铺启停页面外推成订单流程已验收。

所有响应沿用response：列表rows/total顶层，详情data，camelCase；普通业务错误仍为HTTP200中的code。不使用平台身份免除企业隔离。

## 3. 生成来源与实现位置

实际将erp_shop导入现有平台生成器，设置模块erp、业务shop、类ErpShop、说明店铺、父菜单6000，下载Vue3/Go骨架后扩展；没有改生成器模板或盲目重生成SKU/仓库。

- 原始ZIP：`logs/erp-stage3-20261021/erp-shop-generated.zip`（本地忽略产物）。
- ZIP SHA256：`62248db026a0e8a3ed11b01a6ceb52f9a4b9c0f45b5eb324c7a819190e5bf332`；生成元数据见同目录generation.json。
- 模型/DTO：[shop.go](../internal/model/shop.go)；仓库：[shop.go](../internal/repository/shop.go)；服务：[shop.go](../internal/service/shop.go)；处理器：[shop.go](../internal/handler/shop.go)；路由：[shop_routes.go](../internal/router/shop_routes.go)。
- 自有前端：[店铺页](../web/src/views/erp/shop/index.vue)、[渠道页](../web/src/views/erp/channel/index.vue)、[员工范围弹窗](../web/src/views/erp/components/ShopAccess.vue)、[API](../web/src/api/erp/shop.js)。
- 持久写入复用erpTransaction、lockERPActor、erpOwner和租户守卫。写锁次序：当前员工→店铺；授权：当前员工→目标员工→按ID排序的店铺。范围从SQL实时读取，不存入JWT/前端当授权凭据。
- 引用检查按三个固定引用表分别作LIMIT 1查询，不随商品/订单数量逐行查库；使用既有企业/店铺前缀索引，名称包含搜索仍可能扫描企业内数据。

003已应用DDL及校验指纹未改变，没有新增/修改表或索引，也没有自动迁移。GenTable配置和菜单定义是本轮明确的本地元数据写入，不把它们说成零数据库写入。

## 4. 菜单登记与如何使用

新增定义：渠道7000；店铺7100、query7101、add7102、edit7103、grant7104。权限标识同步加入保护集合和独立契约清单，禁止作为任意内部字符串改名。
实际调用InstallERPConsole(false)，并比较全库sys_tenant_menu、sys_role_menu的排序摘要，登记前后相同；证据`logs/erp-stage3-20261021/menu-registration.json`。**未执行-grant-demo**。

对新环境，在独立库完成001/002/003显式升级后，可按现有工具运行：

```powershell
go run ./cmd/erpinit -config configs/multitenant.local.yml
```

该工具只允许代码规定的本地开发库；它检查/安装001、002并幂等登记菜单，不是生产升级工具，不替代003备份门槛。不要为了显示菜单而给命令加-grant-demo。

已有企业不会自动出现新菜单：应由平台明确开通渠道/店铺及对应动作，再由企业给员工配置功能角色和店铺范围。员工授权弹窗还需要system:user:list、erp:shop:list；修改页面加载详情需要erp:shop:query与erp:shop:edit。只隐藏按钮不是权限保障。
新建企业仍沿用既有开户默认目录规则；“已有企业不变”不意味着本轮改了新企业默认套餐。专用验收企业与原示例企业严格分开。

## 5. 已执行的验证与边界

- Go定向测试：service/repository/handler及路由编译通过，包含格式/枚举/长度、显式空集合、上下文取消、实时SQL租户守卫和菜单定义检查。
- 店铺真实HTTP/DB定向测试连续两轮通过：`logs/erp-stage3-20261021/shop-integration-final.log`；8组覆盖目录、身份/功能/企业边界、输入与唯一性、启停/CAS/无删除、显式授权/旧会话撤权、并发授权/审计故障回滚、稳定分页/完整200店铺审计、已引用身份字段保护。
- 完整验证两轮通过：`test/results/erp-20261021T135424339Z-2add6b0c/summary.json`，包含gofmt、Go单测、vet、前端28项（0失败/0跳过）、Vite构建、多租户/库存/权限与新增店铺真实回归。实际时间09:54:24—09:57:13 America/New_York；不是吞吐/容量压测。
- 发现验证脚本嵌套PowerShell时本地LASTEXITCODE遮住native更新造成误报，改为同一全局作用域清零/读取；PowerShell5.1与7脚本自测均通过，新增native formatter+父脚本调用回归。修复后的最终再验见下方最终记录。
- 初次引用夹具使用map被守卫拒绝，后改为实体；再发现测试主键误写shop_item_id，纠正为实际item_id。没有放宽守卫、改表或改生产引用检查。前次两条临时引用已按精确item/tenant/shop及停用测试企业条件清理，定位报告同日志目录；其余测试企业保留并停用。失败日志不删除。
- 浏览器已打开本地真实登录页并填入新专用企业账号；**验证码未代填、未关闭，等待用户确认，登录后页面操作尚未验收**。前端组件状态测试和HTTP接口测试不替代浏览器证据。原demo密码核验继续按用户决定延期。专用UI企业shop-ui-709515440800随本轮自动验收收尾停用保留，不作为常用登录账号；确认后另行准备受控验收窗口。

本地前后端为验收运行在127.0.0.1:8081/5173，未替换其他占用进程；启动前两端口空闲。专用UI企业到期或验收结束自动停用，不把它当常用账号交付。

### 最终记录

修复脚本后的全量两轮回归：`test/results/erp-20261021T140255616Z-a0b438e3/summary.json`，全部6步PASSED，integration=REQUESTED；时间10:02:55—10:05:39 America/New_York。后端此后未改变。
最终又补上弹窗关闭动画与下一次详情请求交错的保护及测试；该补丁后默认检查通过：`test/results/erp-20261021T140838764Z-c5fd9e67/summary.json`，Go/格式/vet、前端29项0失败0跳过、构建均通过；此轮integration=NOT_REQUESTED，不冒充又跑了一轮接口。时间10:08:38—10:09:13 America/New_York。
对步骤2源码快照逐项核对，除本阶段明确修改的文档/验证入口外，原有未提交文件SHA256未变；003 SHA256仍为ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb。Git分支erp、HEAD仍c506bc2；没有commit/push。
本轮结束保存`logs/erp-stage3-20261021/source-snapshot/`的源码副本、补丁和SHA256清单；只保存Git可见未提交源码，不含私有配置、账号、数据库备份、附件或运行日志。这不是Git提交或数据库备份。
结论：阶段3代码/接口/自动回归通过；浏览器验收因computer-use的验证码动作确认要求暂停，尚未完成。未做生产性能、真实平台接入或完整一期订单履约验收。

## 6. 下一阶段

步骤4才是店铺商品/变种及内部SKU配对：继续先用生成器，补店铺范围、版本化配对与引用保护。订单/包裹/导入和库存事务编排仍在步骤5—6，未提前实现；生产部署、真实渠道接入和服务器容量均未在本阶段验证。
