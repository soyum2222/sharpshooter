package protocol

import "testing"

func BenchmarkUnmarshal(b *testing.B) {

	for i := 0; i < b.N; i++ {
		b := make([]byte, 1024)

		for k := range b {
			b[k] = uint8(k)
		}

		Unmarshal(b)
	}

}
