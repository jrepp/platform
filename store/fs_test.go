package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrepp/platform/store"
)

func newStore(t *testing.T, cfg store.FSConfig) (*store.FS, string) {
	t.Helper()
	if cfg.Root == "" {
		cfg.Root = t.TempDir()
	}
	s, err := store.NewFS(cfg)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return s, cfg.Root
}

func put(t *testing.T, s *store.FS, name, body string) store.Object {
	t.Helper()
	obj, err := s.Put(context.Background(), name, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("Put(%q): %v", name, err)
	}
	return obj
}

// Behaviour carried over from tidal's editor component, which this package was
// extracted from. If these change, the extraction changed something it should
// not have.
func TestPutAndRead(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})

	obj := put(t, s, "models/test.fbx", "hello")
	if obj.Name != "models/test.fbx" {
		t.Errorf("Name = %q", obj.Name)
	}
	if obj.Ext != ".fbx" {
		t.Errorf("Ext = %q", obj.Ext)
	}
	if obj.Size != 5 {
		t.Errorf("Size = %d, want 5", obj.Size)
	}

	sum := sha256.Sum256([]byte("hello"))
	if obj.Digest != hex.EncodeToString(sum[:]) {
		t.Errorf("Digest = %q, want the sha256 of what was written", obj.Digest)
	}

	// The bytes are on disk under the name, readable without the store.
	data, err := os.ReadFile(filepath.Join(root, "models", "test.fbx"))
	if err != nil {
		t.Fatalf("read through the filesystem: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("contents = %q", string(data))
	}

	reader, got, err := s.Open(context.Background(), "models/test.fbx")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reader.Close() }()
	body, _ := io.ReadAll(reader)
	if string(body) != "hello" || got.Size != 5 {
		t.Errorf("Open returned %q size %d", string(body), got.Size)
	}
}

// A name that escapes the root must be refused, not repaired. Silently
// rewriting traversal into something inside the root turns an attack into a
// confusing success.
func TestPutRejectsNamesOutsideTheRoot(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})

	for _, name := range []string{
		"",
		"   ",
		"/",
		".",
		"..",
		"../escape.txt",
		"models/../../escape.txt",
		"/../escape.txt",
	} {
		if _, err := s.Put(context.Background(), name, bytes.NewBufferString("nope")); !errors.Is(err, store.ErrInvalidName) {
			t.Errorf("Put(%q) error = %v, want ErrInvalidName", name, err)
		}
	}

	// Nothing escaped: the parent of the root is untouched.
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("a write escaped the store root")
	}
}

// A sibling directory whose name merely starts with the root's name is not
// inside the root.
func TestContainmentRequiresASeparatorBoundary(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "assets")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "assets-old"), 0o755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	s, _ := newStore(t, store.FSConfig{Root: root})

	if _, err := s.Put(context.Background(), "../assets-old/leak.txt", bytes.NewBufferString("x")); !errors.Is(err, store.ErrInvalidName) {
		t.Fatalf("wrote into a sibling directory: %v", err)
	}
}

func TestAllowedExtensions(t *testing.T) {
	s, _ := newStore(t, store.FSConfig{AllowedExt: []string{".png", ".GLB"}})

	put(t, s, "art/ok.png", "x")
	// The allowlist is compared case-insensitively in both directions.
	put(t, s, "art/ok.GLB", "x")
	put(t, s, "art/also.glb", "x")

	if _, err := s.Put(context.Background(), "art/script.sh", bytes.NewBufferString("x")); !errors.Is(err, store.ErrUnsupportedType) {
		t.Fatalf("accepted a disallowed extension: %v", err)
	}

	// A nil allowlist means no restriction. There is deliberately no default
	// list: media types belong to the consumer, not to the store.
	open, _ := newStore(t, store.FSConfig{})
	put(t, open, "anything.sh", "x")
}

func TestPutKeepsBoundedPreviousVersions(t *testing.T) {
	s, root := newStore(t, store.FSConfig{MaxBackups: 2})

	for _, body := range []string{"v1", "v2", "v3", "v4"} {
		put(t, s, "doc.txt", body)
	}

	current, err := os.ReadFile(filepath.Join(root, "doc.txt"))
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if string(current) != "v4" {
		t.Errorf("current = %q, want v4", string(current))
	}
	// bak1 is the most recent previous version.
	for name, want := range map[string]string{"doc.txt.bak1": "v3", "doc.txt.bak2": "v2"} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, string(got), want)
		}
	}
	// The oldest is dropped rather than shifted off the end, which is what
	// bounds the store's growth.
	if _, err := os.Stat(filepath.Join(root, "doc.txt.bak3")); !os.IsNotExist(err) {
		t.Error("kept more versions than MaxBackups")
	}
}

