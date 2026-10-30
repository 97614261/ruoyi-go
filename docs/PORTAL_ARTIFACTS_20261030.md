# 2026-10-30 当前门户v6交付包

当前包包含第二阶段第4步定时发布/下架；只适用于门户v6（29表），见PORTAL_SCHEDULE_ACCEPTANCE。

- [windows 部署包](../dist/portal-windows-20261030-080936-446.zip)：逐文件SHA256核验13项通过；ZIP SHA256：`65718a162a6c839689ffb0c63e680fdefa19f329aa393b7b34470011bcf3120e`。
- [linux 部署包](../dist/portal-linux-20261030-080958-596.zip)：逐文件SHA256核验13项通过；ZIP SHA256：`8b5c84b3507c27c5126bca45c6d6a7f7d90365aad2a34e4e7f46d654e812f608`。

包内未包含私有文件，当前管理员密码/JWT样本扫描未命中。Linux仅交叉编译；Windows包已用于本地运行。
新v6恢复副本ruoyi_go_portal_restore_1793318738524570300核验29表及附件一致，副本未启动。
门户专用测试通过；旧基础套件仍有2条会话断言失败，见验收报告，不能宣称全项目全部通过。

下列v5包保留历史校验记录；不要用它们连接当前v6库。

## 历史v5交付包（第二阶段1—3）

对应第一阶段本地收口与第二阶段1—3；数据库v5/28表。以下时间戳为最终包，不要混用dist里的中间构建目录。

- [windows 部署包](../dist/portal-windows-20261030-072717-898.zip)：逐文件核验12项通过；ZIP SHA256：`0c56368cb05be8413b36a365798a62763ca3764c7cd21bd057dabaa6f0707f68`。
- [linux 部署包](../dist/portal-linux-20261030-072718-094.zip)：逐文件核验12项通过；ZIP SHA256：`eab034f39bec7e597d133ced972692755371916acd73931d3d398eb574deb5ed`。

已核对包内SHA256SUMS，未打包本地配置/账号/日志/备份/上传目录，并检查当前管理员密码与JWT样本不在包内。Linux仅交叉编译，未实际运行。

使用步骤见根目录门户使用说明.md；新装、升级和恢复见PORTAL_DELIVERY.md。验收见PORTAL_PHASE2_ACCEPTANCE与PORTAL_STEP56_ACCEPTANCE。

最终恢复副本：ruoyi_go_portal_restore_1793316135155882200，28表定义/数据与附件SHA256一致；未覆盖源库、未启动副本。副本与原恢复资料包含私有数据，只在本地保留。
