package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestDummyPasswordHashMatchesProductionCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyPasswordHash))
	if err != nil {
		t.Fatalf("dummy bcrypt hash must be valid: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("dummy bcrypt cost=%d, want production default=%d", cost, bcrypt.DefaultCost)
	}
}
