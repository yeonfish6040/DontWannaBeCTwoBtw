//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/yeonfish6040/DontWannaBeCTwoBtw/ipv4"
)

type event struct {
	Time         time.Time    `json:"time"`
	Operation    string       `json:"operation"`
	Path         string       `json:"path"`
	Destination  string       `json:"destination,omitempty"`
	Offset       int64        `json:"offset,omitempty"`
	Length       int          `json:"length,omitempty"`
	PID          uint32       `json:"pid"`
	UID          uint32       `json:"uid"`
	GID          uint32       `json:"gid"`
	Command      string       `json:"command,omitempty"`
	Exe          string       `json:"exe,omitempty"`
	ProcessError string       `json:"process_error,omitempty"`
	PPID         string       `json:"ppid,omitempty"`
	StartTime    string       `json:"start_time,omitempty"`
	Process      *processInfo `json:"process,omitempty"`
	Detection    bool         `json:"detection"`
}
type recorder struct {
	mu      sync.Mutex
	out     *os.File
	queue   chan event
	done    chan struct{}
	cfg     settings
	history []event
}

func (r *recorder) record(ctx context.Context, op, path, dst string, off int64, length int, detection bool) {
	e := event{Time: time.Now().UTC(), Operation: op, Path: path, Destination: dst, Offset: off, Length: length, Detection: detection}
	if c, ok := fuse.FromContext(ctx); ok {
		e.PID = c.Pid
		e.UID = c.Uid
		e.GID = c.Gid
		if c.Pid > 0 {
			p := snapshot(c.Pid)
			e.Process = &p
			e.Command = p.Command
			e.Exe = p.Exe
			e.ProcessError = p.Error
			e.PPID = fmt.Sprint(p.PPID)
			e.StartTime = p.Start
		}
	} else {
		e.ProcessError = "FUSE caller unavailable"
	}

	select {
	case r.queue <- e:
	default:
		log.Print("event queue full: event dropped")
	}

}

type root struct {
	fs.Inode
	name string
	rec  *recorder
}

func (r *root) OnAdd(ctx context.Context) {
	r.AddChild(r.name, r.NewPersistentInode(ctx, &file{name: r.name, rec: r.rec}, fs.StableAttr{Mode: syscall.S_IFREG}), false)
}
func (r *root) Create(ctx context.Context, name string, flags, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	r.rec.record(ctx, "create", name, "", 0, 0, true)
	return nil, nil, 0, syscall.EPERM
}
func (r *root) Unlink(ctx context.Context, name string) syscall.Errno {
	r.rec.record(ctx, "unlink", name, "", 0, 0, true)
	return syscall.EPERM
}
func (r *root) Rename(ctx context.Context, name string, parent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	r.rec.record(ctx, "rename", name, newName, 0, 0, true)
	return syscall.EPERM
}
func (r *root) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	r.rec.record(ctx, "mkdir", name, "", 0, 0, true)
	return nil, syscall.EPERM
}
func (r *root) Rmdir(ctx context.Context, name string) syscall.Errno {
	r.rec.record(ctx, "rmdir", name, "", 0, 0, true)
	return syscall.EPERM
}

type file struct {
	fs.Inode
	name string
	rec  *recorder
}

