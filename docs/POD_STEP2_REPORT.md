# 第二阶段：项目与可编辑画布

日期：2026-11-11。本文是本轮实施与验收快照；当前开发顺序见 [POD_DEVELOPMENT_PLAN.md](./POD_DEVELOPMENT_PLAN.md)，日常操作见 [POD_LOCAL.md](./POD_LOCAL.md)。

> 后续状态：素材输入、assets 开关、assetIds 字段与 schema 已由 [第三阶段](./POD_STEP3_REPORT.md) 扩展。本文保留第二阶段快照，不能将当时的“尚未上传”视为当前状态。

结论：第二阶段功能已实现，自动化检查通过，本地服务已启动。真实浏览器验收受环境限制，尚未完成。

## 1. 完成范围

| 计划项 | 本轮结果 |
|---|---|
| 个人空间 | 首次显式开通；重复请求幂等，不恢复已停用空间或已撤销成员 |
| 项目管理 | 分页、创建、重命名、软删除、回收站恢复；没有永久删除接口 |
| 可编辑画布 | 添加与拖动节点、连线、缩放、适应视图、缩略图、选择与多选 |
| 编辑操作 | 复制、删除、撤销、重做；删除节点同时删除其关联连线 |
| 七类节点 | 配置表单与服务端字段白名单；尚不执行节点 |
| 保存 | 800ms 防抖、串行写入、revision 冲突、失败重试、本地草稿与刷新恢复 |
| 限制 | 节点数、连线数和图大小配置；前后端结构校验，服务端配置值校验 |
| 验收 | Go 测试、真实本地数据库接口测试、前端单元与组件测试、vet/build 通过 |

本轮复用已安装 `sql/pod/migrations/001_foundation.sql` 中的项目表，未改 SQL、表结构或索引，未新增迁移。
仅使用独立本地库 `pod_studio_dev` 和 Redis DB 15。用户已有的 `web/` 管理前端保持原样。
AI 调用、素材上传、实际图片处理、商品展示图生成与结果文件导出均未实现，页面不展示伪造执行结果。

## 2. 接口契约

后端路径如下；Vite 开发代理使用 `/api` 前缀，例如前端请求 `/api/pod/projects`。
全部 POD 路由经过 `Auth → 请求体上限 → pod:studio:access`，资源访问另校验本人有效空间与成员。

| 方法与路径 | 请求 | 成功响应 |
|---|---|---|
| `GET /pod/bootstrap` | 无 | `data`：身份、空间、节点规格、能力、有效 limits |
| `POST /pod/workspace` | 页面提交 `{}`，服务端不读取可选空间 ID | `data`：本人空间 |
| `GET /pod/projects` | `pageNum`、`pageSize`、`deleted=true/false` | 顶层 `rows/total`，列表不含 graph |
| `POST /pod/projects` | `{"name":"我的印花"}` | `data`：新项目、空图、revision `"1"` |
| `GET /pod/projects/:id` | 路径 ID | `data`：有效项目详情，含 graph |
| `PUT /pod/projects/:id` | `revision`，以及 `name/graph` 中至少一项 | `data`：更新后完整项目 |
| `DELETE /pod/projects/:id` | `{"revision":"1"}` | `data`：软删除后项目摘要 |
| `POST /pod/projects/:id/restore` | `{"revision":"2"}` | `data`：恢复后项目摘要 |

项目字段为 `projectId/workspaceId/name/revision/deleted/createTime/updateTime`，详情另带 `graph`。
ID、revision 为规范的正整数十进制字符串，不接受数值类型、前导零、负数、溢出值。
名称去除两端空白后须为 1–100 个 Unicode 字符，不能含控制字符；时间为 `2006-01-02 15:04:05`。
创建、更新、删除和恢复 DTO 拒绝未知 JSON 字段和多段 JSON；客户端不能指定资源归属。

保存示例：

```json
{
  "revision": "1",
  "graph": {
    "schemaVersion": 1,
    "nodes": [
      {
        "id": "brief",
        "type": "textInput",
        "version": 1,
        "position": {"x": 80, "y": 100},
        "config": {"text": "蓝绿色几何花朵，适合白色 T 恤"}
      }
    ],
    "edges": []
  }
}
```

列表默认每页 10 条，上限 100，超出截断；页面每页 12 条。
排序固定为 `update_time DESC, project_id DESC`，不接受客户端排序字段。
静态数据翻页有唯一兜底列；并发编辑导致项目更新时间变化时，跨请求的 OFFSET 分页不提供快照一致性。

普通业务响应沿用 HTTP 200 + `code/msg`：参数错误 400、未登录 401、空间拒绝 403、资源不可见 404、版本冲突 409。
请求体超限等全局中间件行为维持既有约定；内部错误不向客户端透传 SQL、配置或凭据。

