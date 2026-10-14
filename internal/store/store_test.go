package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/mattparavel/a-git/internal/object"
)

func testStores(t *testing.T) map[string]Store {
	fsStore, err := OpenFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return map[string]Store{"mem": NewMem(), "fs": fsStore}
}

func TestConformance(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			if err := s.CreateRepo(ctx, "r"); err != nil {
				t.Fatal(err)
			}
			if err := s.CreateRepo(ctx, "r"); !errors.Is(err, ErrExists) {
				t.Fatalf("duplicate create: %v", err)
			}
			if _, err := s.GetObject(ctx, "missing", object.Zero); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing repo: %v", err)
			}

			// Objects.
			id, stored := object.Encode(object.KindBlob, []byte("hello"))
			if ok, _ := s.HasObject(ctx, "r", id); ok {
				t.Fatal("unexpected object")
			}
			if err := s.PutObject(ctx, "r", id, stored); err != nil {
				t.Fatal(err)
			}
			if err := s.PutObject(ctx, "r", id, stored); err != nil {
				t.Fatal("idempotent put:", err)
			}
			got, err := s.GetObject(ctx, "r", id)
			if err != nil || string(got) != string(stored) {
				t.Fatalf("get object: %q %v", got, err)
			}

			// Changesets and WAL.
			var ids []object.ID
			for i := 0; i < 5; i++ {
				cs := &object.Changeset{Author: "a", Message: fmt.Sprint(i)}
				cid, enc := object.EncodeChangeset(cs)
				created, err := s.PutChangeset(ctx, "r", cid, enc)
				if err != nil || !created {
					t.Fatalf("put changeset: %v %v", created, err)
				}
				created, _ = s.PutChangeset(ctx, "r", cid, enc)
				if created {
					t.Fatal("second put reported created")
				}
				ids = append(ids, cid)
			}
			wal, err := s.ReadWAL(ctx, "r", 0, 0)
			if err != nil || len(wal) != 5 {
				t.Fatalf("wal: %v %v", wal, err)
			}
			for i, e := range wal {
				if e.Seq != int64(i+1) || e.ID != ids[i] {
					t.Fatalf("wal[%d] = %+v", i, e)
				}
			}
			wal, _ = s.ReadWAL(ctx, "r", 3, 1)
			if len(wal) != 1 || wal[0].Seq != 4 {
				t.Fatalf("wal after 3: %+v", wal)
			}
			found, _ := s.FindChangesets(ctx, "r", ids[2].String()[:8], 10)
			if len(found) != 1 || found[0] != ids[2] {
				t.Fatalf("find: %v", found)
			}

			// Refs.
			if err := s.CASRef(ctx, "r", "main", object.Zero, ids[0], "t"); err != nil {
				t.Fatal(err)
			}
			if err := s.CASRef(ctx, "r", "main", object.Zero, ids[1], "t"); !errors.Is(err, ErrCASFailed) {
				t.Fatalf("expected CAS failure, got %v", err)
			}
			if err := s.CASRef(ctx, "r", "main", ids[0], ids[1], "t"); err != nil {
				t.Fatal(err)
			}
			if err := s.CASRef(ctx, "r", "release/1.0", object.Zero, ids[2], "t"); err != nil {
				t.Fatal(err)
			}
			refs, _ := s.ListRefs(ctx, "r", "")
			if len(refs) != 2 || refs[0].Name != "main" || refs[1].Name != "release/1.0" {
				t.Fatalf("refs: %+v", refs)
			}
			if err := s.CASRef(ctx, "r", "release/1.0", ids[2], object.Zero, "t"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetRef(ctx, "r", "release/1.0"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("deleted ref: %v", err)
			}
			log, _ := s.ReadRefLog(ctx, "r", 0, 0)
			if len(log) != 4 || log[3].New != object.Zero || log[1].Old != ids[0] {
				t.Fatalf("reflog: %+v", log)
			}

			// Derived.
			if _, err := s.GetDerived(ctx, "r", "tree", ids[0]); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			s.PutDerived(ctx, "r", "tree", ids[0], []byte("v"))
			if v, _ := s.GetDerived(ctx, "r", "tree", ids[0]); string(v) != "v" {
				t.Fatalf("derived: %q", v)
			}

			// KV with concurrent updates.
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := s.KVUpdate(ctx, "r", "counters", "c1", func(old []byte) ([]byte, error) {
						n := 0
						fmt.Sscan(string(old), &n)
						return []byte(fmt.Sprint(n + 1)), nil
					})
					if err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if v, _ := s.KVGet(ctx, "r", "counters", "c1"); string(v) != "20" {
				t.Fatalf("counter = %q", v)
			}
			keys, _ := s.KVList(ctx, "r", "counters", "")
			if len(keys) != 1 || keys[0] != "c1" {
				t.Fatalf("kv list: %v", keys)
			}
		})
	}
}

func TestConcurrentCAS(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			s.CreateRepo(ctx, "r")
			// Many writers race to advance a ref one step at a time; every
			// successful CAS must be from the value it observed.
			var wg sync.WaitGroup
			var mu sync.Mutex
			wins := 0
			for w := 0; w < 8; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for i := 0; i < 20; i++ {
						cur, err := s.GetRef(ctx, "r", "main")
						if err != nil && !errors.Is(err, ErrNotFound) {
							t.Error(err)
							return
						}
						next := object.Hash(object.KindBlob, []byte(fmt.Sprint(w, i)))
						if err := s.CASRef(ctx, "r", "main", cur, next, ""); err == nil {
							mu.Lock()
							wins++
							mu.Unlock()
						} else if !errors.Is(err, ErrCASFailed) {
							t.Error(err)
						}
					}
				}(w)
			}
			wg.Wait()
			log, _ := s.ReadRefLog(ctx, "r", 0, 10000)
			if len(log) != wins {
				t.Fatalf("reflog has %d entries, %d wins", len(log), wins)
			}
			for i := 1; i < len(log); i++ {
				if log[i].Old != log[i-1].New {
					t.Fatalf("reflog chain broken at %d", i)
				}
			}
		})
	}
}

func TestRefNames(t *testing.T) {
	for _, ok := range []string{"main", "release/1.0", "users/alice/feature-x", "v1"} {
		if !ValidRefName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "/main", "main/", "a//b", "../x", ".hidden", "a/.b", "deadbeefcafe", "a b", "x~1", "x^"} {
		if ValidRefName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
