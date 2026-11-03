package service

import (
	"bytes"
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsNormalizeInquiryFilter(in model.CMSInquiryFilter) (model.CMSInquiryFilter, error) {
	var start, end time.Time
	var e error
	if in.From != "" {
		start, e = time.ParseInLocation("2006-01-02", in.From, types.Location)
		if e != nil {
			return in, errs.New("开始日期须为YYYY-MM-DD")
		}
		in.From = start.Format(types.Layout)
	}
	if in.To != "" {
		end, e = time.ParseInLocation("2006-01-02", in.To, types.Location)
		if e != nil {
			return in, errs.New("结束日期须为YYYY-MM-DD")
		}
		in.To = end.AddDate(0, 0, 1).Format(types.Layout)
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return in, errs.New("日期范围不能倒置")
	}
	return in, nil
}
func CMSInquiryFilteredTasks(ctx context.Context, in model.CMSInquiryFilter, pg page.Query) ([]model.CMSInquiryTask, int64, error) {
	in, e := cmsNormalizeInquiryFilter(in)
	if e != nil {
		return nil, 0, e
	}
	return repository.CMSInquiryFilteredTasks(ctx, in, pg)
}
func CMSGetDataPolicy(ctx context.Context) (model.CMSDataPolicy, error) {
	return repository.CMSGetDataPolicy(ctx)
}
func CMSSetDataPolicy(ctx context.Context, in model.CMSDataPolicyInput) error {
	if !cmsWorkflowText(in.Reason) {
		return errs.New("原因含无效字符")
	}
	seen := map[string]bool{}
	for _, s := range in.Statuses {
		if seen[s] {
			return errs.New("状态不能重复")
		}
		seen[s] = true
	}
	return repository.CMSSetDataPolicy(ctx, in)
}
func CMSPreviewCleanup(ctx context.Context, in model.CMSCleanupInput) (model.CMSCleanupBatch, error) {
	if !cmsWorkflowText(in.Reason) {
		return model.CMSCleanupBatch{}, errs.New("原因含无效字符")
	}
	return repository.CMSPreviewCleanup(ctx, in)
}
func CMSExecuteCleanup(ctx context.Context, id, version int64) error {
	return repository.CMSExecuteCleanup(ctx, id, version)
}
func CMSCleanupBatches(ctx context.Context, pg page.Query) ([]model.CMSCleanupBatch, int64, error) {
	return repository.CMSCleanupBatches(ctx, pg)
}
func CMSCleanupItems(ctx context.Context, id int64, pg page.Query) ([]model.CMSCleanupItemView, int64, error) {
	return repository.CMSCleanupItems(ctx, id, pg)
}
func CMSDataEvents(ctx context.Context, own bool, pg page.Query) ([]model.CMSDataEvent, int64, error) {
	return repository.CMSDataEvents(ctx, own, pg)
}

type cmsExportWriter struct {
	ctx context.Context
	buf *bytes.Buffer
}

func (w cmsExportWriter) Write(p []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	return w.buf.Write(p)
}
func CMSExportInquiries(ctx context.Context, in model.CMSInquiryExport) ([]byte, error) {
	f, e := cmsNormalizeInquiryFilter(in.CMSInquiryFilter)
	if e != nil {
		return nil, e
	}
	in.CMSInquiryFilter = f
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	return repository.CMSExportInquiries(ctx, in, func(rows []model.CMSInquiryExportRow) ([]byte, error) {
		var b bytes.Buffer
		e := excelx.Export(cmsExportWriter{ctx, &b}, "询盘数据", rows)
		return b.Bytes(), e
	})
}
