package jwtx

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignerAcceptsOnlyHS512(t *testing.T) {
	const secret = "test-secret"
	signer := NewSigner(secret)

	valid, err := signer.Sign("session-id", "tester")
	if err != nil {
		t.Fatalf("sign HS512: %v", err)
	}
	if _, err := signer.Parse(valid); err != nil {
		t.Fatalf("parse HS512: %v", err)
	}

	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodHS384} {
		t.Run(method.Alg(), func(t *testing.T) {
			token := jwt.NewWithClaims(method, jwt.MapClaims{
				ClaimLoginUserKey: "session-id",
				ClaimUserName:     "tester",
			})
			signed, err := token.SignedString([]byte(secret))
			if err != nil {
				t.Fatalf("sign %s: %v", method.Alg(), err)
			}
			if _, err := signer.Parse(signed); err == nil {
				t.Fatalf("%s token signed with the same secret must be rejected", method.Alg())
			}
		})
	}
}
