# ruoyi-go 新手学习手册

> 适合读者：刚开始学习 Go，能看懂基本的 Java、JavaScript 或其他语言，希望通过本项目掌握真实 Web 后端开发。
>
> 本文不是单独讲语法的教材，而是一张“边学 Go、边读懂 ruoyi-go”的路线图。遇到文档和代码不一致时，以当前代码、测试和 `docs/DECISIONS.md` 中较新的决议为准。

## 1. 先建立全局认识

这个项目是 RuoYi-Vue Java 后端的 Go 实现，主要技术如下：

| 领域 | 项目采用的技术 | 用来做什么 |
|---|---|---|
| HTTP | Gin | 路由、参数绑定、中间件和响应 |
| 数据库 | MySQL + GORM | 保存用户、角色、菜单、日志等业务数据 |
| 缓存/会话 | Redis | 登录会话、验证码、配置缓存、权限版本 |
| 身份认证 | JWT | 客户端携带访问令牌 |
| 配置 | Viper + YAML | 加载 `configs/application.yml` 和环境变量 |
| 定时任务 | robfig/cron | 执行和管理定时任务 |
| Excel | excelize、xls | 导入和导出 Excel |
| 测试 | Go `testing` | 单元测试、接口测试、并发测试 |

项目要求 Go 1.25，版本来源见 `go.mod`。不要为了兼容本机旧 Go 而手工降低 `go.mod` 中的版本，因为当前依赖已经要求较新的 Go。

## 2. 项目目录怎么读

```text
ruoyi-go/
├─ cmd/
│  ├─ server/          # 正式服务入口
│  ├─ contractcheck/   # Java/Go 双服务接口契约检查
│  ├─ routeaudit/      # 静态路由和权限审计
│  ├─ perfbench/       # 性能压测工具
│  └─ perfseed/        # 压测数据准备工具
├─ configs/            # YAML 配置
├─ internal/
│  ├─ config/          # 配置结构、默认值和启动校验
│  ├─ model/           # 请求体、查询条件、数据库模型
│  ├─ handler/         # HTTP 参数读取和响应
│  ├─ service/         # 业务规则、事务编排、并发控制
│  ├─ repository/      # 唯一允许直接访问数据库的业务层
│  ├─ middleware/      # 登录、权限、日志、限流等中间件
│  ├─ router/          # 路由注册
│  ├─ datascope/       # 数据权限条件
│  └─ job/             # 定时任务调度器
├─ pkg/                # 可复用的基础组件
│  ├─ response/        # 统一响应结构
│  ├─ page/            # 分页与安全排序
│  ├─ redisx/          # Redis 客户端和 Key 契约
│  ├─ errs/            # 业务错误
│  ├─ types/           # 时间等通用类型
│  └─ ...
├─ docs/               # 约定、决议、审计和验证报告
├─ scripts/            # 构建、测试和对拍脚本
├─ sql/                # 初始化或压测 SQL；不要随便在生产执行
├─ test/               # 会访问真实 MySQL/Redis 的接口集成测试
├─ CLAUDE.md           # 项目最高层开发约束
└─ go.mod              # Go 模块名与依赖
```

初学时不要从几千行代码连续往下读。建议始终沿着一条请求链路阅读：

```text
浏览器请求
  → router 路由
  → middleware 鉴权/权限/日志
  → handler 解析参数
  → service 执行业务规则
  → repository 访问 MySQL
  → response 返回统一 JSON
```

## 3. 第一次运行项目

### 3.1 检查环境

```powershell
go version
go env GOPATH GOMOD
```

`go env GOMOD` 应指向当前项目的 `go.mod`。如果显示空值或其他项目路径，说明终端目录不对。

### 3.2 理解配置

主配置文件是：

```text
configs/application.yml
```

重点配置包括：

- `server`：端口、超时、可信代理、跨域来源。
- `mysql`：DSN 和连接池。
- `redis`：地址、密码、数据库编号和连接池。
- `jwt`：密钥、过期时间和续期窗口。
- `upload`：上传目录和大小限制。
- `gen`：代码生成器默认作者、表前缀和安全输出目录。

Viper 允许用环境变量覆盖 YAML。例如：

