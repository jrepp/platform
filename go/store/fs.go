package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FSConfig configures a filesystem-backed store.
type FSConfig struct {
	// Root is the directory the store owns. Every object lives beneath it and
	// nothing outside it is reachable through any name.
	Root string

	// AllowedExt restricts which lowercased extensions may be written,
	// including the leading dot.
	//
	// Nil means no restriction, which is right for a store whose writers are
	// already trusted and wrong for one exposed to uploads. There is
	// deliberately no default list: the extensions a store should accept depend
	// entirely on what the consumer stores, and baking one consumer's media
	// types into the library is how a general store acquires a specific
	// opinion.
	AllowedExt []string

	// MaxBackups is how many previous versions of an overwritten object to keep
	// in BackupRoot. Zero keeps none.
	MaxBackups int

	// BackupRoot owns private backup files, outside Root and TempRoot. It is
	// required when MaxBackups is positive. Legacy name.bakN objects in Root
	// remain ordinary objects and are never rotated by this version.
	BackupRoot string

	// TempRoot optionally places staging outside Root, on the same filesystem.
	// The owner manages crash recovery there. Empty keeps sibling temporaries.
	TempRoot string

	// ReportCleanupError receives failures removing private staging files. It is
	// called outside the commit lock and cannot turn a successful Put into an
	// error. Nil ignores cleanup failures.
	ReportCleanupError func(error)

	// DirMode and FileMode are the permissions for created directories and
	// objects. Zero values become 0o755 and 0o644.
	DirMode  os.FileMode
	FileMode os.FileMode
}

func (c FSConfig) normalize() FSConfig {
	cfg := c
	if cfg.DirMode == 0 {
		cfg.DirMode = 0o755
	}
	if cfg.FileMode == 0 {
		cfg.FileMode = 0o644
	}
	for i, ext := range cfg.AllowedExt {
		cfg.AllowedExt[i] = strings.ToLower(ext)
	}
	return cfg
}

// FS is a Store backed by a directory on the local filesystem.
type FS struct {
	cfg      FSConfig
	commitMu sync.Mutex
	ops      fsOperations
}

var (
	_ Store     = (*FS)(nil)
	_ LocalPath = (*FS)(nil)
)

// NewFS builds a filesystem store rooted at cfg.Root.
//
// The root is not created here and is not required to exist yet: a store whose
// root appears later lists empty rather than failing, which keeps a fresh
// deployment from needing a provisioning step before it can answer.
func NewFS(cfg FSConfig) (*FS, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, errors.New("store: root is required")
	}
	if err := validateRoots(cfg); err != nil {
		return nil, err
	}
	return &FS{cfg: cfg.normalize(), ops: defaultOperations()}, nil
}

// Root reports the directory this store owns.
func (s *FS) Root() string { return s.cfg.Root }

// LocalPath returns the absolute path of a stored object.
func (s *FS) LocalPath(name string) (string, error) {
	full, err := resolve(s.cfg.Root, name)
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	if stat.IsDir() {
		return "", ErrNotFound
	}
	return full, nil
}

// Put publishes complete bytes with one rename. Streaming occurs outside the
// instance commit lock; backup bookkeeping and publication are serialized.
// Failures before rename preserve the target. A canceled context racing with
// publication may still succeed. No directory sync or crash durability is promised.
func (s *FS) Put(ctx context.Context, name string, r io.Reader) (Object, error) {
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	clean := cleanName(name)
	if clean == "" {
		return Object{}, ErrInvalidName
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if !s.extAllowed(ext) {
		return Object{}, ErrUnsupportedType
	}
	full, err := resolve(s.cfg.Root, clean)
	if err != nil {
		return Object{}, err
	}
	dir := filepath.Dir(full)
	if err := s.ops.mkdirAll(dir, s.cfg.DirMode); err != nil {
		return Object{}, err
	}
	temp, obj, err := s.stage(ctx, dir, clean, r)
	if err != nil {
		return Object{}, err
	}
	defer s.cleanupTemp(temp)
	if err := s.commit(ctx, temp, full, clean); err != nil {
		return Object{}, err
	}
	return obj, nil
}

func (s *FS) commit(ctx context.Context, temp, full, clean string) error {
	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.backup(full, clean)
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.ops.rename(temp, full)
}

// Open binds metadata to the opened descriptor even if another writer replaces
// the pathname. Callers close the returned reader.
func (s *FS) Open(ctx context.Context, name string) (io.ReadCloser, Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, Object{}, err
	}
	clean := cleanName(name)
	full, err := resolve(s.cfg.Root, clean)
	if err != nil {
		return nil, Object{}, err
	}
	// #nosec G304 -- resolve bounds the path to the operator-owned store root.
	file, err := s.ops.open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Object{}, ErrNotFound
		}
		return nil, Object{}, err
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		_ = file.Close()
		if err == nil {
			err = ErrNotFound
		}
		return nil, Object{}, err
	}
	return file, Object{
		Name: clean, Ext: strings.ToLower(filepath.Ext(clean)),
		Size: info.Size(), ModTime: info.ModTime(),
	}, nil
}

// Stat describes an object without reading it. The returned Digest is empty;
// call Digest when one is needed.
func (s *FS) Stat(ctx context.Context, name string) (Object, error) {
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	clean := cleanName(name)
	if clean == "" {
		return Object{}, ErrInvalidName
	}
	full, err := resolve(s.cfg.Root, clean)
	if err != nil {
		return Object{}, err
	}
	stat, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return Object{}, ErrNotFound
		}
		return Object{}, err
	}
	if stat.IsDir() {
		return Object{}, ErrNotFound
	}
	return Object{
		Name:    clean,
		Ext:     strings.ToLower(filepath.Ext(clean)),
		Size:    stat.Size(),
		ModTime: stat.ModTime(),
	}, nil
}

// List describes every object under the root, ordered by name.
//
// Dotfiles are skipped. Legacy name.bakN and other non-dot temp-looking keys
// remain ordinary visible objects. New private backups live outside Root. A
// missing root lists empty rather than failing.
func (s *FS) List(ctx context.Context) ([]Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(s.cfg.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Object{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrInvalidName
	}

	objects := make([]Object, 0)
	err = filepath.WalkDir(s.cfg.Root, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// An unreadable subtree is skipped rather than failing the listing:
			// one bad directory should not hide every other object.
			return nil //nolint:nilerr // skipping is the intended handling
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(s.cfg.Root, full)
		if err != nil {
			// The path came from walking this root, so it is relative to it by
			// construction. If that ever fails, skip the entry rather than
			// abandoning the listing.
			return nil //nolint:nilerr // skipping is the intended handling
		}
		name := filepath.ToSlash(rel)
		if strings.HasPrefix(name, ".") {
			return nil
		}
		stat, err := entry.Info()
		if err != nil {
			// The entry disappeared between the walk and the stat. It is not
			// part of the listing, which is the honest answer.
			return nil //nolint:nilerr // skipping is the intended handling
		}
		objects = append(objects, Object{
			Name:    name,
			Ext:     strings.ToLower(filepath.Ext(name)),
			Size:    stat.Size(),
			ModTime: stat.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Name < objects[j].Name })
	return objects, nil
}

// Digest reads an object and returns its lowercase hex SHA-256.
func (s *FS) Digest(ctx context.Context, name string) (string, error) {
	reader, _, err := s.Open(ctx, name)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *FS) extAllowed(ext string) bool {
	if s.cfg.AllowedExt == nil {
		return true
	}
	for _, allowed := range s.cfg.AllowedExt {
		if allowed == ext {
			return true
		}
	}
	return false
}
