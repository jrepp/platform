// Package store is a rooted object store with containment guarantees.
//
// It holds bytes addressed by name, keeps bounded previous versions when a name
// is overwritten, and reports a content digest. It knows nothing about what the
// bytes are for: no media types, no conversion, no notion of a project or a
// tenant. A consumer that needs those supplies them.
//
// The package was extracted from tidal's editor component, where the same
// mechanics served game assets. The behaviour it preserves is deliberate; the
// vocabulary it drops is equally deliberate.
package store

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrInvalidName is returned for a name that escapes the root, is empty, or
	// cannot be represented under it.
	ErrInvalidName = errors.New("invalid object name")
	// ErrNotFound is returned when no object is stored under a name.
	ErrNotFound = errors.New("object not found")
	// ErrUnsupportedType is returned when a name's extension is outside the
	// store's configured allowlist.
	ErrUnsupportedType = errors.New("unsupported object type")
)

// Object describes one stored object.
type Object struct {
	// Name is the store key: a slash-separated path relative to the root, with
	// no leading slash and no parent traversal.
	Name string `json:"name"`
	// Ext is the lowercased file extension including the dot, empty when none.
	Ext string `json:"ext"`
	// Size is the object's length in bytes.
	Size int64 `json:"size"`
	// ModTime is when the object was last written.
	ModTime time.Time `json:"modTime"`
	// Digest is the lowercase hex SHA-256 of the object's contents.
	//
	// Put fills it because hashing while streaming costs nothing. Stat and List
	// leave it empty, because filling it would mean reading every object on
	// every listing. Call Digest when a listing needs one.
	Digest string `json:"digest,omitempty"`
}

// Store holds bytes addressed by name.
//
// The interface is what consumers depend on. It carries a context so that a
// backend which is not a local filesystem -- an object store, a remote host --
// can honour cancellation without the interface changing shape.
type Store interface {
	// Put writes an object, replacing any object already stored under the name
	// and retaining bounded previous versions. The returned Object carries the
	// digest of what was written.
	Put(ctx context.Context, name string, r io.Reader) (Object, error)
	// Open returns the object's contents. The caller closes the reader.
	Open(ctx context.Context, name string) (io.ReadCloser, Object, error)
	// Stat describes an object without reading it.
	Stat(ctx context.Context, name string) (Object, error)
	// List describes every object under the root, ordered by name.
	List(ctx context.Context) ([]Object, error)
	// Digest reads an object and returns its lowercase hex SHA-256.
	Digest(ctx context.Context, name string) (string, error)
}

// LocalPath is implemented by stores whose objects are files on this machine.
//
// It exists for one honest reason: some consumers must hand a path to an
// external process that reads the file itself, and no amount of interface
// design makes that requirement go away. It is an escape hatch, not part of
// Store, so a consumer that uses it declares the coupling in its own type
// assertions rather than forcing every backend to be a filesystem.
type LocalPath interface {
	LocalPath(name string) (string, error)
}
