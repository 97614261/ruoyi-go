package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/service"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/response"
)

func (h PODHandler) Bootstrap(c *gin.Context) {
	bootstrap, err := service.PODBootstrap(c.Request.Context(), currentUser(c), h.Limits)
	if err != nil {
		fail(c, err)
		return
	}
	bootstrap.AssetLimits = service.DefaultPODAssetLimits(h.AssetLimits)
	bootstrap.Execution = h.Execution
	bootstrap.Capabilities.Execution = h.Execution.Mode == "simulation" || h.Execution.Mode == "qwen" || h.Execution.Mode == "managed"
	if bootstrap.Capabilities.Execution {
		for i := range bootstrap.NodeTypes {
			bootstrap.NodeTypes[i].Executable = workflow.Executable(bootstrap.NodeTypes[i].Type, h.Execution.Mode)
		}
	}
	response.OkData(c, bootstrap)

}
