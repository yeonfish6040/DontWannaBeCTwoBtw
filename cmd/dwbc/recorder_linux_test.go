package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvidenceAndLogBound(t *testing.T) {
	dir := t.TempDir()
	out, err := os.OpenFile(filepath.Join(dir, "events"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	evidence := filepath.Join(dir, "evidence")
	if err := os.Mkdir(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	r := &recorder{out: out, queue: make(chan event, 32), done: make(chan struct{}), cfg: settings{MaxLogBytes: 4096, History: 4, EvidenceDir: evidence, MaxEvidence: 2}}
	go r.run()
	for i := 0; i < 32; i++ {
		r.queue <- event{Time: time.Now().Add(time.Duration(i)), Operation: "write", Detection: true, PID: uint32(os.Getpid())}
	}
	close(r.queue)
	<-r.done
	entries, err := os.ReadDir(evidence)
	if err != nil || len(entries) != 2 {
		t.Fatalf("evidence %v %d", err, len(entries))
	}
	b, err := os.ReadFile(filepath.Join(evidence, entries[0].Name()))
	if err != nil || !json.Valid(b) {
		t.Fatalf("invalid evidence %v", err)
	}
	info, _ := out.Stat()
	if info.Size() > 4096 {
		t.Fatalf("log exceeded bound: %d", info.Size())
	}
	if len(r.history) != 4 {
		t.Fatal("history exceeded bound")
	}
}

func TestHookTimeout(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "events")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	r := &recorder{out: out, queue: make(chan event, 1), done: make(chan struct{}), cfg: settings{MaxLogBytes: 4096, TimeoutSeconds: 1, Command: []string{"/bin/sleep", "10"}}}
	start := time.Now()
	go r.run()
	r.queue <- event{Detection: true}
	close(r.queue)
	select {
	case <-r.done:
	case <-time.After(4 * time.Second):
		t.Fatal("hook failed to time out")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("slow timeout")
	}
}
