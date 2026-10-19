package service

import (
	"testing"
)

func TestTenantQuotaRequiresExplicitBoundedValues(t *testing.T) {
	zero, users, storage, upload := int64(0), int64(100), int64(1<<30), int64(10<<20)
	valid := TenantQuotaInput{Version: 1, ConfirmTenantCode: "demo", MaxUsers: &users, MaxStorageBytes: &storage, MaxUploadBytes: &upload, Reason: "test"}
	if err := validateTenantQuota(valid); err != nil {
		t.Fatal(err)
	}
	unlimited := valid
	unlimited.MaxUsers = &zero
	unlimited.MaxStorageBytes = &zero
	unlimited.MaxUploadBytes = &zero
	if err := validateTenantQuota(unlimited); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*TenantQuotaInput){func(b *TenantQuotaInput) { b.MaxUsers = nil }, func(b *TenantQuotaInput) { x := int64(-1); b.MaxStorageBytes = &x }, func(b *TenantQuotaInput) { x := int64(1 << 41); b.MaxStorageBytes = &x }, func(b *TenantQuotaInput) { b.Version = 0 }, func(b *TenantQuotaInput) { b.Reason = " " }} {
		copy := valid
		change(&copy)
		if validateTenantQuota(copy) == nil {
			t.Fatal("invalid quota accepted")
		}
	}
}
