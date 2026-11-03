package portalview

import (
	"ruoyi-go/portal"
	"strings"
	"testing"
)

func TestDownloadPagesEscapeMetadataInEveryTheme(t *testing.T) {
	e, err := New(portal.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"business", "editorial", "industrial"} {
		d := PageData{SiteName: "Test", Downloads: []Download{{Id: 1, Title: "<script>alert(1)</script>", Summary: "<img src=x onerror=alert(1)>", FileName: "A&B.txt"}}, DownloadCategories: []DownloadCategory{{Id: 2, Name: "A&B"}}}
		b, err := e.Render(theme, "downloads", d)
		if err != nil {
			t.Fatal(theme, err)
		}
		s := string(b)
		if strings.Contains(s, "<script>alert(") || strings.Contains(s, "<img src=x") || !strings.Contains(s, "&lt;script&gt;") || !strings.Contains(s, "/downloads/1/file") {
			t.Fatal(theme, "unsafe metadata or missing file URL")
		}
	}
}
