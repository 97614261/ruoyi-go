// Package portalview parses immutable, registered Go portal themes. It performs no I/O to business databases.
package portalview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

var requiredPages = []string{"home", "column", "article", "product", "page", "search", "notfound", "error"}

// Manifest is shipped with trusted application code, never uploaded by a site administrator.
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	DataVersion int    `json:"dataVersion"`
}

// PageData is the initial, text-only rendering contract. Published rich text is added separately after sanitization.
type PageData struct {
	SiteName      string
	FooterLinks   []Card
	Icon          int64
	FontScale     string
	FrameWidth    string
	Radius        string
	Theme         string
	ThemeVersion  string
	Palette       string
	Density       string
	Footer        string
	Phone         string
	Email         string
	Logo          int64
	Links         []Card
	Banners       []Banner
	Sections      []Section
	Title         string
	Description   string
	Body          string
	BodyHTML      template.HTML
	Canonical     string
	NoIndex       bool
	Cards         []Card
	Articles      []Card
	Products      []Card
	Navigation    []Card
	Channels      []Card
	Query         string
	Prev          string
	Next          string
	ProductCode   string
	ProductParams string
	Gallery       []int64
	Cover         int64
	CoverAlt      string
	Author        string
}

type Section struct {
	Title, URL string
	Cards      []Card
}
type Banner struct {
	Title, URL string
	AssetId    int64
}
type Card struct {
	Title, Summary, URL, Kind string
	Cover                     *int64
}
type theme struct {
	manifest Manifest
	pages    *template.Template
}

// Engine is read-only after construction and can render different themes concurrently.
type Engine struct{ themes map[string]theme }

func New(source fs.FS) (*Engine, error) {
	entries, err := fs.ReadDir(source, "themes")
	if err != nil {
		return nil, fmt.Errorf("read theme registry: %w", err)
	}
	e := &Engine{themes: make(map[string]theme)}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if !identifier.MatchString(id) {
			return nil, errors.New("invalid theme directory")
		}
		base := "themes/" + id
		content, err := fs.ReadFile(source, base+"/manifest.json")
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", id, err)
		}
		var m Manifest
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&m); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", id, err)
		}
		if m.ID != id || m.Name == "" || !identifier.MatchString(m.Version) || m.DataVersion != 1 {
			return nil, fmt.Errorf("incompatible manifest: %s", id)
		}
		pages, err := template.New(id).Option("missingkey=error").ParseFS(source, base+"/templates/*.gohtml")
		if err != nil {
			return nil, fmt.Errorf("parse theme %s: %w", id, err)
		}
		for _, page := range requiredPages {
			if pages.Lookup(page) == nil {
				return nil, fmt.Errorf("theme %s lacks page %s", id, page)
			}
		}
		e.themes[id] = theme{manifest: m, pages: pages}
	}
	if len(e.themes) == 0 {
		return nil, errors.New("no registered themes")
	}
	return e, nil
}

// Manifest returns a value copy so callers cannot change the live registry.
func (e *Engine) Manifest(id string) (Manifest, bool) {
	t, ok := e.themes[id]
	return t.manifest, ok
}

const maxHTMLBytes = 2 << 20

type output struct{ buf bytes.Buffer }

func (w *output) Write(p []byte) (int, error) {
	if len(p) > maxHTMLBytes-w.buf.Len() {
		return 0, errors.New("rendered HTML exceeds limit")
	}
	return w.buf.Write(p)
}

// Render never returns partial HTML on an execution error. Theme IDs are registry keys, not file paths.
func (e *Engine) Render(id, page string, data PageData) ([]byte, error) {
	t, ok := e.themes[id]
	if !ok {
		return nil, errors.New("unknown portal theme")
	}
	allowed := false
	for _, name := range requiredPages {
		if name == page {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.New("unknown portal page")
	}
	var result output
	if err := t.pages.ExecuteTemplate(&result, page, data); err != nil {
		return nil, fmt.Errorf("render portal page: %w", err)
	}
	return result.buf.Bytes(), nil
}