```powershell
$env:MYSQL_DSN = "root:password@tcp(127.0.0.1:3306)/ry?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
$env:JWT_SECRET = "your-secret"
go run ./cmd/server
```

环境变量名使用大写加下划线，因为 `internal/config/config.go` 将配置路径中的点转换成了下划线。

### 3.3 常用命令

```powershell
# 启动
go run ./cmd/server

# 编译所有包
go build ./...

# 运行不会主动访问当前业务库的单元测试
go test ./cmd/... ./internal/... ./pkg/... -count=1

# 静态检查
go vet ./...

# 查看未格式化的 Go 文件；正常应无输出
gofmt -l .
```

注意：`go test ./...` 会包含根目录下的 `test/`，其中一些测试会连接并修改配置的数据库。没有隔离测试库时，不要随便执行。

## 4. 用本项目理解 Go 基础语法

### 4.1 package 与 import

每个 Go 文件开头都有包名：

```go
package service
```

同一目录下的普通 `.go` 文件必须属于同一个包。引用其他包时使用 `import`：

```go
import (
    "context"
    "ruoyi-go/internal/model"
    "ruoyi-go/internal/repository"
)
```

本项目模块名是 `ruoyi-go`，所以项目内导入从 `ruoyi-go/...` 开始，而不是从磁盘路径开始。

### 4.2 大小写决定可见性

Go 没有 Java 的 `public`、`private` 关键字：

- 首字母大写：可以被其他包访问，例如 `CreateUser`。
- 首字母小写：只能在当前包访问，例如 `normalizeUserName`。

这条规则同时作用于函数、结构体、字段、常量和变量。

### 4.3 struct 相当于数据结构

```go
type LoginBody struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
}
```

可以把 `struct` 暂时理解成 Java 的 DTO/POJO，但 Go 通常不写 getter/setter。

反引号中的内容叫结构体标签：

- `json:"username"`：JSON 字段名。
- `form:"pageNum"`：查询参数名。
- `gorm:"column:user_id"`：数据库列名。
- `binding:"required"`：Gin 参数校验。

修改字段时要同时考虑 JSON、数据库、前端和 Java 契约，不能只看 Go 类型。

### 4.4 指针

```go
func UpdateUser(ctx context.Context, user *model.SysUser) error
```

`*model.SysUser` 表示传入的是对象地址，函数可以修改原对象。常见判断：

```go
if user == nil {
    return errs.New("用户不能为空")
}
```

项目模型中的 `*string`、`*int64` 还用于区分“没有值”和零值。例如：

- `nil`：客户端没传，或者数据库是 `NULL`。
- `ptr(0)`：明确传了数字 `0`。

这种区别在更新接口中非常重要。

### 4.5 切片与 map

切片是动态数组：

```go
ids := make([]int64, 0, len(users))
for _, user := range users {
    ids = append(ids, user.UserID)
}
```

map 是键值表：

```go
seen := make(map[int64]struct{})
if _, exists := seen[id]; exists {
    // 已经存在
}
seen[id] = struct{}{}
```

项目经常使用 `map[T]struct{}` 做集合，因为空结构体几乎不占额外空间。

### 4.6 多返回值与 error

```go
user, err := repository.SelectUserByID(ctx, userID)
if err != nil {
    return nil, err
}
if user == nil {
    return nil, errs.New("用户不存在")
}
```

Go 没有强制异常机制，失败通常通过最后一个 `error` 返回。要养成习惯：调用可能失败的函数后立刻检查 `err`。

不要写成：

```go
user, _ := repository.SelectUserByID(ctx, userID)
```

忽略错误会把数据库故障伪装成“数据不存在”。

### 4.7 defer

```go
userWriteMu.Lock()
defer userWriteMu.Unlock()
```

`defer` 会在当前函数返回前执行，适合释放锁、关闭文件和恢复资源。锁成功后马上写 `defer Unlock()`，可以减少遗漏解锁造成的死锁。

### 4.8 interface 与依赖边界

Go 的接口是隐式实现的：类型只要拥有接口要求的方法，就自动满足接口，不需要写 `implements`。

本项目没有为了“看起来高级”而给每个 service/repository 都套接口。只有需要替换、测试注入或隔离实现时才抽象接口。新手不要机械照搬 Java 的 interface + impl 结构。

