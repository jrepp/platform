# platform

Shared Go packages for jrepp.com fleet services. Library only: this repository
builds no binary and runs nowhere.

It exists so that fleet services can share mechanics without depending on each
other. `hosting-api` is the platform that hosts tenants; `tidal-go` is one of
them. A tenant must be buildable without the platform's repository, so code they
both need lives here rather than in either.

Rationale and the rules a dependent follows:
[hosting/docs/composability.md](https://github.com/jrepp/hosting/blob/main/docs/composability.md).

## Packages

| Package | What it is |
| --- | --- |
| [`store`](store) | A rooted object store with containment guarantees, bounded version history, and content digests |

## What belongs here

A package earns its place by being needed by two services and by knowing nothing
about either. Concretely:

- **No tenant vocabulary.** No repository names, no game concepts, no media
  types. The store moves bytes; what they mean belongs to the consumer.
- **No framework.** `net/http` and the standard library. A package that picks
  the consumer's router picks the consumer's architecture.
- **Interfaces at the seam.** Consumers depend on an interface; the concrete
  implementation sits behind it, so a backend can change without a fork.
- **No global state, and explicit lifecycles.** Anything that starts a goroutine
  exposes `Start` and `Close`. A package that spawns work when you happen to
  call the right method cannot be tested, drained, or embedded twice.

Code that fails any of these belongs in the service that needs it.

## Versioning

`v0` while packages are still moving. A package reaches `v1` once a second
service consumes it in production, and from then on a breaking change takes a
major version rather than a careful commit message.

## Development

```sh
go build ./... && go vet ./... && go test -race ./...
```

Use a `go.work` when developing a change that spans this repository and a
consumer, so it does not need publish-then-consume round trips. CI builds
against published versions, so the workspace never hides a broken dependency.
