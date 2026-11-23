package service

import (
	"context"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/page"
)

func (s PODRunService) Usage(ctx context.Context, user *model.SysUser, pg page.Query) (*model.PODUsageView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := podProjectUser(ctx, user); err != nil {
		return nil, err
	}
	view, err := repository.SelectPODUsage(ctx, user.UserID, normalizePODPage(pg))
	if err != nil {
		return nil, podRunError(err)
	}
	view.BudgetScope = "instanceCumulative"
	// Publish only the configured ceiling, never global spend or remaining balance.
	if s.Limits.Mode == "qwen" && s.BudgetFen > 0 && s.BudgetFen <= 5000 {
		view.BudgetLimitMicros = s.BudgetFen * 10000
	}
	return view, nil
}
