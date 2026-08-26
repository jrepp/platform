# How this repository is consumed, and what must be proven first

This repository is about to become a dependency of other fleet repositories.
Before anything depends on it, the paths by which it is consumed need to be
real, authenticated, and versioned -- not assumed. This document states the
paths, the problems each one has, and the gates that must pass before a consumer
takes the dependency.

**Decided 2026-08-26: this repository is public.** That choice is what makes
the paths below workable, and the reasoning is kept under *Options* so the cost
of revisiting it is visible. Everything else stays unproven until its gate
records a pass.

## Two paths, very different costs

The repository offers two kinds of thing, and they are consumed by mechanisms
with almost nothing in common:

| Consumed as | Fetched by | Needs a credential in the consumer |
| --- | --- | --- |
| Reusable workflow | GitHub Actions | **No** -- public workflows are callable by any repository |
| Go module | the public module proxy | **No** -- and the checksum database makes it tamper-evident |

Both paths are credential-free because the repository is public. What follows
records why that mattered, because the private version of this table is what
nearly happened.

That asymmetry is the whole design problem, and it is easy to miss because both
look like "just depend on platform".

**Sharing CI configuration from a private repository costs nothing.** The
calling workflow's built-in token can read a sibling private repository's
workflow file, provided this repository's Actions access is opened to the owner.
No secret is stored anywhere.

**Sharing Go code from a private repository costs a credential per consumer.**
`go` fetches modules over git; a private module cannot come from the public
proxy; every consumer's CI therefore needs an authenticated git credential
before it can compile.

This matters more than it first appears. The hosting design's entire thrust is
driving stored credentials toward zero -- federation for CI, no static tokens.
Requiring every fleet service to store a credential *in order to compile* moves
in the opposite direction. A shared library that makes the fleet less credential-
free than before has not obviously paid for itself.

## The private module problem, precisely

Three consequences, only the first of which is obvious:

1. **Every consumer needs an authenticated fetch.** `GOPRIVATE=github.com/jrepp/*`
   plus a git credential. A consumer's own `GITHUB_TOKEN` does **not** grant
   access to a different private repository, so the built-in token is not enough.

2. **The public module proxy cannot serve it.** Every build fetches from GitHub
   directly. Availability now depends on github.com rather than on a cache, and
   builds get slower.

3. **Supply-chain verification is weaker, not stronger.** A private module is
   excluded from the public checksum database, so `go` cannot check that the
   bytes for a given version match what everyone else saw. Public modules are
   tamper-evident through the transparency log; private ones are trusted because
   they came from a repository we control. Privacy costs verifiability here, and
   that is the opposite of the usual intuition.

## Options, weighed

### Make this repository public

Dissolves all three problems at once. The module comes from the proxy, the
checksum database makes it tamper-evident, and no consumer stores anything.
Reusable workflows stop needing an access setting.

The admission rules already guarantee there is nothing here to protect: no
tenant vocabulary, no host details, no credentials, no operational records. A
package that could not be public would, by those rules, not belong here at all.

Precedent on this fleet: `runner-base` is public while `runner` is private, and
`docuchango` is public. Splitting a generic substrate out of a private service
and publishing it is an established move here.

Cost: the code is visible. For an object store and CI definitions that is a
disclosure of technique, not of anything operational.

### Keep it private, mint a GitHub App token per run

The fleet already does exactly this. `t1-hosting`'s native macOS canary workflow
mints a scoped, short-lived installation token to read a sibling private
repository:

```yaml
- uses: actions/create-github-app-token@fee1f7d63c2ff003460e3d139729b119787bc349
  with:
    app-id: ${{ vars.TURBO_OGRE_APP_ID }}
    private-key: ${{ secrets.TURBO_OGRE_APP_PRIVATE_KEY }}
    owner: jrepp
    repositories: runner
    permission-contents: read
```

The same shape, with `repositories: platform`, gives each consumer a token that
is short-lived and scoped to contents-read on one repository. That is a good
credential as credentials go.

Cost: it is still a stored credential -- the App private key -- in every
consumer repository, and it leaves the module outside the checksum database.

### Vendor the dependency

`go mod vendor` in each consumer. Zero credentials at build time and no
availability coupling. Costs update friction, repository bloat, and a dependency
graph that no longer shows what is actually in use.

Worth keeping in mind for a consumer that must build in an environment with no
network access at all, rather than as the default.

