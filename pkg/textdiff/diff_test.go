package textdiff

import (
	"strings"
	"testing"
)

func TestDiffRoundTripAndBound(t *testing.T) {
	for _, pair := range [][2]string{{"你好世界", "你好门户"}, {"a b\n旧段落", "a c\n新段落"}, {"<img src='/media/1'>", "<img src='/media/2'>"}, {"", "x"}, {"same", "same"}, {strings.Repeat("a", 10000), strings.Repeat("中", 10000)}} {
		parts, _ := Diff(pair[0], pair[1])
		a, b := "", ""
		for _, p := range parts {
			if p.Op != "insert" {
				a += p.Text
			}
			if p.Op != "delete" {
				b += p.Text
			}
		}
		if a != pair[0] || b != pair[1] {
			t.Fatal("diff lost text")
		}
	}
}
