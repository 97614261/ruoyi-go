package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestSupplierValidationAndOptionalContacts(t *testing.T) {
	good := model.ERPSupplierWrite{SupplierCode: "00001", SupplierName: " 供应商 ", Status: "0"}
	out, err := prepareSupplier(good)
	if err != nil || out.SupplierCode != "00001" || out.SupplierName != "供应商" || out.Email != "" {
		t.Fatal("optional contacts/normalization", err)
	}
	for _, change := range []func(*model.ERPSupplierWrite){
		func(v *model.ERPSupplierWrite) { v.SupplierCode = "a/b" }, func(v *model.ERPSupplierWrite) { v.SupplierName = " " }, func(v *model.ERPSupplierWrite) { v.SupplierName = strings.Repeat("中", 121) }, func(v *model.ERPSupplierWrite) { v.ContactName = strings.Repeat("a", 65) }, func(v *model.ERPSupplierWrite) { v.Phone = "123\n456" }, func(v *model.ERPSupplierWrite) { v.Email = "Contact <x@example.com>" }, func(v *model.ERPSupplierWrite) { v.Email = "bad" }, func(v *model.ERPSupplierWrite) { v.Status = "2" }, func(v *model.ERPSupplierWrite) { v.Address = strings.Repeat("中", 501) }, func(v *model.ERPSupplierWrite) { v.Remark = "\x00" },
	} {
		v := good
		change(&v)
		if _, err := prepareSupplier(v); err == nil {
			t.Fatal("invalid supplier accepted")
		}
	}
	good.Email = "a@example.com"
	good.Address = "line1\nline2"
	good.Remark = ""
	if _, err := prepareSupplier(good); err != nil {
		t.Fatal(err)
	}
}
