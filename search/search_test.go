package search

import (
	"fmt"
	"strings"
	"testing"
)

const baseStr = "this is a string with many "

var repeats = []int{10, 100, 1000, 10000}

func BenchmarkTextWrap(b *testing.B) {
	for _, rep := range repeats {
		str := baseStr + strings.Repeat("words ", rep)
		b.Run(fmt.Sprintf("input_size_%d", rep), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				out := wordWrap(str, 40)
				strings.Split(out, "\n")
			}
		})
	}
}
