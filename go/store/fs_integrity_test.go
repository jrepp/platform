package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPutStageFailuresPreserveFinal(t *testing.T) {
	tests := []struct {
		name string
		hook func(*hookedStagedFile)
	}{
		{name: "write", hook: func(file *hookedStagedFile) { file.writeErr = errors.New("write") }},
		{name: "sync", hook: func(file *hookedStagedFile) { file.syncErr = errors.New("sync") }},
		{name: "chmod", hook: func(file *hookedStagedFile) { file.chmodErr = errors.New("chmod") }},
		{name: "stat", hook: func(file *hookedStagedFile) { file.statErr = errors.New("stat") }},
		{name: "close", hook: func(file *hookedStagedFile) { file.closeErr = errors.New("close") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root, _ := newIntegrityStore(t, 1)
			mustPutInternal(t, s, "object", "old")
			originalCreate := s.ops.createTemp
			s.ops.createTemp = func(dir, pattern string) (stagedFile, error) {
				file, err := originalCreate(dir, pattern)
				if err != nil {
					return nil, err
				}
				hooked := &hookedStagedFile{stagedFile: file}
				tt.hook(hooked)
				return hooked, nil
			}

			if _, err := s.Put(context.Background(), "object", strings.NewReader("new")); err == nil {
				t.Fatal("Put succeeded")
			}
			assertFileContents(t, filepath.Join(root, "object"), "old")
			assertNoStoreTemps(t, root)
		})
	}
}

func TestPutCreateFailurePreservesFinal(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	s.ops.createTemp = func(string, string) (stagedFile, error) {
		return nil, errors.New("create")
	}
	if _, err := s.Put(context.Background(), "object", strings.NewReader("new")); err == nil {
		t.Fatal("Put succeeded")
	}
	assertFileContents(t, filepath.Join(root, "object"), "old")
}

func TestRenameFailurePreservesFinalWithBackups(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	originalRename := s.ops.rename
	s.ops.rename = func(oldPath, newPath string) error {
		if newPath == filepath.Join(root, "object") {
			return errors.New("publish rename")
		}
		return originalRename(oldPath, newPath)
	}
	if _, err := s.Put(context.Background(), "object", strings.NewReader("new")); err == nil {
		t.Fatal("Put succeeded")
	}
	assertFileContents(t, filepath.Join(root, "object"), "old")
	assertNoStoreTemps(t, root)
}

func TestBackupSnapshotFailureSkipsBackupWithoutBlockingPut(t *testing.T) {
	s, root, backupRoot := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	s.ops.link = func(string, string) error { return errors.New("link") }
	if _, err := s.Put(context.Background(), "object", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, filepath.Join(root, "object"), "new")
	if _, err := os.Stat(backupSlot(s.backupDir("object"), 1)); !os.IsNotExist(err) {
		t.Fatalf("backup slot exists after link failure: %v (root %s)", err, backupRoot)
	}
}

func TestPrivateBackupsDoNotCollideWithObjectNames(t *testing.T) {
	s, root, backupRoot := newIntegrityStore(t, 2)
	mustPutInternal(t, s, "x", "x-old")
	mustPutInternal(t, s, "x.bak1", "independent")
	mustPutInternal(t, s, "x.tmp123", "temp-looking")
	mustPutInternal(t, s, "x", "x-new")

	assertFileContents(t, filepath.Join(root, "x"), "x-new")
	assertFileContents(t, filepath.Join(root, "x.bak1"), "independent")
	assertFileContents(t, filepath.Join(root, "x.tmp123"), "temp-looking")
	assertFileContents(t, backupSlot(s.backupDir("x"), 1), "x-old")
	if !pathContains(backupRoot, s.backupDir("x")) {
		t.Fatalf("backup path %q escaped %q", s.backupDir("x"), backupRoot)
	}

	objects, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x", "x.bak1", "x.tmp123"}
	if len(objects) != len(want) {
		t.Fatalf("objects = %#v", objects)
	}
	for idx := range want {
		if objects[idx].Name != want[idx] {
			t.Fatalf("object %d = %q, want %q", idx, objects[idx].Name, want[idx])
		}
	}
}

func TestPutCancellationAfterCommitLockPreservesFinal(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	s.commitMu.Lock()
	staged := make(chan struct{})
	originalCreate := s.ops.createTemp
	s.ops.createTemp = func(dir, pattern string) (stagedFile, error) {
		file, err := originalCreate(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &closeSignalFile{stagedFile: file, closed: staged}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.Put(ctx, "object", strings.NewReader("new"))
		done <- err
	}()
	<-staged
	cancel()
	s.commitMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Put error = %v", err)
	}
	assertFileContents(t, filepath.Join(root, "object"), "old")
}

func TestPutCancellationImmediatelyBeforeRenamePreservesFinal(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	ctx, cancel := context.WithCancel(context.Background())
	originalLink := s.ops.link
	s.ops.link = func(oldPath, newPath string) error {
		err := originalLink(oldPath, newPath)
		cancel()
		return err
	}
	if _, err := s.Put(ctx, "object", strings.NewReader("new")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put error = %v", err)
	}
	assertFileContents(t, filepath.Join(root, "object"), "old")
}