## 3. 授权与写入一致性

- 普通创作者只能访问自己的项目，所有读取包含 owner、有效空间和有效成员条件；管理员也不绕过私有资源边界。
- 创建、保存、重命名、删除、恢复在 repository 事务内锁定空间与成员，写入时再次检查归属。
- 更新项目时锁定项目行并比较 revision，成功后版本加一；两个页面用同版本同时保存，只能一个成功。
- 删除与恢复同样比较并递增 revision，项目内容保留，其他用户不能恢复；已删除项目不能按有效详情读取。
- 开通空间时锁定本人账号，已有有效空间直接返回；停用空间或撤销成员不会被 init 请求重新激活。

当前个人创作动作共用 `pod:studio:access`，没有另做逐动作运营角色，也不支持团队邀请或协同编辑。
`admin` 默认没有创作空间，可以显式开通自己的空间，不能用管理员身份读取其他创作者的项目。

新增接口前的七项核对：

| 问题 | 本阶段答案 |
|---|---|
| 分页 | 唯一业务列表为 projects，服务端上限 100；详情与单次写入不返回无限集合 |
| 索引 | 复用 owner 唯一索引、成员主键/用户索引、项目主键和 `(workspace_id,del_flag,update_time,project_id)`；尚未做大数据 EXPLAIN 或吞吐验收，不宣称无扫描或无排序 |
| N+1 | 列表 count + 一次列表查询，详情为单资源查询；无逐项目查库 |
| 超时 | 请求 context 贯穿 service/repository，复用 DB 超时；本轮无模型或其他外部 HTTP 调用 |
| 权限 | 登录 + `pod:studio:access` + 本人有效空间/成员；写事务内再次检查 |
| 错误 | 统一 response/BizError，401/403/404/409 等为业务码，内部细节不透传 |
| JSON | 显式 camelCase，ID/revision 字符串，列表顶层 rows/total，其余 data |

## 4. 图与节点配置

`schemaVersion=1`，节点类型 `version=1`。节点/边 ID 匹配 `[A-Za-z0-9_-]{1,64}`，各自唯一；
位置为有限数值且绝对值不超过 1,000,000。节点 config 必须是对象，每个不超过 64 KiB。
端口规格见 [POD_FOUNDATION.md](./POD_FOUNDATION.md)。

服务端拒绝缺失节点、自环、循环、重复 ID、不存在或不兼容的端口，以及同一输入端口的多个上游。
允许一个输出连多个下游。草稿允许空图、暂缺必填连线和未配置字段，不等于通过执行前完整性校验。

所有节点均可选填字符串 `title`（最多 60 字符），其余配置只接受下列字段：

| 类型 | 字段与范围 |
|---|---|
| `textInput` | `text` 字符串，最多 12000 字符 |
| `imageInput` | `assetIds` 目前只接受空数组，等待素材阶段 |
| `textGenerate` | `instruction` 字符串，最多 12000 字符 |
| `imageGenerate` | `prompt` 最多 12000 字符；`ratio` 为 1:1/3:4/4:3/9:16/16:9；`count` 整数 1–4 |
| `imageProcess` | `operation` 为 resize/crop/removeBackground/upscale；`width/height` 整数 64–8192；`scale` 为 2 或 4 |
| `productPreview` | `product=tshirt`；`color` 为六位十六进制色；`printX/printY` 为 0–1；`printScale` 为 0.05–1.5 |
| `export` | `format` 为 png/jpeg/webp；`fileName` 为 1–80 个字母、数字、下划线、空格或短横线；`mode` 为 print/mockup/image |

未知字段（包括 apiKey）拒绝；字段存在时必须满足类型和范围，不能用 null 代替。
模型、供应商参数和私有素材归属尚未接入，不能把这些配置当作可执行任务。

| POD 配置 | 默认 | 可配置范围 |
|---|---|---|
| `maxNodes` | 100 | 1–100；0 或不填使用默认 |
| `maxEdges` | 300 | 1–300；0 或不填使用默认 |
| `maxGraphBytes` | 1048576（1 MiB） | 1–8388608；0 或不填使用默认 |

负值或超出上限拒绝启动。bootstrap 返回有效 limits，前端依此约束添加/连线和图大小，服务端完整复核。
图大小限制与 HTTP 请求体上限分别生效，提高图上限不自动提高全局请求体上限。

## 5. 页面操作与保存

