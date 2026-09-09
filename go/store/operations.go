package store

import (
	"io"
	"os"
)

type stagedFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
	Chmod(os.FileMode) error
	Stat() (os.FileInfo, error)
}

type openedFile interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
}

type fsOperations struct {
	createTemp func(string, string) (stagedFile, error)
	mkdirAll   func(string, os.FileMode) error
	remove     func(string) error
	rename     func(string, string) error
	link       func(string, string) error
	lstat      func(string) (os.FileInfo, error)
	open       func(string) (openedFile, error)
}

func defaultOperations() fsOperations {
	return fsOperations{
		createTemp: func(dir, pattern string) (stagedFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		mkdirAll: os.MkdirAll,
		remove:   os.Remove,
		rename:   os.Rename,
		link:     os.Link,
		lstat:    os.Lstat,
		open: func(name string) (openedFile, error) {
			// #nosec G304 -- callers pass paths resolved beneath the store root.
			return os.Open(name)
		},
	}
}
