// Package ipv4 generates the byte stream of all IPv4 addresses, one per line.
package ipv4

import (
	"io"
	"strconv"
)

const Count uint64 = 1 << 32
const Size uint64 = 61337501696

// digitsPrefix returns the sum of decimal digit counts of 0 through n-1,
// where n is at most 256.
func digitsPrefix(n uint64) uint64 {
	if n <= 10 {
		return n
	}
	if n <= 100 {
		return 10 + (n-10)*2
	}
	return 190 + (n-100)*3
}

// Offset returns the first byte of line n, including the end sentinel Count.
func Offset(n uint64) uint64 {
	if n > Count {
		n = Count
	}
	sum := n * 4 // three dots and newline
	for _, shift := range []uint{24, 16, 8, 0} {
		block := uint64(1) << shift
		cycles := n / (block * 256)
		rem := n % (block * 256)
		full := rem / block
		sum += cycles*block*digitsPrefix(256) + block*digitsPrefix(full) + (rem%block)*uint64(len(strconv.FormatUint(full, 10)))
	}
	return sum
}

// ReadAt generates a slice of the virtual file without storing it.
func ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.EOF
	}
	pos := uint64(off)
	if pos >= Size {
		return 0, io.EOF
	}
	lo, hi := uint64(0), Count
	for lo+1 < hi {
		m := (lo + hi) / 2
		if Offset(m) <= pos {
			lo = m
		} else {
			hi = m
		}
	}
	n := 0
	skip := pos - Offset(lo)
	for i := lo; i < Count && n < len(p); i++ {
		var b []byte
		b = strconv.AppendUint(b, (i>>24)&255, 10)
		b = append(b, '.')
		b = strconv.AppendUint(b, (i>>16)&255, 10)
		b = append(b, '.')
		b = strconv.AppendUint(b, (i>>8)&255, 10)
		b = append(b, '.')
		b = strconv.AppendUint(b, i&255, 10)
		b = append(b, '\n')
		n += copy(p[n:], b[skip:])
		skip = 0
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
