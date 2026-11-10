package main

import (
	"os"
	"testing"
)

func TestSnapshot(t *testing.T) {
	p := snapshot(uint32(os.Getpid()))
	if p.Error != "" || p.Start == "" || p.PPID == 0 {
		t.Fatalf("%+v", p)
	}
	for _, arg := range p.Cmdline {
		if arg != "[REDACTED]" {
			t.Fatal("unmasked argument")
		}
	}
}
func TestMissingProcess(t *testing.T) {
	p := snapshot(4294967295)
	if p.Error == "" {
		t.Fatal("missing process accepted")
	}
}
func TestStatParser(t *testing.T) {
	if _, _, err := identity([]byte("malformed")); err == nil {
		t.Fatal("accepted malformed stat")
	}
}
