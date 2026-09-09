package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// backup takes a best-effort hard-link snapshot without moving the accepted
// object. Snapshot or rotation failures never make publication fall back to the
// old move-first behavior.
func (s *FS) backup(fullPath, cleanName string) {
	if s.cfg.MaxBackups <= 0 {
		return
	}
	info, err := s.ops.lstat(fullPath)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	dir := s.backupDir(cleanName)
	if err := s.ops.mkdirAll(dir, s.cfg.DirMode); err != nil {
		return
	}
	snapshot := filepath.Join(dir, "0")
	_ = s.ops.remove(snapshot)
	if err := s.ops.link(fullPath, snapshot); err != nil {
		return
	}
	defer func() { _ = s.ops.remove(snapshot) }()

	for slot := s.cfg.MaxBackups; slot >= 1; slot-- {
		current := backupSlot(dir, slot)
		if slot == s.cfg.MaxBackups {
			_ = s.ops.remove(current)
			continue
		}
		_ = s.ops.rename(current, backupSlot(dir, slot+1))
	}
	_ = s.ops.rename(snapshot, backupSlot(dir, 1))
}

func (s *FS) backupDir(cleanName string) string {
	digest := sha256.Sum256([]byte(cleanName))
	return filepath.Join(s.cfg.BackupRoot, hex.EncodeToString(digest[:]))
}

func backupSlot(dir string, slot int) string {
	return filepath.Join(dir, fmt.Sprintf("%d", slot))
}