func (f *file) Getattr(ctx context.Context, h fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFREG | 0644
	out.Size = ipv4.Size
	out.Nlink = 1
	return 0
}
func (f *file) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_TRUNC|syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		f.rec.record(ctx, "open_write", f.name, "", 0, 0, true)
		return nil, 0, syscall.EPERM
	}
	return nil, fuse.FOPEN_DIRECT_IO, 0
}
func (f *file) Read(ctx context.Context, h fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off < 0 {
		return nil, syscall.EINVAL
	}
	n, _ := ipv4.ReadAt(dest, off)
	f.rec.record(ctx, "read", f.name, "", off, n, false)
	return fuse.ReadResultData(dest[:n]), 0
}
func (f *file) Write(ctx context.Context, h fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	f.rec.record(ctx, "write", f.name, "", off, len(data), true)
	return 0, syscall.EPERM
}
func (f *file) Setattr(ctx context.Context, h fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	f.rec.record(ctx, "setattr", f.name, "", 0, 0, true)
	return syscall.EPERM
}
func main() {
	mount := flag.String("mountpoint", "", "empty directory to mount")
	name := flag.String("filename", "access.log", "decoy filename")
	logPath := flag.String("log", "", "JSONL event log outside mountpoint")
	configPath := flag.String("config", "", "optional JSON configuration")
	flag.Parse()
	if *mount == "" || *logPath == "" || *name == "" || *name == "." || *name == ".." || filepath.Base(*name) != *name || strings.Contains(*name, "/") {
		log.Fatal("provide -mountpoint, -log, and a valid -filename")
	}
	m, err := filepath.EvalSymlinks(*mount)
	if err != nil {
		log.Fatal(err)
	}
	entries, err := os.ReadDir(m)
	if err != nil || len(entries) > 0 {
		log.Fatal("mountpoint must be an empty directory")
	}
	l, err := filepath.Abs(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(l))
	if err != nil {
		log.Fatal(err)
	}
	l = filepath.Join(parent, filepath.Base(l))
	rel, err := filepath.Rel(m, l)
	if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
		log.Fatal("event log must be outside mountpoint")
	}
	out, err := os.OpenFile(l, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	info, err := out.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		log.Fatal("log must be regular and owner-only")
	}

	cfg := settings{MaxLogBytes: 64 << 20, Queue: 1024, History: 64, TimeoutSeconds: 5, MaxEvidence: 128}
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.MaxLogBytes < 4096 || cfg.Queue < 1 || cfg.Queue > 65536 || cfg.History < 0 || cfg.History > 1024 || cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > 60 {
		log.Fatal("invalid resource limits")
	}
	if cfg.MaxEvidence < 0 || cfg.MaxEvidence > 10000 {
		log.Fatal("invalid evidence limit")
	}
	if cfg.EvidenceDir != "" {
		d, err := filepath.EvalSymlinks(cfg.EvidenceDir)
		if err != nil {
			log.Fatal(err)
		}
		d, err = filepath.Abs(d)
		if err != nil {
			log.Fatal(err)
		}
		rel, err := filepath.Rel(m, d)
		if err != nil || rel == "." || (!strings.HasPrefix(rel, "../") && rel != "..") {
			log.Fatal("evidence must be outside mount")
		}
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			log.Fatal("evidence directory must be owner-only")
		}
		cfg.EvidenceDir = d
	}
	if len(cfg.Command) > 0 {
		if !filepath.IsAbs(cfg.Command[0]) {
			log.Fatal("hook executable must be absolute")
		}
		allowed := false
		for _, v := range cfg.AllowExecutables {
			if v == cfg.Command[0] {
				allowed = true
			}
		}
		if !allowed {
			log.Fatal("hook executable is not allowlisted")
		}
	}
	rec := &recorder{out: out, queue: make(chan event, cfg.Queue), done: make(chan struct{}), cfg: cfg}
	go rec.run()
	defer func() { close(rec.queue); <-rec.done }()
	r := &root{name: *name, rec: rec}
	server, err := fs.Mount(m, r, &fs.Options{MountOptions: fuse.MountOptions{FsName: "dwbc", Name: "dwbc", DisableXAttrs: true}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "mounted %s/%s\n", m, *name)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() { server.Wait(); close(done) }()
	select {
	case <-ch:
		if err := server.Unmount(); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("unmount failed; filesystem still active: %v", err)
		}
	case <-done:
	}
	server.Wait()
}