### 4.9 goroutine 和 channel

```go
go worker()
```

会并发启动一个轻量任务。channel 用于 goroutine 之间传递信号或数据：

```go
wake := make(chan struct{}, 1)
wake <- struct{}{}
```

并发代码最容易出现数据竞争、泄漏和关闭顺序问题。初学阶段先读普通 CRUD，再读 `internal/service/permission_recovery.go`、异步池和任务调度器。

## 5. 读懂一条 HTTP 请求

以下以常见列表接口为例。

### 5.1 Router：请求先去哪里

路由位于 `internal/router/router.go`。典型写法：

```go
group.GET("/list",
    middleware.HasPermission("system:user:list"),
    handler.UserList,
)
```

它表达三件事：

1. HTTP 方法是 GET。
2. 路径是 `/system/user/list`。
3. 必须拥有 `system:user:list` 权限，才能进入 `UserList`。

权限标识是接口契约，不要随意改字符串。菜单表 `sys_menu.perms`、前端按钮和后端路由必须一致。

### 5.2 Middleware：进入 handler 前的关卡

全局中间件执行顺序可在 `router.New` 中看到，大致包括：

1. Trace：生成请求追踪 ID。
2. Recovery：捕获 panic，避免整个进程退出。
3. Logger：记录请求日志和慢请求。
4. CORS：处理跨域。
5. RequestTimeout：给请求设置总超时。
6. Authenticate：从 JWT 和 Redis 恢复登录用户。
7. HasPermission/HasRole：判断权限。
8. OperLog：对增删改操作记录日志。

中间件顺序会影响安全性和日志完整性，不要随意调整。

### 5.3 Handler：只处理 HTTP 细节

Handler 通常负责：

- 从路径、查询字符串或 JSON 读取参数。
- 调用 service。
- 使用统一 response 返回。

示意代码：

```go
func UserList(c *gin.Context) {
    var query model.UserQuery
    if err := c.ShouldBindQuery(&query); err != nil {
        response.Fail(c, "查询参数错误")
        return
    }

    list, total, err := service.ListUserPage(
        c.Request.Context(),
        query,
        page.Parse(c, model.UserSortColumns),
    )
    if err != nil {
        fail(c, err)
        return
    }
    response.Page(c, list, total)
}
```

Handler 不应该直接写 SQL，也不应该持有 `*gorm.DB`。

### 5.4 Service：决定业务是否允许发生

Service 负责：

- 参数的业务校验。
- 权限和数据范围判断。
- 唯一性检查。
- 多个 repository 操作的编排。
- 缓存和会话刷新。
- 单实例并发写入顺序。

例如新增用户不是简单地 `INSERT`：还要检查用户名、手机号、角色、部门和当前操作人权限，并维护用户与岗位/角色的关联。

### 5.5 Repository：唯一的数据库入口

Repository 位于 `internal/repository`，这里可以使用 GORM：

```go
func SelectUserByID(ctx context.Context, userID int64) (*model.SysUser, error) {
    var user model.SysUser
    err := DB(ctx).Where("user_id = ?", userID).Take(&user).Error
    // 处理未找到和真实数据库错误
    return &user, nil
}
```

SQL 条件中的用户输入必须使用 `?` 参数绑定，不要字符串拼接：

```go
// 正确
db.Where("user_name = ?", userName)

// 错误：存在 SQL 注入风险
db.Where("user_name = '" + userName + "'")
```

## 6. GORM 和数据库应该重点学什么

### 6.1 查询一条数据

```go
err := repository.DB(ctx).
    Table("sys_user").
    Where("user_id = ?", id).
    Take(&user).Error
```

区分两类结果：

- `gorm.ErrRecordNotFound`：正常的“没有这条数据”。
- 其他 error：连接断开、超时、SQL 错误等系统故障。

### 6.2 分页

项目统一使用 `pkg/page`：

```go
pg := page.Parse(c, model.UserSortColumns)
db.Order(pg.Stable("user_id", "user_id")).
    Offset(pg.Offset()).
    Limit(pg.PageSize)
```

这里有三个安全点：

- 页面大小最大 100，避免一次查出海量数据。
- 排序字段只能来自白名单，避免 ORDER BY 注入。
- 即使用户指定的排序列有重复值，也追加唯一主键保证翻页稳定。

