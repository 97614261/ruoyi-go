package portalsearch

import (
	"strings"
	"testing"
)

func TestPlainAndChinese(t *testing.T) {
	s := Plain(`<p>跨境<b>电商</b> &amp; 仓储</p><script>private</script><style>leak</style><p>配送</p>`)
	if !strings.Contains(s, "跨境电商 & 仓储") || strings.Contains(s, "private") || strings.Contains(s, "leak") {
		t.Fatal(s)
	}
	if len(Hashes("电商仓储")) != 3 || len(Hashes("仓")) != 1 {
		t.Fatal("gram query wrong")
	}
	if Normalize(" ＧＯ  部署 ") != "go 部署" {
		t.Fatal("normalization")
	}
	p := Highlight("这是跨境电商仓储的说明", "电商", 6)
	text := ""
	matched := false
	for _, v := range p {
		text += v.Text
		matched = matched || v.Match
	}
	if !matched || !strings.Contains(text, "电商") {
		t.Fatal("snippet lost match")
	}
	if len(Build("电商", "", "电商正文")) == 0 {
		t.Fatal("postings empty")
	}
}
