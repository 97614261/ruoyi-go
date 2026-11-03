package portalview

import (
	"ruoyi-go/portal"
	"strings"
	"testing"
)

func TestSearchHighlightTextStaysInertInEveryTheme(t *testing.T) {
	e, err := New(portal.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"business", "editorial", "industrial"} {
		d := PageData{SiteName: "test", Title: "search", Cards: []Card{{Title: "fallback", URL: "/p/test", TitleParts: []TextPart{{Text: "<script>alert(1)</script>", Match: true}}, SummaryParts: []TextPart{{Text: "<img src=x onerror=alert(1)>", Match: true}}}}}
		b, err := e.Render(id, "search", d)
		if err != nil {
			t.Fatal(id, err)
		}
		s := string(b)
		if !strings.Contains(s, "<mark>&lt;script&gt;") || !strings.Contains(s, "<mark>&lt;img") || strings.Contains(s, "<script>alert(") || strings.Contains(s, "<img src=x") {
			t.Fatal(id, "highlight must remain escaped text")
		}
	}
}
