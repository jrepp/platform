# How this repository is consumed, and what must be proven first

This repository is about to become a dependency of other fleet repositories.
Before anything depends on it, the paths by which it is consumed need to be
real, authenticated, and versioned -- not assumed. This document states the
paths, the problems each one has, and the gates that must pass before a consumer
takes the dependency.

Nothing here is settled. Treat every path as unproven until its gate below
records a pass.

## Two paths, very different costs

The repository offers two kinds of thing, and they are consumed by mechanisms
with almost nothing in common:

| Consumed as | Fetched by | Needs a credential in the consumer |
| --- | --- | --- |
| Reusable workflow | GitHub Actions, using the caller's own `GITHUB_TOKEN` | **No** |
| Go module | `go` via git over HTTPS | **Yes**, while this repository is private |

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
GitHub access at all, rather than as the default.

## Versioning

The fleet's convention is release-please with `release-type: simple` and a
`VERSION` file (`auth`), with `include-component-in-tag: false` where tags must
stay bare (`runner`). This repository should match it, with one constraint the
other repositories do not have.

**Go pins the tag format.** A module whose path is `github.com/jrepp/platform`
and whose `go.mod` sits at the repository root can only be versioned by bare
`vX.Y.Z` tags. That is not a preference; the module system will not resolve
anything else.

**Multiple languages cannot share one version line.** Go packages, Python
packages, and shared tooling change at different rates, and a Python fix that
bumps the repository's only version would tell every Go consumer that something
changed for them. The layout that survives:

| Contents | Location | Tag |
| --- | --- | --- |
| Go module | repository root | `vX.Y.Z` |
| Python packages | `python/<name>/` | `python-<name>-vX.Y.Z` |
| Reusable workflows | `.github/workflows/` | pinned by commit SHA |

Reusable workflows are pinned by SHA rather than by tag because a workflow
reference is executed, not linked: a moving tag on something that runs in CI is
a supply-chain hazard, and this fleet already pins actions by digest
(`actions/checkout@d23441a4...`, `actions/setup-go@924ae3a1...`). Consumers of
the Go module pin `vX.Y.Z` in `go.mod`, where the module system's own integrity
checks apply.

Below `v1`, a breaking change takes a minor bump and the pull request names the
consumers it breaks. At `v1` it takes a major version.

## Gates

None of the above is proven. Each gate is cheap, reversible, and produces
evidence rather than a feeling. **No consumer takes a dependency on this
repository until the gate its path relies on has passed.**

| # | Gate | Passes when |
| --- | --- | --- |
| C1 | Actions access | With this repository's Actions access opened to the owner, a workflow in another private `jrepp` repository calls a reusable workflow here and runs, using only its own `GITHUB_TOKEN`. With access closed, the same call fails. Both directions observed. |
| C2 | Module fetch without credentials | A consumer's CI runs `go get github.com/jrepp/platform@vX.Y.Z` with no credential configured. **Expected to fail while private.** Recording the failure is the point: it proves the credential is genuinely required rather than assumed. |
| C3 | Module fetch by the chosen path | The same fetch succeeds by whichever path is decided -- public repository, or an App-minted token scoped to contents-read on this repository alone. |
| C4 | Release produces a resolvable version | release-please cuts `vX.Y.Z`, and `go list -m -versions github.com/jrepp/platform` reports it from a machine that has never seen this working tree. |
| C5 | Verification posture recorded | Whether the module is covered by the checksum database under the chosen path, stated plainly. If it is not, that is an accepted cost with a named reason, not an omission. |

Current state, observed 2026-08-26:

- Actions access on every `jrepp` repository inspected: `none`. C1 has not been
  attempted, and no cross-repository workflow call can succeed today.
- This repository has no tags, so `go list -m -versions github.com/jrepp/platform`
  reports no versions. A consumer today would resolve a pseudo-version from a
  branch, which is not a dependency anyone should ship against.
- The store package is proposed but unconsumed, which is the right order: the
  extraction can be reviewed on its merits before the consumption path is settled.

## What must not happen yet

- No consumer adds `github.com/jrepp/platform` to its `go.mod` before C3 and C4
  pass. A red build in a service repository is a bad way to discover a
  credential gap.
- No repository deletes its own CI in favour of a shared workflow before C1
  passes.
- `tidal` does not drop its copy of the store code before it has actually run
  against this package, per the extraction rules in `AGENTS.md`.
