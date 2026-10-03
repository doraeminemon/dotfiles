package module_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
)

func TestMemFSWriteReadPerm(t *testing.T) {
	var m module.MemFS
	dir := "/home/u/.config/fish/conf.d"
	if err := m.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "secrets.fish")
	data := []byte("set -gx K v\n")
	if err := m.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X' // MemFS must have kept its own copy.

	got, err := m.ReadFile(name)
	if err != nil || string(got) != "set -gx K v\n" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	info, err := m.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.IsDir() || info.Size() != int64(len(got)) || info.Name() != "secrets.fish" {
		t.Errorf("Stat = mode %v dir %v size %d name %q", info.Mode(), info.IsDir(), info.Size(), info.Name())
	}
	for _, d := range []string{"/home", "/home/u/.config", dir} {
		info, err := m.Stat(d)
		if err != nil || !info.IsDir() {
			t.Errorf("Stat(%s) = %v, %v; want a directory", d, info, err)
		}
	}
}

func TestMemFSNotExist(t *testing.T) {
	var m module.MemFS
	if _, err := m.Stat("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat err = %v, want fs.ErrNotExist", err)
	}
	if _, err := m.ReadFile("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile err = %v, want fs.ErrNotExist", err)
	}
	if err := m.WriteFile("/no/parent/f", nil, 0o644); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("WriteFile without parent err = %v, want fs.ErrNotExist", err)
	}
	if err := m.Rename("/nope", "/other"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Rename err = %v, want fs.ErrNotExist", err)
	}
}

func TestMemFSMkdirAllOverFile(t *testing.T) {
	var m module.MemFS
	if err := m.WriteFile("/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.MkdirAll("/f/sub", 0o755); !errors.Is(err, fs.ErrExist) {
		t.Errorf("MkdirAll err = %v, want fs.ErrExist", err)
	}
}

func TestMemFSRename(t *testing.T) {
	var m module.MemFS
	for _, d := range []string{"/h/.config/a", "/backup"} {
		if err := m.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.WriteFile("/h/.config/a/x", []byte("1"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("/h/.config", "/backup/config"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("/h/.config/a/x"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old path still exists: %v", err)
	}
	info, err := m.Stat("/backup/config/a/x")
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("Stat moved file = %v, %v", info, err)
	}
	if got, _ := m.ReadFile("/backup/config/a/x"); string(got) != "1" {
		t.Errorf("moved contents = %q", got)
	}
}

func TestOSFS(t *testing.T) {
	var o module.OSFS
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := o.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "f")
	if err := o.WriteFile(name, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := name + ".bak"
	if err := o.Rename(name, moved); err != nil {
		t.Fatal(err)
	}
	if got, err := o.ReadFile(moved); err != nil || string(got) != "hi" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if info, err := o.Stat(moved); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("Stat = %v, %v", info, err)
	}
	if _, err := o.Stat(name); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat old = %v, want fs.ErrNotExist", err)
	}
}

func TestMemFSSymlinkLstatRemove(t *testing.T) {
	var m module.MemFS
	if err := m.MkdirAll("/r", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/r/target", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/r/target", "/r/link"); err != nil {
		t.Fatal(err)
	}
	if fi, err := m.Lstat("/r/link"); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("Lstat(link) = %v, %v; want symlink", fi, err)
	}
	if fi, err := m.Stat("/r/link"); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("Stat(link) = %v, %v; want regular file", fi, err)
	}
	if b, err := m.ReadFile("/r/link"); err != nil || string(b) != "hi" {
		t.Fatalf("ReadFile(link) = %q, %v", b, err)
	}
	if err := m.Remove("/r"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Remove(non-empty dir) = %v, want ErrExist", err)
	}
	if err := m.Remove("/r/link"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("/r/link"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Remove(missing) = %v, want ErrNotExist", err)
	}
	if _, err := m.Stat("/r/target"); err != nil {
		t.Fatalf("removing the link removed the target: %v", err)
	}
}
