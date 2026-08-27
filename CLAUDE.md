# ruoyi-go

用 Go 复刻 RuoYi 后端。**前端和数据库都不动**：

- 前端直接用 `../RuoYi-Vue3-master`，一行不改
- 表结构直接用 `../RuoYi-Vue-master/sql/ry_20260417.sql`，一行不改
- Go 侧只替换中间层

所以**接口契约是硬约束，不是参考**。任何"这样设计更合理"的改动，只要前端不认，就是错的。
参考实现在 `../RuoYi-Vue-master`，行为有疑问时以那份 Java 代码为准。

契约细则见 @docs/CONVENTIONS.md ，历史决议见 @docs/DECISIONS.md ，压测方法见 [docs/PERF.md](./docs/PERF.md)。

## 技术栈

| 用途 | 选型 |
|---|---|
| Web | `gin-gonic/gin` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/mysql` |
| 缓存 | `redis/go-redis/v9` |
| JWT | `golang-jwt/jwt/v5` |
| 配置 | `spf13/viper` |
| 日志 | `log/slog`（标准库） |
| 参数校验 | `go-playground/validator`（gin 内置） |
| 定时任务 | `robfig/cron/v3` |
| Excel | `xuri/excelize/v2` |
| 密码 | `golang.org/x/crypto/bcrypt` |

不要引入其他 Web 框架或 ORM。需要加新依赖时先说明理由再加。

## 分层与依赖方向

```
configs/             application.yml（本地覆盖用 application.local.yml，已 gitignore）
cmd/server/          入口、装配、优雅关闭
cmd/perfseed/        压测数据生成器（独立工具，见 docs/PERF.md）
internal/
  config/    [L0]    配置加载
  model/     [L1]    GORM 实体、请求/响应 DTO
  job/       [L1]    定时任务注册表（不依赖 DB，需要 DB 的任务由 service 注册）
  datascope/ [L2]    数据权限条件构造，返回 GORM Scope
  repository/[L2]    数据访问，只有这一层能持有 *gorm.DB
  service/   [L3]    业务逻辑、事务边界
  handler/   [L4]    HTTP 处理器
  middleware/[L4]    Gin 中间件
  router/    [L5]    路由注册、权限标识挂载
pkg/
  response/          统一响应封装（AjaxResult / TableDataInfo 语义）
  page/              分页与排序参数解析（含 orderByColumn 白名单）
  errs/              业务错误类型 BizError
  types/             types.Time 等基础类型
  jwtx/              JWT 签发与校验
  logx/              traceId 注入与 slog Handler 包装
  cronx/             Quartz 表达式到 robfig/cron 的适配（星期编号差 1，见 DECISIONS）
  redisx/            Redis 客户端与 key 常量
  excelx/            Excel 导出（struct tag 驱动，对齐 @Excel）
  validate/          自定义校验规则（notblank / xss / dicttype）
docs/
```

**只能从高层引低层，禁止反向引用，禁止跨层跳跃。** handler 不准直接引 repository，必须经 service。

同层依赖只允许一条：**handler → middleware**（取当前会话、挂权限中间件），不得反向。

> 注：Go 包名不能以数字开头，所以层级用上面的 `[Ln]` 标注表达，不像其他项目那样用 `00-` 前缀做目录名。改动前先确认没有破坏这个方向。

## 硬约束（不可违反）

1. **响应必须走 `pkg/response`**，禁止在 handler 里手写 `c.JSON` 拼裸 map
2. **所有列表接口必须分页**，`pageSize` 硬上限 100，超出则截断而不是报错
3. **所有 DB / Redis / HTTP 调用必须带 `context.Context` 和超时**，禁止 `context.TODO()` 进主干代码
4. **只有 repository 层能获取、保存或直接操作 `*gorm.DB`**。`internal/datascope`
   可以构造 GORM Scope；service 可以组合和传递 `func(*gorm.DB) *gorm.DB` 类型的
   Scope，但不得获取数据库连接、调用 GORM 查询方法或直接执行 SQL
5. **禁止用 panic 控制流程**，错误一律用 error 返回；middleware 统一 recover 并返回 500
6. **循环里禁止查库**，遇到 N+1 用批量查询或 `Preload`
7. **密码只用 bcrypt**，禁止出现在任何日志、响应、错误信息里；Cookie、Token 同理
8. **时间统一 `Asia/Shanghai`**，JSON 序列化格式固定 `2006-01-02 15:04:05`
9. **JSON 字段名必须是 camelCase**（`userName` / `deptId` / `createTime`），跟 Java bean 一致。Go struct tag 必须显式写，不能靠默认
10. **新建业务表必须带** `create_by` `create_time` `update_by` `update_time` `remark`，对齐 RuoYi 的 `BaseEntity`

## 新增接口前必须回答的七个问题

写代码之前先在回复里逐条回答，答不上来的先问：

1. 分页了吗？`pageSize` 有上限吗？
2. 查询条件涉及的列有索引吗？会不会全表扫？
3. 循环里有查询吗？（N+1）
4. 外部调用带 context 和超时了吗？
5. 权限标识是什么（形如 `system:user:list`）？需不需要数据权限过滤？
6. 错误路径返回的是统一 code 吗？敏感信息会不会漏进 msg 或日志？
7. 响应的 JSON 字段名跟前端期望一致吗？（camelCase，且平铺规则见 CONVENTIONS）

## 常用命令

```bash
go build ./...            # 编译
go vet ./...              # 静态检查
gofmt -l .                # 列出未格式化文件
go test ./...             # 测试
go run ./cmd/server       # 本地启动
```

提交前 `gofmt` + `go vet` 必须干净。

## 工作方式

- 改动前先读相关的 Java 实现，别凭记忆猜 RuoYi 的行为
- **性能和能力上的判断必须有数字，不能只靠原理推断。** 这条是拿三次教训换来的：

  | 推断 | 实测 |
  |---|---|
  | 「`find_in_set` 走不了索引，是明确的性能问题」 | 只占 3.6%，真凶是旁边的 DISTINCT 和 JOIN |
  | 「Java 能后台配任意 bean，Go 做不到，能力有差距」 | Java 白名单锁死在一个包，同样要改代码 |
  | 「StreamWriter 流式写，瓶颈在数据切片不在工作簿」 | 切片只占 16%，反了 |

  三次的共同点：推理链条本身没错，**错在没验证前提**。
  写下"因为 X 所以 Y"之前，先问一句 X 有没有量过。
- 优先小步修改，不要推倒重写已经跑通的模块
- 模块化 CRUD 一律用代码生成器产出，不要手写；生成器模板改了要说明
- 做完一个模块，先补 `test/` 下的接口测试并全绿，再说完成；前端点一遍是补充验证不是主证据
- 跑测试：`.\scripts\test.ps1 [用例名正则]`，结果在 `test/results/summary.log`
- 任何影响契约、分层、依赖的决定，追加一条到 @docs/DECISIONS.md
