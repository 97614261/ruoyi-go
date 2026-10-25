package repository

import (
	"encoding/json"
	"gorm.io/gorm/schema"
	"ruoyi-go/internal/model"
	"strings"
	"sync"
	"testing"
)

func TestSalesReturnReadProjectionHasNoGormRelation(t *testing.T) {
	s, e := schema.Parse(&model.ERPSalesReturnable{}, &sync.Map{}, schema.NamingStrategy{})
	if e != nil {
		t.Fatal(e)
	}
	if s.LookUpField("Lines").DBName != "" {
		t.Fatal("computed detail must not be mapped by list Scan")
	}
	raw, _ := json.Marshal(model.ERPSalesReturnable{LegacyStockReturn: true})
	if !strings.Contains(string(raw), `"netShippedQuantity":null`) {
		t.Fatal("unknown legacy net must be null")
	}
}
func TestSalesReturnProofRejectsBeforeDBAccess(t *testing.T) {
	src := model.ERPDocument{DocumentID: 3}
	id := int64(3)
	var missing *salesReturnSource
	for _, p := range []*salesReturnSource{missing, {}, {returnID: 1, packageID: 2}} {
		if p.verify(nil, &model.ERPDocument{Kind: "PR", Status: "DRAFT", SourceDocumentID: &id}, &src) == nil {
			t.Fatal("wrong kind accepted")
		}
	}
	for _, doc := range []model.ERPDocument{{Kind: "SR", Status: "POSTED", SourceDocumentID: &id}, {Kind: "SR", Status: "DRAFT", Reversal: true, SourceDocumentID: &id}, {Kind: "SR", Status: "DRAFT"}} {
		if (&salesReturnSource{returnID: 1, packageID: 2}).verify(nil, &doc, &src) == nil {
			t.Fatal("wrong capability state accepted")
		}
	}
}
