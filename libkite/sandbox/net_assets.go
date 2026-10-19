package sandbox

import (
	"os"
	"path/filepath"
	"slices"
)

// ResolveNetworkAssetMounts inspects system DNS resolver configuration and TLS CA
// certificate directories, resolving symlinks (such as systemd-resolved stub resolvers
// on modern Linux distributions) and appending read-only bind mounts to the existing
// mounts slice when network access is enabled.
func ResolveNetworkAssetMounts(existing []Mount) []Mount {
	mounts := slices.Clone(existing)
	destSeen := make(map[string]bool, len(mounts))
	for _, m := range mounts {
		destSeen[filepath.Clean(m.Destination)] = true
	}

	addMount := func(p string) {
		clean := filepath.Clean(p)
		if clean == "" || destSeen[clean] {
			return
		}
		destSeen[clean] = true
		mounts = append(mounts, Mount{
			Source:      clean,
			Destination: clean,
			Type:        MountBind,
			Mode:        MountRO,
		})
	}

	for _, p := range platformNetworkReadPaths {
		clean := filepath.Clean(p)
		if _, err := os.Lstat(clean); err == nil {
			addMount(clean)
			if canonical, err := filepath.EvalSymlinks(clean); err == nil {
				addMount(canonical)
			}
		}
	}

	return mounts
}
