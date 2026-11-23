# 千问真实小额试跑记录

日期：2026-11-12，时区 Asia/Shanghai。本报告仅记录本次本机真实样本，不代表全部模型功能验收。

> 后续节点内润色、两张生成、单参考图与五种比例的真实复核，以及故障回归见 [主流程边界报告](./POD_BOUNDARY_ACCEPTANCE_20261112.md)。本文保留首次小样本及当时费用快照。

本地 API、创作端和 Worker 已重启为千问模式。创作端：`http://127.0.0.1:5177/`。
测试使用既有 `pod_creator` 个人空间，未改账号密码、成员权限或全局验证码开关。

## 使用的模型与预算

| 用途 | 模型 | 当前估算单价 |
|---|---|---|
| 文字 | `qwen-flash` | 输入 0.15 元/百万 Token，输出 1.5 元/百万 Token |
| 图片 | `qwen-image-2.1-turbo` | 0.10 元/张 |

本机忽略提交的 `configs/pod.local.yml` 已设置 `mode: qwen`、`allowPaidCalls: true`、
`budgetFen: 100`，全局、用户、供应商并发均为 1。密钥留在本地配置。
预算是本数据库所有用户累计的估算上限，包含未知调用预占，不会每天自动重置，也不能替代供应商侧费用控制。

切换前确认没有未完成运行、供应商调用或费用预占。没有迁移或重置数据库，仍为 `pod_studio_dev`、schema 001—005，Redis 6387/DB 15。

## 实测结果

| 检查 | 结果 |
|---|---|
| API 健康、MySQL、Redis | 正常，`/health` 返回 200 |
| 创作端入口 | HTTP 200 |
| Worker 与 API 模式 | `qwen`，bootstrap 返回上述两个实际模型 |
| 短文字生成 | 成功，项目 421 / 运行 307，约 2.1 秒观察到完成 |
| 文生图 | 成功，项目 422 / 运行 308，生成一张 1:1 图片 |
| 图片归档 | 素材 650，PNG，1024×1024，750747 字节；已生成私有原图与缩略图 |
| 鉴权下载 | 原图内容接口与下载接口字节一致，图片可解码；已查看图片为白底蓝花，无文字 |
| 账号隔离 | `pod_creator_b` 读取上述运行与生成素材均被拒绝 |
| 模型调用次数 | 总共 2 次；归档恢复没有新增供应商调用 |

文字输入 49 Token，输出 20 Token，按整数微元向上取整估算为 38 微元，即 0.000038 元。
一张图片估算 100000 微元，即 0.10 元。合计 **0.100038 元**，费用预占已结清，无未完成运行。
未访问百炼控制台账单；试用额度、优惠及最终实际扣费以控制台为准。

文字结果：

> Simple blue wildflower pattern on white, flat shapes, clean lines, no text, minimalist design.

## 这次发现并修复的问题

生图接口约 5 秒内返回成功和一张图片，但实际图片地址使用
`dashscope-a717.oss-accelerate.aliyuncs.com`，旧下载白名单只允许 `dashscope-result...` 域名，
因此运行进入 `awaitingReconciliation / archivePending`。

在 `pkg/executor/qwen_download.go` 补充该精确域名，并为成功下载、相似恶意域名、任意 OSS 桶、
非 HTTPS、用户凭据和自定义端口补回归测试。公网 IP 校验、固定拨号、禁止重定向与大小限制继续执行。

重启应用修复后，Worker 读取已经持久化的原调用结果，下载并归档原图；没有重复调用模型。
原图运行于 11:36:58 完成。归档排查占用了中间时间，不能把该运行总耗时当成模型生图性能。

相关自动化：`go test ./pkg/executor ./internal/service ./internal/config` 通过；本地启动脚本重新构建 API/Worker 成功。
本次有限样本经过真实 HTTP API、运行快照、数据库/outbox、Redis 队列、Worker、供应商接口和私有素材链路。
本地机器证据在忽略提交的 `.local/qwen-smoke/result.json` 与 `.local/qwen-smoke/generated.png`，没有保存密钥或签名 URL。

## 继续试用

登录创作端后可打开名称为 `Qwen low-cost smoke text ...` 和 `Qwen low-cost smoke image ...` 的项目查看结果，
也可以新建“文本输入 → AI 生图”，数量先设 1；等待保存后在执行面板勾选付费确认。
新建真实运行会继续累计本机估算费用，达到 1 元上限后拒绝新的预占。

参考图生图、多图、其他比例、真实浏览器内完整运行操作、异常收费明细与备份恢复未在本轮验收。
后续按需求选取有限样本，不用真实付费模型做无限重试或压测。
