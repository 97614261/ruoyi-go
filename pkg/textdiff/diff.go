package textdiff

import "unicode"

type Part struct {
	Text string
	Op   string
}

// Tokens preserve whitespace and markup as inert text. The middle LCS has a strict memory/work budget.
func tokens(s string) []string {
	r := []rune(s)
	out := []string{}
	for i := 0; i < len(r); {
		j := i + 1
		if r[i] < 128 && unicode.IsLetter(r[i]) {
			for j < len(r) && r[j] < 128 && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j])) {
				j++
			}
		}
		out = append(out, string(r[i:j]))
		i = j
	}
	return out
}
func Diff(left, right string) ([]Part, bool) {
	a, b := tokens(left), tokens(right)
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	end := 0
	for end < len(a)-start && end < len(b)-start && a[len(a)-1-end] == b[len(b)-1-end] {
		end++
	}
	out := []Part{}
	add := func(op, text string) {
		if text == "" {
			return
		}
		if len(out) > 0 && out[len(out)-1].Op == op {
			out[len(out)-1].Text += text
		} else {
			out = append(out, Part{text, op})
		}
	}
	join := func(v []string) string {
		n := 0
		for _, s := range v {
			n += len(s)
		}
		buf := make([]byte, 0, n)
		for _, s := range v {
			buf = append(buf, s...)
		}
		return string(buf)
	}
	add("equal", join(a[:start]))
	x, y := a[start:len(a)-end], b[start:len(b)-end]
	coarse := len(x) > 800 || len(y) > 800 || len(x)*len(y) > 60000
	if coarse {
		add("delete", join(x))
		add("insert", join(y))
	} else {
		width := len(y) + 1
		dp := make([]uint16, (len(x)+1)*width)
		for i := len(x) - 1; i >= 0; i-- {
			for j := len(y) - 1; j >= 0; j-- {
				v := dp[(i+1)*width+j]
				if dp[i*width+j+1] > v {
					v = dp[i*width+j+1]
				}
				if x[i] == y[j] {
					v = dp[(i+1)*width+j+1] + 1
				}
				dp[i*width+j] = v
			}
		}
		i, j := 0, 0
		for i < len(x) || j < len(y) {
			if i < len(x) && j < len(y) && x[i] == y[j] {
				add("equal", x[i])
				i++
				j++
			} else if j < len(y) && (i == len(x) || dp[i*width+j+1] >= dp[(i+1)*width+j]) {
				add("insert", y[j])
				j++
			} else {
				add("delete", x[i])
				i++
			}
		}
	}
	add("equal", join(a[len(a)-end:]))
	return out, coarse
}
