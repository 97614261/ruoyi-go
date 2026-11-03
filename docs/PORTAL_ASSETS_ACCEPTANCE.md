# 第三阶段第2项：素材分组、检索和容量提示

> 此文保留v10快照。当前跳转功能为v11/31表，见[跳转验收](./PORTAL_REDIRECT_ACCEPTANCE.md)，旧v10包不适用于当前库。

2026-11-02，portal分支，本地门户127.0.0.1:18085。当前v10/30表；第三阶段第3项301映射、第4项自动保存尚未实施，Linux生产实测仍待环境。

## 功能

- 素材上传可以填写分组，同名为一组；修改弹窗可换组或留空恢复未分组。分组是标签，不是文件目录，不改变storage_key、assetId、URL或历史引用。数据库原有大小写/重音比较规则适用于标签匹配。
- 素材管理及所有AssetPicker共享检索：名称前缀、全部/未分组/指定分组、存储PNG/JPEG、最小/最大宽度、最小/最大文件大小。0为不限。WebP上传后仍转成PNG，所以按存储格式检索。
- 当前筛选显示素材数量、重编码后的合计大小、最大宽/高。最大宽与最大高可来自不同图片；统计是查询时结果，不是磁盘剩余空间或配额，不含备份及停用待清理文件，也不承诺与分页行跨请求强一致。
- 名称中的%和_按字面前缀查找；文件不因分组被移动或重编码。原来的8MB/1200万像素上限、私有图片权限、引用不可删保障继续生效。

## 接口和数据边界

既有 GET /cms/asset/list 增加可选筛选，不改变原分页字段。新 GET /cms/asset/statistics 使用同一筛选，要求cms:asset:list，返回data中的total、byteSize、maxWidth、maxHeight。
groupName参数省略=所有分组，显式空串=未分组，其他值=完整标签匹配。页面使用ungrouped=true明确表示未分组，避免通用查询序列化忽略空串；它与非空groupName不能同时使用。宽度范围单位像素，minBytes/maxBytes单位字节；绑定拒绝负数、超限和倒置范围。
上传multipart可带groupName；PUT素材可带groupName，省略保留旧值、空串清空。仍按version进行CAS，旧版本不覆盖新分组。标签最多40字，去首尾空白，拒绝尖括号、斜线和控制字符。

009_asset_groups.sql是v9→v10的显式迁移，只为门户cms_asset增加group_name默认空串和(is_deleted,group_name,asset_id)索引。旧素材归入未分组，无菜单或角色授权变化。名称索引保留；格式/尺寸/大小的组合范围及统计仍需读取匹配行，统计另限3秒，未据此宣称生产大数据性能。旧v9程序会被版本守卫拒绝，不得仅替换旧exe回退。

## 验证证据

- 真实TestPortalAssetGroups通过：上传/改组/清空、旧客户端省略兼容、CAS拒绝、非法标签/范围、文字通配符、组合筛选、空集合/删除后统计、图片字节不变、仅list权限可看统计但不能看私图或编辑。
- 浏览器用本轮专用asset-ui-20261102.png（640×360）验证改组保存成功、名称/完整分组/600—800px组合筛选及1张/1.5KB汇总。临时素材已按记录清理，原有素材未修改；验证码恢复true，GUI账号退出。
- 第一轮全量回归与GUI临时关闭验证码发生交叠，TestPortalBootstrap正确拒绝关闭验证码的状态；记录保留full-test.log。该轮不算通过，最终在恢复验证码、结束GUI后重新执行完整回归，结果见full-test-final.log。

私有升级前备份：logs/pre-v10-20261102.local.sql。工作区原始证据：D:/WorkSpace/CodeX/.portal-assets-20261102，含integration.log、frontend-final.log和asset-groups-proof.png。凭据、SQL备份和原始日志不入发布包。

## 最终结果

恢复验证码并结束GUI后，首轮修复后程序运行状态下 `PORTAL_INTEGRATION=1 PORTAL_TEST_PORT=18085 go test -count=1 ./...` 全部通过（门户套件40.844秒），`go vet ./...`通过，Vite生产构建通过。未用失败的首轮代替通过证据。
v10恢复工具已创建全新 `ruoyi_go_portal_restore_1793603219402686400`：30表定义/数据校验一致、附件SHA256一致，源库未覆盖，副本未启动。完整私有备份在 `logs/portal-step56/recovery-1793603219402686400`。
随后重新启动18085并通过验证码开启时的登录、管理员身份/菜单、匿名拒绝及退出会话撤销检查。原账号密码未更改。Linux只生成交叉编译包，不代表Linux运行/容量验收，新机v10初装也未另开实例重演。

随后发现通用请求序列化会省略空串，已补ungrouped=true及冲突拒绝，并真实验证页面在“全部分组”1张、“未分组”0张之间正确切换。含此修复的最终完整回归记录为full-test-accepted.log（门户套件38.513秒），全项目测试与vet再次通过；截图ungrouped-proof.png。早期150817两个包是该修复前的构建候选，不作为交付。
