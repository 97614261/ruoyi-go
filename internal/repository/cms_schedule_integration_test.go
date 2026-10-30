package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"os"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/types"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCMSScheduleAtomicCommit(t *testing.T) {
	if os.Getenv("PORTAL_INTEGRATION") != "1" {
		t.Skip("explicit local integration required")
	}
	cfg, e := config.Load("../../configs/portal.local.yml")
	if e != nil {
		t.Fatal("private config unavailable")
	}
	if !strings.Contains(cfg.MySQL.DSN, "/ruoyi_go_portal?") || cfg.Redis.DB != 14 || cfg.Server.Port != 8085 {
		t.Fatal("isolated portal only")
	}
	old := db
	if e = Init(cfg.MySQL); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { Close(); db = old })
	ctx := context.Background()
	if e = ValidatePortalSchema(ctx); e != nil {
		t.Fatal(e)
	}
	slug := "atomic-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	id, e := CMSSaveDraft(ctx, "PAGE", model.CMSDraftInput{Slug: slug}, model.CMSRevision{Title: slug, BodyHTML: "<p>atomic</p>"}, nil, "admin")
	if e != nil {
		t.Fatal(e)
	}
	detail, e := CMSContentDetail(ctx, "PAGE", id)
	if e != nil {
		t.Fatal(e)
	}
	row := model.CMSSchedule{ContentId: id, Kind: "PAGE", Action: "publish", ContentVersion: detail.Version, OwnerUserId: 1, DueAt: types.Time(time.Now().Add(time.Hour))}
	if e = CMSCreateSchedule(ctx, &row); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Callback().Update().Remove("portal:test-schedule-fail")
		var job model.CMSSchedule
		if e := cmsDB(ctx).Where("schedule_id=?", row.ScheduleId).Take(&job).Error; e != nil {
			t.Error(e)
			return
		}
		if job.Status == "PENDING" {
			if e := CMSCancelSchedule(ctx, "PAGE", job.ScheduleId, job.Version, 1); e != nil {
				t.Error(e)
			}
		}
		d, e := CMSContentDetail(ctx, "PAGE", id)
		if e != nil {
			t.Error(e)
			return
		}
		if e = CMSChangeContent(ctx, "PAGE", id, model.CMSContentAction{Version: d.Version}, "remove", "admin", true, nil); e != nil {
			t.Error(e)
		}
	})
	injected := errors.New("test schedule status write failure")
	if e = db.Callback().Update().Before("gorm:update").Register("portal:test-schedule-fail", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "cms_schedule" {
			tx.AddError(injected)
		}
	}); e != nil {
		t.Fatal(e)
	}
	normalize := func(r model.CMSRevision) (string, []int64, error) { return r.BodyHTML, nil, nil }
	now := time.Time(row.DueAt).Add(time.Second)
	if e = CMSExecuteSchedule(ctx, row, now, nil, normalize); !errors.Is(e, injected) {
		t.Fatal("failure not injected", e)
	}
	detail, e = CMSContentDetail(ctx, "PAGE", id)
	if e != nil {
		t.Fatal(e)
	}
	var saved model.CMSSchedule
	if e = cmsDB(ctx).Where("schedule_id=?", row.ScheduleId).Take(&saved).Error; e != nil {
		t.Fatal(e)
	}
	if detail.IsPublished != 0 || detail.Version != row.ContentVersion || saved.Status != "PENDING" || saved.Version != 1 {
		t.Fatal("content and task did not roll back together")
	}
	if e = db.Callback().Update().Remove("portal:test-schedule-fail"); e != nil {
		t.Fatal(e)
	}
	if e = CMSExecuteSchedule(ctx, row, now, nil, normalize); e != nil {
		t.Fatal(e)
	}
	if e = CMSExecuteSchedule(ctx, row, now, nil, normalize); e != nil {
		t.Fatal(e)
	}
	detail, e = CMSContentDetail(ctx, "PAGE", id)
	if e != nil {
		t.Fatal(e)
	}
	if e = cmsDB(ctx).Where("schedule_id=?", row.ScheduleId).Take(&saved).Error; e != nil {
		t.Fatal(e)
	}
	if detail.IsPublished != 1 || detail.Version != row.ContentVersion+1 || saved.Status != "SUCCESS" || saved.Version != 2 {
		t.Fatal("retry did not commit exactly once")
	}
	t.Log("PASS injected task status write failure rolls back publication; retry commits both once")
}
