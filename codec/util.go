package codec

import (
	"fmt"
	"strings"
)

func slicesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func chars(b []byte) string {
	var res []string
	for _, c := range b {
		res = append(res, fmt.Sprintf("%c", c))
	}
	return strings.Join(res, ", ")
}
