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
  of that exact shape; a bare `vX.Y.Z` is refused with
  `unknown revision <package-path>/vX.Y.Z`.
- At major version 2 and above a Go module path carries the suffix
  (`module github.com/jrepp/platform/go/<name>/v2`) while the tag prefix stays
  the subdirectory path (`go/<name>/v2.0.0`). The directory does not move.
- Build each Go module with `GOWORK=off`, the way CI does. `go/go.work` is found
  by walking up from a module directory, so without it a build runs in workspace
  mode and can mask a dependency a consumer would be missing.
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

## Go discipline

The linter configuration in `.golangci.yml` is the contract, not a suggestion.
Two rules about changing it:

- **Never raise a complexity threshold to make code pass.** `gocognit` and
  `gocyclo` sit near the tool's defaults on purpose. A threshold set to whatever
  the current worst function scores measures nothing and can only ever be
  raised. Split the function instead.
- **Suppress with a reason or not at all.** A `//nolint` directive states which
  linter and why on the same line. A bare `//nolint` is a silent exception and
  will be asked about in review.

Race detection is not optional. Every package's tests run under `-race` in CI,
because a data race is exactly the failure that survives review, `go vet`, and a
green test run.

`govulncheck` runs on every change, matching the rest of the fleet.

## Validation

Each package validates the way CI does, from its own directory, with the
workspace off so the module builds as a consumer receives it:

```sh
GOWORK=off go build ./... && \
GOWORK=off go vet ./... && \
GOWORK=off go test -race ./... && \
GOWORK=off golangci-lint run --config ../../.golangci.yml ./...
```
