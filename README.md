# platform

Shared packages and tooling for jrepp.com fleet services. Library only: this
repository builds no service and runs nowhere.

It exists so fleet services can share mechanics without depending on each other.
`hosting-api` is the platform that hosts tenants; `tidal-go` is one of them. A
tenant must be buildable without the platform's repository, so code they both
need lives here rather than in either.

Public, deliberately. The admission rules below guarantee there is nothing here
to protect, and being public means consumers fetch from the public module proxy
with no credential and with the checksum database making releases
tamper-evident. A package that could not be public does not belong here.

## Layout

One directory per language, one package per directory, each package released on
its own version line.

```text
go/            Go modules, one per package
python/        Python packages
node/          Node packages
java/          Java artifacts
rust/          Rust crates
.github/       Shared workflows and this repository's own CI
```

Language directories appear when a package needs them; empty ones are not
scaffolded ahead of use.

## Packages

| Package | Language | Version | Supported toolchains |
| --- | --- | --- | --- |
| [`go/store`](go/store) | Go | unreleased | 1.25.x, 1.26.x |

## Versioning

Every package is versioned independently. A fix to a Python package must not
tell a Go consumer that something changed for them, which is what a single
repository-wide version line would do.

Releases are cut by release-please from conventional commits, tagged
`<package-path>/vX.Y.Z` -- `go/store/v0.1.0`. That format is not a preference
for Go: a module in a subdirectory can only be resolved by a tag of exactly that
shape, and giving every language the same shape keeps one convention instead of
two.

Below `v1` a breaking change takes a minor bump and the pull request names the
consumers it breaks. At `v1` it takes a major version.

Reusable workflows are pinned by commit SHA rather than by tag, because a
workflow reference is executed rather than linked, and a moving tag on something
that runs in CI is a supply-chain hazard. This repository pins its own actions
the same way.

## Support matrix

Each package declares the toolchain versions it supports, and CI proves every
one of them rather than testing the newest and assuming the rest. The declared
floor is the package's own minimum -- `go.mod`'s `go` directive, a Python
`requires-python`, a crate's `rust-version` -- so a consumer can read the
support claim from the package itself and not only from this table.

The intended ranges as languages arrive:

| Language | Range | Rationale |
| --- | --- | --- |
| Go | current and previous minor | Matches Go's own support window |
| Python | 3.12 through 3.14 inclusive | Covers the versions in use across the fleet |
| Node | active LTS lines | |
| Java | current and previous LTS | |
| Rust | stable, plus a declared minimum supported version | |

Dropping a supported version is a breaking change and takes the version bump
that implies.

## What belongs here

A package earns its place by being needed by two services and by knowing
nothing about either:

- **No tenant vocabulary.** No repository names, no game concepts, no media
  types. The store moves bytes; what they mean belongs to the consumer.
- **No framework.** Standard library first. A package that picks the consumer's
  router picks the consumer's architecture.
- **Interfaces at the seam.** Consumers depend on an interface; the concrete
  implementation sits behind it, so a backend can change without a fork.
- **No global state, and explicit lifecycles.** Anything that starts concurrent
  work exposes start and close. A package that spawns work when you happen to
  call the right method cannot be tested, drained, or embedded twice.
- **Nothing that could not be public.**

Code failing any of these belongs in the service that needs it.

## Development

Each package builds on its own, the way CI builds it:

```sh
cd go/store && go build ./... && go vet ./... && go test -race ./...
```

`go/go.work` exists so a change spanning two Go modules can be built locally
without publishing first. CI builds each module separately, where the workspace
does not apply, so it cannot hide a broken dependency.

## Consumption

How consumers depend on this repository, what each path costs, and the gates
that had to pass first: [docs/consumption.md](docs/consumption.md).
