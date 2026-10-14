package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/fetch"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/textdiff"
	"github.com/mattparavel/a-git/internal/workcopy"
)

func registerAll() {
	register("clone", "REPO [DIR] [--rev REV] [--sparse PATHS] [--server ADDR]", "create a (sparse) working copy", cmdClone)
	register("status", "[PATHS...] [--json]", "show local changes", cmdStatus)
	register("diff", "[PATHS...] [-r REV [-r REV]] [--stat]", "show changes (local, or between revisions on the server)", cmdDiff)
	register("commit", "-m MSG [PATHS...]", "record local changes as a changeset on the server", cmdCommit)
	register("update", "[REV]", "move the working copy to a revision (local edits are merged)", cmdUpdate)
	register("sync", "", "rebase local changesets onto the latest target and update", cmdSync)
	register("land", "[--onto REF] [--review ID]", "land the local stack onto the target ref", cmdLand)
	register("stack", "", "list local changesets that are not on the target yet", cmdStack)
	register("rebase", "[--onto REV]", "rebase the local stack on the server", cmdRebase)
	register("merge", "REV [-m MSG]", "merge a revision into the working copy's changeset (on the server)", cmdMerge)
	register("revert", "REV [--onto REF] [--land]", "create a changeset that undoes REV", cmdRevert)
	register("sparse", "list|add|remove|set [PATHS...]", "change which paths are materialized", cmdSparse)
	registerRead()
	registerReview()
	registerTools()
}

func cmdClone(ctx context.Context, args []string) error {
	fs := newFlags("clone", "REPO [DIR]")
	rev := fs.String("rev", "main", "revision to check out")
	sparse := fs.String("sparse", "", "comma-separated paths to materialize (default: everything)")
	server := fs.String("server", "", "address of the daemon, cache or server")
	targetRef := fs.String("target", "main", "ref that 'land' and 'sync' use")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 || len(pos) > 2 {
		fs.Usage()
		return flag2("clone needs REPO [DIR]")
	}
	repoName := pos[0]
	dir := repoName
	if len(pos) == 2 {
		dir = pos[1]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if ents, err := os.ReadDir(abs); err == nil && len(ents) > 0 {
		return fmt.Errorf("%s exists and is not empty", dir)
	}
	cfg := workcopy.Config{Repo: repoName, Remote: defaultRemote(*server), Target: *targetRef}
	for _, p := range strings.Split(*sparse, ",") {
		if p = object.CleanPath(p); p != "" {
			cfg.Sparse = append(cfg.Sparse, p)
		}
	}
	start := time.Now()
	wc, err := workcopy.Init(abs, cfg)
	if err != nil {
		return err
	}
	wc.SetToken(token())
	wc.Client.User = userName()
	cs, err := wc.Repo.Resolve(ctx, *rev)
	if err != nil {
		os.RemoveAll(filepath.Join(abs, workcopy.MetaDir))
		return err
	}
	if err := wc.Checkout(ctx, cs, progress); err != nil {
		return err
	}
	fmt.Printf("checked out %s at %s (%d files) in %v\n", repoName, short(cs), len(wc.Files), time.Since(start).Round(time.Millisecond))
	return nil
}

func flag2(msg string) error { return exitError{2, msg} }

func cmdStatus(ctx context.Context, args []string) error {
	fs := newFlags("status", "[PATHS...]")
	asJSON := fs.Bool("json", false, "machine-readable output")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	paths, err := repoPaths(wc, pos)
	if err != nil {
		return err
	}
	st, err := wc.Status(ctx, paths)
	if err != nil {
		return err
	}
	if *asJSON {
		if st == nil {
			st = []workcopy.FileStatus{}
		}
		return printJSON(map[string]interface{}{"base": wc.State.Base, "sparse": wc.Config.Sparse, "changes": st})
	}
	if len(st) == 0 && len(pos) == 0 {
		fmt.Printf("on %s, no local changes\n", short(wc.State.Base))
		return nil
	}
	for _, s := range st {
		fmt.Printf("%s %s\n", s.Status, s.Path)
	}
	return nil
}

func repoPaths(wc *workcopy.WorkCopy, args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		p, err := wc.RepoPath(a)
		if err != nil {
			return nil, err
		}
		if p == "" {
			return nil, nil // the root: everything
		}
		out = append(out, p)
	}
	return out, nil
}

