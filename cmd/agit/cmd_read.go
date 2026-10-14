package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/textdiff"
)

func registerRead() {
	register("log", "[REV] [-n N] [--path P] [--follow] [--first-parent] [--oneline]", "show history (computed on the server)", cmdLog)
	register("show", "[REV] [--stat]", "show a changeset and its diff", cmdShow)
	register("cat", "[REV:]PATH", "print a file at a revision without checking it out", cmdCat)
	register("ls", "[REV] [PATH]", "list a directory at a revision", cmdLs)
	register("blame", "PATH [-r REV]", "show which changeset last touched each line", cmdBlame)
	register("search", "PATTERN [-r REV] [--prefix DIR]", "find files by path (glob or substring)", cmdSearch)
	register("refs", "[PREFIX]", "list refs", cmdRefs)
	register("ref", "set|delete NAME [REV] [--old REV]", "move or delete a ref (compare-and-swap)", cmdRef)
	register("merge-base", "A B", "print the best common ancestor(s)", cmdMergeBase)
	register("events", "[--follow] [--after N]", "stream the changeset log and ref updates", cmdEvents)
	register("info", "[--metrics]", "describe the endpoint (tier, upstream, cache statistics)", cmdInfo)
}

func cmdLog(ctx context.Context, args []string) error {
	fs := newFlags("log", "[REV]")
	n := fs.Int("n", 20, "maximum number of changesets")
	path := fs.String("path", "", "only changesets that touched this path")
	follow := fs.Bool("follow", false, "with --path: continue across renames")
	firstParent := fs.Bool("first-parent", false, "follow only first parents")
	oneline := fs.Bool("oneline", false, "one line per changeset")
	exclude := fs.String("exclude", "", "hide changesets reachable from this revision")
	asJSON := fs.Bool("json", false, "machine-readable output")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	rev := t.defaultRev()
	if len(pos) > 0 {
		rev = t.rev(pos[0])
	}
	p := *path
	if p != "" && t.wc != nil {
		if rp, err := t.wc.RepoPath(p); err == nil {
			p = rp
		}
	}
	opts := client.LogOptions{Rev: rev, Path: p, Limit: *n, Follow: *follow, FirstParent: *firstParent}
	if *exclude != "" {
		opts.Exclude = t.rev(*exclude)
	}
	entries, err := t.repo.Log(ctx, opts)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(entries)
	}
	labels := refLabels(ctx, t.repo)
	for _, e := range entries {
		printChangeset(e.ID, e.Changeset, labels[e.ID], *oneline)
	}
	return nil
}

func refLabels(ctx context.Context, r *client.Repo) map[object.ID]string {
	out := map[object.ID]string{}
	refs, err := r.Refs(ctx, "")
	if err != nil {
		return out
	}
	for _, ref := range refs {
		if out[ref.ID] != "" {
			out[ref.ID] += ", "
		}
		out[ref.ID] += ref.Name
	}
	return out
}

func printChangeset(id object.ID, cs *object.Changeset, label string, oneline bool) {
	if label != "" {
		label = " (" + label + ")"
	}
	if oneline {
		fmt.Printf("%s%s %s\n", short(id), label, cs.Summary())
		return
	}
	fmt.Printf("changeset %s%s\n", id, label)
	if len(cs.Parents) > 1 {
		var ps []string
		for _, p := range cs.Parents {
			ps = append(ps, short(p))
		}
		fmt.Printf("Merge:  %s\n", strings.Join(ps, " "))
	}
	fmt.Printf("Author: %s\nDate:   %s\n", cs.Author, formatTime(cs.AuthorTime, cs.AuthorTZ))
	if g := cs.Extra[object.ExtraGitCommit]; g != "" {
		fmt.Printf("Git:    %s\n", g)
	}
	if r := cs.Extra[object.ExtraRebasedFrom]; r != "" {
		fmt.Printf("Rebased-from: %s\n", r[:12])
	}
	fmt.Printf("\n%s\n\n", indent(cs.Message, "    "))
}

func cmdShow(ctx context.Context, args []string) error {
	fs := newFlags("show", "[REV]")
	stat := fs.Bool("stat", false, "summary only")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	rev := t.defaultRev()
	if len(pos) > 0 {
		rev = t.rev(pos[0])
	}
	id, err := t.repo.Resolve(ctx, rev)
	if err != nil {
		return err
	}
	cs, err := t.repo.GetChangeset(ctx, id)
	if err != nil {
		return err
	}
	printChangeset(id, cs, refLabels(ctx, t.repo)[id], false)
	d, err := t.repo.Diff(ctx, client.DiffOptions{To: id.String(), Patch: !*stat, Stat: *stat})
	if err != nil {
		return err
	}
	printDiff(d, *stat)
	return nil
}

