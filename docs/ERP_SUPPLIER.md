# 采购步骤3：供应商模块及本地验收

日期：2026-10-22。适用 `erp` 分支、独立本地 `ruoyi_go_mt`。这是本轮快照，不是采购全流程或生产认证。

> 后续导航：步骤4[采购单后台](./ERP_PURCHASE.md)、步骤5收货及步骤6三页的本地范围已完成，最终回归/GUI/恢复及边界见[步骤5—6第10节](./ERP_PURCHASING_ACCEPTANCE.md#10-仅收货员工实际页面补验与最终收口)。下文“采购/收货未实现”只描述步骤3当时范围。

## 1. 完成范围

已完成供应商分页查询、详情、新增、编辑和启停，菜单及紧凑页面接入。编码创建后固定，同企业编码唯一；更新必须携带持久版本，冲突不能静默覆盖。联系人可不填，启停不删除历史。

不新增表或修改已登记001—004；步骤2的六张表已存在，本轮只开放 `erp_supplier` 的业务。采购单、价格计算、确认/取消/关闭、分批收货和采购托管库存仍未实现。原手工IN/PR不是采购订单或采购收货模块。

未操作远程、原Java/main库、参考Vue或已有密码；未自动给已有企业/员工开通，也未提交/推送Git。原demo凭据核验仍按用户选择延期。

## 2. 生成来源与实现

先从实际 `erp_supplier` 表使用项目现有生成器导入并生成，再补租户、领域校验、版本、DTO和隐私边界；没有改生成器模板。原生成包保存在 `logs/erp-supplier-20261022/supplier-generated.zip`，SHA256：

`fca21897f05ccf9efad32c370fe6ab655cb5c9e200d1a271580d600c3a4829cc`

生成包的删除/导出和菜单SQL未直接执行。最终实现位置：

- `internal/model/supplier.go`、`internal/repository/supplier.go`、`internal/service/supplier.go`、`internal/handler/supplier.go`、`internal/router/supplier_routes.go`。
- `web/src/api/erp/supplier.js`、`web/src/views/erp/supplier/index.vue`、`web/src/views/erp/components/SupplierWorkspace.vue`。
- `internal/repository/erp_console.go`、权限契约清单及HTTP日志隐私规则。

| 接口 | 权限 | 返回边界 |
|---|---|---|
| GET `/erp/supplier/list` | `erp:supplier:list` | 顶层rows/total；只含ID、编码、名称、状态、版本及创建/更新时间 |
| GET `/erp/supplier/:supplierId` | `erp:supplier:query` | data详情，才返回联系人/电话/邮箱/地址/备注 |
| POST `/erp/supplier` | `erp:supplier:add` | data仅supplierId/version，不回显联系人 |
| PUT `/erp/supplier/:supplierId` | `erp:supplier:edit` | 输入version；data仅supplierId/version |

没有DELETE、remove或export接口。列表上限100，默认主键倒序，自定义白名单排序补唯一主键；名称包含搜索转义通配符，不宣称该条件能用范围索引或已通过容量压测。编码按精确值查询。

权限为当前客户身份、企业开通及员工功能的交集；平台身份拒绝。供应商是企业共享资料，不凭空增加店铺/仓库授权系统。写事务按现有员工锁检查身份，再锁供应商、校验version；成功审计与业务同事务，失败整笔回滚。浏览器编辑需要query和edit同时具备；API的edit不隐含query，也不通过写响应泄露详情。

编码为1—64位字母数字及 `._-`，以字母数字开头；名称1—120字符。联系人/电话各64、邮箱128、地址/备注500，均可空；有值邮箱校验格式，非法编码/控制字符拒绝，地址/备注允许换行。status是领域启停开关 `0`/`1`，不受可编辑字典扩展为新业务状态。

服务端决定企业、ID归属、审计字段和初始版本；忽略客户端伪造的归属/审计字段。CAS修改、空字符串清空可选资料、状态切换均由同一更新入口处理。

## 3. 隐私和菜单

HTTP供应商域不记录整个查询串（包括未知参数），不挂通用正文/响应审计；事务成功日志只含ID、动作、前后版本。前端写请求关闭持久防重正文缓存，私有详情不放浏览器持久存储。接口设置no-store；关闭、失活或卸载清空详情，旧请求不得重新填回；缓存页面重新激活会重查列表。

菜单7600/list、7601/query、7602/add、7603/edit只登记定义。`menu-registration.json`记录登记前后企业/角色授权摘要完全一致；新测试企业仍按既有新企业初始化规则创建，不代表旧企业自动获权。

已有企业要使用该页，需平台显式开通对应节点，再由企业管理员分配员工权限。本轮没有替demo/demo-b做这一步。不要为了看见菜单运行 `erpinit -grant-demo`；它是另一个明确授权动作。本地已登记菜单，不需要重复执行生成包SQL或改004。

## 4. 测试与真实页面

最终命令（仓库根目录，工具路径按本机配置）：

```powershell
$env:GOCACHE='D:\WorkSpace\CodeX\.mt-cache'
.\scripts\verify-erp.ps1 -GoPath 'C:\Program Files\Go\bin\go.exe' -NodePath 'C:\Users\FF\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' -Integration -Count 2
```

最终证据：`test/results/erp-20261022T124154677Z-c2e60441/summary.json`，六步全部通过，真实集成连续两轮；前端68项、0失败、0跳过。前一轮 `erp-20261022T123059724Z-ca18eeaf` 也通过，但早于快切修复，不能代替最终轮。

| 验证 | 实际覆盖 |
|---|---|
| Go定向/完整单测及vet | 绑定/类型/校验、缺失租户/平台拒绝、显式列表投影、稳定排序、菜单/权限契约及日志不落查询串 |
| 供应商真实API六组，每轮重跑 | 两企业同编码、跨企业读写、权限拆分、伪造字段、可选资料/重复编码、不可改编码、启停/CAS并发、授权员工无需仓库范围、无删除/导出、隐私/no-store及旧会话撤权 |
| 真实数据库故障注入 | 测试进程对repository事务注入审计失败；新增/修改全部回滚，再经服务器读回核对；未向运行服务器植入故障开关 |
| 前端供应商8项 | 提交busy覆盖校验、最小白名单、失败保留原资料/version、权限拆分、关闭/失活作废旧请求、读取失败非假0、缓存页面再激活、关闭过渡防快切竞态 |
| 实际浏览器 | 新增UI-SUP-001；详情显示合成联系人；编码不可编辑；修改名称并停用后v1→v2；重复编码拒绝且输入保留；重新打开资料清空 |
| 仅列表员工 | 实际登录并看到1条脱敏列表，没有新增/详情/编辑按钮；后端对应越权拒绝另由API验证 |
| 四视口 | 320×568、375×812、768×600、1280×800的带失败提示表单，弹窗和底部两按钮均在视口内；不是手机真机认证 |

浏览器证据在 `logs/erp-supplier-20261022/ui/`：`acceptance.json`、`layout.json`、`supplier-mobile.png`、`supplier-employee.png`。数据库集成会写入新专用企业/审计等，随后停用保留；不能称测试全程只读。

### 发现、修复与清理

1. 最初7项组件测试有1项失败：缓存页面重载的测试只等一次微任务，未等异步load。激活回调返回load promise，测试等待真实完成，保留原断言后7项通过。
2. 真实浏览器发现关闭详情后立即编辑，旧close事件可能作废新请求。新增同步closing保护及过渡期禁用，在closed后再允许打开，补第8项测试并重新跑全套。没有把组件测试当作真实页面证据。
3. 首组页面企业10669自然超时停用，原清理助手用过期owner token，验证码恢复失败。原失败证据 `ui-cleanup.json` 保留；另仅恢复该已停用企业的原验证码值并精确清掉该配置缓存，未重开企业，修复证据 `ui-cleanup-repair.json`。
4. 新组企业10670完成浏览器验收；清理助手改为先重新认证测试owner再恢复。`ui-v2-cleanup.json` 确认验证码恢复true、企业停用true、记录保留。测试浏览器已退出并恢复视口；没有修改真实账号密码或全局关闭验证码。

## 5. 本地运行与后续

后台已在单实例本地8081运行供应商代码，前端5173可用。后台构建SHA256 `287a174a4b2bf92a0beb1831c8c55aa90ab881e5a4930659f76bfa8347017d3e`，位置 `logs/erp-supplier-20261022/server-windows-amd64.exe`；本轮本地切换是强制停止旧进程后启动，不冒充优雅排空认证。页面最后修复只涉及前端，最终生产构建另由上述回归核验。

没有重做本轮业务数据的完整备份恢复、远程部署、容量压测、跨浏览器/真机、生产升级/回退或负载认证；步骤2恢复证据仅证明当时结构窗口，不能冒充包含供应商新数据的最新恢复。

下一步为[采购计划步骤4](./ERP_PURCHASING_DESIGN.md)：采购草稿、精确金额、主数据快照、确认/取消/关闭及SKU引用保护。分批收货在步骤5，不应先拼两个公开库存事务或提前开放托管IN/PR旁路。
