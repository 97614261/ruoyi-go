package service

import (
	"strings"
	"testing"
)

func TestCMSBodyBoundary(t *testing.T) {
	for _, s := range []string{`<p><img src="https://example.com/x.png"></p>`, `<p><img src="/media/1?x=1"></p>`, `<p><img src="/media/01"></p>`, `<iframe src="https://example.com"></iframe>`, `<p><br></p>`, `<p><img src="https://example.com/x" src="/media/1"></p>`, strings.Repeat("x", (256<<10)+1)} {
		if _, _, err := CMSBody(s); err == nil {
			t.Errorf("accepted invalid body %.80s", s)
		}
	}
	out, ids, err := CMSBody(`<h2>标题</h2><p onclick="alert(1)">正文<script>alert(2)</script><img src="/media/12"><img src="/media/12"></p>`)
	if err != nil || len(ids) != 1 || ids[0] != 12 || strings.Contains(out, "script") || strings.Contains(out, "onclick") || !strings.Contains(out, "<h2>") {
		t.Fatal("body boundary failed", err, out, ids)
	}
}