func TestPutWithoutBackupsOverwrites(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})
	put(t, s, "doc.txt", "v1")
	put(t, s, "doc.txt", "v2")

	if _, err := os.Stat(filepath.Join(root, "doc.txt.bak1")); !os.IsNotExist(err) {
		t.Error("kept a backup when MaxBackups is zero")
	}
}

// A failed write must leave the previous object intact and no temporary file
// behind, because a reader of the store sees either version but never a
// half-written one.
func TestFailedPutLeavesThePreviousObjectIntact(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})
	put(t, s, "doc.txt", "good")

	_, err := s.Put(context.Background(), "doc.txt", iotest{err: errors.New("stream died")})
	if err == nil {
		t.Fatal("Put succeeded with a failing reader")
	}

	data, err := os.ReadFile(filepath.Join(root, "doc.txt"))
	if err != nil || string(data) != "good" {
		t.Fatalf("previous object = %q, %v", string(data), err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("left %d entries behind, want only the object itself", len(entries))
	}
}

type iotest struct{ err error }

func (r iotest) Read([]byte) (int, error) { return 0, r.err }

func TestStatAndDigest(t *testing.T) {
	s, _ := newStore(t, store.FSConfig{})
	put(t, s, "a/b.txt", "contents")

	obj, err := s.Stat(context.Background(), "a/b.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if obj.Size != 8 || obj.Ext != ".txt" {
		t.Errorf("Stat = %+v", obj)
	}
	// Stat leaves Digest empty; filling it would mean reading every object on
	// every listing.
	if obj.Digest != "" {
		t.Errorf("Stat filled Digest = %q", obj.Digest)
	}

	digest, err := s.Digest(context.Background(), "a/b.txt")
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	sum := sha256.Sum256([]byte("contents"))
	if digest != hex.EncodeToString(sum[:]) {
		t.Errorf("Digest = %q", digest)
	}
}

func TestMissingObjects(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})
	ctx := context.Background()

	if _, err := s.Stat(ctx, "absent.txt"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Stat(absent) = %v, want ErrNotFound", err)
	}
	if _, _, err := s.Open(ctx, "absent.txt"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Open(absent) = %v, want ErrNotFound", err)
	}
	if _, err := s.Digest(ctx, "absent.txt"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Digest(absent) = %v, want ErrNotFound", err)
	}

	// A directory is not an object.
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := s.Stat(ctx, "adir"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Stat(dir) = %v, want ErrNotFound", err)
	}
}

func TestList(t *testing.T) {
	s, root := newStore(t, store.FSConfig{MaxBackups: 1})
	ctx := context.Background()

	put(t, s, "props/crate.fbx", "data")
	put(t, s, "art/logo.png", "data")
	put(t, s, "props/crate.fbx", "newer") // produces crate.fbx.bak1

	// A dotfile is not an object.
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dotfile: %v", err)
	}

	objects, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, obj := range objects {
		names = append(names, obj.Name)
	}
	want := []string{"art/logo.png", "props/crate.fbx", "props/crate.fbx.bak1"}
	if len(names) != len(want) {
		t.Fatalf("List = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("List = %v, want %v (ordered by name)", names, want)
		}
	}
}

// A store whose root appears later lists empty rather than failing, so a fresh
// deployment answers before anything has provisioned its directory.
func TestListMissingRootIsEmpty(t *testing.T) {
	s, _ := newStore(t, store.FSConfig{Root: filepath.Join(t.TempDir(), "not-created-yet")})

	objects, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 0 {
		t.Errorf("List = %v, want empty", objects)
	}
}

func TestLocalPath(t *testing.T) {
	s, root := newStore(t, store.FSConfig{})
	put(t, s, "models/test.fbx", "hello")

	full, err := s.LocalPath("models/test.fbx")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	if full != filepath.Join(root, "models", "test.fbx") {
		t.Errorf("LocalPath = %q", full)
	}
	if _, err := s.LocalPath("../escape"); !errors.Is(err, store.ErrInvalidName) {
		t.Errorf("LocalPath(traversal) = %v, want ErrInvalidName", err)
	}
	if _, err := s.LocalPath("absent"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("LocalPath(absent) = %v, want ErrNotFound", err)
	}
}

func TestNewFSRequiresRoot(t *testing.T) {
	if _, err := store.NewFS(store.FSConfig{}); err == nil {
		t.Error("accepted an empty root")
	}
	if _, err := store.NewFS(store.FSConfig{Root: "   "}); err == nil {
		t.Error("accepted a blank root")
	}
}

func TestContextCancellation(t *testing.T) {
	s, _ := newStore(t, store.FSConfig{})
	put(t, s, "doc.txt", "x")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Put(ctx, "other.txt", bytes.NewBufferString("x")); !errors.Is(err, context.Canceled) {
		t.Errorf("Put with a cancelled context = %v", err)
	}
	if _, err := s.Stat(ctx, "doc.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Stat with a cancelled context = %v", err)
	}
	if _, err := s.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List with a cancelled context = %v", err)
	}
}
