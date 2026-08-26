package store

import (
	"fmt"
	"os"
)

// rotate moves the file at fullPath aside, keeping at most maxVersions previous
// copies as name.bak1 through name.bakN, where bak1 is the most recent.
//
// Rotation walks from the oldest slot down so that no rename overwrites a copy
// that has not been moved yet. The oldest is deleted rather than shifted off the
// end, which is what bounds the store's growth: without it, a name written in a
// loop would fill the disk with history nobody asked for.
//
// Failures are ignored on purpose. Losing a backup must not fail the write that
// triggered the rotation; the caller's data is the thing that matters.
func rotate(fullPath string, maxVersions int) {
	if maxVersions <= 0 {
		return
	}
	if _, err := os.Stat(fullPath); err != nil {
		return
	}
	for idx := maxVersions; idx >= 1; idx-- {
		candidate := backupName(fullPath, idx)
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if idx == maxVersions {
			_ = os.Remove(candidate)
			continue
		}
		_ = os.Rename(candidate, backupName(fullPath, idx+1))
	}
	_ = os.Rename(fullPath, backupName(fullPath, 1))
}

func backupName(fullPath string, idx int) string {
	return fmt.Sprintf("%s.bak%d", fullPath, idx)
}
