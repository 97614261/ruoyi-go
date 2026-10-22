# ERP 一期步骤4：店铺商品、变种与内部 SKU 配对

日期：2026-10-21，America/New_York。适用erp分支及独立本地ruoyi_go_mt。
代码、自动回归和本页列出的真实页面流程已验收。仅实现商品配对基础，不是订单/平台同步/库存履约；步骤5—8未实施。

## 1. 做了什么

- 一行表示一个店铺可售变种：店铺SKU编码、可选外部商品/变种ID、标题、状态、备注、版本；没有SPU、组合商品、价格财务或平台商品拉取。
- 商品列表/详情/新增/完整编辑；停用代替物理删除，不提供商品DELETE或导出入口。
- 一个变种配一个内部SKU，多个变种或店铺可以配同一SKU；支持建立、替换、解绑、重新配对。
- 列表关联店铺、配对和SKU，一次批量JOIN读取，不按行查询；MAPPED表示存在status=0的配对，不等于当前店铺/商品/SKU全部可用，停用状态分别显示。
- 员工必须同时具备功能权限和实时店铺范围，默认无范围；平台不能代入。资料维护也允许具备对应权限的已授权员工，不把“仅所有者管理店铺资料”的规则误套到商品。
- 编辑/配对/解绑共享商品version作为CAS令牌；配对另有mappingVersion，保留行和历史，成功审计与两个版本同事务。
- 前端新增店铺商品配对页面及API；提交busy覆盖校验/确认/请求，失败保留草稿，旧详情/SKU搜索响应失效，解绑使用确认前的ID和版本快照。

## 2. 生成来源、文件和结构边界

实际导入erp_shop_item到既有生成器，设置模块erp、业务shopItem、类ErpShopItem、说明店铺商品、父菜单6000，下载Go/Vue3骨架后扩展领域规则；生成出的shopitem.go按项目阅读习惯落为shop_item.go。没有修改生成模板，也没有重新生成已扩展的SKU/仓库/店铺。

- 原始ZIP：`logs/erp-stage4-20261021/erp-shop-item-generated.zip`。
- SHA256：`3205845d90fa55a4c7657e13fbc2574e44bbc2826729a7e687f8204b3997d021`；同目录generation.json记录来源时间及表。
- [模型/DTO](../internal/model/shop_item.go)、[仓库事务](../internal/repository/shop_item.go)、[服务校验](../internal/service/shop_item.go)、[处理器](../internal/handler/shop_item.go)、[路由](../internal/router/shop_item_routes.go)。
- [页面逻辑](../web/src/views/erp/components/ShopItemWorkspace.vue)、[路由页面](../web/src/views/erp/shopItem/index.vue)、[前端API](../web/src/api/erp/shopItem.js)。
- 配对是领域动作，不为erp_shop_sku_mapping再暴露生成器通用增删改；它使用步骤2已安装表及复合FK。

本轮没有DDL、索引/旧表修改或自动迁移，003指纹不变。既有SKU单位冻结规则未改，配对不等于库存单据；订单审核时仍须另存单位/SKU快照。SKU已被配对行引用时，既有FK拒绝物理删除，即使配对已停用也保留历史关联。
共用店铺Scope仅提取为带受审服务端列常量的内部函数，旧店铺查询条件保持；从不拼接客户端列名。

## 3. 接口契约和权限

| 接口 | 权限 | 请求边界 |
|---|---|---|
| GET /erp/shopItem/list | erp:shopItem:list | shopId、shopItemCode精确、title包含、status、mappingState=MAPPED/UNMAPPED；页上限100 |
| GET /erp/shopItem/:itemId | erp:shopItem:query | 一条当前店铺范围内详情，含配对及SKU展示字段 |
| POST /erp/shopItem | erp:shopItem:add | 服务器企业/主键/审计，version强制1 |
| PUT /erp/shopItem/:itemId | erp:shopItem:edit | 完整资料和当前商品version；shopId不可转移 |
| PUT /erp/shopItem/:itemId/mapping | erp:shopItem:map | version为商品版本，skuId为本企业启用SKU，remark可选 |
| DELETE /erp/shopItem/:itemId/mapping?version=N | erp:shopItem:map | version在查询参数中，为商品版本；只停用配对行 |

