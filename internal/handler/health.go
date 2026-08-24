package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/service"
)

// Health GET /health
//
// 【这个接口故意不遵守"HTTP 状态码一律 200"的约定】
// 那条规则是为 RuoYi 前端定的 —— 它只看 body 里的 code。
// 但健康检查的消费者是负载均衡器、K8s 探针、监控告警，
// 它们只认 HTTP 状态码，根本不解析 body。恒返 200 等于探活永远通过。
//
// 依赖不可用时返回 503，让上游把这个实例摘掉。
func Health(c *gin.Context) {
	result := service.Health(c.Request.Context())

	status := http.StatusOK
	if result.Status != "up" {
		status = http.StatusServiceUnavailable
	}

	// 不走 response 包：那一层的语义是"给前端的 AjaxResult"，
	// 这里要的是探针能读的裸结构 + 真实状态码
	c.JSON(status, result)
}