// splitRevPath parses "REV:PATH" (or just "PATH").
func splitRevPath(t *target, s string) (string, string) {
	if i := strings.Index(s, ":"); i > 0 {
		return t.rev(s[:i]), object.CleanPath(s[i+1:])
	}
	p := s
	if t.wc != nil {
		if rp, err := t.wc.RepoPath(s); err == nil {
			p = rp
		}
	}
	return t.defaultRev(), object.CleanPath(p)
}

func cmdCat(ctx context.Context, args []string) error {
	fs := newFlags("cat", "[REV:]PATH")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("cat needs [REV:]PATH")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	rev, p := splitRevPath(t, pos[0])
	data, err := t.repo.File(ctx, rev, p)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func cmdLs(ctx context.Context, args []string) error {
	fs := newFlags("ls", "[REV] [PATH]")
	rf := addRepoFlags(fs)
	long := fs.Bool("l", false, "show modes and object IDs")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	rev, p := t.defaultRev(), ""
	switch len(pos) {
	case 1:
		if strings.Contains(pos[0], ":") {
			rev, p = splitRevPath(t, pos[0])
		} else {
			rev = t.rev(pos[0])
		}
	case 2:
		rev, p = t.rev(pos[0]), object.CleanPath(pos[1])
	}
	tr, err := t.repo.Tree(ctx, rev, p)
	if err != nil {
		return err
	}
	if !tr.Mode.IsDir() {
		fmt.Printf("%s %s %s\n", tr.Mode, short(tr.ID), tr.Path)
		return nil
	}
	for _, e := range tr.Entries {
		name := e.Name
		if e.Mode.IsDir() {
			name += "/"
		}
		if *long {
			fmt.Printf("%s %s %s\n", e.Mode, short(e.ID), name)
		} else {
			fmt.Println(name)
		}
	}
	return nil
}

func cmdBlame(ctx context.Context, args []string) error {
	fs := newFlags("blame", "PATH [-r REV]")
	rev := fs.String("r", "", "revision (default: the working copy base, or main)")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("blame needs a path")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	r, p := splitRevPath(t, pos[0])
	if *rev != "" {
		r = t.rev(*rev)
	}
	ranges, err := t.repo.Blame(ctx, r, p)
	if err != nil {
		return err
	}
	data, err := t.repo.File(ctx, r, p)
	if err != nil {
		return err
	}
	lines := textdiff.SplitLines(data)
	// Fetch the changesets involved to show authors and dates.
	ids := map[object.ID]bool{}
	for _, rg := range ranges {
		ids[rg.Changeset] = true
	}
	var list []object.ID
	for id := range ids {
		list = append(list, id)
	}
	css, err := t.repo.GetChangesets(ctx, list)
	if err != nil {
		return err
	}
	for _, rg := range ranges {
		cs := css[rg.Changeset]
		who, when := "?", ""
		if cs != nil {
			who = cs.Author
			if i := strings.IndexByte(who, '<'); i > 0 {
				who = strings.TrimSpace(who[:i])
			}
			when = time.Unix(cs.AuthorTime, 0).UTC().Format("2006-01-02")
		}
		for k := 0; k < rg.Count; k++ {
			ln := rg.Start + k
			text := ""
			if ln < len(lines) {
				text = strings.TrimRight(lines[ln], "\n")
			}
			fmt.Printf("%s (%-12.12s %s %4d) %s\n", short(rg.Changeset), who, when, ln+1, text)
		}
	}
	return nil
}

func cmdSearch(ctx context.Context, args []string) error {
	fs := newFlags("search", "PATTERN")
	rev := fs.String("r", "", "revision")
	prefix := fs.String("prefix", "", "only search under this directory")
	limit := fs.Int("limit", 1000, "maximum results")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return flag2("search needs a pattern")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	res, err := t.repo.Search(ctx, t.rev(*rev), object.CleanPath(*prefix), pos[0], *limit)
	if err != nil {
		return err
	}
	for _, m := range res {
		fmt.Println(m.Path)
	}
	return nil
}

func cmdRefs(ctx context.Context, args []string) error {
	fs := newFlags("refs", "[PREFIX]")
	asJSON := fs.Bool("json", false, "machine-readable output")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	prefix := ""
	if len(pos) > 0 {
		prefix = pos[0]
	}
	refs, err := t.repo.Refs(ctx, prefix)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(refs)
	}
	for _, r := range refs {
		fmt.Printf("%s %s\n", short(r.ID), r.Name)
	}
	return nil
}

