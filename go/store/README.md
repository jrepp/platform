# store

A rooted object store. It holds bytes addressed by name, keeps bounded previous
versions when a name is overwritten, and reports content digests.

Extracted from `tidal`'s editor component, where the same mechanics served game
assets.

```go
s, err := store.NewFS(store.FSConfig{
    Root:       "/srv/objects",
    AllowedExt: []string{".png", ".glb"}, // nil means no restriction
    MaxBackups: 5,
    BackupRoot: "/srv/store-backups",
    TempRoot:   "/srv/store-staging", // optional; empty stages beside each object
    ReportCleanupError: func(err error) {
        // Account for leaked staging capacity; reporting cannot fail Put.
    },
})

obj, err := s.Put(ctx, "art/logo.png", reader)  // obj.Digest is the sha256 of what landed
reader, obj, err := s.Open(ctx, "art/logo.png")
objects, err := s.List(ctx)
```

## Properties worth knowing

**Containment is enforced, not assumed.** Every name resolves to a path proven
to be under the root, with a separator boundary required so that `/srv/objects-old`
is not inside `/srv/objects`. A name containing a `..` segment is *rejected*
rather than cleaned: repairing it would store the object under a different name
than the caller asked for, which hides the caller's bug and makes the audit
record disagree with the request.

**Writes publish atomically.** Bytes land in a temporary file, are flushed, and
are renamed over the destination. Streaming occurs outside a per-`FS` commit
lock. The lock orders snapshot, backup rotation, and publication for writers
through that instance; it does not serialize other processes or direct
filesystem writers. Cancellation is checked after taking the lock and directly
before rename, and between completed staging reads. It cannot interrupt an
arbitrary blocked `Read`. Cancellation racing after the final check may still
publish. Rename gives atomic visibility, but the package does not promise
directory-sync crash durability.

By default staging files are siblings of their destination. `TempRoot` places
them in a separate operator-owned directory on the same filesystem. `Root`,
`TempRoot`, and `BackupRoot` must neither overlap nor resolve through existing
symlinks to overlapping paths. Root validation resolves the nearest existing
ancestor; operators remain responsible for directory ownership and for avoiding
later filesystem mutation. `ReportCleanupError` reports removal failures for
private staging files, except already-absent files. It runs outside the commit
lock and cannot turn an accepted write into a rejection.

**Digests describe what landed.** `Put` hashes the same stream it writes, so the
digest cannot disagree with the bytes. `Stat` and `List` leave `Digest` empty
because filling it would mean reading every object on every listing; call
`Digest` when a listing needs one.

**History is bounded and private.** When `MaxBackups` is positive, `BackupRoot`
is required on the same filesystem. Before replacement, the current regular
file is hard-linked into `BackupRoot/sha256(object-name)/1`, with higher numeric
slots holding older versions. Snapshot and rotation are best effort: a failure
skips that backup and never falls back to moving the accepted object away.
Backup files are outside the caller's object-key namespace and do not appear in
`List`; operators set their directory permissions through `DirMode` and owned
parent directories.

**There is no default extension allowlist.** `AllowedExt` nil means no
restriction. A store exposed to untrusted uploads must set one; the extensions
that make sense depend entirely on what the consumer stores, and a default here
would be one consumer's opinion baked into a general package.

## LocalPath

`FS` implements `LocalPath`, which returns the absolute path of a stored object.

It is an escape hatch for consumers that must hand a path to an external process
that reads the file itself. It is deliberately not part of `Store`, so a
consumer that needs it declares the coupling in its own type assertion rather
than forcing every backend to be a filesystem.

## Migrating from 0.1 to 0.2

This configuration and listing change is breaking while the module is below
v1. A consumer using `MaxBackups > 0` must provision and configure a distinct,
same-filesystem `BackupRoot` before constructing the store. It may also provide
an operator-owned `TempRoot` and cleanup-error callback for capacity accounting.

Version 0.1 wrote generated backups into `Root` as `name.bakN`, where they were
indistinguishable from valid object names and appeared in `List`. Version 0.2
does not migrate or delete those files. Every existing `.bakN` or temp-looking
key under `Root` remains an ordinary object with unchanged bytes and listing
behavior. New replacements write history only into the private hashed backup
tree. Operators may reconcile legacy objects separately after confirming their
ownership; rollback requires draining writers and restoring the previous binary
and configuration, and does not move or delete either legacy or private files.
