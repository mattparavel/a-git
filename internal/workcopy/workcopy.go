// Package workcopy manages a-git working copies.
//
// A working copy is optional and lightweight: a directory of files plus a
// small .agit/ directory recording which changeset the files came from, which
// paths are materialized (sparse checkouts), and stat information used to
// detect edits without rehashing every file. There is no local history and no
// packfile: everything else lives on the server and is reached through the
// API (directly or via the machine's daemon).
//
// Unlike git there is no staging area: every non-ignored file inside the
// sparse profile is tracked, so "commit" records whatever changed.
package workcopy

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
)

// MetaDir is the name of the metadata directory at the working copy root.
const MetaDir = ".agit"

// Config is the persistent configuration of a working copy.
type Config struct {
	Repo   string   `json:"repo"`
	Remote string   `json:"remote"`           // http(s):// or unix:// address of the daemon/cache/server
	Target string   `json:"target"`           // ref this working copy lands onto (default "main")
	Sparse []string `json:"sparse,omitempty"` // materialized path prefixes; empty means everything
}

// State records what the working copy is based on.
type State struct {
	Base object.ID `json:"base"` // changeset the files were checked out from
	Tree object.ID `json:"tree"` // its root tree
}

// FileState is the recorded state of a materialized file.
type FileState struct {
	Mode  object.Mode
	ID    object.ID
	Size  int64
	MTime int64 // unix nanoseconds
	Ino   uint64
}

// WorkCopy is an open working copy.
type WorkCopy struct {
	Root   string
	Config Config
	State  State
	Files  map[string]FileState
	// stamp is when the dirstate was last written; files modified at or after
	// it cannot be trusted by stat alone.
	stamp int64

	Client *client.Client
	Repo   *client.Repo

	lock *os.File
}

// ErrNotWorkCopy is returned when no working copy encloses a directory.
var ErrNotWorkCopy = errors.New("not inside an a-git working copy (run 'agit clone')")

// Find locates the working copy enclosing dir.
func Find(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; {
		if fi, err := os.Stat(filepath.Join(d, MetaDir, "config.json")); err == nil && !fi.IsDir() {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", ErrNotWorkCopy
		}
		d = parent
	}
}

// Open opens the working copy at root and connects to its remote.
func Open(root, token string) (*WorkCopy, error) {
	w := &WorkCopy{Root: root, Files: map[string]FileState{}}
	if err := readJSON(filepath.Join(root, MetaDir, "config.json"), &w.Config); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(root, MetaDir, "state.json"), &w.State); err != nil {
		return nil, err
	}
	if w.Config.Target == "" {
		w.Config.Target = "main"
	}
	if err := w.readDirstate(); err != nil {
		return nil, err
	}
	c, err := client.New(w.Config.Remote, token)
	if err != nil {
		return nil, err
	}
	w.Client = c
	w.Repo = c.Repo(w.Config.Repo)
	return w, nil
}

// Lock takes an exclusive lock on the working copy for the duration of a
// mutating command.
func (w *WorkCopy) Lock() error {
	f, err := os.OpenFile(filepath.Join(w.Root, MetaDir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("working copy is locked by another agit process")
	}
	w.lock = f
	return nil
}

// Unlock releases the lock.
func (w *WorkCopy) Unlock() {
	if w.lock != nil {
		syscall.Flock(int(w.lock.Fd()), syscall.LOCK_UN)
		w.lock.Close()
		w.lock = nil
	}
}

func readJSON(p string, v interface{}) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeFileAtomic(p string, data []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func writeJSON(p string, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, append(b, '\n'))
}

// Save persists config, state and dirstate.
func (w *WorkCopy) Save() error {
	meta := filepath.Join(w.Root, MetaDir)
	if err := writeJSON(filepath.Join(meta, "config.json"), &w.Config); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(meta, "state.json"), &w.State); err != nil {
		return err
	}
	return w.writeDirstate()
}

// Dirstate format: a header line "agit-dirstate 1 <stamp>" followed by one
// line per file: "<mode> <id> <size> <mtime> <ino> <path>".
func (w *WorkCopy) writeDirstate() error {
	paths := make([]string, 0, len(w.Files))
	for p := range w.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	w.stamp = time.Now().UnixNano()
	fmt.Fprintf(&b, "agit-dirstate 1 %d\n", w.stamp)
	for _, p := range paths {
		f := w.Files[p]
		fmt.Fprintf(&b, "%s %s %d %d %d %s\n", f.Mode, f.ID, f.Size, f.MTime, f.Ino, p)
	}
	return writeFileAtomic(filepath.Join(w.Root, MetaDir, "dirstate"), []byte(b.String()))
}

func (w *WorkCopy) readDirstate() error {
	f, err := os.Open(filepath.Join(w.Root, MetaDir, "dirstate"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	if sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) != 3 || parts[0] != "agit-dirstate" {
			return fmt.Errorf("corrupt dirstate header")
		}
		w.stamp, _ = strconv.ParseInt(parts[2], 10, 64)
	}
	for sc.Scan() {
		line := sc.Text()
		parts := strings.SplitN(line, " ", 6)
		if len(parts) != 6 {
			return fmt.Errorf("corrupt dirstate line %q", line)
		}
		mode, err := object.ParseMode(parts[0])
		if err != nil {
			return err
		}
		id, err := object.ParseID(parts[1])
		if err != nil {
			return err
		}
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		mtime, _ := strconv.ParseInt(parts[3], 10, 64)
		ino, _ := strconv.ParseUint(parts[4], 10, 64)
		w.Files[parts[5]] = FileState{Mode: mode, ID: id, Size: size, MTime: mtime, Ino: ino}
	}
	return sc.Err()
}

// Init creates the metadata directory for a new working copy.
func Init(root string, cfg Config) (*WorkCopy, error) {
	if _, err := os.Stat(filepath.Join(root, MetaDir)); err == nil {
		return nil, fmt.Errorf("%s is already a working copy", root)
	}
	if err := os.MkdirAll(filepath.Join(root, MetaDir), 0o755); err != nil {
		return nil, err
	}
	if cfg.Target == "" {
		cfg.Target = "main"
	}
	w := &WorkCopy{Root: root, Config: cfg, Files: map[string]FileState{}}
	c, err := client.New(cfg.Remote, "")
	if err != nil {
		return nil, err
	}
	w.Client = c
	w.Repo = c.Repo(cfg.Repo)
	return w, nil
}

// SetToken sets the bearer token used for requests.
func (w *WorkCopy) SetToken(token string) { w.Client.Token = token }

// InSparse reports whether a path is inside the sparse profile. Directories
// that are ancestors of a profile entry are also "in" (they must be walked).
func (w *WorkCopy) InSparse(p string, isDir bool) bool {
	if len(w.Config.Sparse) == 0 {
		return true
	}
	for _, s := range w.Config.Sparse {
		if object.HasPathPrefix(p, s) || isDir && object.HasPathPrefix(s, p) {
			return true
		}
	}
	return false
}

// Abs returns the filesystem path of a repository path.
func (w *WorkCopy) Abs(p string) string { return filepath.Join(w.Root, filepath.FromSlash(p)) }

// RepoPath converts a filesystem path (absolute or relative to cwd) into a
// repository path.
func (w *WorkCopy) RepoPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(w.Root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the working copy", p)
	}
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}
