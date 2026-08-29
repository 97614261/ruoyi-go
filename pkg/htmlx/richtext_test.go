package htmlx

import (
	"strings"
	"testing"
)

func TestSanitizeQuillPreservesEditorFormatting(t *testing.T) {
	input := `<h2 class="ql-align-center">标题</h2>` +
		`<p><strong>粗体</strong><em>斜体</em><u>下划线</u><s>删除线</s></p>` +
		`<blockquote>引用</blockquote><pre class="ql-syntax">code()</pre>` +
		`<div class="ql-code-block-container" spellcheck="false"><div class="ql-code-block">line()</div></div>` +
		`<ol><li data-list="bullet" class="ql-indent-2"><span class="ql-ui"></span>列表</li></ol>` +
		`<p><span style="color: rgb(230, 0, 0); background-color: #ffff00">彩色</span></p>` +
		`<p><a href="https://example.com/docs" target="_blank" rel="noopener noreferrer">链接</a></p>` +
		`<p><img src="/profile/upload/demo.png" alt="图片" width="320"></p>` +
		`<iframe class="ql-video" frameborder="0" allowfullscreen src="https://example.com/video"></iframe>`

	got := SanitizeQuill(input)
	for _, want := range []string{
		`<h2 class="ql-align-center">标题</h2>`,
		`<strong>粗体</strong>`,
		`class="ql-syntax"`,
		`class="ql-code-block-container"`,
		`spellcheck="false"`,
		`class="ql-code-block"`,
		`data-list="bullet"`,
		`class="ql-indent-2"`,
		`class="ql-ui"`,
		`color: rgb(230, 0, 0)`,
		`background-color: #ffff00`,
		`href="https://example.com/docs"`,
		`src="/profile/upload/demo.png"`,
		`class="ql-video"`,
		`src="https://example.com/video"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("净化后应保留 %q，实际=%s", want, got)
		}
	}
}

func TestSanitizeQuillRemovesExecutableMarkup(t *testing.T) {
	input := `<script>alert(1)</script>` +
		`<p OnClIcK="alert(2)">正文<img src="x" oNeRrOr="alert(3)"></p>` +
		`<a href="jav&#x61;script:alert(4)">危险链接</a>` +
		`<iframe src="data:text/html,&lt;script&gt;alert(5)&lt;/script&gt;"></iframe>` +
		`<object data="https://example.com/x"></object><svg onload="alert(6)"></svg>` +
		`<span style="color: transparent; background-image: url(javascript:alert(7)); position: fixed">颜色</span>`

	got := SanitizeQuill(input)
	for _, forbidden := range []string{
		"<script", "alert(1)", "onclick", "onerror", "javascript:",
		"data:text/html", "<object", "<svg", "background-image", "position:",
	} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Errorf("净化后不应包含 %q，实际=%s", forbidden, got)
		}
	}
	for _, want := range []string{"正文", "危险链接", "颜色", `style="color: transparent"`} {
		if !strings.Contains(got, want) {
			t.Errorf("净化后应保留安全内容 %q，实际=%s", want, got)
		}
	}
}
