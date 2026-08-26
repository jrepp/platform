# platform Agent Instructions

## Scope

- Shared Go packages for fleet services. Library only; no binaries, no
  deployment, no host state.
- Placement, pins, and operational records belong in `jrepp/t1-hosting`.
- Keep changes ASCII unless a file already uses Unicode.

## Admission rules

A package belongs here only if two services need it and it knows nothing about
either. Before adding one, name both consumers in the pull request.

- **No tenant vocabulary.** No repository names, no game concepts, no media
  type defaults. If a default would encode one consumer's opinion, leave it
  unset and make the consumer supply it.
- **No framework.** Standard library only. No router, no logging framework, no
  configuration system.
- **Interfaces at the seam.** Export the interface consumers depend on; keep
  implementations behind it.
- **Explicit lifecycles.** Anything starting a goroutine exposes `Start(ctx)`
  and `Close()`. Nothing starts work as a side effect of another call.
- **Context on anything that can block or reach a network.**

## Extraction rules

Most packages here arrive by extraction from a service rather than being
written fresh.

- Port the original's tests with the code. They are the evidence that the
  extraction preserved behaviour, and they are more valuable than new tests
  written against the new shape.
- When the port changes behaviour, say so explicitly in the commit and the pull
  request, with the reason. A silent behaviour change during a refactor is the
  hardest kind of bug to find later.
- Leave the original in place until a consumer has actually run against the
  extracted package. Delete it in a separate change.

## Compatibility

Consumers pin versions. A breaking change to an exported name takes a major
version, not a careful commit message. Below `v1`, say in the pull request which
consumers a change breaks and confirm they can move.

## Validation

```sh
go build ./... && go vet ./... && go test -race ./...
```
