package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (s *FS) stage(ctx context.Context, objectDir, cleanName string, r io.Reader) (string, Object, error) {
	tempDir := objectDir
	if s.cfg.TempRoot != "" {
		tempDir = s.cfg.TempRoot
	}
	if err := s.ops.mkdirAll(tempDir, s.cfg.DirMode); err != nil {
		return "", Object{}, err
	}
	temp, err := s.ops.createTemp(tempDir, ".store-tmp-*")
	if err != nil {
		return "", Object{}, err
	}
	tempName := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = temp.Close()
			s.cleanupTemp(tempName)
		}
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(temp, hasher), contextReader{ctx: ctx, reader: r})
	if err != nil {
		return "", Object{}, err
	}
	if err := temp.Sync(); err != nil {
		return "", Object{}, err
	}
	if err := temp.Chmod(s.cfg.FileMode); err != nil {
		return "", Object{}, err
	}
	info, err := temp.Stat()
	if err != nil {
		return "", Object{}, err
	}
	if err := temp.Close(); err != nil {
		return "", Object{}, err
	}
	keep = true
	return tempName, Object{
		Name:    cleanName,
		Ext:     strings.ToLower(filepath.Ext(cleanName)),
		Size:    size,
		ModTime: info.ModTime(),
		Digest:  hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (s *FS) cleanupTemp(name string) {
	err := s.ops.remove(name)
	if err == nil || os.IsNotExist(err) || s.cfg.ReportCleanupError == nil {
		return
	}
	s.cfg.ReportCleanupError(err)
}
