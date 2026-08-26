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

**Writes are atomic.** Bytes land in a temporary file beside the destination,
are flushed, and are renamed into place. A reader sees either the previous
object or the complete new one, never a half-written file. A failed write leaves
the previous object intact and no temporary behind.

**Digests describe what landed.** `Put` hashes the same stream it writes, so the
digest cannot disagree with the bytes. `Stat` and `List` leave `Digest` empty
because filling it would mean reading every object on every listing; call
`Digest` when a listing needs one.

**History is bounded.** `MaxBackups` previous versions are kept as `name.bak1`
through `name.bakN`, with `bak1` the most recent. The oldest is deleted rather
than shifted off the end, which is what stops a name written in a loop from
filling the disk.

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
