// Command agit is the a-git command-line client.
//
// Most commands are thin wrappers over server APIs: history, diffs, blame,
// merges, rebases and lands all run on the server, so they work with or
// without a working copy. Commands that touch files (clone, status, commit,
// update, sparse) operate on a lightweight working copy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/paths"
	"github.com/mattparavel/a-git/internal/workcopy"
)

type command struct {
	name    string
	args    string
	summary string
	run     func(ctx context.Context, args []string) error
}

var commands []command

func register(name, args, summary string, run func(ctx context.Context, args []string) error) {
	commands = append(commands, command{name, args, summary, run})
}

func main() {
	registerAll()
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		usage()
		if len(os.Args) < 2 {
			os.Exit(2)
		}
		return
	}
	name := os.Args[1]
	for _, c := range commands {
		if c.name == name {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			err := c.run(ctx, os.Args[2:])
			stop()
			if err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return
				}
				var ee exitError
				if errors.As(err, &ee) {
					if ee.msg != "" {
						fmt.Fprintln(os.Stderr, "agit:", ee.msg)
					}
					os.Exit(ee.code)
				}
				fmt.Fprintln(os.Stderr, "agit:", describe(err))
				os.Exit(1)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "agit: unknown command %q (see 'agit help')\n", name)
	os.Exit(2)
}

type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

func describe(err error) string {
	if conflicts, ok := client.IsConflict(err); ok {
		var b strings.Builder
		b.WriteString(err.Error())
		for _, c := range conflicts {
			fmt.Fprintf(&b, "\n  conflict (%s): %s", c.Kind, c.Path)
		}
		return b.String()
	}
	return err.Error()
}

func usage() {
	fmt.Fprintf(os.Stderr, "agit — a server-centric version control client\n\nUsage: agit <command> [arguments]\n\nCommands:\n")
	sorted := append([]command(nil), commands...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })
	for _, c := range sorted {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", c.name, c.summary)
	}
	fmt.Fprintf(os.Stderr, `
Environment:
  AGIT_SERVER   address used outside a working copy when no daemon is running
                (http(s)://host:port or unix:///path/to/agitd.sock)
  AGIT_REPO     repository used outside a working copy
  AGIT_TOKEN    bearer token for authenticated servers
  AGIT_AUTHOR   author identity for new changesets ("Name <email>")

Run 'agit <command> -h' for command flags.
`)
}

func newFlags(name, args string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: agit %s %s\n", name, args)
		fs.PrintDefaults()
	}
	return fs
}

// parse parses flags, allowing flags after positional arguments
// ("agit log main -n 5"). Everything after "--" is positional verbatim.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for i, a := range args {
		if a == "--" {
			args, rest = args[:i], args[i+1:]
			break
		}
	}
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return append(pos, rest...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// ---- connection helpers ----

func token() string { return os.Getenv("AGIT_TOKEN") }

// userName is the identity declared to servers that run without IAM:
// $AGIT_USER, else the email in $AGIT_AUTHOR, else the login name.
func userName() string {
	if u := os.Getenv("AGIT_USER"); u != "" {
		return u
	}
	a := workcopy.DefaultAuthor()
	if i, j := strings.IndexByte(a, '<'), strings.IndexByte(a, '>'); i >= 0 && j > i {
		return a[i+1 : j]
	}
	return a
}

// newClient connects to addr with the caller's credentials.
func newClient(addr string) (*client.Client, error) {
	c, err := client.New(addr, token())
	if err != nil {
		return nil, err
	}
	c.User = userName()
	return c, nil
}

// defaultRemote picks the endpoint to use: an explicit address, $AGIT_SERVER,
// the local daemon if it is running, or a server on localhost.
func defaultRemote(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if s := os.Getenv("AGIT_SERVER"); s != "" {
		return s
	}
	if sock := paths.DefaultSocket(); fileExists(sock) {
		return "unix://" + sock
	}
	return "http://127.0.0.1:7070"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// target is what read-side commands operate on: a working copy if there is
// one, otherwise a repository named by flags or the environment.
type target struct {
	wc   *workcopy.WorkCopy
	repo *client.Repo
}

type repoFlags struct {
	repo   *string
	server *string
}

func addRepoFlags(fs *flag.FlagSet) repoFlags {
	return repoFlags{
		repo:   fs.String("repo", "", "repository (default: the working copy's, or $AGIT_REPO)"),
		server: fs.String("server", "", "server, cache or daemon address (default: the working copy's remote)"),
	}
}

func (rf repoFlags) open() (*target, error) {
	if *rf.repo == "" {
		if root, err := workcopy.Find("."); err == nil {
			wc, err := openWCAt(root)
			if err != nil {
				return nil, err
			}
			if *rf.server != "" {
				c, err := newClient(*rf.server)
				if err != nil {
					return nil, err
				}
				wc.Client, wc.Repo = c, c.Repo(wc.Config.Repo)
			}
			return &target{wc: wc, repo: wc.Repo}, nil
		}
	}
	name := *rf.repo
	if name == "" {
		name = os.Getenv("AGIT_REPO")
	}
	if name == "" {
		return nil, fmt.Errorf("not in a working copy: pass --repo or set AGIT_REPO")
	}
	c, err := newClient(defaultRemote(*rf.server))
	if err != nil {
		return nil, err
	}
	return &target{repo: c.Repo(name)}, nil
}

// defaultRev is the revision read commands use when none is given: the
// working copy's base, or main.
func (t *target) defaultRev() string {
	if t.wc != nil && !t.wc.State.Base.IsZero() {
		return t.wc.State.Base.String()
	}
	return "main"
}

// rev expands "@" (the working copy's base) in revision expressions.
func (t *target) rev(r string) string {
	if r == "" {
		return t.defaultRev()
	}
	if strings.HasPrefix(r, "@") && t.wc != nil {
		return t.wc.State.Base.String() + r[1:]
	}
	return r
}

func openWC() (*workcopy.WorkCopy, error) {
	root, err := workcopy.Find(".")
	if err != nil {
		return nil, err
	}
	return openWCAt(root)
}

func openWCAt(root string) (*workcopy.WorkCopy, error) {
	wc, err := workcopy.Open(root, token())
	if err != nil {
		return nil, err
	}
	wc.Client.User = userName()
	return wc, nil
}

// ---- output helpers ----

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func short(id object.ID) string { return id.Short() }

func formatTime(unix int64, tzMinutes int) string {
	loc := time.FixedZone("", tzMinutes*60)
	return time.Unix(unix, 0).In(loc).Format("Mon Jan 2 15:04:05 2006 -0700")
}

func progress(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