type settings struct {
	MaxLogBytes      int64    `json:"max_log_bytes"`
	Queue            int      `json:"queue"`
	History          int      `json:"history"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	Command          []string `json:"command"`
	EvidenceDir      string   `json:"evidence_dir"`
	MaxEvidence      int      `json:"max_evidence"`
	AllowExecutables []string `json:"allow_executables"`
}

// A single worker bounds concurrent evidence/action work. Event payloads never
// become shell arguments; hooks receive JSON through standard input.
func (r *recorder) run() {
	defer close(r.done)
	lastAction := time.Time{}
	evidenceCount := 0
	if r.cfg.EvidenceDir != "" {
		f, err := os.Open(r.cfg.EvidenceDir)
		if err != nil {
			log.Printf("evidence: %v", err)
			evidenceCount = r.cfg.MaxEvidence
		} else {
			entries, _ := f.Readdirnames(r.cfg.MaxEvidence + 1)
			evidenceCount = len(entries)
			f.Close()
		}
	}
	for e := range r.queue {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		b = append(b, '\n')
		info, err := r.out.Stat()
		if err != nil {
			log.Printf("stat log: %v", err)
			continue
		}
		if info.Size()+int64(len(b)) > r.cfg.MaxLogBytes {
			if err := r.out.Truncate(0); err != nil {
				log.Printf("truncate log: %v", err)
				continue
			}
		}
		r.mu.Lock()
		_, err = r.out.Write(b)
		r.mu.Unlock()
		if err != nil {
			log.Printf("write log: %v", err)
		}
		if r.cfg.History > 0 {
			r.history = append(r.history, e)
			if len(r.history) > r.cfg.History {
				r.history = r.history[1:]
			}
		}
		if e.Detection {
			if r.cfg.EvidenceDir != "" && evidenceCount < r.cfg.MaxEvidence {
				evidenceCount++
				tree := []processInfo{}
				pid := e.PID
				for depth := 0; depth < 8 && pid > 0; depth++ {
					p := snapshot(pid)
					tree = append(tree, p)
					if p.Error != "" || p.PPID == pid {
						break
					}
					pid = p.PPID
				}
				payload := struct {
					Event       event
					History     []event
					Tree        []processInfo
					VirtualSize uint64
				}{e, r.history, tree, ipv4.Size}
				data, err := json.Marshal(payload)
				if err == nil {
					path := filepath.Join(r.cfg.EvidenceDir, fmt.Sprintf("event-%d.json", e.Time.UnixNano()))
					f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
					if err == nil {
						_, err = f.Write(data)
						f.Close()
					}
					if err != nil {
						log.Printf("evidence: %v", err)
					}
				}
			}

			if err := r.out.Sync(); err != nil {
				log.Printf("sync: %v", err)
			}
			if len(r.cfg.Command) > 0 && time.Since(lastAction) >= time.Second {
				lastAction = time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.cfg.TimeoutSeconds)*time.Second)
				cmd := exec.CommandContext(ctx, r.cfg.Command[0], r.cfg.Command[1:]...)
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
				cmd.WaitDelay = time.Second
				cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
				cmd.Stdin = strings.NewReader(string(b))
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				if err := cmd.Run(); err != nil {
					log.Printf("hook: %v", err)
				}
				cancel()
			}
		}
	}
}

func (r *root) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	r.rec.record(ctx, "symlink", name, "", 0, 0, true)
	return nil, syscall.EPERM
}
func (r *root) Link(ctx context.Context, target fs.InodeEmbedder, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	r.rec.record(ctx, "link", name, "", 0, 0, true)
	return nil, syscall.EPERM
}
func (r *root) Mknod(ctx context.Context, name string, mode, dev uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	r.rec.record(ctx, "mknod", name, "", 0, 0, true)
	return nil, syscall.EPERM
}
func (r *root) Setattr(ctx context.Context, h fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	r.rec.record(ctx, "directory_setattr", ".", "", 0, 0, true)
	return syscall.EPERM
}
