# 商品基础010

商品基础1—5本地验收已完成，010已安装。最终证据和限制见[验收报告](./ERP_CATALOG_ACCEPTANCE.md)，设计见[设计](./ERP_CATALOG_DESIGN.md)。菜单已注册，既有企业在租户管理按需开通商品档案/分类及按钮权限；没有自动给demo等旧企业授权。

## 结构与使用

新增商品分类/SPU/SKU档案/商品图片/动作五表；原SKU编号继续用于库存、店铺商品配对、采购和订单。旧SKU不自动归组，可在商品编辑中明确选入；归组后从商品页管理，不能移除或改归属。历史商品不会因扩展变成新编号。SKU状态负责是否可用于新业务，没有独立SPU状态造成两套启停规则。

商品页填写商品编码、名称和可空分类，至少一个SKU；每个SKU含编码、规格、单位、启停、可空条码与重量尺寸。条码是文本，可保留前导零；重量用克、尺寸用毫米，只收1—1000000000正整数，未知留空。每商品最多50个SKU。已有业务引用的SKU编码、规格、单位禁止修改；名称和档案可更新，历史单据快照保持原样。商品/分类编码建立后固定，分类可停用，不删除历史。

图片从商品页专用上传，每张≤8MB、≤2000万像素，仅PNG/JPEG/GIF/WebP；每商品最多5张。图片属于企业共享附件，商品权限撤回不等于撤回企业媒体访问。移除关联、关闭表单或保存失败不自动删除附件，仍占用配额。

批量导入先下载模板，编码和条码保持文本单元格。每行一个SKU，同商品编码且名称/分类/备注一致时合并；先建所需分类。最多100行、单表、10MB，拒绝公式，只新增，不覆盖，整批成功或回滚。不要依赖Excel把长数字/前导零丢失后的内容自动恢复。图片另传。导出依当前筛选按SKU展开，含重量尺寸及分类；受系统统一导出条数上限约束，不含图片二进制。

## 接口与授权

列表均分页、pageSize≤100；列表rows/total、详情与动作data，业务错误使用统一响应。

| 路径/方法 | 权限 | 说明 |
|---|---|---|
| GET /erp/product/list | erp:product:list | 编码、名称、SKU、条码、分类过滤 |
| GET /erp/product/:id | erp:product:query | SKU及引用标志、图片、版本 |
| POST /erp/product；PUT /erp/product/:id | erp:product:add / edit | 聚合写SKU；requestKey，编辑version |
| POST /erp/product/image | erp:product:image | 专用图片上传，返回storageKey |
| GET /erp/product/template；POST /erp/product/import | erp:product:import | multipart file、requestKey；原文件重试 |
| POST /erp/product/export | erp:product:export | XLSX下载 |
| GET /erp/productCategory/list | erp:productCategory:list | 分类完整信息分页读取 |
| POST /erp/productCategory；PUT /erp/productCategory/:id | erp:productCategory:add / edit | requestKey，编辑version |

功能权限限定企业共享商品，不按仓库/店铺隐式筛选。平台不能代入企业。查询权限不包含上传，上传权限不包含商品新增/编辑；商品新增/编辑/导入权限包含其内部SKU写入，不另要求旧SKU:add/edit。菜单9000—9006与9100—9102只注册定义，不自动授权已有企业/员工。

## 本地升级与验证

先停止本地写入，备份完整独立数据库及公开/私有附件，恢复到新的副本逐表/逐文件核验。001—009不修改；先在本轮恢复副本安装010、校验并生成真实表代码。

`go run ./cmd/erpcatalog` 默认只读。只有取得开发库安装确认后，才执行：

```powershell
go run ./cmd/erpcatalog -apply -confirm-database ruoyi_go_mt -backup-report <本轮bundle目录>/verified-report.json
```

CLI只允许localhost的ruoyi_go_mt，拒绝原库/远程/恢复副本；备份报告24小时内有效，完整数据库、两附件根检查及SQL摘要必须吻合。报告是操作证据，不是签名、停写机制或生产发布授权。恢复副本安装使用专用本地测试工具，不把CLI限制放宽到生产。

启动只读校验，全无为可选；部分结构/孤儿版本/漂移拒绝，不自动修复。未安装时商品接口提示先安装。安装后显式注册菜单定义，不自动给demo等旧企业加权。

```powershell
scripts/verify-erp.ps1 -GoPath 'C:\Program Files\Go\bin\go.exe' -NodePath '<本机node路径>'
# 安装确认、安装和服务健康后，再运行真实接口两轮：
scripts/verify-erp.ps1 -GoPath 'C:\Program Files\Go\bin\go.exe' -NodePath '<本机node路径>' -Integration -Count 2
```

真实接口用例`test/multitenant/erp_catalog_test.go`要求本地已登记010，仅创建当轮新企业和资源，停用保留；默认不连数据库。结构故障/事务测试要求显式ERP_CATALOG_RESTORE_DB且为本轮新副本；不得对源库运行故障注入。商品GUI及最终恢复已完成，浏览器下载事件捕获限制见最终报告；不代表完整ERP、Linux部署或生产容量通过。
