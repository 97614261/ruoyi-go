package model

import (
	"encoding/json"
	"github.com/gin-gonic/gin/binding"
	"testing"
)

func TestERPWarehouseGrantRequiresExplicitSet(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"version":1}`, false}, {`{"version":1,"warehouseIds":null}`, false},
		{`{"version":1,"warehouseIds":[]}`, true}, {`{"version":1,"warehouseIds":[7]}`, true},
		{`{"version":1,"warehouseIds":[0]}`, false}, {`{"warehouseIds":[]}`, false},
	} {
		var input ERPAccessRequest
		if err := json.Unmarshal([]byte(tc.body), &input); err != nil {
			t.Fatal(err)
		}
		err := binding.Validator.ValidateStruct(input)
		if (err == nil) != tc.ok {
			t.Fatalf("body=%s valid=%v expected=%v", tc.body, err == nil, tc.ok)
		}
	}
}
