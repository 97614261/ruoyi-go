package modelcatalog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func testRing(t *testing.T, edit func(*keyring)) string {
	t.Helper()
	r := keyring{ActiveKeyID: "v1", Keys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}, FingerprintKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))}
	if edit != nil {
		edit(&r)
	}
	b, _ := json.Marshal(r)
	p := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVaultAuthenticationAndRotation(t *testing.T) {
	p := testRing(t, nil)
	v, err := NewVault(p, "independent-jwt", nil)
	if err != nil {
		t.Fatal(err)
	}
	id, nonce, ciphertext, err := v.Seal(13, 2, "fake-unit-key")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("fake-unit-key")) {
		t.Fatal("plaintext stored")
	}
	plain, err := v.Open(13, 2, id, nonce, ciphertext)
	if err != nil || plain != "fake-unit-key" {
		t.Fatal("roundtrip failed")
	}
	for _, pair := range [][2]int64{{14, 2}, {13, 3}} {
		if _, err := v.Open(pair[0], pair[1], id, nonce, ciphertext); err == nil {
			t.Fatal("AAD not bound")
		}
	}
	changed := append([]byte(nil), ciphertext...)
	changed[0] ^= 1
	for _, c := range [][]byte{changed, nil} {
		if _, err := v.Open(13, 2, id, nonce, c); err == nil {
			t.Fatal("tampering accepted")
		}
	}
	if _, err := v.Open(13, 2, "missing", nonce, ciphertext); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := v.Open(13, 2, id, nil, ciphertext); err == nil {
		t.Fatal("invalid nonce accepted")
	}
	rotated, err := NewVault(testRing(t, func(r *keyring) {
		r.ActiveKeyID = "v2"
		r.Keys["v2"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	}), "jwt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.Open(13, 2, id, nonce, ciphertext); err != nil {
		t.Fatal("rotation lost old key")
	}
	h1, _ := v.Fingerprint("provider.credential", map[string]string{"secret": "fake-unit-key"})
	h2, _ := rotated.Fingerprint("provider.credential", map[string]string{"secret": "fake-unit-key"})
	h3, _ := v.Fingerprint("provider.update", map[string]string{"secret": "fake-unit-key"})
	if len(h1) != 64 || h1 != h2 || h1 == h3 {
		t.Fatal("fingerprint domain/rotation invalid")
	}
}

func TestVaultFailClosed(t *testing.T) {
	v, err := NewVault("", "jwt", nil)
	if err != nil || v.Ready() {
		t.Fatal("missing keyring must be read-only")
	}
	if _, err := v.Fingerprint("op", nil); err == nil {
		t.Fatal("unkeyed fingerprint accepted")
	}
	for _, edit := range []func(*keyring){func(r *keyring) { r.Keys["v1"] = r.FingerprintKey }, func(r *keyring) { r.ActiveKeyID = "absent" }, func(r *keyring) { r.Keys["v1"] = "short" }} {
		if _, err := NewVault(testRing(t, edit), "jwt", nil); err == nil {
			t.Fatal("invalid keyring accepted")
		}
	}
	jwt := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if _, err := NewVault(testRing(t, nil), jwt, nil); err == nil {
		t.Fatal("JWT key reuse accepted")
	}
	p := testRing(t, nil)
	if err := os.WriteFile(p, []byte(strings.Repeat(" ", 32769)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVault(p, "jwt", nil); err == nil {
		t.Fatal("oversized keyring accepted")
	}
	if _, err := NewVault("", "jwt", []string{"BAD NAME"}); err == nil {
		t.Fatal("invalid env name accepted")
	}
	v, err = NewVault(testRing(t, nil), "jwt", []string{"POD_UNIT_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("POD_UNIT_KEY", "fake-env-key")
	if _, err := v.ResolveEnv("POD_UNIT_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.ResolveEnv("OTHER_KEY"); err == nil {
		t.Fatal("unlisted env accepted")
	}
	for _, secret := range []string{"short", "abcdefgh\n", "abcdefgh 中", strings.Repeat("x", 513)} {
		if ValidSecret(secret) {
			t.Fatal("bad secret accepted")
		}
	}
}

func textSpec() model.PODCatalogWrite {
	return model.PODCatalogWrite{OfficialModelID: "qwen-plus", TimeoutSeconds: 120, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 32000, MaxOutputTokens: 2048}, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", InputPerMillion: 1000000}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", InputPerMillion: 100}}
}

func TestTypedSpecsAndSnapshotIntegrity(t *testing.T) {
	r := textSpec()
	if err := ValidateSpec("text", r); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://dashscope.aliyuncs.com", "https://localhost", "https://dashscope.aliyuncs.com:443", "https://dashscope.aliyuncs.com/v1", "https://dashscope.aliyuncs.com.evil.test"} {
		if ValidateProvider("qwen-v1", "cn-beijing", endpoint) == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, change := range []func(*model.PODCatalogWrite){func(r *model.PODCatalogWrite) { r.OfficialModelID = "invalid model id" }, func(r *model.PODCatalogWrite) { r.CostRule.PerImage = 1 }, func(r *model.PODCatalogWrite) { r.CostRule.Currency = "USD" }, func(r *model.PODCatalogWrite) { r.PointRule.InputPerMillion = -1 }, func(r *model.PODCatalogWrite) { r.Capabilities.MaxImages = 1 }, func(r *model.PODCatalogWrite) { r.TimeoutSeconds = 301 }} {
		r := textSpec()
		change(&r)
		if ValidateSpec("text", r) == nil {
			t.Fatal("invalid typed spec accepted")
		}
	}
	p := model.PODProvider{ProviderID: 3, AdapterKind: "qwen-v1", Region: "cn-beijing", Endpoint: "https://dashscope.aliyuncs.com"}
	m := model.PODCatalogModel{ModelID: 7, Kind: "text"}
	v := model.PODCatalogModelVersion{ModelID: 7, OfficialModelID: r.OfficialModelID, AdapterKind: p.AdapterKind, AdapterVersion: "1", Region: p.Region, Endpoint: p.Endpoint, TimeoutSeconds: r.TimeoutSeconds}
	v.CapabilitiesJSON, _ = json.Marshal(r.Capabilities)
	v.CostRuleJSON, _ = json.Marshal(r.CostRule)
	v.PointRuleJSON, _ = json.Marshal(r.PointRule)
	v.SpecHash = SpecHash(p.ProviderID, v, r)
	if err := ValidateVersion(p, m, v); err != nil {
		t.Fatal(err)
	}
	v.AdapterVersion = "2"
	if ValidateVersion(p, m, v) == nil {
		t.Fatal("adapter version drift accepted")
	}
	v.AdapterVersion = "1"
	v.TimeoutSeconds++
	if ValidateVersion(p, m, v) == nil {
		t.Fatal("snapshot tampering accepted")
	}
	image := textSpec()
	image.OfficialModelID = "qwen-image-2.1-turbo"
	image.Capabilities = &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 4, MaxReferenceImages: 3, Ratios: []string{"1:1", "3:4"}}
	image.CostRule.InputPerMillion = 0
	image.CostRule.PerImage = 100
	image.PointRule.InputPerMillion = 0
	image.PointRule.PerImage = 1
	if err := ValidateSpec("image", image); err != nil {
		t.Fatal(err)
	}
	image.Capabilities.Ratios = []string{"1:1", "1:1"}
	if ValidateSpec("image", image) == nil {
		t.Fatal("duplicate ratio accepted")
	}
}
