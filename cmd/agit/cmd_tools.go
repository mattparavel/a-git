package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/gitbridge"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/tier"
)

func registerTools() {
	register("repos", "[--server ADDR]", "list repositories", cmdRepos)
	register("init-repo", "NAME [--server ADDR]", "create a repository on the server", cmdInitRepo)
	register("import-git", "GIT_DIR [--refs main,dev] [--state DIR]", "ingest git history (incrementally with --state)", cmdImportGit)
	register("export-git", "GIT_DIR [--rev main] [--ref refs/heads/main]", "write history to a git repository (mirror)", cmdExportGit)
	register("snapshot", "--out FILE [--rev main] [--prefix DIR]", "write a snapshot pack for seeding agitd", cmdSnapshot)
	register("bisect", "--good REV --bad REV [--] CMD...", "find the first bad changeset by running CMD", cmdBisect)
}

func cmdRepos(ctx context.Context, args []string) error {
	fs := newFlags("repos", "")
	server := fs.String("server", "", "address")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	c, err := newClient(defaultRemote(*server))
	if err != nil {
		return err
	}
	repos, err := c.ListRepos(ctx)
	if err != nil {
		return err
	}
	for _, r := range repos {
		fmt.Println(r)
	}
	return nil
}

func cmdInitRepo(ctx context.Context, args []string) error {
	fs := newFlags("init-repo", "NAME")
	server := fs.String("server", "", "address")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("init-repo needs a name")
	}
	c, err := newClient(defaultRemote(*server))
	if err != nil {
		return err
	}
	if err := c.CreateRepo(ctx, pos[0]); err != nil {
		return err
	}
	fmt.Printf("created repository %s on %s\n", pos[0], c.Address())
	return nil
}

func cmdImportGit(ctx context.Context, args []string) error {
	fs := newFlags("import-git", "GIT_DIR")
	refs := fs.String("refs", "HEAD", "comma-separated git refs to import")
	state := fs.String("state", "", "directory for incremental-import state")
	create := fs.Bool("create", false, "create the repository if it does not exist")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("import-git needs a git directory")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	if *create {
		if err := t.repo.Client().CreateRepo(ctx, t.repo.Name()); err != nil && !client.IsCode(err, "exists") {
			return err
		}
	}
	var refList []string
	for _, r := range strings.Split(*refs, ",") {
		if r = strings.TrimSpace(r); r != "" {
			refList = append(refList, r)
		}
	}
	// "HEAD" alone would import under no branch name; map it to main.
	refMap := gitbridge.DefaultRefMap
	if len(refList) == 1 && refList[0] == "HEAD" {
		out, err := exec.Command("git", "-C", pos[0], "symbolic-ref", "-q", "HEAD").Output()
		if err == nil {
			refList = []string{strings.TrimSpace(string(out))}
		}
	}
	start := time.Now()
	stats, err := gitbridge.Import(ctx, t.repo, gitbridge.ImportOptions{
		GitDir: pos[0], Refs: refList, StateDir: *state, RefMap: refMap,
		Progress: progress, Warn: func(f string, a ...interface{}) { fmt.Fprintf(os.Stderr, "warning: "+f+"\n", a...) },
	})
	if err != nil {
		return err
	}
	fmt.Printf("imported %d commit(s) and %d blob(s) in %v\n", stats.Commits, stats.Blobs, time.Since(start).Round(time.Millisecond))
	for name, id := range stats.Refs {
		fmt.Printf("  %s -> %s\n", name, id[:12])
	}
	return nil
}

func cmdExportGit(ctx context.Context, args []string) error {
	fs := newFlags("export-git", "GIT_DIR")
	rev := fs.String("rev", "main", "revision to export")
	ref := fs.String("ref", "refs/heads/main", "git ref to write")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("export-git needs a git directory")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	start := time.Now()
	stats, err := gitbridge.Export(ctx, t.repo, gitbridge.ExportOptions{GitDir: pos[0], Rev: t.rev(*rev), GitRef: *ref, Progress: progress})
	if err != nil {
		return err
	}
	fmt.Printf("exported %d changeset(s) to %s; %s is %s (%v)\n", stats.Commits, pos[0], *ref, stats.Head, time.Since(start).Round(time.Millisecond))
	return nil
}

func cmdSnapshot(ctx context.Context, args []string) error {
	fs := newFlags("snapshot", "--out FILE")
	out := fs.String("out", "", "output file")
	rev := fs.String("rev", "main", "revision")
	prefix := fs.String("prefix", "", "only objects under this directory")
	rf := addRepoFlags(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *out == "" {
		return flag2("snapshot needs --out FILE")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if err := tier.WritePackHeader(w); err != nil {
		return err
	}
	n := 0
	cs, err := t.repo.Snapshot(ctx, t.rev(*rev), object.CleanPath(*prefix), func(id object.ID, stored []byte) error {
		n++
		return api.WriteFrame(w, id, stored)
	})
	if err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %d objects for %s to %s\n", n, short(cs), *out)
	return nil
}

func cmdBisect(ctx context.Context, args []string) error {
	fs := newFlags("bisect", "--good REV --bad REV -- CMD...")
	good := fs.String("good", "", "a known-good revision")
	bad := fs.String("bad", "", "a known-bad revision (default: the working copy base)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *good == "" || len(pos) == 0 {
		return flag2("usage: agit bisect --good REV [--bad REV] -- CMD...")
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if st, err := wc.Status(ctx, nil); err != nil {
		return err
	} else if len(st) > 0 {
		return fmt.Errorf("bisect needs a clean working copy (%d local change(s))", len(st))
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	start := wc.State.Base
	badRev := *bad
	if badRev == "" {
		badRev = start.String()
	}
	// Candidates: first-parent changesets in good..bad, oldest first. The
	// server computes the range; only the working copy is moved locally.
	entries, err := wc.Repo.Log(ctx, client.LogOptions{Rev: badRev, Exclude: *good, FirstParent: true, Limit: 10000})
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no changesets between %s and %s", *good, badRev)
	}
	cands := make([]object.ID, len(entries))
	for i, e := range entries {
		cands[len(entries)-1-i] = e.ID
	}
	// Invariant: cands[hi] is bad; everything before lo is good.
	lo, hi := 0, len(cands)-1
	skipped := map[int]bool{}
	steps := 0
	for lo < hi {
		mid := lo + (hi-lo)/2
		for skipped[mid] && mid < hi {
			mid++
		}
		if mid == hi {
			break
		}
		steps++
		if _, err := wc.Update(ctx, cands[mid], true); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, pos[0], pos[1:]...)
		cmd.Dir = wc.Root
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if ee, ok := runErr.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				return runErr
			}
		}
		verdict := "good"
		switch {
		case code == 125:
			verdict = "skip"
			skipped[mid] = true
		case code != 0:
			verdict = "bad"
			hi = mid
		default:
			lo = mid + 1
		}
		fmt.Printf("step %d: %s is %s (%d candidate(s) left)\n", steps, short(cands[mid]), verdict, hi-lo+1)
	}
	first := cands[hi]
	cs, err := wc.Repo.GetChangeset(ctx, first)
	if err != nil {
		return err
	}
	fmt.Printf("\nfirst bad changeset: %s\n", first)
	printChangeset(first, cs, "", false)
	if rv, err := wc.Repo.FindReview(ctx, first.String()); err == nil {
		fmt.Printf("landed by review R%d: %s\n", rv.Review.ID, rv.Review.Title)
	}
	if _, err := wc.Update(ctx, start, true); err != nil {
		return err
	}
	fmt.Printf("working copy restored to %s\n", short(start))
	return nil
}
