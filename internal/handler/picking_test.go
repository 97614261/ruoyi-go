package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ruoyi-go/pkg/errs"
	"strings"
	"testing"
)

func TestPickingDistinguishesBusinessRefusalFromUnknownDatabaseOutcome(t *testing.T) {
	for _, tc := range []struct {
		e    error
		code string
	}{{errs.New("数量不符"), `"code":400`}, {errors.New("PRIVATE_DB_FAILURE"), `"code":500`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/erp/picking/1/check", nil)
		pickingFail(c, tc.e)
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "PRIVATE_DB_FAILURE") {
			t.Fatal("error semantics or privacy changed")
		}
	}
}
