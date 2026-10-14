package gitbridge

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/store"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// Isolate from the user's git configuration (e.g. commit signing, which
	// fast-export does not preserve).
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com",
		"GIT_COMMITTER_NAME=Bob", "GIT_COMMITTER_EMAIL=bob@example.com",
		"GIT_AUTHOR_DATE=2024-01-02T03:04:05+0530", "GIT_COMMITTER_DATE=2024-01-03T03:04:05-0700")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, p)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeGitRepo(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "README.md", "# demo\n")
	write(t, dir, "src/a.go", "package src\n")
	write(t, dir, "weird name\twith tab.txt", "tabs\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "initial commit\n\nwith a body")
	// Executable bit and symlink.
	write(t, dir, "run.sh", "#!/bin/sh\necho hi\n")
	os.Chmod(filepath.Join(dir, "run.sh"), 0o755)
	os.Symlink("src/a.go", filepath.Join(dir, "link"))
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "exec and symlink")
	// A branch that is merged back.
	git(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "feature.txt", "feature\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "feature work")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "src/b.go", "package src\n\nvar B = 1\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "main work")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	// Rename, delete, and a file replaced by a directory.
	os.MkdirAll(filepath.Join(dir, "pkg"), 0o755)
	git(t, dir, "mv", "src/b.go", "pkg/b.go")
	git(t, dir, "rm", "-q", "README.md")
	git(t, dir, "commit", "-q", "-m", "move and delete")
	git(t, dir, "rm", "-q", "feature.txt")
	write(t, dir, "feature.txt/inner.txt", "now a dir\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "file becomes directory")
	git(t, dir, "tag", "v1")
	return dir
}

func TestRoundTrip(t *testing.T) {
	src := makeGitRepo(t)
	srv := httptest.NewServer(api.NewServer(api.ServerConfig{Store: store.NewMem()}).Handler())
	defer srv.Close()
	c, _ := client.New(srv.URL, "")
	ctx := context.Background()
	c.CreateRepo(ctx, "demo")
	r := c.Repo("demo")

	state := t.TempDir()
	stats, err := Import(ctx, r, ImportOptions{GitDir: src, Refs: []string{"main", "feature", "v1"}, StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Commits != 7 {
		t.Fatalf("imported %d commits", stats.Commits)
	}
	if stats.Refs["main"] == "" || stats.Refs["feature"] == "" || stats.Refs["tags/v1"] == "" {
		t.Fatalf("refs: %v", stats.Refs)
	}
	// The imported tree matches git's.
	if b, err := r.File(ctx, "main", "pkg/b.go"); err != nil || string(b) != "package src\n\nvar B = 1\n" {
		t.Fatalf("pkg/b.go: %q %v", b, err)
	}
	tree, _ := r.Tree(ctx, "main", "run.sh")
	if tree.Mode.String() != "100755" {
		t.Fatalf("run.sh mode %s", tree.Mode)
	}
	// Rename detection works on imported history.
	hist, err := r.History(ctx, "main", "pkg/b.go", 10, true)
	if err != nil || len(hist) != 2 {
		t.Fatalf("history with follow: %+v %v", hist, err)
	}

	// Incremental import picks up only new commits.
	write(t, src, "late.txt", "late\n")
	git(t, src, "add", "-A")
	git(t, src, "commit", "-q", "-m", "late commit")
	stats, err = Import(ctx, r, ImportOptions{GitDir: src, Refs: []string{"main"}, StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Commits != 1 {
		t.Fatalf("incremental import: %d commits", stats.Commits)
	}

	// Export to a fresh git repository and compare commit hashes.
	dst := filepath.Join(t.TempDir(), "mirror")
	ex, err := Export(ctx, r, ExportOptions{GitDir: dst, Rev: "main"})
	if err != nil {
		t.Fatal(err)
	}
	want := git(t, src, "rev-parse", "main")
	if ex.Head != want {
		t.Logf("src log:\n%s", git(t, src, "log", "--format=%H %T %P %s", "main"))
		t.Logf("dst log:\n%s", git(t, dst, "log", "--format=%H %T %P %s", "main"))
		srcRoot := git(t, src, "rev-list", "--max-parents=0", "main")
		dstRoot := git(t, dst, "rev-list", "--max-parents=0", "main")
		t.Logf("src root:\n%s", git(t, src, "cat-file", "-p", srcRoot))
		t.Logf("dst root:\n%s", git(t, dst, "cat-file", "-p", dstRoot))
		t.Fatalf("exported head %s, want %s (history not byte-identical)", ex.Head, want)
	}
	if got := git(t, dst, "rev-parse", "refs/heads/main"); got != want {
		t.Fatalf("mirror main = %s, want %s", got, want)
	}
	if out := git(t, dst, "fsck", "--no-progress"); strings.Contains(out, "error") {
		t.Fatalf("fsck: %s", out)
	}
	// A second export is a no-op; a new changeset is appended.
	ex, err = Export(ctx, r, ExportOptions{GitDir: dst, Rev: "main"})
	if err != nil || ex.Commits != 0 {
		t.Fatalf("re-export: %+v %v", ex, err)
	}
}