全部接口需当前客户身份及实时店铺范围；菜单权限不能代替范围，旧会话下一次请求立即执行撤回后的范围。普通业务错误沿用HTTP200中的code，列表rows/total顶层、详情data、camelCase；无配对的mappingId/mappingVersion/mappingStatus/skuId/skuStatus为null。没有客户可控的tenantId/主键/审计/直接skuId资料写入口。

店铺选择器复用erp:shop:list，SKU选择器复用erp:sku:list；打开详情/编辑/配对弹窗需erp:shopItem:query。给员工配角色时需明确这些依赖，不能为了下拉框擅自追加菜单授权。内部SKU是企业级共享目录，配对权限允许引用该企业SKU，不能跨企业。

### 字段规则

- shopId正整数，创建后不可变；商品/配对版本正整数且更新需匹配当前值。
- shopItemCode为1—128字文本，保留前导零和大小写，不强转数字；外部商品/变种ID同样为文本且≤128字。
- 编号拒绝首尾空白和控制字符，避免MySQL5.7即使_bin字符序也忽略尾随空格造成标识混淆。可选外部ID的空串或null都归一为SQL NULL，多个未填变种ID不互相占唯一键。
- 企业+店铺+shopItemCode唯一，外部变种ID如存在也在店铺内唯一；不同店铺/企业可有同编码。
- 标题trim后非空且≤200字，备注≤500字，状态仅0/1。
- PUT是完整资料更新，需带回已有的可选标识；遗漏/空串/null代表清空。如果已有配对历史或订单引用，清空/改变识别信息同样拒绝。

## 4. 配对版本、停用和历史

例：商品v1首次配对→商品v2/配对v1；替换SKU→商品v3/配对v2；解绑→商品v4/配对v3(status=1)；重配→商品v5/配对v4，mappingId始终不变。商品普通编辑同样推进商品version，因此旧配对弹窗不能覆盖后续资料编辑。
配对行的创建人/时间保留，变更更新修改人/时间；审计记录前后SKU、两个版本和动作ID，不存原始商品文本或凭据。普通商品CRUD沿用通用操作日志，不能把它说成同样的事务审计保障。

建立/替换要求店铺、商品、SKU均启用；停用店铺不能新增/编辑商品，仍可按权限读历史和解绑。停用商品不能建立配对，但可以解绑。停用不自动取消历史订单或删除配对。

有任何配对行（含已停用）或订单引用后，店铺SKU编码、外部商品ID、外部变种ID冻结，标题/备注/启停仍可改。配对被非SHIPPED/CANCELLED订单行引用时，替换/解绑拒绝；未配对待审核行不妨碍首次建立配对。终态历史允许后续版本演进，但旧订单行的mappingVersion、SKU及单位快照绝不反写。

写入顺序：当前员工→店铺→商品→配对→内部SKU。店铺父锁与员工行锁分别串行化同店铺业务写入和当前员工撤权。后续订单审核/取消/发货必须先锁同一店铺；此约定是引用保护的前提，不能以后绕过父锁单独改订单行。
失败，包括成功审计插入失败，回滚商品版本、配对版本和配对行；并发普通编辑/配对只能一个同版本请求成功。数据库唯一约束和FK为最后一道守卫，不依赖客户端按钮。

## 5. 菜单和已有企业

新定义7200店铺商品配对；7201查询、7202新增、7203编辑、7204配对。权限保护集合与独立契约manifest同步；没有remove/export。
仅调用InstallERPConsole(false)登记，未运行grant-demo。登记前后所有sys_tenant_menu/sys_role_menu的排序摘要相同，证据`logs/erp-stage4-20261021/menu-registration.json`；最终只读确认demo/demo-b没有7200段新开通项。
已有企业需平台明确开通，再配置员工角色和店铺范围；新建企业仍按现有默认开户目录初始化。本轮未改变默认套餐规则。