### 6.3 更新

本项目偏向显式更新字段：

```go
updates := map[string]any{
    "nick_name": user.NickName,
    "status":    user.Status,
}
result := DB(ctx).Table("sys_user").Where("user_id = ?", user.UserID).Updates(updates)
```

显式字段比直接 `Save(&user)` 更容易控制哪些字段允许修改，也能避免客户端把不该改的字段一起写入。

### 6.4 事务

```go
err := repository.Transaction(ctx, func(tx *gorm.DB) error {
    if err := tx.Table("main_table").Create(&main).Error; err != nil {
        return err
    }
    if err := tx.Table("sub_table").Create(&children).Error; err != nil {
        return err
    }
    return nil
})
```

回调返回 error 时事务回滚，返回 nil 时提交。事务内必须一直使用传入的 `tx`，不能又调用普通 `DB(ctx)`，否则后一个操作可能跑到事务外。

代码生成器生成的主子表新增、修改和删除，就是学习事务的好例子。

### 6.5 为什么当前项目还有进程锁

例如“先检查用户名不存在，再插入用户”是两个数据库动作。两个请求同时执行时，都可能检查通过。

当前明确部署边界是单服务器、单 Go 进程，所以 service 使用领域锁串行化同类写操作。它不能保护两个 Go 进程。如果以后改成多实例，必须重新设计数据库唯一约束或可靠的分布式协调，不能以为 `sync.Mutex` 能跨进程工作。

## 7. Context、超时和取消

Handler 使用：

```go
ctx := c.Request.Context()
```

并把它一路传入 service 和 repository。这样客户端断开或请求超时时，数据库和 Redis 操作可以尽快取消。

函数签名建议保持：

```go
func DoSomething(ctx context.Context, ...) error
```

不要在业务函数中随意改成 `context.Background()`，那会切断请求取消信号。只有明确需要脱离原请求完成的提交后补偿工作，才创建独立且有短超时的 context。

## 8. 登录、JWT、Redis 和权限

### 8.1 登录的大致过程

可以按下面顺序阅读：

1. `internal/router/router.go` 中的 `/login`。
2. `internal/handler/login.go`。
3. `internal/service/login.go`。
4. `internal/repository/user.go`。
5. `internal/service/token.go`。
6. `internal/middleware/auth.go`。

基本流程：

```text
校验验证码
→ 查询用户账户
→ 校验状态和密码
→ 查询角色/权限
→ 生成随机会话 UUID
→ 会话详情写入 Redis
→ JWT 只携带会话定位信息
→ 返回 token
```

JWT 并不保存完整权限。真正的会话和权限快照在 Redis 中，因此不能只会解析 JWT 就认为用户已经登录。

### 8.2 Redis Key 是契约

`pkg/redisx/keys.go` 集中定义 Redis Key 前缀。部署脚本、测试清理和运行时逻辑都依赖它们。

修改前缀相当于做一次数据迁移，不能当作普通重命名。尤其要理解：

- 登录会话 Key。
- 用户到会话的反向索引。
- 用户会话代数。
- 用户权限版本。
- 验证码和配置缓存。

### 8.3 权限为什么需要版本号

如果管理员修改了某角色权限，旧会话仍可能保存旧权限。项目会先推进 Redis 中的用户权限版本，再提交数据库变更。

后续请求发现“会话中的权限版本落后”时，会重新加载数据库权限并刷新会话。版本号保存在 Redis，所以即使 Go 进程重启，也不会因为内存状态丢失而恢复旧权限。

这是一个很好的学习主题：它同时涉及安全、并发、失败顺序、缓存一致性和进程重启。

## 9. 错误和响应

业务错误使用 `pkg/errs`：

```go
return errs.New("用户不存在")
```

底层错误需要保留原因：

```go
return fmt.Errorf("查询用户失败: %w", err)
```

`%w` 会保留错误链，方便上层判断和日志追踪。

HTTP 响应统一使用 `pkg/response`：

```go
response.Ok(c)
response.OkData(c, data)
response.Page(c, rows, total)
response.Fail(c, "参数错误")
```

不要在新 Handler 里随意发明另一套 JSON 结构。前端依赖 `code`、`msg`、`data`、`rows`、`total` 等既有字段。

