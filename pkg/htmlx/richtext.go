// Package htmlx contains HTML handling shared by services that return markup.
package htmlx

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

var quillPolicy = newQuillPolicy()

// SanitizeQuill keeps the formatting emitted by the project's Quill editor
// while removing executable markup and unsafe resource URLs.
func SanitizeQuill(value string) string {
	return quillPolicy.Sanitize(value)
}

func newQuillPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	textElements := []string{
		"p", "div", "span", "br",
		"strong", "b", "em", "i", "u", "s",
		"blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"ol", "ul", "li",
	}
	allElements := append(append([]string{}, textElements...), "a", "img", "iframe")
	p.AllowElements(allElements...)

	// Quill represents alignment, indentation, size, font, direction, code,
	// list UI and video formats as a small, fixed set of CSS classes.
	classToken := `ql-(?:align-(?:center|right|justify)|indent-[1-8]|size-(?:small|large|huge)|font-(?:serif|monospace)|direction-rtl|video|syntax|ui|code-block(?:-container)?)`
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^(?:` + classToken + `)(?:\s+` + classToken + `)*$`)).OnElements(allElements...)
	p.AllowAttrs("spellcheck").Matching(regexp.MustCompile(`^false$`)).OnElements("div")

	// Color pickers are the only enabled Quill formats that emit inline CSS.
	// Semicolons, url(), expression() and other CSS properties cannot pass.
	colorValue := regexp.MustCompile(`(?i)^(?:#(?:[0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})|(?:rgb|hsl)a?\(\s*[-+]?[0-9.]+%?(?:\s*,\s*[-+]?[0-9.]+%?){2}(?:\s*,\s*(?:0|1|0?\.[0-9]+|[0-9]+%))?\s*\)|transparent)$`)
	p.AllowStyles("color", "background-color").Matching(colorValue).OnElements(allElements...)

	p.AllowAttrs("data-list").Matching(regexp.MustCompile(`^(?:ordered|bullet|checked|unchecked)$`)).OnElements("li")

	p.AllowAttrs("href", "title").OnElements("a")
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_(?:blank|self)$`)).OnElements("a")
	p.AllowAttrs("rel").Matching(regexp.MustCompile(`^(?:(?:noopener|noreferrer|nofollow)(?:\s+|$))+$`)).OnElements("a")

	p.AllowAttrs("src", "alt", "title").OnElements("img")
	p.AllowAttrs("width", "height").Matching(regexp.MustCompile(`^[1-9][0-9]{0,4}$`)).OnElements("img")

	p.AllowAttrs("src").OnElements("iframe")
	p.AllowAttrs("frameborder").Matching(regexp.MustCompile(`^[01]$`)).OnElements("iframe")
	p.AllowAttrs("allowfullscreen").OnElements("iframe")

	// URL attributes on links, images and iframes are parsed by bluemonday.
	// This deliberately excludes javascript:, vbscript: and data: URLs.
	p.AllowURLSchemes("http", "https")
	p.AllowRelativeURLs(true)
	return p
}