func cmdRef(ctx context.Context, args []string) error {
	fs := newFlags("ref", "set|delete NAME [REV]")
	old := fs.String("old", "", "expected current value (default: whatever it is now)")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return flag2("usage: agit ref set NAME REV | agit ref delete NAME")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	name := pos[1]
	var cur object.ID
	if *old != "" {
		if cur, err = t.repo.Resolve(ctx, t.rev(*old)); err != nil {
			return err
		}
	} else if cur, err = t.repo.Resolve(ctx, name); err != nil && !client.IsNotFound(err) {
		return err
	}
	switch pos[0] {
	case "set":
		if len(pos) != 3 {
			return flag2("usage: agit ref set NAME REV")
		}
		id, err := t.repo.Resolve(ctx, t.rev(pos[2]))
		if err != nil {
			return err
		}
		if err := t.repo.UpdateRef(ctx, name, cur, id); err != nil {
			return err
		}
		fmt.Printf("%s: %s -> %s\n", name, short(cur), short(id))
	case "delete":
		if err := t.repo.UpdateRef(ctx, name, cur, object.Zero); err != nil {
			return err
		}
		fmt.Printf("deleted %s (was %s)\n", name, short(cur))
	default:
		return flag2("ref: unknown operation " + pos[0])
	}
	return nil
}

func cmdMergeBase(ctx context.Context, args []string) error {
	fs := newFlags("merge-base", "A B")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return flag2("merge-base needs two revisions")
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	bases, err := t.repo.MergeBases(ctx, t.rev(pos[0]), t.rev(pos[1]))
	if err != nil {
		return err
	}
	if len(bases) == 0 {
		return exitError{1, "no common ancestor"}
	}
	for _, b := range bases {
		fmt.Println(b)
	}
	return nil
}

func cmdEvents(ctx context.Context, args []string) error {
	fs := newFlags("events", "[--follow]")
	follow := fs.Bool("follow", false, "keep polling for new events")
	after := fs.Int64("after", 0, "start after this changeset log position")
	interval := fs.Duration("interval", time.Second, "poll interval with --follow")
	rf := addRepoFlags(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	walPos, refPos := *after, int64(0)
	for {
		wal, err := t.repo.WAL(ctx, walPos, 1000)
		if err != nil {
			return err
		}
		for _, e := range wal {
			fmt.Printf("changeset #%d %s %s\n", e.Seq, short(e.ID), time.Unix(0, e.Time).UTC().Format(time.RFC3339))
			walPos = e.Seq
		}
		refs, err := t.repo.RefLog(ctx, refPos, 1000)
		if err != nil {
			return err
		}
		for _, u := range refs {
			fmt.Printf("ref #%d %s %s -> %s by %s\n", u.Seq, u.Name, short(u.Old), short(u.New), u.Who)
			refPos = u.Seq
		}
		if !*follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*interval):
		}
	}
}

func cmdInfo(ctx context.Context, args []string) error {
	fs := newFlags("info", "")
	metrics := fs.Bool("metrics", false, "include metrics")
	rf := addRepoFlags(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	var c *client.Client
	if t, err := rf.open(); err == nil {
		c = t.repo.Client()
		if t.wc != nil {
			fmt.Printf("working copy: %s\nrepository:   %s\nbase:         %s\ntarget:       %s\n", t.wc.Root, t.wc.Config.Repo, t.wc.State.Base, t.wc.Config.Target)
			if len(t.wc.Config.Sparse) > 0 {
				fmt.Printf("sparse:       %s\n", strings.Join(t.wc.Config.Sparse, ", "))
			}
		}
	} else {
		cc, err := newClient(defaultRemote(*rf.server))
		if err != nil {
			return err
		}
		c = cc
	}
	info, err := c.Info(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("endpoint:     %s (%s tier, %s)\n", c.Address(), info.Tier, info.Version)
	if info.Upstream != "" {
		fmt.Printf("upstream:     %s\n", info.Upstream)
	}
	if *metrics {
		m, err := c.Metrics(ctx)
		if err != nil {
			return err
		}
		return printJSON(m)
	}
	return nil
}
