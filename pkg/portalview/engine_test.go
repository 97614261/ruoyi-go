package portalview

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"ruoyi-go/portal"
)

func TestEmbeddedScaffoldEscapesAllPages(t *testing.T) {
	e, err := New(portal.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range requiredPages {
		t.Run(page, func(t *testing.T) {
			body, err := e.Render("business", page, PageData{Title: `<script>alert(1)</script>`, SiteName: `A&B`, Body: `<img src=x onerror=alert(1)>`})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "<script>") || strings.Contains(string(body), "<img src=x") || !strings.Contains(string(body), "A&amp;B") {
				t.Fatal("plain text must stay escaped")
			}
		})
	}
	for _, id := range []string{"../business", "business/../../", "unknown"} {
		if body, err := e.Render(id, "home", PageData{}); err == nil || body != nil {
			t.Fatal("unregistered theme accepted")
		}
	}
	if _, err := e.Render("business", "header", PageData{}); err == nil {
		t.Fatal("internal fragment exposed as page")
	}
	if body, err := e.Render("business", "article", PageData{Body: strings.Repeat("x", maxHTMLBytes)}); err == nil || body != nil {
		t.Fatal("oversized or partial output returned")
	}
}

func fixture(id, marker, home string) fstest.MapFS {
	var text strings.Builder
	for _, page := range requiredPages {
		value := marker + `{{.Title}}`
		if page == "home" && home != "" {
			value = home
		}
		fmt.Fprintf(&text, `{{define %q}}%s{{end}}`, page, value)
	}
	return fstest.MapFS{
		"themes/" + id + "/manifest.json":          &fstest.MapFile{Data: []byte(fmt.Sprintf(`{"id":%q,"name":"test","version":"v1","dataVersion":1}`, id))},
		"themes/" + id + "/templates/pages.gohtml": &fstest.MapFile{Data: []byte(text.String())},
	}
}

func TestRejectsInvalidAndIncompleteThemes(t *testing.T) {
	for _, mutate := range []func(fstest.MapFS){
		func(f fstest.MapFS) {
			f["themes/a/manifest.json"].Data = []byte(`{"id":"b","name":"bad","version":"v1","dataVersion":1}`)
		},
		func(f fstest.MapFS) {
			f["themes/a/manifest.json"].Data = []byte(`{"id":"a","name":"bad","version":"v1","dataVersion":99}`)
		},
		func(f fstest.MapFS) {
			f["themes/a/templates/pages.gohtml"].Data = []byte(`{{define "home"}}only home{{end}}`)
		},
		func(f fstest.MapFS) { f["themes/a/templates/pages.gohtml"].Data = []byte(`{{define`) },
	} {
		f := fixture("a", "A", "")
		mutate(f)
		if _, err := New(f); err == nil {
			t.Fatal("invalid theme registry accepted")
		}
	}
}

func TestExecutionFailureDiscardsPartialOutput(t *testing.T) {
	e, err := New(fixture("a", "A", `prefix{{index .Title 1000}}`))
	if err != nil {
		t.Fatal(err)
	}
	if body, err := e.Render("a", "home", PageData{Title: "x"}); err == nil || body != nil {
		t.Fatal("partial HTML escaped the render boundary")
	}
}

func TestConcurrentThemesKeepSeparateNamespaces(t *testing.T) {
	f := fixture("a", "A", "")
	for path, file := range fixture("b", "B", "") {
		f[path] = file
	}
	e, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := e.Manifest("a")
	m.Name = "changed"
	if current, _ := e.Manifest("a"); current.Name == m.Name {
		t.Fatal("manifest changed through returned copy")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, marker := "a", "A"
			if i%2 == 1 {
				id, marker = "b", "B"
			}
			body, err := e.Render(id, "home", PageData{Title: "title"})
			if err != nil || string(body) != marker+"title" {
				t.Errorf("theme contamination: %q, %v", body, err)
			}
		}(i)
	}
	wg.Wait()
}
