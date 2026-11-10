package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type processInfo struct {
	PID     uint32   `json:"pid"`
	PPID    uint32   `json:"ppid"`
	Start   string   `json:"start_time"`
	Command string   `json:"command,omitempty"`
	Exe     string   `json:"exe,omitempty"`
	Cmdline []string `json:"cmdline,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func boundedRead(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 8192))
}
func identity(b []byte) (uint32, string, error) {
	i := strings.LastIndex(string(b), ")")
	if i < 0 {
		return 0, "", fmt.Errorf("malformed stat")
	}
	v := strings.Fields(string(b)[i+1:])
	if len(v) < 20 {
		return 0, "", fmt.Errorf("short stat")
	}
	pp, e := strconv.ParseUint(v[1], 10, 32)
	return uint32(pp), v[19], e
}

// Compare start times around metadata reads. Exit/reuse clears potentially
// misattributed metadata. This cannot make procfs an atomic snapshot.
func snapshot(pid uint32) processInfo {
	p := processInfo{PID: pid}
	base := fmt.Sprintf("/proc/%d/", pid)
	a, e := boundedRead(base + "stat")
	if e != nil {
		p.Error = e.Error()
		return p
	}
	p.PPID, p.Start, e = identity(a)
	if e != nil {
		p.Error = e.Error()
		return p
	}
	b, e := boundedRead(base + "comm")
	if e == nil {
		p.Command = strings.TrimSpace(string(b))
	} else {
		p.Error = e.Error()
	}
	p.Exe, _ = os.Readlink(base + "exe")
	b, e = boundedRead(base + "cmdline")
	if e == nil {
		args := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
		for range args {
			p.Cmdline = append(p.Cmdline, "[REDACTED]")
		}
	}
	b, e = boundedRead(base + "stat")
	_, end, parseErr := identity(b)
	if e != nil || parseErr != nil || end != p.Start {
		return processInfo{PID: pid, Error: "process exited or identity changed during capture"}
	}
	return p
}
