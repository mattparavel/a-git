package tier

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/fetch"
	"github.com/mattparavel/a-git/internal/object"
)

// PrefetchStats reports what a prefetch did.
type PrefetchStats struct {
	Changeset object.ID     `json:"changeset"`
	Files     int           `json:"files"`
	Fetched   int           `json:"fetched"`
	Cached    int           `json:"cached"`
	Duration  time.Duration `json:"duration"`
}

// Prefetch makes sure every tree and blob reachable from rev (under prefix)
// is in the local object cache, fetching only what is missing. Daemons run
// it periodically for hot repositories so that checkouts on the machine are
// served entirely from local disk.
func (p *Proxy) Prefetch(ctx context.Context, repoName, rev, prefix, token string) (*PrefetchStats, error) {
	start := time.Now()
	up := *p.up
	up.Token = token
	rc := up.Repo(repoName)
	cs, err := rc.Resolve(ctx, rev)
	if err != nil {
		return nil, err
	}
	root, err := rc.RootTree(ctx, cs)
	if err != nil {
		return nil, err
	}
	f := &fetch.Fetcher{Src: rc, Local: p.objects, CS: cs}
	include := func(path string, isDir bool) bool {
		return prefix == "" || object.HasPathPrefix(path, prefix) || isDir && object.HasPathPrefix(prefix, path)
	}
	var blobs []fetch.Want
	err = f.Walk(ctx, root, include, func(e fetch.Entry) error {
		blobs = append(blobs, fetch.Want{ID: e.ID, Path: e.Path})
		return nil
	})
	if err != nil {
		return nil, err
	}
	var need []fetch.Want
	for _, b := range blobs {
		if !p.objects.Has(b.ID) {
			need = append(need, b)
		}
	}
	if err := f.Get(ctx, need, func(fetch.Want, object.Kind, []byte) error { return nil }); err != nil {
		return nil, err
	}
	return &PrefetchStats{
		Changeset: cs, Files: len(blobs), Fetched: f.Requested, Cached: f.FromLocal + len(blobs) - len(need),
		Duration: time.Since(start),
	}, nil
}

// packMagic starts a snapshot pack file: a header line followed by object
// frames (see api.WriteFrame).
const packMagic = "AGITPACK 1\n"

// WritePackHeader writes the pack header.
func WritePackHeader(w io.Writer) error {
	_, err := io.WriteString(w, packMagic)
	return err
}

// ImportPack loads a snapshot pack into an object cache, verifying every
// object. Machines can be provisioned with a recent pack so their daemon
// starts warm.
func ImportPack(r io.Reader, cache ObjectCache) (int, error) {
	br := bufio.NewReader(r)
	hdr, err := br.ReadString('\n')
	if err != nil || hdr != packMagic {
		return 0, fmt.Errorf("not an a-git snapshot pack")
	}
	fr := api.NewFrameReader(br)
	n := 0
	for {
		id, stored, err := fr.Next()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		if stored == nil || cache.Has(id) {
			continue
		}
		if _, _, err := object.Verify(id, stored); err != nil {
			return n, err
		}
		cache.Put(id, stored)
		n++
	}
}
