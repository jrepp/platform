# platform Agent Instructions

## Scope

- Shared packages and tooling for fleet services. Library only; no services, no
  deployment, no host state.
- Placement, pins, and operational records belong in `jrepp/t1-hosting`.
- This repository is public. Nothing that could not be public may be added to
  it, including host names, inventory, credentials, and operational records.
- Keep changes ASCII unless a file already uses Unicode.
- Do not put model or vendor branding in commits, pull requests, code comments,
  or documentation.

## Layout

One directory per language, one package per directory, each package its own
independently released unit.

```text
go/<name>/     a Go module: module github.com/jrepp/platform/go/<name>
python/<name>/ a Python package
node/<name>/   a Node package
java/<name>/   a Java artifact
rust/<name>/   a Rust crate
```

Every Go package is its own module rather than a package inside a shared one.
That is what lets a consumer depend on exactly what it uses and take version
bumps only when that package changes. Add the module to `go/go.work` so local
development across modules keeps working.

Do not scaffold a language directory before a package needs it.

## Admission rules

A package belongs here only if two services need it and it knows nothing about
either. Before adding one, name both consumers in the pull request.

- **No tenant vocabulary.** No repository names, no game concepts, no media type
  defaults. If a default would encode one consumer's opinion, leave it unset and
  make the consumer supply it.
- **No framework.** Standard library first. No router, no logging framework, no
  configuration system.
- **Interfaces at the seam.** Export the interface consumers depend on; keep
  implementations behind it.
- **Explicit lifecycles.** Anything starting concurrent work exposes start and
  close. Nothing starts work as a side effect of another call.
- **Cancellation on anything that can block or reach a network.**

## Versioning and support

- Every package is released on its own version line, tagged
  `<package-path>/vX.Y.Z`. A Go module in a subdirectory resolves only by a tag
  of that exact shape.
- Releases come from conventional commits through release-please. Scope the
  commit to the package: `feat(store):`, `fix(store):`.
- Each package declares its supported toolchain versions in its own manifest,
  and CI proves every one of them. Testing the newest and assuming the rest is
  not a support claim.
- Dropping a supported toolchain version is a breaking change and takes the
  version bump that implies.
- Below `v1`, say in the pull request which consumers a change breaks and
  confirm they can move.

## Extraction rules

Most packages here arrive by extraction from a service rather than being written
fresh.

- Port the original's tests with the code. They are the evidence the extraction
  preserved behaviour, and they are worth more than tests written fresh against
  the new shape.
- When the port changes behaviour, say so explicitly in the commit and the pull
  request, with the reason. A silent behaviour change during a refactor is the
  hardest kind of bug to find later.
- Leave the original in place until a consumer has actually run against the
  extracted package. Delete it in a separate change.

## Validation

Each package validates the way CI does, from its own directory. For Go:

```sh
go build ./... && go vet ./... && go test -race ./...
```
