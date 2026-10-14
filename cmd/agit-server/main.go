// Command agit-server runs the authoritative a-git tier: it owns storage,
// computes derived data and performs every write (commits, merges, rebases,
// lands).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/store"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7070", "address to listen on")
	data := flag.String("data", "", "data directory (default: in-memory, lost on exit)")
	iamFile := flag.String("iam", "", "IAM policy JSON file (default: allow everything)")
	protect := flag.String("protect", "", "comma-separated ref globs that may only advance by landing approved reviews (e.g. main,release/*)")
	createRepos := flag.String("create", "", "comma-separated repositories to create at startup if missing")
	flag.Parse()
	logger := log.New(os.Stderr, "agit-server: ", log.LstdFlags)

	var st store.Store
	if *data == "" {
		st = store.NewMem()
		logger.Printf("using in-memory storage (pass -data DIR to persist)")
	} else {
		fsStore, err := store.OpenFS(*data)
		if err != nil {
			logger.Fatal(err)
		}
		st = fsStore
	}
	var policy *iam.Policy
	if *iamFile != "" {
		p, err := iam.Load(*iamFile)
		if err != nil {
			logger.Fatal(err)
		}
		policy = p
	}
	for _, name := range splitList(*createRepos) {
		if err := st.CreateRepo(context.Background(), name); err != nil && !errors.Is(err, store.ErrExists) {
			logger.Fatalf("creating %s: %v", name, err)
		}
	}
	srv := api.NewServer(api.ServerConfig{Store: st, Policy: policy, ProtectedRefs: splitList(*protect), Logger: logger})
	hs := &http.Server{Addr: *listen, Handler: srv.Handler(), ReadHeaderTimeout: 30 * time.Second}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(ctx)
	}()
	logger.Printf("serving on http://%s", *listen)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
