# 部署与切换检查单

本页记录生产部署时必须人工确认的运行时事项。接口契约与开发规则仍以
[CONVENTIONS.md](./CONVENTIONS.md) 为准，设计理由见 [DECISIONS.md](./DECISIONS.md)。

## Java 完全切换到 Go：清理残余登录会话

Go 会为每个用户维护 `login_user_sessions:<userId>` 反向索引。Java 和旧 Go 创建的
`login_tokens:*` 可能没有该索引；如果本次部署是 **Go 完全替换 Java 的停机切换**，
应清理残余登录会话，让恢复流量后的所有会话都由新版 Go 重新建立。

新版 Go 的按用户权限刷新和会话撤销只读取该反向索引，不会再全量扫描
`login_tokens:*` 兼容无索引旧会话。因此部署检查发现任何旧会话残余时，必须在恢复流量
前完成本页清理，不能带着残余直接启动新版服务。

新版 Go 会话还包含内部的 `sessionRevision`，用于阻止并发权限刷新互相覆盖。Java 和旧
Go 会话没有这一并发协议，也是硬切时必须清理旧会话、切换后禁止 Java 继续写入同一
Redis DB 的原因之一。

该操作会强制所有在线用户重新登录。它只适用于明确安排了重新登录窗口的硬切换，普通
Go 版本重启或滚动升级不要执行。

### 切换顺序

1. 从负载均衡或 Nginx 摘除业务流量。
2. 停止全部 Java 和旧 Go 实例，等待在途请求结束。
3. 核对目标 Redis 地址和 DB，确保操作的是 Go 生产环境使用的 DB。
4. 检查下面三个前缀是否存在残余 key；存在时按批次清理。
5. 再次检查三个前缀均为 0。
6. 启动新版 Go，完成健康检查后恢复流量。
7. 验证旧 Token 返回未认证，新登录会创建用户会话索引。

必须清理的会话前缀：

```text
login_tokens:*
login_user_sessions:*
login_user_generation:*
```

`login_user_generation:*` 也要一起清理，但只能在旧服务已停止、没有登录请求正在执行时
操作。否则删除代数 key 可能让一个尚未结束的旧登录请求重新写回会话。

### Linux/redis-cli 示例

先根据 `configs/application.yml` 或生产环境变量确认 Redis 地址、端口和 DB。密码通过
`REDISCLI_AUTH` 传递，不要放进命令行历史；无密码时不要设置该变量。

```bash
export REDISCLI_AUTH='实际 Redis 密码'
RUOYI_REDIS_HOST='127.0.0.1'
RUOYI_REDIS_PORT='6379'
RUOYI_REDIS_DB='0'

redis_cmd=(redis-cli -h "$RUOYI_REDIS_HOST" -p "$RUOYI_REDIS_PORT" -n "$RUOYI_REDIS_DB")

for pattern in 'login_tokens:*' 'login_user_sessions:*' 'login_user_generation:*'; do
  count=$("${redis_cmd[@]}" --scan --pattern "$pattern" | wc -l)
  echo "$pattern residual=$count"
done
```

只有确认旧服务已停止、DB 正确且残余 key 需要清理后，才执行：

```bash
for pattern in 'login_tokens:*' 'login_user_sessions:*' 'login_user_generation:*'; do
  "${redis_cmd[@]}" --scan --pattern "$pattern" |
    xargs -r -n 200 "${redis_cmd[@]}" UNLINK
done
```

清理后重复第一段检查，三个 `residual` 必须全部为 `0`。Redis 低于 4.0、不支持
`UNLINK` 时可在停机窗口内改用分批 `DEL`，仍然禁止一次性把大量 key 作为一个命令提交。
完成后执行 `unset REDISCLI_AUTH`。

### 严禁扩大清理范围

禁止使用 `FLUSHDB`、`FLUSHALL` 或 `KEYS pattern`。不要删除以下非会话数据：

```text
captcha_codes:*   验证码
pwd_err_cnt:*     密码错误锁定
repeat_submit:*   防重复提交
rate_limit:*      接口限流
sys_config:*      参数配置缓存
sys_dict:*        字典缓存
```

`sys_config:__ruoyi_go_revision__` 是 Go 在现有 `sys_config:` 前缀下使用的内部缓存代数，
缓存监控不会展示或单独删除它。升级前没有 TTL 的参数缓存会在首次读取时自动补上 30 分钟
TTL，不需要为本次升级扩大 Redis 清理范围。

Java/Go 双端契约测试不能共用 Redis DB。对拍仍按 [CONVENTIONS.md](./CONVENTIONS.md)
使用独立 DB；生产切换完成后，Java 不得继续向 Go 的 Redis DB 创建会话。

### 不适用场景

- Java 和 Go 需要长期并行写入同一个 Redis DB：不支持，必须先隔离 DB。
- 无停机滚动切换且不能强制用户重新登录：不能清理，需要保留旧会话兼容路径。
- 只是重启当前 Go 版本：不清理，否则会无故踢掉全部在线用户。

## 本次运行时配置升级

部署前必须显式复核以下配置；缺失项会使用默认值，但生产环境不能不看默认值就上线：

```yaml
server:
  requestTimeout: 60s
  trustedProxies: []
mysql:
  connectTimeout: 5s
  readTimeout: 30s
  writeTimeout: 30s
upload:
  maxSizeMB: 10
  maxRequestSizeMB: 20
```

直连服务保持 `trustedProxies: []`。如果 Nginx 与 Go 在同机，可按实际监听方式配置
`["127.0.0.1", "::1"]`；跨主机反代只填固定代理地址或最小 CIDR。配置错误会在启动时失败，
这是预期保护，不要通过删除校验绕过。上传总上限必须覆盖一个合法单文件，并结合反向代理的
`client_max_body_size` 一起设置；代理上限应不小于 Go 上限，否则客户端先收到代理错误页。

上线验证至少包含：伪造 `X-Forwarded-For` 不改变直连客户端 IP；超出 multipart 总上限返回
JSON 结构的 413；MySQL 网络异常在配置时间内结束请求；普通用户不能读取草稿公告；完整编辑
停用用户后旧 Token 立即失效。
