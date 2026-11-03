package portalview

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"ruoyi-go/portal"
	"testing"
)

func TestBuiltinCatalogMatchesEmbeddedThemes(t *testing.T) {
	engine, e := New(portal.Resources)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range BuiltinThemes() {
		m, ok := engine.Manifest(o.ID)
		if !ok || m.Version != o.Version || m.Name != o.Name {
			t.Fatal("catalog/manifest mismatch", o.ID)
		}
		for _, name := range requiredPages {
			if _, e := engine.Render(o.ID, name, PageData{Title: "<unsafe>", SiteName: "Test", Theme: o.ID, ThemeVersion: o.Version, IsListing: true, ListingLayout: "list"}); e != nil {
				t.Fatal(o.ID, name, e)
			}
		}
	}
}
func TestImmutableExistingStyleBytes(t *testing.T) {
	expected := map[string]string{"themes/business/static/site.css": "81b1ea390238ec747052d5961b7092ec49557cd5db865bac6adc50ae0ece8c42", "themes/business/static-v2/site.css": "fbd5b4ce90456c5e5ae6c8987bb9e28c4273bd4b04c86b6874e79adb2e5ed2e3", "themes/editorial/static/site.css": "cd243c94d4b78519342912704f795db19896f3248a8c618560eb2621686c7846"}
	for p, want := range expected {
		b, e := fs.ReadFile(portal.Resources, p)
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatal("immutable CSS changed", p)
		}
	}
}
