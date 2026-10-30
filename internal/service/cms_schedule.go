package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsScheduleTime(raw string, now time.Time) (time.Time, error) {
	due, err := time.ParseInLocation(types.Layout, raw, types.Location)
	if err != nil || !due.After(now.Add(5*time.Second)) || due.After(now.Add(366*24*time.Hour)) {
		return time.Time{}, errs.New("执行时间须为北京时间，至少5秒后且不超过366天")
	}
	return due, nil
}
func CMSCreateSchedule(ctx context.Context, kind string, in model.CMSScheduleInput, actor int64) (int64, error) {
	due, err := cmsScheduleTime(in.DueAt, time.Now())
	if err != nil {
		return 0, err
	}
	if in.Action == "publish" {
		check, err := CMSCheckPublication(ctx, kind, in.ContentId)
		if err != nil {
			return 0, err
		}
		if check.Version != in.ContentVersion {
			return 0, errs.New("内容已变化，请刷新")
		}
		if !check.Ready {
			return 0, errs.New("发布前检查未通过：" + check.Issues[0].Message)
		}
	}
	row := &model.CMSSchedule{Kind: kind, ContentId: in.ContentId, ContentVersion: in.ContentVersion, Action: in.Action, OwnerUserId: actor, DueAt: types.Time(due)}
	err = repository.CMSCreateSchedule(ctx, row)
	return row.ScheduleId, err
}
func CMSSchedulePage(ctx context.Context, kind string, q model.CMSScheduleQuery, pg page.Query) ([]model.CMSSchedule, int64, error) {
	return repository.CMSSchedulePage(ctx, kind, q, pg)
}
func CMSCancelSchedule(ctx context.Context, kind string, id, version, actor int64) error {
	return repository.CMSCancelSchedule(ctx, kind, id, version, actor)
}
func CMSRunSchedules(ctx context.Context) error {
	now := time.Now().In(types.Location)
	rows, err := repository.CMSDueSchedules(ctx, now)
	if err != nil {
		return err
	}
	var failures []error
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var preflight error
		if row.Action == "publish" {
			check, e := CMSCheckPublication(ctx, row.Kind, row.ContentId)
			if e != nil {
				preflight = e
			} else if check.Version != row.ContentVersion {
				preflight = errs.New("内容已修改，原定时任务不再执行")
			} else if !check.Ready {
				preflight = errs.New("发布前检查未通过：" + check.Issues[0].Message)
			}
		}
		err = repository.CMSExecuteSchedule(ctx, row, now, preflight, func(r model.CMSRevision) (string, []int64, error) {
			body, _, err := CMSBody(r.BodyHTML)
			ids, assetErr := cmsRevisionAssets(r)
			if err != nil {
				return "", nil, err
			}
			return body, ids, assetErr
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("执行定时任务%d: %w", row.ScheduleId, err))
		}
	}
	return errors.Join(failures...)
}

type CMSScheduleWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startCMSScheduleWorker(parent context.Context, run func(context.Context) error) *CMSScheduleWorker {
	ctx, cancel := context.WithCancel(parent)
	w := &CMSScheduleWorker{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			cycle, cancelCycle := context.WithTimeout(ctx, 15*time.Second)
			err := run(cycle)
			cancelCycle()
			if err != nil && ctx.Err() == nil {
				slog.Error("门户定时任务扫描失败，保留待执行状态重试", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return w
}
func StartCMSScheduleWorker(ctx context.Context) *CMSScheduleWorker {
	return startCMSScheduleWorker(ctx, CMSRunSchedules)
}
func (w *CMSScheduleWorker) Stop(ctx context.Context) error {
	w.cancel()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