1. 用本地 `.local/accounts.json` 中的 `pod_creator` 登录 `http://127.0.0.1:5177/`，填写验证码。
2. 在“项目”点击新建并填写名称；打开项目后，点击左侧节点，或将其拖入画布。
3. 拖动节点标题调整位置，从输出圆点连到同类型输入圆点；也可在右侧用表单选择连接。
4. 点击节点，在右侧配置参数；文本输入节点也可直接填写文字。等待顶部显示“已保存”。
5. 返回项目列表可重命名或移入回收站；在回收站恢复项目，节点和配置保持不变。

| 操作 | 快捷键 |
|---|---|
| 保存 | `Ctrl+S`，输入框内也有效 |
| 多选 / 框选 | `Ctrl/Command` 多选；`Shift` 框选 |
| 全选节点 / 复制选中节点 | `Ctrl+A` / `Ctrl+D` |
| 删除节点或连线 | `Delete` 或 `Backspace` |
| 撤销 / 重做 | `Ctrl+Z` / `Ctrl+Shift+Z` 或 `Ctrl+Y` |

除保存外，上述快捷键在输入框编辑期间不接管文字操作；Mac 可使用 Command。
撤销最多保留 50 步，复制时保留所选节点间的连线，不复制到未选节点的连接。
视口缩放/位置单独缓存于浏览器，节点位置、配置与连线保存到服务端。

自动保存防抖 800ms，提交按次串行；保存期间的新编辑使用上次成功返回的版本继续提交。
使用语义 JSON 比较，MySQL 调整键顺序不会把内容误判成新修改。

失败的修改立即缓存到本机浏览器，键按 `userId/workspaceId/projectId` 隔离。
网络恢复或点击重试可再次保存；遇到 409 则停止自动覆盖，可以下载 JSON 草稿、另存为新项目，或明确放弃后加载服务端版本。
刷新后发现未保存草稿会提示选择；旧版本草稿恢复后保持冲突状态，不悄悄覆盖新版本。
本地缓存不可用或容量不足会提示；离开仍有修改的页面提供提醒。
缓存只属于当前浏览器，未加密，也不是服务端备份，换浏览器不会自动同步未提交内容。

## 6. 生成器与实现依据

按仓库约束，先通过现有 `renderGeneratedFiles` 对 `pod_project` 元数据生成 9 个文件，
产物保存在忽略提交的 `.local/generated/project/`，再取 model/repository/service/handler 四层骨架适配。
生成用临时测试已移除；未修改生成器模板，未安装缺少归属约束的通用导出、路由、管理 Vue 页面或菜单 SQL。

主要实现：`internal/{model,repository,service,handler}/pod_project.go`、`internal/workflow/`、
`studio/src/Projects.vue`、`ProjectEditor.vue`、`WorkflowNode.vue`、`draft.ts` 和 `graph.ts`。
Vue Flow 与测试依赖锁定在 package/lockfile；仅允许已核对的 `vue-demi` Vue 版本桥接安装脚本，未开放全部生命周期脚本。

## 7. 验收证据与限制

本轮 `scripts/pod.ps1 -Action test` 完整退出码为 0，包含：

- Go 单元测试：`./internal/... ./pkg/... ./cmd/... ./sql/pod`。
- POD 专项真实数据库集成：原 13 个基础叶子用例 + 新 10 个项目叶子用例，共 23 个；不把父级测试分组重复计数。
- 项目用例覆盖保存重开、跨用户读写删除恢复、同版本并发写、旧版本冲突、非法字段/图配置、回收站、首次开通、撤销成员、103 条记录分页与上限。
- 前端 16 个测试：API 3、图编辑 4、保存状态 4、Vue Flow + ProjectEditor 组件 5；覆盖输入、连线、位置更新、复制/删除、撤销/重做、保存失败重试和旧草稿。
- `go vet ./...`、`go build ./...`、前端类型检查与生产构建通过；`git diff --check` 通过。

专项测试在专用库创建并按精确 ID 清理测试项目、临时受限账号与会话，测试中的成员/空间状态恢复。
验证码保持开启；未跑继承根 test 的默认旧库集成，也未操作 demo/demo-b 或其他业务库。

本地运行探测：API `18110/health` 返回 HTTP 200，MySQL/Redis 均为 up；创作端 `5177/` 返回 HTTP 200，
`5177/api/health` 代理正常；未登录请求 `5177/api/pod/projects` 返回业务码 401。

尚未验收：

- 浏览器宿主连接不可用，尚无真实鼠标拖拽、触控、小屏视觉、跨浏览器和截图证据；happy-dom 组件测试不能替代这些检查。
- 当前已有 SQL 备份，但尚未在新库完成恢复演练。
- 未做大规模项目列表性能、长时间编辑或生产部署验收。

下一开发步骤是私有素材：上传、缩略图、素材选择、归属校验、鉴权读取与下载，再将 imageInput 接到真实素材。
