# 已迁移接口契约复核

复核日期：2026-08-24  
基准：当前工作区的 Go 实现、Java RuoYi 实现和 Vue3 实际请求代码。

## 范围

本次只核对已迁移的核心业务接口：认证、系统管理、监控、定时任务、通知公告和通用文件。

明确排除：

- `/tool/gen/**` 代码生成器：按当前决定不做，不计入缺口、不提出实现建议。
- `/test/user/**`：Java Swagger 示例接口，不属于业务接口。
- Swagger UI、Druid 页面、健康检查：属于文档或运维页面，不作为业务 DTO 契约核对对象。

## 路由核对

| 项目 | 结果 |
|---|---|
| Java 管理端和 Quartz Controller 注解 | 133 条，其中包含 5 条 `/test/user/**` 示例 |
| Go Router 注册 | 131 条 |
| 已迁移核心路由 | HTTP 方法和路径已对齐 |
| Java `/logout` | 由 Spring Security 注册，Go 已注册 `POST /logout` |
| Go `/health` | 运维附加接口，不属于 Java 兼容接口 |
| `/system/user/` 尾斜杠 | Go 使用 Gin 的规范路径 `/system/user`，前端可正常访问 |

## 当前字段与参数核对

| 接口 | Vue3 实际请求 | Java 接收方式 | Go 接收方式 | 结果 |
|---|---|---|---|---|
| `PUT /system/user/profile/updatePwd` | JSON `oldPassword`、`newPassword` | `@RequestBody Map<String,String>` | `ShouldBindJSON(UpdatePwdBody)` | 对齐 |
| `GET /system/notice/readUsers/list` | query `noticeId`、`searchValue` | `Long noticeId, String searchValue` | `NoticeReadUserQuery` 的同名 `form` 字段 | 对齐 |
| `GET /system/notice/{noticeId}` | path `noticeId` | 仅全局登录认证 | Go 未附加单独权限中间件 | 对齐 |
| `PUT /system/user/authRole` | query `userId`、`roleIds` | Spring 参数绑定 | Go query 绑定 | 对齐 |
| `PUT /system/role/authUser/cancelAll`、`selectAll` | query `roleId`、`userIds` | Spring 参数绑定 | Go query 绑定 | 对齐 |
| 新增/修改用户、角色、岗位、部门、菜单、字典、配置、公告、任务 | JSON Body | `@RequestBody` 实体/DTO | `ShouldBindJSON` 实体/DTO | 对齐 |
| 列表接口 | query 分页和筛选字段 | Java Bean 参数绑定 | Go `ShouldBindQuery` | 对齐 |
| 导出接口 | POST 表单参数 | Java Bean 参数绑定 | Go `ShouldBind` | 对齐 |

主要字段类型已保持一致：Java `Long` 对应 Go `int64`，Java `String` 对应 Go `string`，Java primitive `boolean` 对应 Go `bool`，ID 数组对应 `[]int64`；JSON 字段均采用前端所需的 camelCase。

## 校验差异

未发现未登记的字段校验差异。以下差异是项目明确保留的“有意加严”，已在 `docs/CONVENTIONS.md` 登记，不能视为遗漏：

| 字段 | Java | Go | 说明 |
|---|---|---|---|
| `postSort`、`orderNum`、`roleSort` | `@NotNull`，未限制负数 | 指针 + `required,min=0` | 保留 `0` 合法和必填语义，并拒绝无业务意义的负数 |
| `dictSort` | 可空 | 指针 + `required,min=0` | 防止 NULL 排序造成列表顺序不确定 |
| 部分状态/类型字段 | Java 未全部做 Bean Validation | Go `required` | 依据数据库非空或前端必传约束加严 |

`dictType` 的小写字母、数字、下划线规则在当前 Java `SysDictType` 和 Go 中均存在，已对齐。

## 回归证据

当前测试源码已覆盖本轮最容易漂移的契约：

- `test/user_test.go`：密码更新必须使用 JSON Body，query 参数不得生效。
- `test/notice_test.go`：公告已读用户使用 `searchValue` 搜索；普通登录用户可读取公告详情。
- `test/dept_test.go`、`test/menu_test.go`、`test/role_test.go`、`test/post_test.go`：排序字段缺失、`null`、`0`、负数与类型错误。
- `test/dict_test.go`：`dictType` 格式和 `dictSort` 边界。

本轮未执行这些集成测试，因为它们会创建、更新、删除数据库数据；此文档结论是源码静态复核结果。

## 仍需运行时对拍的范围

静态核对无法保证响应字节级一致，后续若要继续，应在隔离的 MySQL/Redis 环境对拍 Java 与 Go：

- Java `null` 与 Go 零值、指针、`omitempty`。
- 时间格式和动态字段，例如 `nextValidTime`。
- 非法请求的错误文案、错误字段顺序和响应 JSON。
- Excel 下载的表头、文件名、Content-Type 与日期单元格。
- 多角色数据权限、缓存和真实数据库 NULL 值下的结果集合。
