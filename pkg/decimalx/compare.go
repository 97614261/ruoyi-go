package decimalx

// Compare validates both operands; never compare decimal strings lexically.
func Compare(a, b string) (int, error) {
	x, e := units(a)
	if e != nil {
		return 0, e
	}
	y, e := units(b)
	if e != nil {
		return 0, e
	}
	return x.Cmp(y), nil
}

// Subtract rejects a negative result rather than wrapping or silently clamping.
func Subtract(a, b string) (string, error) {
	x, e := units(a)
	if e != nil {
		return "", e
	}
	y, e := units(b)
	if e != nil {
		return "", e
	}
	return format(x.Sub(x, y))
}