## 10. 数据权限怎么理解

功能权限回答的是“能不能调用这个接口”，数据权限回答的是“调用后能看到哪些行”。

例如用户列表可能只能看到：

- 全部数据。
- 自定义部门。
- 本部门。
- 本部门及以下。
- 仅本人。

数据权限条件在 service 中决定，在 repository 查询上应用。不要把客户端传来的部门 ID 当成授权依据；客户端参数只能缩小查询范围，不能扩大当前用户本来允许看到的数据。

阅读入口是 `internal/datascope`，然后搜索各 service 中的数据权限构造和 repository 的 Scope 参数。

## 11. 代码生成器怎么用和怎么学

代码生成器位于：

- `internal/model/gen.go`
- `internal/repository/gen.go`
- `internal/service/gen.go`
- `internal/service/gen_sql.go`
- `internal/service/gen_render.go`
- `internal/handler/gen.go`
- `internal/handler/gen_contract.go`

### 11.1 使用流程

```text
进入代码生成页面
→ 从当前数据库选择普通业务表
→ 导入表和字段元数据
→ 编辑模块名、业务名、字段控件和模板类型
→ 预览
→ 下载 ZIP
→ 人工审查生成代码和菜单 SQL
→ 再放入项目
```

支持三种模板：

- `crud`：普通分页增删改查。
- `tree`：树形数据；前端按 100 条一页拉齐后组树，避免只显示第一页。
- `sub`：主子表；详情读取子项，增改删在数据库事务中处理主表和子表。

生成的业务模块还包含 Excel 导出接口、按钮和菜单权限。导出复用查询条件，并受最大行数和
全局并发限制保护；树形模板支持选择父节点、从当前行新增子节点，并在修改时排除自身子树。
生成的 service 还会独立检查父节点是否存在，并拒绝把节点挂到自身或自身后代。要记住：
前端过滤只能改善体验，不能代替后端的数据完整性校验，因为接口可以脱离页面直接调用。

支持前端类型：

- Element UI（Vue2）。
- Element Plus（Vue3 JavaScript）。
- Element Plus（Vue3 TypeScript）。

### 11.2 安全边界

- 元数据只读取当前 `DATABASE()`。
- 不能导入 `qrtz_`、`gen_` 表。
- 建表入口只接受未指定数据库名的 `CREATE TABLE`。
- 不接受 DROP、DELETE、存储过程、`DELIMITER` 或 MySQL 执行注释。
- 默认只预览/下载，不允许直接覆盖本地文件。
- 开启写盘后也只能写在 `gen.outputRoot` 中。
- 菜单 SQL 只生成，不自动执行。

即使生成成功，也必须人工检查：

- 字段类型是否符合业务含义。
- 必填和长度校验是否足够。
- 唯一性规则是否存在。
- 数据权限是否需要加入。
- 删除前是否要检查业务引用。
- 菜单上级 ID 和权限标识是否正确。
- 生成的接口是否需要导出、上传等扩展能力。

代码生成器解决重复劳动，不替代业务设计。

## 12. 测试应该怎么学

### 12.1 最小单元测试结构

```go
func TestSomething(t *testing.T) {
    got := Something("input")
    want := "expected"
    if got != want {
        t.Fatalf("got %q, want %q", got, want)
    }
}
```

测试文件以 `_test.go` 结尾，测试函数以 `Test` 开头。

### 12.2 表驱动测试

```go
func TestNormalize(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {name: "trim", input: " a ", want: "a"},
        {name: "empty", input: " ", want: ""},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := normalize(tt.input); got != tt.want {
                t.Fatalf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

项目中的白名单、边界值和非法参数校验很适合表驱动测试。

### 12.3 测试层级

| 层级 | 特点 | 适合验证 |
|---|---|---|
| 纯单元测试 | 不连接外部服务，快且稳定 | 解析、校验、渲染、排序、状态机 |
| Repository 测试 | 需要隔离数据库 | SQL、事务、行数和锁行为 |
| HTTP 集成测试 | 启动服务并访问 MySQL/Redis | 参数、权限、响应和副作用 |
| 双服务对拍 | 同时访问 Java 与 Go | 可观察契约是否一致 |
| 压测/稳定性测试 | 持续并发请求 | 吞吐、延迟、资源和恢复能力 |

### 12.4 常用测试命令

```powershell
# 某个包
go test ./internal/service -count=1

