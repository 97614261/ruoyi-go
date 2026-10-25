package service

import (
	"context"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
)

func TestTemporaryDownloadDeletionBoundary(t *testing.T) {
	ctx := tenantctx.With(context.Background(), 10000, false)
	file := model.SysFile{TenantRecord: model.TenantRecord{TenantID: 10000}, FileID: 7, UploaderUserID: 9, StorageKey: "/profile/tenants/10000/download/id_report.txt"}
	cases := []struct {
		name    string
		ctx     context.Context
		key     string
		owner   int64
		allowed bool
	}{
		{"own_temporary_file", ctx, file.StorageKey, 9, true},
		{"another_user", ctx, file.StorageKey, 10, false},
		{"no_user", ctx, file.StorageKey, 0, false},
		{"no_tenant", context.Background(), file.StorageKey, 9, false},
		{"another_tenant", tenantctx.With(context.Background(), 10001, false), file.StorageKey, 9, false},
		{"platform", tenantctx.With(context.Background(), 10000, true), file.StorageKey, 9, false},
		{"system", tenantctx.System(ctx), file.StorageKey, 9, false},
		{"catalog_image", ctx, "/profile/tenants/10000/catalog/image.png", 9, false},
		{"regular_upload", ctx, "/profile/tenants/10000/upload/report.txt", 9, false},
		{"avatar", ctx, "/profile/tenants/10000/avatar/photo.png", 9, false},
		{"tenant_path_mismatch", ctx, "/profile/tenants/10001/download/report.txt", 9, false},
		{"directory_prefix_collision", ctx, "/profile/tenants/10000/download-other/report.txt", 9, false},
		{"parent_path", ctx, "/profile/tenants/10000/download/../catalog/image.png", 9, false},
		{"noncanonical", ctx, "/profile/tenants/10000/download//report.txt", 9, false},
		{"backslash", ctx, "/profile/tenants/10000/download/..\\catalog\\image.png", 9, false},
		{"query", ctx, "/profile/tenants/10000/download/report.txt?x=1", 9, false},
		{"fragment", ctx, "/profile/tenants/10000/download/report.txt#x", 9, false},
		{"unmanaged_prefix", ctx, "/other/tenants/10000/download/report.txt", 9, false},
		{"unapproved_extension", ctx, "/profile/tenants/10000/download/run.exe", 9, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := file
			f.StorageKey = tc.key
			if got := ValidateTemporaryDownload(tc.ctx, &f, tc.owner, "/profile") == nil; got != tc.allowed {
				t.Fatalf("deletion allowed=%v, want %v", got, tc.allowed)
			}
			// 被拒绝的请求也必须在访问数据库前被再次拒绝。
			if !tc.allowed {
				if removed, err := ForgetDownloadedFile(tc.ctx, &f, tc.owner, "/profile"); err == nil || removed {
					t.Fatal("invalid deletion reached persistence")
				}
			}
		})
	}
	if ValidateTemporaryDownload(ctx, nil, 9, "/profile") == nil {
		t.Fatal("nil registration accepted")
	}
	file.FileID = 0
	if ValidateTemporaryDownload(ctx, &file, 9, "/profile") == nil {
		t.Fatal("unpersisted registration accepted")
	}
}
