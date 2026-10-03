package module

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// WriteFS is the file access that modules need. Paths are OS paths, not
// io/fs paths, so absolute paths under Env.Home work.
type WriteFS interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Stat(name string) (fs.FileInfo, error)
	// Lstat is Stat without following a final symlink.
	Lstat(name string) (fs.FileInfo, error)
	Rename(oldpath, newpath string) error
	// Remove deletes a file or an empty directory.
	Remove(name string) error
}

// OSFS is the WriteFS of the real file system.
type OSFS struct{}

var _ WriteFS = OSFS{}

// ReadFile calls os.ReadFile.
func (OSFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

// WriteFile calls os.WriteFile.
func (OSFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// MkdirAll calls os.MkdirAll.
func (OSFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }

// Stat calls os.Stat.
func (OSFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

// Lstat calls os.Lstat.
func (OSFS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }

// Remove calls os.Remove.
func (OSFS) Remove(name string) error { return os.Remove(name) }

// Rename calls os.Rename.
func (OSFS) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }

// MemFS is an in-memory WriteFS for tests. The zero value is an empty file
// system that holds only the root directory. Like the OS, WriteFile needs an
// existing parent directory, and errors wrap fs.ErrNotExist, fs.ErrExist, or
// fs.ErrInvalid in an *fs.PathError. MemFS records each file's permission
// bits as given, with no umask, so tests can assert modes such as 0600.
type MemFS struct {
	mu      sync.Mutex
	entries map[string]memEntry
}

var _ WriteFS = (*MemFS)(nil)

type memEntry struct {
	data []byte
	mode fs.FileMode // includes fs.ModeDir for directories
	link string      // non-empty for a symlink: the cleaned target path
}

// ReadFile returns a copy of the contents of name.
func (m *MemFS) ReadFile(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.follow(clean(name))
	switch {
	case !ok:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	case e.mode.IsDir():
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	return slices.Clone(e.data), nil
}

// WriteFile stores a copy of data at name with mode perm. It replaces an
// existing file and its mode.
func (m *MemFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(name)
	if parent, ok := m.lookup(filepath.Dir(p)); !ok || !parent.mode.IsDir() {
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if e, ok := m.lookup(p); ok && e.mode.IsDir() {
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	m.set(p, memEntry{data: slices.Clone(data), mode: perm.Perm()})
	return nil
}

// MkdirAll creates path and every missing parent with mode perm.
func (m *MemFS) MkdirAll(path string, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(path)
	var missing []string
	for dir := p; ; dir = filepath.Dir(dir) {
		e, ok := m.lookup(dir)
		if ok {
			if !e.mode.IsDir() {
				return &fs.PathError{Op: "mkdir", Path: dir, Err: fs.ErrExist}
			}
			break
		}
		missing = append(missing, dir)
	}
	for _, dir := range missing {
		m.set(dir, memEntry{mode: fs.ModeDir | perm.Perm()})
	}
	return nil
}

// Stat describes name. A missing name is an error that wraps fs.ErrNotExist.
func (m *MemFS) Stat(name string) (fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(name)
	e, ok := m.follow(p)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return memInfo{name: filepath.Base(p), size: int64(len(e.data)), mode: e.mode}, nil
}

// Lstat describes name without following it when it is a symlink.
func (m *MemFS) Lstat(name string) (fs.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(name)
	e, ok := m.lookup(p)
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
	}
	if e.link != "" {
		return memInfo{name: filepath.Base(p), size: int64(len(e.link)), mode: fs.ModeSymlink | 0o777}, nil
	}
	return memInfo{name: filepath.Base(p), size: int64(len(e.data)), mode: e.mode}, nil
}

// Symlink creates name as a symlink to target. Stat and ReadFile follow it;
// Lstat does not. It exists for tests and is not part of WriteFS.
func (m *MemFS) Symlink(target, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(name)
	if parent, ok := m.lookup(filepath.Dir(p)); !ok || !parent.mode.IsDir() {
		return &os.LinkError{Op: "symlink", Old: target, New: name, Err: fs.ErrNotExist}
	}
	if _, ok := m.lookup(p); ok {
		return &os.LinkError{Op: "symlink", Old: target, New: name, Err: fs.ErrExist}
	}
	m.set(p, memEntry{link: clean(target)})
	return nil
}

// Remove deletes a file or an empty directory.
func (m *MemFS) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := clean(name)
	e, ok := m.lookup(p)
	if !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}
	if e.mode.IsDir() {
		prefix := p + string(filepath.Separator)
		for q := range m.entries {
			if strings.HasPrefix(q, prefix) {
				return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrExist}
			}
		}
	}
	delete(m.entries, p)
	return nil
}

// Rename moves a file, or a directory and everything under it.
func (m *MemFS) Rename(oldpath, newpath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	from, to := clean(oldpath), clean(newpath)
	e, ok := m.lookup(from)
	if !ok {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrNotExist}
	}
	if parent, ok := m.lookup(filepath.Dir(to)); !ok || !parent.mode.IsDir() {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrNotExist}
	}
	if from == to {
		return nil
	}
	if e.mode.IsDir() && strings.HasPrefix(to, from+string(filepath.Separator)) {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrInvalid}
	}
	if dst, ok := m.lookup(to); ok && dst.mode.IsDir() {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrExist}
	}
	prefix := from + string(filepath.Separator)
	for p, sub := range m.entries {
		if strings.HasPrefix(p, prefix) {
			delete(m.entries, p)
			m.entries[to+string(filepath.Separator)+strings.TrimPrefix(p, prefix)] = sub
		}
	}
	delete(m.entries, from)
	m.set(to, e)
	return nil
}

// lookup finds the entry at the cleaned path p. The root and "." always
// exist as directories. The caller holds m.mu.
func (m *MemFS) lookup(p string) (memEntry, bool) {
	if p == "." || p == string(filepath.Separator) {
		return memEntry{mode: fs.ModeDir | 0o755}, true
	}
	e, ok := m.entries[p]
	return e, ok
}

// follow is lookup that resolves symlinks, up to a fixed depth. The caller
// holds m.mu.
func (m *MemFS) follow(p string) (memEntry, bool) {
	for range 8 {
		e, ok := m.lookup(p)
		if !ok || e.link == "" {
			return e, ok
		}
		p = e.link
	}
	return memEntry{}, false
}

// set stores e at p, creating the map on first use. The caller holds m.mu.
func (m *MemFS) set(p string, e memEntry) {
	if m.entries == nil {
		m.entries = make(map[string]memEntry)
	}
	m.entries[p] = e
}

func clean(name string) string { return filepath.Clean(name) }

type memInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return i.mode }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.mode.IsDir() }
func (i memInfo) Sys() any           { return nil }
