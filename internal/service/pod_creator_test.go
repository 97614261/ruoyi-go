package service

import (
	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestPODCreatorValidation(t *testing.T) {
	limits := model.PODExecutionLimits{UserConcurrency: 2, MaxUserPending: 10}
	valid := func() model.PODCreatorWrite {
		return model.PODCreatorWrite{RequestID: uuid.NewString(), Reason: "local creator fixture", UserName: "creator_test", NickName: "测试", InitialPassword: "a-valid-password", Policy: &model.PODCreatorPolicyInput{Status: "active", MaxConcurrentNodes: 1, MaxPendingRuns: 10, MaxDailySubmissions: 1000, MaxStorageBytes: "1073741824"}}
	}
	for _, tc := range []struct {
		name   string
		change func(*model.PODCreatorWrite)
	}{
		{"login username max", func(r *model.PODCreatorWrite) { r.UserName = strings.Repeat("a", 21) }},
		{"login password max", func(r *model.PODCreatorWrite) { r.InitialPassword = strings.Repeat("a", 21) }},
		{"short password", func(r *model.PODCreatorWrite) { r.InitialPassword = "short" }},
		{"secret in reason", func(r *model.PODCreatorWrite) { r.Reason = r.InitialPassword }},
		{"role style field", func(r *model.PODCreatorWrite) { r.Amount = "100" }},
		{"zero concurrent", func(r *model.PODCreatorWrite) { r.Policy.MaxConcurrentNodes = 0 }},
		{"instance concurrent", func(r *model.PODCreatorWrite) { r.Policy.MaxConcurrentNodes = 3 }},
		{"instance pending", func(r *model.PODCreatorWrite) { r.Policy.MaxPendingRuns = 11 }},
		{"zero daily", func(r *model.PODCreatorWrite) { r.Policy.MaxDailySubmissions = 0 }},
		{"zero storage", func(r *model.PODCreatorWrite) { r.Policy.MaxStorageBytes = "0" }},
		{"storage overflow", func(r *model.PODCreatorWrite) { r.Policy.MaxStorageBytes = "10737418241" }},
		{"date impossible", func(r *model.PODCreatorWrite) { s := "2026-02-30 10:00:00"; r.Policy.ValidUntil = &s }},
		{"ambiguous date", func(r *model.PODCreatorWrite) { s := "2026-1-1 10:00:00"; r.Policy.ValidUntil = &s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := valid()
			tc.change(&r)
			if _, err := validateCreatorWrite("creator.create", 0, r, limits); err == nil {
				t.Fatal("invalid creator request accepted")
			}
		})
	}
	r := valid()
	if _, err := validateCreatorWrite("creator.create", 0, r, limits); err != nil {
		t.Fatal(err)
	}
	r.UserName = strings.Repeat("a", 20)
	r.InitialPassword = strings.Repeat("字", 20)
	if _, err := validateCreatorWrite("creator.create", 0, r, limits); err != nil {
		t.Fatal("login-compatible unicode password rejected", err)
	}
	empty := []string{}
	r = model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: "1", Reason: "revoke model scope", ModelIDs: &empty}
	if _, err := validateCreatorWrite("creator.models", 1, r, limits); err != nil {
		t.Fatal(err)
	}
	r.ModelIDs = nil
	if _, err := validateCreatorWrite("creator.models", 1, r, limits); err == nil {
		t.Fatal("missing model intent accepted")
	}
	for _, s := range []string{"0", "-1", "1.1", "01", "1000000001", "9223372036854775808"} {
		r = model.PODCreatorWrite{RequestID: uuid.NewString(), AccountRevision: "1", Reason: "local points fixture", Kind: "grant", Amount: s}
		if _, err := validateCreatorWrite("points.adjust", 1, r, limits); err == nil {
			t.Fatal("invalid points accepted", s)
		}
	}
}