func cmdDiff(ctx context.Context, args []string) error {
	fs := newFlags("diff", "[PATHS...] [-r REV [-r REV]]")
	var revs multiFlag
	fs.Var(&revs, "r", "revision (once: compare with its first parent... or with the working copy base; twice: compare the two)")
	stat := fs.Bool("stat", false, "show a summary instead of patches")
	ctxLines := fs.Int("U", 3, "lines of context")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(revs) > 0 || *rf.repo != "" {
		t, err := rf.open()
		if err != nil {
			return err
		}
		opts := client.DiffOptions{Patch: !*stat, Stat: *stat, Context: *ctxLines}
		switch len(revs) {
		case 0:
			opts.To = t.defaultRev()
		case 1:
			opts.To = t.rev(revs[0])
		default:
			opts.From, opts.To = t.rev(revs[0]), t.rev(revs[1])
		}
		for _, p := range pos {
			if t.wc != nil {
				if rp, err := t.wc.RepoPath(p); err == nil {
					p = rp
				}
			}
			opts.Paths = append(opts.Paths, p)
		}
		d, err := t.repo.Diff(ctx, opts)
		if err != nil {
			return err
		}
		printDiff(d, *stat)
		return nil
	}
	// Local changes against the base.
	wc, err := openWC()
	if err != nil {
		return err
	}
	paths, err := repoPaths(wc, pos)
	if err != nil {
		return err
	}
	st, err := wc.Status(ctx, paths)
	if err != nil {
		return err
	}
	f := &fetch.Fetcher{Src: wc.Repo, CS: wc.State.Base}
	var wants []fetch.Want
	for _, s := range st {
		if old, ok := wc.Files[s.Path]; ok {
			wants = append(wants, fetch.Want{ID: old.ID, Path: s.Path})
		}
	}
	oldContent := map[object.ID][]byte{}
	if err := f.Get(ctx, wants, func(w fetch.Want, _ object.Kind, raw []byte) error {
		oldContent[w.ID] = raw
		return nil
	}); err != nil {
		return err
	}
	for _, s := range st {
		var a, b []byte
		aName, bName := "a/"+s.Path, "b/"+s.Path
		if old, ok := wc.Files[s.Path]; ok {
			a = oldContent[old.ID]
		} else {
			aName = "/dev/null"
		}
		if s.Status != "D" {
			if b, err = os.ReadFile(wc.Abs(s.Path)); err != nil {
				if target, lerr := os.Readlink(wc.Abs(s.Path)); lerr == nil {
					b = []byte(target)
				} else {
					return err
				}
			}
		} else {
			bName = "/dev/null"
		}
		al, bl := textdiff.SplitLines(a), textdiff.SplitLines(b)
		if *stat {
			add, del := textdiff.Stats(textdiff.Lines(al, bl))
			fmt.Printf(" %s %s | +%d -%d\n", s.Status, s.Path, add, del)
			continue
		}
		fmt.Printf("diff --agit a/%s b/%s\n", s.Path, s.Path)
		if textdiff.IsBinary(a) || textdiff.IsBinary(b) {
			fmt.Printf("Binary files %s and %s differ\n", aName, bName)
			continue
		}
		fmt.Print(textdiff.Unified(aName, bName, al, bl, *ctxLines))
	}
	return nil
}