func TestPutCancellationBetweenReadsPreservesFinal(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterRead{cancel: cancel}
	if _, err := s.Put(ctx, "object", reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put error = %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("underlying reads = %d, want 1", reader.calls)
	}
	assertFileContents(t, filepath.Join(root, "object"), "old")
	assertNoStoreTemps(t, root)
}

func TestOpenMetadataComesFromOpenedDescriptor(t *testing.T) {
	s, _, _ := newIntegrityStore(t, 0)
	mustPutInternal(t, s, "object", "old")
	reader, object, err := s.Open(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	mustPutInternal(t, s, "object", "a much longer replacement")
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old" || object.Size != int64(len(body)) {
		t.Fatalf("Open returned %q with size %d", body, object.Size)
	}
}

func TestOpenReplacementBetweenPathResolutionAndDescriptorOpen(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 0)
	mustPutInternal(t, s, "object", "old")
	originalOpen := s.ops.open
	s.ops.open = func(name string) (openedFile, error) {
		if err := os.WriteFile(filepath.Join(root, "replacement"), []byte("new-longer"), 0o600); err != nil {
			return nil, err
		}
		if err := os.Rename(filepath.Join(root, "replacement"), filepath.Join(root, "object")); err != nil {
			return nil, err
		}
		return originalOpen(name)
	}
	reader, object, err := s.Open(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "new-longer" || object.Size != int64(len(body)) {
		t.Fatalf("Open returned %q with size %d", body, object.Size)
	}
}

func TestPutMetadataDoesNotUsePostCommitPathStat(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	mustPutInternal(t, s, "object", "old")
	originalRename := s.ops.rename
	postCommitTime := time.Unix(1, 0)
	s.ops.rename = func(oldPath, newPath string) error {
		if err := originalRename(oldPath, newPath); err != nil {
			return err
		}
		if newPath == filepath.Join(root, "object") {
			return os.Chtimes(newPath, postCommitTime, postCommitTime)
		}
		return nil
	}
	object, err := s.Put(context.Background(), "object", strings.NewReader("new"))
	if err != nil {
		t.Fatal(err)
	}
	if object.ModTime.Equal(postCommitTime) {
		t.Fatalf("Put returned post-commit pathname time %v", object.ModTime)
	}
	info, err := os.Stat(filepath.Join(root, "object"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(postCommitTime) {
		t.Fatalf("test did not alter final pathname time: %v", info.ModTime())
	}
}

func TestOpenBytesAndMetadataStayCoherentDuringReplacement(t *testing.T) {
	s, _, _ := newIntegrityStore(t, 0)
	mustPutInternal(t, s, "object", strings.Repeat("a", 17))
	for attempt := 0; attempt < 100; attempt++ {
		reader, object, err := s.Open(context.Background(), "object")
		if err != nil {
			t.Fatal(err)
		}
		replacement := strings.Repeat("b", 41)
		if object.Size == int64(len(replacement)) {
			replacement = strings.Repeat("a", 17)
		}
		mustPutInternal(t, s, "object", replacement)
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if object.Size != int64(len(body)) {
			t.Fatalf("Open returned size %d with %d bytes", object.Size, len(body))
		}
	}
}

func TestConcurrentPutsReturnMetadataForTheirOwnBytes(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 2)
	mustPutInternal(t, s, "object", "initial")
	bodies := []string{strings.Repeat("a", 17), strings.Repeat("b", 41)}
	var wg sync.WaitGroup
	errs := make(chan error, len(bodies))
	for _, body := range bodies {
		body := body
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj, err := s.Put(context.Background(), "object", strings.NewReader(body))
			if err == nil {
				digest := sha256.Sum256([]byte(body))
				if obj.Size != int64(len(body)) || obj.Digest != hex.EncodeToString(digest[:]) {
					err = errors.New("metadata does not match staged bytes")
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	final, err := os.ReadFile(filepath.Join(root, "object"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(final, []byte(bodies[0])) && !bytes.Equal(final, []byte(bodies[1])) {
		t.Fatalf("final contains partial bytes: %q", final)
	}
	backupOne, err := os.ReadFile(backupSlot(s.backupDir("object"), 1))
	if err != nil {
		t.Fatal(err)
	}
	backupTwo, err := os.ReadFile(backupSlot(s.backupDir("object"), 2))
	if err != nil {
		t.Fatal(err)
	}
	if string(backupTwo) != "initial" {
		t.Fatalf("oldest backup = %q, want initial", backupTwo)
	}
	if bytes.Equal(final, backupOne) || (!bytes.Equal(backupOne, []byte(bodies[0])) && !bytes.Equal(backupOne, []byte(bodies[1]))) {
		t.Fatalf("current/backup ordering is incoherent: final=%q backup=%q", final, backupOne)
	}
}

func TestTempRootAndCleanupReporting(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "objects")
	tempRoot := filepath.Join(base, "staging")
	var createdIn string
	var reported error
	s, err := NewFS(FSConfig{
		Root: root, TempRoot: tempRoot,
		ReportCleanupError: func(err error) { reported = err },
	})
	if err != nil {
		t.Fatal(err)
	}
	originalCreate := s.ops.createTemp
	s.ops.createTemp = func(dir, pattern string) (stagedFile, error) {
		createdIn = dir
		return originalCreate(dir, pattern)
	}
	originalRemove := s.ops.remove
	cleanupErr := errors.New("cleanup")
	s.ops.remove = func(name string) error {
		if strings.Contains(filepath.Base(name), ".store-tmp-") {
			return cleanupErr
		}
		return originalRemove(name)
	}
	if _, err := s.Put(context.Background(), "object", strings.NewReader("bytes")); err != nil {
		t.Fatal(err)
	}
	if createdIn != tempRoot {
		t.Fatalf("temp created in %q, want %q", createdIn, tempRoot)
	}
	if !errors.Is(reported, cleanupErr) {
		t.Fatalf("reported error = %v", reported)
	}
}

func TestNewFSRootSeparation(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "objects")
	backup := filepath.Join(base, "backups")
	temp := filepath.Join(base, "temp")
	if _, err := NewFS(FSConfig{Root: root, MaxBackups: 1}); err == nil {
		t.Fatal("accepted backups without BackupRoot")
	}
	for _, cfg := range []FSConfig{
		{Root: root, MaxBackups: 1, BackupRoot: root},
		{Root: root, MaxBackups: 1, BackupRoot: filepath.Join(root, "private")},
		{Root: root, TempRoot: filepath.Join(root, "temp")},
		{Root: root, MaxBackups: 1, BackupRoot: backup, TempRoot: backup},
	} {
		if _, err := NewFS(cfg); err == nil {
			t.Fatalf("accepted overlapping roots: %+v", cfg)
		}
	}
	if _, err := NewFS(FSConfig{Root: root, MaxBackups: 1, BackupRoot: backup, TempRoot: temp}); err != nil {
		t.Fatalf("refused separated roots: %v", err)
	}
}

func TestNewFSRejectsSymlinkRootAliases(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFS(FSConfig{Root: realRoot, MaxBackups: 1, BackupRoot: alias}); err == nil {
		t.Fatal("accepted symlink aliases for root and backup root")
	}
}

func TestNewFSRejectsDanglingBackupAliasIntoFutureObjectTree(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "objects")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "independent")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "backup-alias")
	if err := os.Symlink(filepath.Join(root, "future-private"), alias); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFS(FSConfig{Root: root, MaxBackups: 1, BackupRoot: alias}); err == nil {
		t.Fatal("accepted dangling backup alias into the object root")
	}
	assertFileContents(t, keep, "keep")
}

func TestReplacingSymlinkSkipsUnsafeBackup(t *testing.T) {
	s, root, _ := newIntegrityStore(t, 1)
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "object")); err != nil {
		t.Fatal(err)
	}
	mustPutInternal(t, s, "object", "stored")
	assertFileContents(t, external, "external")
	assertFileContents(t, filepath.Join(root, "object"), "stored")
	if _, err := os.Stat(backupSlot(s.backupDir("object"), 1)); !os.IsNotExist(err) {
		t.Fatalf("unsafe symlink backup exists: %v", err)
	}
}

