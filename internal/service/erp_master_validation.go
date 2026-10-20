package service

import (
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"strings"
	"unicode/utf8"
)

var erpCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func validateERPSku(s *model.ErpSku) error {
	s.SkuCode = strings.TrimSpace(s.SkuCode)
	s.ProductName = strings.TrimSpace(s.ProductName)
	s.Unit = strings.TrimSpace(s.Unit)
	if !erpCode.MatchString(s.SkuCode) || s.ProductName == "" || utf8.RuneCountInString(s.ProductName) > 120 || utf8.RuneCountInString(s.Specification) > 120 || s.Unit == "" || utf8.RuneCountInString(s.Unit) > 20 || utf8.RuneCountInString(s.Remark) > 500 || (s.Status != "0" && s.Status != "1") {
		return errs.New("SKU编码、名称、单位、状态或字段长度不合法")
	}
	return nil
}
func validateERPWarehouse(w *model.ErpWarehouse) error {
	w.WarehouseCode = strings.TrimSpace(w.WarehouseCode)
	w.WarehouseName = strings.TrimSpace(w.WarehouseName)
	if !erpCode.MatchString(w.WarehouseCode) || w.WarehouseName == "" || utf8.RuneCountInString(w.WarehouseName) > 120 || utf8.RuneCountInString(w.Remark) > 500 || (w.Status != "0" && w.Status != "1") {
		return errs.New("仓库编码、名称、状态或字段长度不合法")
	}
	return nil
}
