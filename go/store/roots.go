package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func validateRoots(cfg FSConfig) error {
	paths, err := configuredRootPaths(cfg)
	if err != nil {
		return err
	}
	if err := resolveRootPaths(paths); err != nil {
		return err
	}
	if err := validateRootSeparation(paths); err != nil {
		return err
	}
	return validateRootDevices(paths)
}

func configuredRootPaths(cfg FSConfig) ([]rootPath, error) {
	if cfg.BackupRoot != "" && strings.TrimSpace(cfg.BackupRoot) == "" {
		return nil, errors.New("store: backup root must not be blank")
	}
	if cfg.TempRoot != "" && strings.TrimSpace(cfg.TempRoot) == "" {
		return nil, errors.New("store: temp root must not be blank")
	}
	if cfg.MaxBackups > 0 && strings.TrimSpace(cfg.BackupRoot) == "" {
		return nil, errors.New("store: backup root is required when backups are enabled")
	}
	paths := []rootPath{{name: "root", path: cfg.Root}}
	if strings.TrimSpace(cfg.BackupRoot) != "" {
		paths = append(paths, rootPath{name: "backup root", path: cfg.BackupRoot})
	}
	if strings.TrimSpace(cfg.TempRoot) != "" {
		paths = append(paths, rootPath{name: "temp root", path: cfg.TempRoot})
	}
	return paths, nil
}

func resolveRootPaths(paths []rootPath) error {
	for idx := range paths {
		var err error
		paths[idx].canonical, paths[idx].ancestor, err = canonicalFuturePath(paths[idx].path)
		if err != nil {
			return fmt.Errorf("store: resolve %s: %w", paths[idx].name, err)
		}
	}
	return nil
}

func validateRootSeparation(paths []rootPath) error {
	for left := range paths {
		for right := left + 1; right < len(paths); right++ {
			if pathsOverlap(paths[left].canonical, paths[right].canonical) {
				return fmt.Errorf("store: %s and %s must not overlap", paths[left].name, paths[right].name)
			}
		}
	}
	return nil
}

func validateRootDevices(paths []rootPath) error {
	for idx := 1; idx < len(paths); idx++ {
		if !sameDevice(paths[0].ancestor, paths[idx].ancestor) {
			return fmt.Errorf("store: root and %s must be on the same filesystem", paths[idx].name)
		}
	}
	return nil
}

type rootPath struct {
	name      string
	path      string
	canonical string
	ancestor  os.FileInfo
}

// canonicalFuturePath resolves every symlink in the nearest existing ancestor
// and appends the missing suffix. These directories are operator-owned; later
// hostile filesystem mutation is outside this configuration check's scope.
func canonicalFuturePath(path string) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	abs = filepath.Clean(abs)
	ancestor := abs
	var suffix []string
	for {
		resolved, info, exists, err := inspectAncestor(ancestor)
		if err != nil {
			return "", nil, err
		}
		if exists {
			for idx := len(suffix) - 1; idx >= 0; idx-- {
				resolved = filepath.Join(resolved, suffix[idx])
			}
			return filepath.Clean(resolved), info, nil
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", nil, errors.New("no existing path ancestor")
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
}

func inspectAncestor(path string) (string, os.FileInfo, bool, error) {
	linkInfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if linkInfo.Mode()&os.ModeSymlink != 0 {
			return "", nil, false, fmt.Errorf("dangling symlink %s: %w", path, err)
		}
		return "", nil, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, false, err
	}
	return resolved, info, true, nil
}

func pathsOverlap(left, right string) bool {
	return left == right || pathContains(left, right) || pathContains(right, left)
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func sameDevice(left, right os.FileInfo) bool {
	leftStat, leftOK := left.Sys().(*syscall.Stat_t)
	rightStat, rightOK := right.Sys().(*syscall.Stat_t)
	return leftOK && rightOK && leftStat.Dev == rightStat.Dev
}
