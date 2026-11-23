package repository

import (
	"math"
	"ruoyi-go/internal/model"
	"testing"
)

func TestPODPointInvariant(t *testing.T) {
	for _, a := range []model.PODPointsAccount{
		{Revision: 1}, {Available: 10, Held: 5, Spent: 2, GrantedTotal: 20, DeductedTotal: 3, Revision: 1}, {Available: math.MaxInt64, GrantedTotal: math.MaxInt64, Revision: 1},
	} {
		if !validPODPoints(a) {
			t.Fatal("valid points rejected")
		}
	}
	for _, a := range []model.PODPointsAccount{
		{Revision: 0}, {Available: -1, Revision: 1}, {Held: -1, Revision: 1}, {Spent: -1, Revision: 1}, {Available: 1, Revision: 1}, {Available: math.MaxInt64, Held: 1, GrantedTotal: math.MaxInt64, Revision: 1}, {GrantedTotal: 2, DeductedTotal: 3, Revision: 1},
	} {
		if validPODPoints(a) {
			t.Fatal("invalid points accepted")
		}
	}
}
