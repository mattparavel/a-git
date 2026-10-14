// Command agit-cache runs a regional caching tier. It serves the full a-git
// protocol, answers object and immutable-query requests from memory, and
// forwards everything else to exactly one upstream (normally agit-server).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/tier"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7071", "address to listen on")
	upstream := flag.String("upstream", "http://127.0.0.1:7070", "upstream tier (agit-server)")
	maxBytes := flag.Int64("max-bytes", 2<<30, "object cache size in bytes")
	respBytes := flag.Int64("response-bytes", 256<<20, "immutable response cache size in bytes")
	flag.Parse()
	logger := log.New(os.Stderr, "agit-cache: ", log.LstdFlags)

	up, err := client.New(*upstream, "")
	if err != nil {
		logger.Fatal(err)
	}
	up.Tier = api.TierCache
	p := tier.NewProxy(tier.Config{
		Tier: api.TierCache, Upstream: up, Objects: tier.NewMemCache(*maxBytes),
		ResponseBytes: *respBytes, Logger: logger,
	})
	hs := &http.Server{Addr: *listen, Handler: p.Handler(), ReadHeaderTimeout: 30 * time.Second}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(ctx)
	}()
	logger.Printf("serving on http://%s, upstream %s", *listen, *upstream)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
