# Shared GitHub Actions workflows

Shared workflows live in this repository. Callers retain triggers, concurrency,
permissions, runner placement and service-specific checks. The initial consumers
are platform's Go modules and hosting-api. No deployment or release credential
is part of the reusable checks.

| Workflow | Inputs | Required checks |
| --- | --- | --- |
| `.github/workflows/go-check.yml` | `module` (default `.`), `lint-config` relative to that module, `runs-on` as a JSON label array | Format, tidy (go.mod and go.sum), build, vet, race tests, golangci-lint and govulncheck |
| `.github/workflows/workflow-check.yml` | `runs-on` as a JSON label array | actionlint and immutable external action/workflow references |

Go checks use Go 1.27.1, `GOWORK=off`, CGO enabled and clang. The caller must
provide a runner with clang, bash and Python available. Hosted Ubuntu is the
default. Fleet labels belong only in callers; public fork checks in platform
remain on hosted runners. The workflows request only contents-read, persist no
checkout credential and inherit no secrets. A workflow failure fails the call.

Platform calls its own workflows by relative path so a PR tests the exact local
workflow change. Other repositories pin the workflow to a published full commit
SHA. Checkout inside the reusable workflow reads the caller's source, so module
paths and linter configuration refer to that caller. Do not checkout platform
over the caller's work or assume a caller has platform's scripts installed.

Publish the platform workflow commit before updating external callers to that
SHA. The producer and caller PRs should record the pinned version and validation
results. Do not point a caller at an unpushed commit, a branch or a mutable tag.
Changes to job/check names also require checking any caller branch-protection
requirements before merge; this change does not edit those settings.

The module support floor changes from Go 1.25 to 1.27.1 for the next store
release. This is a breaking compatibility change under the package policy and
must be released as a pre-v1 minor bump; existing 0.1.0 consumers remain pinned
until they opt into that release. Hosting's toolchain update does not silently
upgrade its store dependency. New L0 modules start on the same Go baseline.

The baseline was verified against the [official Go download manifest](https://go.dev/dl/?mode=json)
on 2026-09-09. Bump the shared workflow, module/workspace directives and local
version files together for a later toolchain update. Per-module release config
validation stays in platform; hosting's own declaration validation stays there.
Existing service-specific artifact publication and release-please wiring also
remain with their owning repositories until a second consumer needs that exact
mechanism.