# 某个用例
go test ./internal/service -run TestParseCreateTableStatements -count=1

# 重复 100 次，发现时序和偶发问题
go test ./internal/service -run TestName -count=100

# 显示详细日志
go test ./internal/service -run TestName -v -count=1
```

`-count=1` 表示不使用测试缓存。修改代码后如果忘了它，可能误以为刚才的新代码已经被测试。

### 12.5 Race Detector

```powershell
go test -race ./internal/... ./pkg/... -count=1
```

Race Detector 用于发现并发读写冲突。在 Windows 上通常需要 CGO 和 GCC；缺少编译器时不能声称 race 已通过。普通单元测试通过也不能替代 race 检查。

## 13. 推荐阅读顺序

### 第一阶段：能看懂普通 CRUD

1. `go.mod`
2. `cmd/server/main.go`
3. `internal/router/router.go`
4. `internal/model/sys_post.go`
5. `internal/handler/post.go`
6. `internal/service/post.go`
7. `internal/repository/post.go`
8. `pkg/page/page.go`
9. `pkg/response/response.go`

岗位管理字段少、关系清晰，适合作为第一条完整链路。

### 第二阶段：用户、角色和数据权限

1. 用户 CRUD。
2. 用户与角色/岗位关联。
3. 角色菜单权限。
4. 数据权限 Scope。
5. 权限变更后的在线会话刷新。

这一阶段重点不是语法，而是“一个写操作会影响哪些表、缓存和在线用户”。

### 第三阶段：登录和 Redis

1. 验证码。
2. 登录。
3. JWT 创建与解析。
4. Redis 会话。
5. 自动续期。
6. 会话代数与权限版本。

### 第四阶段：并发和后台任务

1. service 领域锁。
2. 异步池。
3. 权限补偿 worker。
4. cron 调度器。
5. 优雅关闭。

### 第五阶段：代码生成器和契约工具

1. 元数据读取。
2. 配置导入与字段同步。
3. SQL 安全解析。
4. Go/Vue 模板渲染。
5. 路由静态审计。
6. Java/Go 双服务对拍。

## 14. 一套可执行的练习计划

### 练习 1：追踪岗位列表

目标：从路由开始，找到岗位列表最终执行的数据库查询。

完成标准：你能画出 Router → Handler → Service → Repository，并解释分页参数在哪里限制为最大 100。

### 练习 2：给纯函数补测试

找一个字符串解析、枚举校验或 ID 去重函数，为正常值、空值、边界值和非法值各写一个测试。

完成标准：

```powershell
go test <对应包> -run <你的测试名> -count=100
```

连续通过。

### 练习 3：新增一个只读字段

在不改变数据库结构的前提下，找一个已有数据库列，沿 Model → Repository → Handler 响应确认它如何返回前端。

完成标准：能说明 JSON 字段名、Go 类型、数据库列名和 Java 字段各是什么。

### 练习 4：用生成器生成演示模块

只能在隔离测试库执行：

1. 建一张 `demo_note` 表。
2. 导入代码生成器。
3. 预览普通 CRUD。
4. 下载 ZIP。
5. 检查生成的 Go 文件能否 `gofmt` 和 `go test`。
6. 检查菜单 SQL，但不要让生成器自动执行。

### 练习 5：模拟依赖故障

在测试环境停止 Redis 或 MySQL，访问 `/health` 和一个业务接口，然后恢复依赖。

完成标准：能解释为什么健康检查返回 503，以及业务恢复后是否需要重启 Go 服务。

## 15. 新手最容易犯的错误

### 错误 1：把所有逻辑写进 Handler

结果是业务规则无法复用、无法单测、事务边界混乱。Handler 应该薄，业务规则放 service，数据库放 repository。

### 错误 2：忽略 error

不要用 `_` 吞掉错误。至少返回、包装或记录它。

### 错误 3：直接拼 SQL

所有外部值使用参数绑定；ORDER BY 只能从白名单选择。

### 错误 4：随便启动 goroutine

必须考虑：谁取消、谁等待、panic 怎么办、服务关闭时如何排空。

### 错误 5：认为 map 是有序的

Go map 遍历顺序不保证稳定。对外顺序有要求时，把键放入切片并显式排序。

### 错误 6：返回 nil 切片导致契约变化

JSON 中 `nil` 切片通常是 `null`，空切片是 `[]`。Java/Vue 契约对某些字段会区分两者，不能凭感觉统一。

### 错误 7：把测试通过理解成全部正确

测试只证明覆盖到的条件。要问：异常分支测了吗、并发测了吗、数据库副作用测了吗、Java 契约测了吗？

### 错误 8：直接修改 Redis Key 或契约数据名字

配置键、字典类型、菜单权限和 Redis 前缀会被代码、前端、部署或测试按名字引用。改名需要迁移方案。

### 错误 9：把 Java 行为当成永远正确

Java 是兼容参考，不是缺陷免责依据。比如无稳定排序的分页行为不应该为了逐行复刻而保留。

### 错误 10：未审查生成代码就上线

生成器不知道你的业务唯一性、数据权限和删除引用关系。生成结果是起点，不是最终产品。

## 16. 遇到问题时怎么定位

建议按这个顺序：

1. 记录请求方法、URL、参数、返回码和 traceId。
2. 在 `internal/router/router.go` 找到路由。
3. 确认中间件有没有提前拒绝。
4. 阅读 Handler 的参数绑定位置。
5. 阅读 Service 的业务判断和调用顺序。
6. 找到 Repository 的 SQL/GORM 条件。
7. 判断失败属于参数、权限、业务、MySQL、Redis还是超时。
8. 写一个最小测试稳定复现，再修改代码。
9. 运行受影响包测试、全量单元测试、build 和 vet。

搜索代码优先使用：

```powershell
rg -n "函数名或路由" internal pkg cmd
rg -n "system:user:list" .
rg --files internal/service
```

## 17. 修改代码后的检查清单

- [ ] Handler 没有直接访问数据库。
- [ ] Service 没有持有 `*gorm.DB`，Scope 签名例外按项目约定处理。
- [ ] Repository 查询继承传入的 context。
- [ ] 外部 SQL 值使用参数绑定。
- [ ] 分页有最大值、排序白名单和唯一兜底列。
- [ ] 枚举、ID、必填值和批量数量有服务端校验。
- [ ] 多表写操作有明确事务边界。
- [ ] 数据库提交与 Redis/内存副作用的顺序经过失败场景分析。
- [ ] 权限标识同时核对菜单、路由、前端按钮和 Java 参考。
- [ ] 新行为有正常、边界、错误和必要的并发测试。
- [ ] 没有修改数据库结构、索引或契约数据，除非任务明确授权。
- [ ] `go test ./cmd/... ./internal/... ./pkg/... -count=1` 通过。
- [ ] `go build ./...`、`go vet ./...` 通过。
- [ ] `gofmt -l .`、`git diff --check` 无输出。
- [ ] 可观察行为变化已经写入 `docs/DECISIONS.md`。

## 18. 推荐继续阅读的项目文档

- `CLAUDE.md`：开发硬约束和已经踩过的坑。
- `docs/CONVENTIONS.md`：稳定编码规则。
- `docs/DECISIONS.md`：为什么采用当前设计。
- `docs/RUNTIME_AUDIT.md`：当前问题结论和验证边界。
- `docs/DEPLOYMENT.md`：部署与切换注意事项。
- `docs/PERF.md`：性能测试方法。
- `docs/API_ROUTE_COVERAGE_CURRENT.md`：Go 与 Java 路由和权限覆盖。

## 19. 最后给新手的建议

学习这个项目时，一次只解决一类问题：

1. 先让代码能读懂。
2. 再让一个小改动有测试。
3. 再理解事务和权限。
4. 最后碰并发、缓存一致性和性能。

不要追求一次记住所有语法。真正有效的方法是沿一条请求链路反复回答四个问题：

1. 输入从哪里来？
2. 谁判断它是否合法？
3. 数据最终在哪里读写？
4. 失败或并发发生时，系统会留下什么状态？

能稳定回答这四个问题，你就已经从“会写 Go 语法”走向“能维护 Go 服务”了。
