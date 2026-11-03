package portalsearch

import (
	"crypto/sha256"
	"golang.org/x/net/html"
	"sort"
	"strings"
	"unicode"
)

type Posting struct {
	Hash   []byte
	Weight int
}
type Part struct {
	Text  string
	Match bool
}

// Plain keeps inline text joined, separates block elements, and never indexes hidden executable markup.
func Plain(raw string) string {
	doc, e := html.Parse(strings.NewReader(raw))
	if e != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "template", "noscript", "head":
				return
			case "br", "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "section", "pre", "blockquote":
				b.WriteByte('\n')
			}
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "li", "tr", "section", "pre", "blockquote":
				b.WriteByte('\n')
			}
		}
	}
	walk(doc)
	return strings.TrimSpace(b.String())
}
func fold(r rune) rune {
	if r >= 0xff01 && r <= 0xff5e {
		r -= 0xfee0
	}
	if r == 0x3000 || unicode.IsSpace(r) {
		return ' '
	}
	return unicode.ToLower(r)
}
func normalized(s string) ([]rune, []int) {
	out := []rune{}
	pos := []int{}
	for i, r := range []rune(s) {
		r = fold(r)
		if r == ' ' && (len(out) == 0 || out[len(out)-1] == ' ') {
			continue
		}
		out = append(out, r)
		pos = append(pos, i)
	}
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
		pos = pos[:len(pos)-1]
	}
	return out, pos
}
func Normalize(s string) string { r, _ := normalized(s); return string(r) }
func Terms(q string) []string   { return strings.Fields(Normalize(q)) }
func grams(s string, fn func(string)) {
	r := []rune(Normalize(s))
	for i, c := range r {
		if c == ' ' {
			continue
		}
		fn(string(c))
		if i+1 < len(r) && r[i+1] != ' ' {
			fn(string(r[i : i+2]))
		}
	}
}
func Hashes(q string) [][]byte {
	seen := map[[32]byte]bool{}
	for _, term := range Terms(q) {
		r := []rune(term)
		if len(r) == 1 {
			seen[sha256.Sum256([]byte(term))] = true
		} else {
			for i := 0; i+1 < len(r); i++ {
				seen[sha256.Sum256([]byte(string(r[i:i+2])))] = true
			}
		}
	}
	keys := make([][32]byte, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return string(keys[i][:]) < string(keys[j][:]) })
	out := [][]byte{}
	for _, k := range keys {
		x := make([]byte, 32)
		copy(x, k[:])
		out = append(out, x)
	}
	return out
}
func Build(title, summary, body string) []Posting {
	counts := map[[32]byte]int{}
	add := func(s string, weight int) {
		seen := map[[32]byte]int{}
		grams(s, func(g string) {
			h := sha256.Sum256([]byte(g))
			if seen[h] < 16 {
				seen[h]++
				counts[h] += weight
			}
		})
	}
	add(title, 12)
	add(summary, 4)
	add(body, 1)
	out := make([]Posting, 0, len(counts))
	for h, w := range counts {
		k := make([]byte, 32)
		copy(k, h[:])
		out = append(out, Posting{k, w})
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i].Hash) < string(out[j].Hash) })
	return out
}

// Highlight uses original text, renders only text pieces, and bounds body excerpts to maxRunes.
func Highlight(raw, q string, maxRunes int) []Part {
	r := []rune(raw)
	norm, pos := normalized(raw)
	mark := make([]bool, len(r))
	first := -1
	for _, term := range Terms(q) {
		t := []rune(term)
		for i := 0; i+len(t) <= len(norm); i++ {
			match := true
			for j := range t {
				if norm[i+j] != t[j] {
					match = false
					break
				}
			}
			if match {
				lo, hi := pos[i], pos[i+len(t)-1]+1
				if first < 0 || lo < first {
					first = lo
				}
				for j := lo; j < hi; j++ {
					mark[j] = true
				}
			}
		}
	}
	start, end := 0, len(r)
	if maxRunes > 0 && end > maxRunes {
		if first > maxRunes/3 {
			start = first - maxRunes/3
		}
		end = start + maxRunes
		if end > len(r) {
			end = len(r)
			start = end - maxRunes
		}
	}
	out := []Part{}
	if start > 0 {
		out = append(out, Part{"…", false})
	}
	for i := start; i < end; {
		j := i + 1
		for j < end && mark[j] == mark[i] {
			j++
		}
		out = append(out, Part{string(r[i:j]), mark[i]})
		i = j
	}
	if end < len(r) {
		out = append(out, Part{"…", false})
	}
	return out
}
