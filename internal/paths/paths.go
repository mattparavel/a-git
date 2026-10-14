// Package paths holds default filesystem locations shared by the binaries.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSocket is where agitd listens and where the CLI looks for it.
// AGIT_DAEMON_SOCKET overrides it.
func DefaultSocket() string {
	if s := os.Getenv("AGIT_DAEMON_SOCKET"); s != "" {
		return s
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "agit", "agitd.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("agit-%d", os.Getuid()), "agitd.sock")
}

// DefaultCacheDir is where agitd keeps its object store.
func DefaultCacheDir() string {
	if d := os.Getenv("AGIT_CACHE_DIR"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "agit", "objects")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("agit-%d", os.Getuid()), "objects")
}
