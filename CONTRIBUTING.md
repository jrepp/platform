# Contributing and merging

Create a topic branch from current main, make a bounded change and open a PR.
Run `scripts/check` locally. Package changes name two consumers and follow the
module admission and compatibility rules in [AGENTS.md](AGENTS.md).

Use a Conventional Commit title, including the package scope and `!` when a
change breaks compatibility. The squash commit uses the PR title and body;
release-please reads that resulting commit, not the individual branch commits.

## Merge flow

1. Finish the PR description and mark the PR ready after local validation.
2. Resolve review conversations. Update the branch if main moved, then let CI
   validate that new head. GitHub's Update branch control is enabled.
3. Choose squash auto-merge, or merge once all required checks are green:

   ```sh
   gh pr merge <number> --auto --squash --delete-branch
   ```

Main requires `store / check`, `workflows / check` and `hygiene` from GitHub
Actions, tested against current main. New modules must join the CI workflow and
the required-check policy before their release. Direct pushes, force pushes and
main deletion are blocked. The active ruleset has no bypass actors. Squash is
the only merge method; merged source branches are deleted automatically.

No second-person approval is required, so a solo maintainer can complete a PR.
Review threads still have to be resolved, and anyone reviewing a change can
record feedback. Do not merge unfinished work or use an administrative bypass.
Enabling repository auto-merge makes it available; it does not select existing
PRs for merging automatically.

## Release pull requests

Release-please opens package release PRs and maintains independent package
versions. Do not push release edits directly to main. A PR created with the
default workflow token may need CI started explicitly; if checks are absent,
run the Go workflow on that PR's exact branch:

```sh
gh workflow run go.yml --ref <release-pr-branch>
```

Confirm the required checks belong to the PR head before selecting auto-merge.
Do not waive a missing check. If that branch predates the dispatch-enabled
workflow, update it from main first. No extra stored credential is required for
this manual fallback. Package publication remains the release workflow's job.

## Policy maintenance

Desired repository settings are in [.github/repository-settings.json](.github/repository-settings.json).
The main ruleset is in [.github/main-ruleset.json](.github/main-ruleset.json).
These files describe policy; committing them does not automatically apply it.
Change live settings only under an authorized repository-administration task,
read them back afterward, and keep the files aligned with the effective rules.
Do not create a second overlapping ruleset when updating the existing one.

Shared CI mechanics belong in platform's [reusable workflows](docs/shared-workflows.md).
Callers own their runner selection, credentials, deployments and local contracts.
