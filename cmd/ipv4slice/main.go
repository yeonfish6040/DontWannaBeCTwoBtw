package main

import (
	"flag"
	"fmt"
	"github.com/yeonfish6040/DontWannaBeCTwoBtw/ipv4"
	"os"
)

func main() {
	offset := flag.Int64("offset", 0, "byte offset")
	length := flag.Int("length", 128, "number of bytes (maximum 1 MiB)")
	flag.Parse()
	if *offset < 0 || *length < 0 || *length > 1<<20 {
		fmt.Fprintln(os.Stderr, "invalid offset or length")
		os.Exit(2)
	}
	p := make([]byte, *length)
	n, _ := ipv4.ReadAt(p, *offset)
	if _, err := os.Stdout.Write(p[:n]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
