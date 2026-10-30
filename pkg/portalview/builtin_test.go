package portalview

import (
	"ruoyi-go/portal"
	"strings"
	"testing"
)

func TestBothBuiltinThemesRenderEveryPage(t *testing.T) {
	engine, err := New(portal.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"business", "editorial"} {
		m, ok := engine.Manifest(theme)
		if !ok {
			t.Fatal("missing theme", theme)
		}
		for _, name := range requiredPages {
			data := PageData{Theme: theme, ThemeVersion: m.Version, SiteName: "A&B", Title: "<script>bad</script>", Description: "description", Palette: "ocean", Density: "compact"}
			body, err := engine.Render(theme, name, data)
			if err != nil {
				t.Fatal(theme, name, err)
			}
			if strings.Contains(string(body), "<script>bad") || !strings.Contains(string(body), "A&amp;B") {
				t.Fatal("template escaping failed", theme, name)
			}
		}
	}
}