type hookedStagedFile struct {
	stagedFile
	writeErr error
	syncErr  error
	closeErr error
	chmodErr error
	statErr  error
}

type closeSignalFile struct {
	stagedFile
	closed chan struct{}
	once   sync.Once
}

type cancelAfterRead struct {
	cancel context.CancelFunc
	calls  int
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	r.calls++
	p[0] = 'x'
	if r.calls == 1 {
		r.cancel()
	}
	return 1, nil
}

func (f *closeSignalFile) Close() error {
	err := f.stagedFile.Close()
	f.once.Do(func() { close(f.closed) })
	return err
}

func (f *hookedStagedFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.stagedFile.Write(p)
}

func (f *hookedStagedFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.stagedFile.Sync()
}

func (f *hookedStagedFile) Close() error {
	if f.closeErr != nil {
		err := f.closeErr
		f.closeErr = nil
		_ = f.stagedFile.Close()
		return err
	}
	return f.stagedFile.Close()
}

func (f *hookedStagedFile) Chmod(mode os.FileMode) error {
	if f.chmodErr != nil {
		return f.chmodErr
	}
	return f.stagedFile.Chmod(mode)
}

func (f *hookedStagedFile) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return f.stagedFile.Stat()
}

func newIntegrityStore(t *testing.T, maxBackups int) (*FS, string, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "objects")
	backupRoot := filepath.Join(base, "backups")
	s, err := NewFS(FSConfig{Root: root, MaxBackups: maxBackups, BackupRoot: backupRoot})
	if err != nil {
		t.Fatal(err)
	}
	return s, root, backupRoot
}

func mustPutInternal(t *testing.T, s *FS, name, body string) Object {
	t.Helper()
	obj, err := s.Put(context.Background(), name, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func assertFileContents(t *testing.T, name, want string) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func assertNoStoreTemps(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && strings.Contains(entry.Name(), ".store-tmp-") {
			t.Fatalf("temporary remains: %s", entry.Name())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
