// Command agitd is the per-machine a-git daemon. Every working copy and job
// on the machine talks to it over a Unix socket; it keeps one shared,
// size-bounded object store on local disk and talks only to its regional
// cache.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/paths"
	"github.com/mattparavel/a-git/internal/tier"
)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	socket := flag.String("socket", paths.DefaultSocket(), "Unix socket to listen on")
	listen := flag.String("listen", "", "optional TCP address to listen on as well")
	upstream := flag.String("upstream", "http://127.0.0.1:7071", "upstream tier (agit-cache)")
	cacheDir := flag.String("cache-dir", paths.DefaultCacheDir(), "object store directory")
	maxBytes := flag.Int64("max-bytes", 10<<30, "object store size bound in bytes")
	seedPack := flag.String("seed-pack", "", "snapshot pack to import at startup (see 'agit snapshot')")
	var prefetch listFlag
	flag.Var(&prefetch, "prefetch", "keep REPO[@REV][:PREFIX] warm (repeatable)")
	interval := flag.Duration("prefetch-interval", 10*time.Minute, "how often to refresh prefetched revisions")
	token := flag.String("token", os.Getenv("AGIT_TOKEN"), "token used for prefetching")
	flag.Parse()
	logger := log.New(os.Stderr, "agitd: ", log.LstdFlags)

	disk, err := tier.OpenDiskCache(*cacheDir, *maxBytes)
	if err != nil {
		logger.Fatal(err)
	}
	if *seedPack != "" {
		f, err := os.Open(*seedPack)
		if err != nil {
			logger.Fatal(err)
		}
		n, err := tier.ImportPack(f, disk)
		f.Close()
		if err != nil {
			logger.Fatalf("importing %s: %v", *seedPack, err)
		}
		logger.Printf("imported %d objects from %s", n, *seedPack)
	}
	up, err := client.New(*upstream, "")
	if err != nil {
		logger.Fatal(err)
	}
	up.Tier = api.TierDaemon
	p := tier.NewProxy(tier.Config{Tier: api.TierDaemon, Upstream: up, Objects: disk, Logger: logger})

	if err := os.MkdirAll(filepath.Dir(*socket), 0o700); err != nil {
		logger.Fatal(err)
	}
	os.Remove(*socket)
	ln, err := net.Listen("unix", *socket)
	if err != nil {
		logger.Fatal(err)
	}
	os.Chmod(*socket, 0o600)
	servers := []*http.Server{{Handler: p.Handler(), ReadHeaderTimeout: 30 * time.Second}}
	go serve(logger, servers[0], ln)
	logger.Printf("serving on unix://%s, upstream %s, objects in %s", *socket, *upstream, *cacheDir)
	if *listen != "" {
		tl, err := net.Listen("tcp", *listen)
		if err != nil {
			logger.Fatal(err)
		}
		hs := &http.Server{Handler: p.Handler(), ReadHeaderTimeout: 30 * time.Second}
		servers = append(servers, hs)
		go serve(logger, hs, tl)
		logger.Printf("also serving on http://%s", *listen)
	}

	ctx, cancel := context.WithCancel(context.Background())
	for _, spec := range prefetch {
		go keepWarm(ctx, logger, p, spec, *token, *interval)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	for _, hs := range servers {
		hs.Shutdown(sctx)
	}
	os.Remove(*socket)
}

func serve(logger *log.Logger, hs *http.Server, ln net.Listener) {
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}

// keepWarm periodically prefetches REPO[@REV][:PREFIX].
func keepWarm(ctx context.Context, logger *log.Logger, p *tier.Proxy, spec, token string, every time.Duration) {
	repoName, rev, prefix := spec, "main", ""
	if i := strings.IndexByte(repoName, ':'); i >= 0 {
		repoName, prefix = repoName[:i], repoName[i+1:]
	}
	if i := strings.IndexByte(repoName, '@'); i >= 0 {
		repoName, rev = repoName[:i], repoName[i+1:]
	}
	for {
		st, err := p.Prefetch(ctx, repoName, rev, prefix, token)
		if err != nil {
			logger.Printf("prefetch %s: %v", spec, err)
		} else {
			logger.Printf("prefetch %s: %s, %d files, fetched %d objects in %v", spec, st.Changeset.Short(), st.Files, st.Fetched, st.Duration.Round(time.Millisecond))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