**Decided:** vendoring is the fallback, applied per consumer, when a build must
not depend on fetching at all. It is not the default, because the public proxy
plus checksum verification is both cheaper and more verifiable than a vendored
copy nobody re-checks.

## Versioning

The fleet's convention is release-please with `release-type: simple` and a
`VERSION` file (`auth`), with `include-component-in-tag: false` where tags must
stay bare (`runner`). This repository should match it, with one constraint the
other repositories do not have.

**Multiple languages cannot share one version line.** Go packages, Python
packages, and shared tooling change at different rates, and a Python fix that
bumped the repository's only version would tell every Go consumer that something
changed for them.

An earlier draft of this document put the Go module at the repository root and
concluded that Go's tag rules forced a bare `vX.Y.Z` on the whole repository,
with other languages taking prefixed tags around it. Moving Go under `go/`
removes that constraint entirely: a module in a subdirectory resolves by a tag
of exactly `<subdirectory>/vX.Y.Z`, so no language owns the bare tag and every
package gets the same tag shape.

| Contents | Location | Tag |
| --- | --- | --- |
| Go modules | `go/<name>/` | `go/<name>/vX.Y.Z` |
| Python packages | `python/<name>/` | `python/<name>/vX.Y.Z` |
| Node, Java, Rust packages | `<language>/<name>/` | `<language>/<name>/vX.Y.Z` |
| Reusable workflows | `.github/workflows/` | pinned by commit SHA |

Every Go package is its own module rather than a package inside a shared one,
which is what lets a consumer depend on exactly what it uses and take a version
bump only when that package changes.

Reusable workflows are pinned by SHA rather than by tag because a workflow
reference is executed, not linked: a moving tag on something that runs in CI is
a supply-chain hazard, and this fleet already pins actions by digest
(`actions/checkout@d23441a4...`, `actions/setup-go@924ae3a1...`). Consumers of a
Go module pin `<name>/vX.Y.Z` in `go.mod`, where the module system's own
integrity checks apply.

Releases are cut by release-please from conventional commits scoped to the
package. Below `v1`, a breaking change takes a minor bump and the pull request
names the consumers it breaks. At `v1` it takes a major version.

Each package also declares the toolchain versions it supports, and CI proves
every one of them. Dropping a supported version is a breaking change.

## Gates

None of the above is proven. Each gate is cheap, reversible, and produces
evidence rather than a feeling. **No consumer takes a dependency on this
repository until the gate its path relies on has passed.**

| # | Gate | Passes when |
| --- | --- | --- |
| C1 | Workflow consumption | A workflow in another `jrepp` repository calls a reusable workflow here and runs. No access setting is involved now that this repository is public. |
| C2 | Module fetch without credentials | Superseded by the decision to publish. Retained so the reasoning is not lost if the repository is ever made private again. |
| C3 | Module fetch by the chosen path | A consumer's CI runs `go get github.com/jrepp/platform/go/<name>@<tag>` with no credential configured and no `GOPRIVATE`, resolving through the public proxy. |
| C4 | Release produces a resolvable version | release-please cuts `go/<name>/vX.Y.Z`, and `go list -m -versions` reports it from a machine that has never seen this working tree. |
| C5 | Verification posture recorded | Whether the module is covered by the checksum database under the chosen path, stated plainly. If it is not, that is an accepted cost with a named reason, not an omission. |

Current state, observed 2026-08-26:

- **The repository is public**, so C1 no longer depends on an Actions access
  setting: a public repository's reusable workflows are callable by any
  repository. The setting was `none` on every `jrepp` repository inspected,
  which is what a private version of this plan would have had to change first.
- C2's failure case is now unreachable and was never observed, because the
  decision to publish came before the experiment. That is worth stating plainly
  rather than marking the gate passed: the credential requirement for a private
  module is documented from the module system's behaviour, not from a run here.
- The repository has no tags yet, so no version resolves. C4 is open until
  release-please cuts the first one.
- The store package is proposed but unconsumed, which is the right order: the
  extraction can be reviewed on its merits before anything depends on it.

## What must not happen yet

- No consumer adds a `github.com/jrepp/platform/go/...` module to its `go.mod`
  before C3 and C4 pass. A red build in a service repository is a bad way to
  discover that a version does not resolve.
- No repository deletes its own CI in favour of a shared workflow before C1
  passes.
- `tidal` does not drop its copy of the store code before it has actually run
  against this package, per the extraction rules in `AGENTS.md`.