func printDiff(d *api.DiffResponse, stat bool) {
	for _, f := range d.Files {
		if stat {
			if f.Binary {
				fmt.Printf(" %-8s %s | binary\n", f.Status, f.Path)
			} else {
				fmt.Printf(" %-8s %s | +%d -%d\n", f.Status, f.Path, f.Added, f.Deleted)
			}
			continue
		}
		fmt.Printf("diff --agit a/%s b/%s\n", f.Path, f.Path)
		if f.OldMode != f.NewMode && f.OldMode != 0 && f.NewMode != 0 {
			fmt.Printf("old mode %s\nnew mode %s\n", f.OldMode, f.NewMode)
		}
		if f.Binary {
			fmt.Printf("Binary files differ\n")
			continue
		}
		fmt.Print(f.Patch)
	}
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func cmdCommit(ctx context.Context, args []string) error {
	fs := newFlags("commit", "-m MSG [PATHS...]")
	msg := fs.String("m", "", "commit message")
	author := fs.String("author", "", "author (default $AGIT_AUTHOR or user@host)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	paths, err := repoPaths(wc, pos)
	if err != nil {
		return err
	}
	start := time.Now()
	id, st, err := wc.Commit(ctx, workcopy.CommitOptions{Message: *msg, Author: *author, Paths: paths})
	if errors.Is(err, workcopy.ErrNothingToCommit) {
		return exitError{1, "nothing to commit"}
	}
	if err != nil {
		return err
	}
	fmt.Printf("created changeset %s (%d file(s)) in %v\n", short(id), len(st), time.Since(start).Round(time.Millisecond))
	return nil
}

func printUpdate(res *workcopy.UpdateResult) {
	if res.Updated+res.Removed > 0 {
		fmt.Printf("%d file(s) updated, %d removed\n", res.Updated, res.Removed)
	}
	for _, p := range res.Merged {
		fmt.Printf("merged local changes: %s\n", p)
	}
	for _, p := range res.Conflicts {
		fmt.Printf("CONFLICT: %s\n", p)
	}
}

func cmdUpdate(ctx context.Context, args []string) error {
	fs := newFlags("update", "[REV]")
	force := fs.Bool("force", false, "discard local edits to files that change")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	rev := wc.Config.Target
	if len(pos) > 0 {
		rev = pos[0]
	}
	id, err := wc.Repo.Resolve(ctx, rev)
	if err != nil {
		return err
	}
	res, err := wc.Update(ctx, id, *force)
	if err != nil {
		return err
	}
	fmt.Printf("now at %s\n", short(id))
	printUpdate(res)
	if len(res.Conflicts) > 0 {
		return exitError{1, ""}
	}
	return nil
}

func cmdSync(ctx context.Context, args []string) error {
	fs := newFlags("sync", "")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	tip, err := wc.Repo.Resolve(ctx, wc.Config.Target)
	if err != nil {
		return err
	}
	dest := tip
	if ok, err := wc.Repo.IsAncestor(ctx, wc.State.Base.String(), tip.String()); err != nil {
		return err
	} else if !ok {
		rb, err := wc.Repo.Rebase(ctx, api.RebaseRequest{Head: wc.State.Base.String(), Onto: tip.String()})
		if err != nil {
			return err
		}
		dest = rb.Head
		fmt.Printf("rebased %d local changeset(s) onto %s\n", len(rb.Mapping), short(tip))
	}
	res, err := wc.Update(ctx, dest, false)
	if err != nil {
		return err
	}
	fmt.Printf("now at %s\n", short(dest))
	printUpdate(res)
	return nil
}

func cmdLand(ctx context.Context, args []string) error {
	fs := newFlags("land", "[--onto REF] [--review ID]")
	onto := fs.String("onto", "", "ref to land on (default: the working copy's target)")
	reviewID := fs.String("review", "", "land this approved review instead of the local stack")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	if st, err := wc.Status(ctx, nil); err != nil {
		return err
	} else if len(st) > 0 {
		fmt.Fprintf(os.Stderr, "note: %d uncommitted change(s) are not included\n", len(st))
	}
	ref := *onto
	if ref == "" {
		ref = wc.Config.Target
	}
	start := time.Now()
	var newHead object.ID
	if *reviewID != "" {
		res, err := wc.Repo.LandReview(ctx, *reviewID)
		if err != nil {
			return err
		}
		newHead = res.Land.New
		fmt.Printf("landed review %d on %s: %s -> %s (%d attempt(s))\n", res.Review.ID, res.Land.Ref, short(res.Land.Old), short(res.Land.New), res.Land.Attempts)
	} else {
		res, err := wc.Repo.Land(ctx, api.LandRequest{Head: wc.State.Base.String(), Ref: ref})
		if client.IsCode(err, "nothing_to_land") {
			return exitError{1, "nothing to land: the stack is already on " + ref}
		}
		if err != nil {
			return err
		}
		newHead = res.New
		how := "fast-forward"
		if res.Rebase != nil {
			how = fmt.Sprintf("rebased %d changeset(s)", len(res.Rebase.Mapping))
		}
		fmt.Printf("landed on %s: %s -> %s (%s, %d attempt(s), %v)\n", ref, short(res.Old), short(res.New), how, res.Attempts, time.Since(start).Round(time.Millisecond))
	}
	upd, err := wc.Update(ctx, newHead, false)
	if err != nil {
		return err
	}
	printUpdate(upd)
	return nil
}

func cmdStack(ctx context.Context, args []string) error {
	fs := newFlags("stack", "")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	entries, err := wc.Repo.Log(ctx, client.LogOptions{Rev: wc.State.Base.String(), Exclude: wc.Config.Target, Limit: 1000})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Printf("no local changesets; %s is on %s\n", short(wc.State.Base), wc.Config.Target)
		return nil
	}
	for _, e := range entries {
		fmt.Printf("%s %s\n", short(e.ID), e.Changeset.Summary())
	}
	return nil
}

func cmdRebase(ctx context.Context, args []string) error {
	fs := newFlags("rebase", "[--onto REV]")
	onto := fs.String("onto", "", "revision to rebase onto (default: the target ref)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	if *onto == "" {
		*onto = wc.Config.Target
	}
	rb, err := wc.Repo.Rebase(ctx, api.RebaseRequest{Head: wc.State.Base.String(), Onto: *onto, Base: wc.Config.Target})
	if err != nil {
		return err
	}
	for old, nw := range rb.Mapping {
		if nw.IsZero() {
			fmt.Printf("%s dropped (already applied)\n", old[:12])
		} else {
			fmt.Printf("%s -> %s\n", old[:12], short(nw))
		}
	}
	res, err := wc.Update(ctx, rb.Head, false)
	if err != nil {
		return err
	}
	fmt.Printf("now at %s\n", short(rb.Head))
	printUpdate(res)
	return nil
}

func cmdMerge(ctx context.Context, args []string) error {
	fs := newFlags("merge", "REV [-m MSG]")
	msg := fs.String("m", "", "merge message")
	allow := fs.Bool("allow-conflicts", false, "record conflict markers instead of failing")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("merge needs exactly one revision")
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	res, err := wc.Repo.Merge(ctx, api.MergeRequest{Ours: wc.State.Base.String(), Theirs: pos[0], Message: *msg, AllowConflicts: *allow, Author: workcopy.DefaultAuthor()})
	if err != nil {
		return err
	}
	switch {
	case res.ID == wc.State.Base:
		fmt.Println("already up to date")
		return nil
	case res.FastForward:
		fmt.Printf("fast-forward to %s\n", short(res.ID))
	default:
		fmt.Printf("created merge %s (base %s)\n", short(res.ID), short(res.Base))
		for _, c := range res.Conflicts {
			fmt.Printf("CONFLICT (%s): %s\n", c.Kind, c.Path)
		}
	}
	upd, err := wc.Update(ctx, res.ID, false)
	if err != nil {
		return err
	}
	printUpdate(upd)
	return nil
}

func cmdRevert(ctx context.Context, args []string) error {
	fs := newFlags("revert", "REV [--onto REF] [--land]")
	onto := fs.String("onto", "", "revision to apply the revert on (default: the working copy base, or main)")
	land := fs.Bool("land", false, "land the revert on the target ref immediately")
	msg := fs.String("m", "", "message")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("revert needs exactly one revision")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	base := *onto
	if base == "" {
		base = t.defaultRev()
	}
	id, err := t.repo.Revert(ctx, api.RevertRequest{Changeset: t.rev(pos[0]), Onto: t.rev(base), Message: *msg, Author: workcopy.DefaultAuthor()})
	if err != nil {
		return err
	}
	fmt.Printf("created revert %s\n", short(id))
	ref := "main"
	if t.wc != nil {
		ref = t.wc.Config.Target
	}
	if *land {
		res, err := t.repo.Land(ctx, api.LandRequest{Head: id.String(), Ref: ref})
		if err != nil {
			return err
		}
		fmt.Printf("landed on %s: %s\n", ref, short(res.New))
		id = res.New
	}
	if t.wc != nil {
		if err := t.wc.Lock(); err != nil {
			return err
		}
		defer t.wc.Unlock()
		upd, err := t.wc.Update(ctx, id, false)
		if err != nil {
			return err
		}
		printUpdate(upd)
	}
	return nil
}

func cmdSparse(ctx context.Context, args []string) error {
	fs := newFlags("sparse", "list|add|remove|set [PATHS...]")
	force := fs.Bool("force", false, "remove locally modified files too")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		pos = []string{"list"}
	}
	wc, err := openWC()
	if err != nil {
		return err
	}
	op, paths := pos[0], pos[1:]
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		clean = append(clean, object.CleanPath(p))
	}
	cur := wc.Config.Sparse
	var next []string
	switch op {
	case "list":
		if len(cur) == 0 {
			fmt.Println("(everything)")
		}
		for _, p := range cur {
			fmt.Println(p)
		}
		return nil
	case "add":
		if len(cur) == 0 {
			fmt.Println("already materializing everything")
			return nil
		}
		next = append(append([]string(nil), cur...), clean...)
	case "remove":
		drop := map[string]bool{}
		for _, p := range clean {
			drop[p] = true
		}
		for _, p := range cur {
			if !drop[p] {
				next = append(next, p)
			}
		}
		if len(next) == 0 {
			return fmt.Errorf("refusing to remove every path from the sparse profile (use 'sparse set' with the paths you want)")
		}
	case "set":
		next = clean
	default:
		return flag2("sparse: unknown operation " + op)
	}
	if err := wc.Lock(); err != nil {
		return err
	}
	defer wc.Unlock()
	added, removed, err := wc.SetSparse(ctx, next, *force)
	if err != nil {
		return err
	}
	fmt.Printf("materialized %d file(s), removed %d\n", added, removed)
	return nil
}
