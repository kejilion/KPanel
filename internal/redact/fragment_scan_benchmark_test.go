package redact

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkBracketLogScan(b *testing.B) {
	for _, size := range []int{16384, 65536, 262144, 1048576} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			value := strings.Repeat("[x", size/2)
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = Text(value)
			}
		})
	}
}
