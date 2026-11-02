//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/yeonfish6040/DontWannaBeCTwoBtw/ipv4"
)

type event struct {
	Time         time.Time `json:"time"`
	Operation    string    `json:"operation"`
	Path         string    `json:"path"`
	Destination  string    `json:"destination,omitempty"`
	Offset       int64     `json:"offset,omitempty"`
	Length       int       `json:"length,omitempty"`
	PID          uint32    `json:"pid"`
	UID          uint32    `json:"uid"`
	GID          uint32    `json:"gid"`
	Command      string    `json:"command,omitempty"`
	Exe          string    `json:"exe,omitempty"`
	ProcessError string    `json:"process_error,omitempty"`
	Detection    bool      `json:"detection"`
}
type recorder struct {
	mu  sync.Mutex
	out *os.File
}

func (r *recorder) record(ctx context.Context, op, path, dst string, off int64, length int, detection bool) {
	e := event{Time: time.Now().UTC(), Operation: op, Path: path, Destination: dst, Offset: off, Length: length, Detection: detection}
	if c, ok := fuse.FromContext(ctx); ok {
		e.PID = c.Pid
		e.UID = c.Uid
		e.GID = c.Gid
		if c.Pid > 0 {
			base := filepath.Join("/proc", strconv.FormatUint(uint64(c.Pid), 10))
			b, err := os.ReadFile(filepath.Join(base, "comm"))
			if err == nil {
				e.Command = strings.TrimSpace(string(b))
			} else {
				e.ProcessError = err.Error()
			}
			if exe, err := os.Readlink(filepath.Join(base, "exe")); err == nil {
				e.Exe = exe
			}
		}
	} else {
		e.ProcessError = "FUSE caller unavailable"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := json.NewEncoder(r.out).Encode(e); err != nil {
		log.Printf("event log: %v", err)
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
	flag.Parse()
	if *mount == "" || *logPath == "" || *name == "." || *name == ".." || filepath.Base(*name) != *name || strings.Contains(*name, "/") {
		log.Fatal("provide -mountpoint, -log, and a valid -filename")
	}
	m, err := filepath.Abs(*mount)
	if err != nil {
		log.Fatal(err)
	}
	l, err := filepath.Abs(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	rel, err := filepath.Rel(m, l)
	if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
		log.Fatal("event log must be outside mountpoint")
	}
	out, err := os.OpenFile(l, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	r := &root{name: *name, rec: &recorder{out: out}}
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
			log.Printf("unmount: %v", err)
		}
	case <-done:
	}
}
