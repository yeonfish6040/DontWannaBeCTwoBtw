package ipv4

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
)

func TestOffsetsAndSlices(t *testing.T) {
	if Offset(Count) != Size {
		t.Fatalf("size %d", Offset(Count))
	}
	var s strings.Builder
	for i := 0; i < 65536; i++ {
		fmt.Fprintf(&s, "0.0.%d.%d\n", i/256, i%256)
	}
	want := s.String()
	for i := 0; i < 1000; i++ {
		off := rand.New(rand.NewSource(int64(i))).Intn(len(want))
		p := make([]byte, 137)
		n, err := ReadAt(p, int64(off))
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		end := off + n
		if string(p[:n]) != want[off:end] {
			t.Fatalf("offset %d", off)
		}
	}
	for _, n := range []uint64{9, 10, 99, 100, 255, 256, 65535, 65536, Count - 1} {
		line := fmt.Sprintf("%d.%d.%d.%d\n", n>>24, (n>>16)&255, (n>>8)&255, n&255)
		p := make([]byte, len(line))
		_, err := ReadAt(p, int64(Offset(n)))
		if err != nil || string(p) != line {
			t.Fatalf("line %d: %q %v", n, p, err)
		}
	}
}
func BenchmarkReadAt(b *testing.B) {
	p := make([]byte, 128*1024)
	b.SetBytes(int64(len(p)))
	for i := 0; i < b.N; i++ {
		ReadAt(p, int64(i)*int64(len(p)))
	}
}