## 6. 实际测试和页面验收

Go定向单测/编译覆盖文本标识、空值、长度/状态、版本溢出、带企业谓词的JOIN/实时Scope、菜单边界。前端组件状态测试覆盖重复提交、确认阶段busy、快照、取消/冲突保留草稿、旧详情和旧SKU搜索响应失效。

真实HTTP/DB测试：[erp_shop_item_test.go](../test/multitenant/erp_shop_item_test.go)。覆盖两企业同编码、外部变种唯一/前导零、无功能权限、员工默认无范围、跨店铺/企业拒绝、旧会话撤权、服务器字段防伪造、无物理删除、编辑/配对共享CAS、复用配对行、停用边界、审计故障创建/解绑回滚、稳定JOIN分页、配对筛选与引用历史保护。
订单引用保护使用专用企业内的最小直接数据库夹具，并按精确行ID先删行再删头；这验证引用守卫，不是已实现商业订单流程。最终确认临时引用订单0条；其他验收企业/商品/配对/审计记录保留且企业停用。

完整验证最终两轮通过：`test/results/erp-20261021T145150343Z-07826500/summary.json`；2026-10-21 10:51:50—10:54:35 America/New_York。6步全部PASSED、integration=REQUESTED；Go格式/单测/vet、前端34项0失败0跳过、生产构建，以及既有多租户/店铺/库存+新商品配对真实回归均通过。较早完整结果`test/results/erp-20261021T143834311Z-d522f3cd/`保留，不用旧结果冒充最终补强后的测试。

### 浏览器实际操作

用户先确认验证码操作，随后明确允许测试时关闭验证码。本轮仅在新专用企业item-ui-691919969000临时关闭该配置，未给原企业追加新菜单、未动平台/demo/demo-b配置或密码；旧专用企业shop-ui-709515440800短时恢复测试窗口后仍停用。选择新企业是为了沿用默认开户目录测试新菜单，不偷改旧企业开通。

真实浏览器通过：登录→新增店铺→修改名称→新增店铺变种000001（外部商品000002/变种000003）→配对UI-SKU→解绑→尝试更改冻结编码被拒绝且编辑内容保留→纠正编码、改标题并停用。版本1→2→3→4，停用后配对按钮禁用。

- [配对成功截图](../logs/erp-stage4-20261021/ui-mapping-saved.png)
- [失败保留草稿截图](../logs/erp-stage4-20261021/ui-edit-rejected-preserves-draft.png)
- [停用最终状态截图](../logs/erp-stage4-20261021/ui-item-disabled.png)

截图是本地忽略证据，不随源码公开。没有把这组所有者页面操作外推成员工授权弹窗、所有浏览器/移动端或全部异常网络页面均已点测；员工功能/范围及撤权以真实接口测试为证据。
收尾工具已恢复该测试企业原验证码值并停用企业，`logs/erp-stage4-20261021/closeout.json`再次只读验证恢复值为true、状态停用、临时引用清理和三个结构校验通过。原demo凭据核验继续按用户此前决定延期。

本地8081后台更新为本轮构建、5173前端运行中；仅核对可执行文件路径后替换上一轮由本任务启动的本地测试后台，未停止其他服务。原库、参考Vue、Redis前缀、SKU单位规则及已应用DDL均未改；无远程部署、性能压测、Git提交/推送或新第三方依赖。
与步骤3源码快照逐项对照，除本阶段明确修改的文件外，原有未提交源文件SHA256未变；003指纹仍为ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb。最终源码副本/补丁/校验清单保存在`logs/erp-stage4-20261021/source-snapshot/`，不含配置、账号、数据库或附件；它不是Git提交或数据库备份。浏览器收尾时原测试会话已被拒绝并回到登录页。

## 7. 下一步

步骤5再实现商业订单手工录入/模板导入及任务：保留原始商品文本、读取当前配对、审核时固定版本和快照，单据/包裹/库存整体事务在步骤6完成。还需专门审查收件隐私、导入文件配额/下载/清理、幂等和旧库存入口旁路，不能把商品配对完成等同于订单可上线。
