// Package decimalx implements nonnegative DECIMAL(20,4) without floating point.
package decimalx

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var pattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,15})(\.[0-9]{1,4})?$`)
var maximum, _ = new(big.Int).SetString("99999999999999999999", 10)
var ErrRange = errors.New("金额须为非负DECIMAL(20,4)十进制字符串，且计算结果不得溢出")

func units(s string) (*big.Int, error) {
	if !pattern.MatchString(s) {
		return nil, ErrRange
	}
	parts := strings.SplitN(s, ".", 2)
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	n, ok := new(big.Int).SetString(parts[0]+frac+strings.Repeat("0", 4-len(frac)), 10)
	if !ok || n.Cmp(maximum) > 0 {
		return nil, ErrRange
	}
	return n, nil
}
func format(n *big.Int) (string, error) {
	if n.Sign() < 0 || n.Cmp(maximum) > 0 {
		return "", ErrRange
	}
	s := n.String()
	if len(s) < 5 {
		s = strings.Repeat("0", 5-len(s)) + s
	}
	return s[:len(s)-4] + "." + s[len(s)-4:], nil
}
func Normalize(s string) (string, error) {
	n, e := units(s)
	if e != nil {
		return "", e
	}
	return format(n)
}
func Multiply(s string, quantity int64) (string, error) {
	n, e := units(s)
	if e != nil || quantity < 0 {
		return "", ErrRange
	}
	return format(n.Mul(n, big.NewInt(quantity)))
}
func Add(a, b string) (string, error) {
	x, e := units(a)
	if e != nil {
		return "", e
	}
	y, e := units(b)
	if e != nil {
		return "", e
	}
	return format(x.Add(x, y))
}
