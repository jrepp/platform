# L0 domain package map

Status: Draft package design, 2026-09-09. The owner selected platform as the home
for shared L0 domain contracts. Package names and API details below are proposed;
no identity provider, registry, access grants or new Go modules are implemented
by this document. Coordination: [RFC-009][rfc] and [L0 domain model][domain].

## Package boundaries

Use three independently versioned Go modules. Keep their types small and useful
to actual service adapters; a shared domain package is not a shared database.
The named initial consumers are auth and hosting-api. Each implementation PR
must include concrete use in both, or keep the abstraction local until it does.

| Proposed module | Shared representation and behavior | Excluded responsibility |
| --- | --- | --- |
| `go/identity` | PrincipalID, principal kind, ExternalIdentityKey (issuer/subject), actor/delegator attribution; validation and exact identity comparison | Token verification, sessions, provider discovery, account linking, principal persistence and lifecycle administration |
| `go/scope` | OrganizationID, ProjectID, EnvironmentID, explicit organization/project/environment scope references; structural validation and exact scope matching | Organization/project CRUD, repository trust, provisioning, implicit inheritance or grant issuance |
| `go/access` | Action identifiers, role and binding references, principal/group subjects, scoped grant/request/decision records and reason vocabulary; bounded policy snapshot contract | Product role defaults, membership queries, token minting, policy refresh transport, approval workflows and HTTP enforcement |

`identity` and `scope` depend only on the standard library. `access` may depend
on their released modules. None imports `store`, a service, a router, a database
driver or another language SDK. Consumers adapt their authoritative state into
these contracts. Avoid a fourth generic IDs module or a monolithic domain module.

```text
identity       scope
    \           /
       access
          |
   service adapters
```

The diagram shows composition: access imports identity/scope; adapters import
only the modules they use. A scope reference carries no permission by itself.

## Mapping the complete L0 domain

| Domain object | Shared package surface | Authoritative state and behavior |
| --- | --- | --- |
| Principal, ExternalIdentity | identity IDs, kind and external key | Auth owns principal records and verified identity linking |
| Organization, Project | scope IDs and references | Hosting registry proposal owns descriptors, lifecycle and relationships |
| RepositoryBinding | ProjectID reference only; provider identifiers stay in adapter | Hosting owns verified repository/owner/ref/workflow trust and migration |
| OrganizationMembership | PrincipalID and OrganizationID references | Auth owns membership state and changes; not a platform membership store |
| Group, GroupMembership | access group subject/reference used by bindings | Auth owns group descriptors, direct membership and organization consistency |
| RoleDefinition, RoleBinding | access versioned action sets, subject, scope and binding references | Auth owns assignment/administration; receiving services define supported actions |
| ApplicationClient | Opaque audience/client constraints in access requests where needed | Provider owns registrations, redirects, flows and attenuation |
| AccessRequest | Requested subject/scope/action data can reuse access types | Auth owns review workflow and authorized resulting mutations |
| Authentication Session | Verified actor reference only | Auth/provider owns cookies, expiry, revocation and session persistence |
| WorkloadGrant | access bounded audience/action/resource/time constraints and attribution | Delegating authority issues it; work supplies attempt limits; receiver verifies it |
| AccessChangeRecord | identity attribution plus access action/decision references | Mutating service owns event envelope, retention, privacy and audit sink |

Group, client, role and grant IDs are owned by access only when its first real
contract needs them. Do not generate every future record or administration method
from this table. A dedicated audit module needs independent evidence of reuse.

## Contract rules before implementation

- IDs are distinct opaque types with explicit validation. Do not equate project
  IDs with repository IDs, names, paths or issuers. Preserve existing IDs through
  service-owned mappings; shared types do not introduce a new issuer or require
  a GCID migration.
- Compare verified issuer/subject pairs exactly under the provider contract.
  Never lowercase, trim, merge by email, or reinterpret a subject to infer
  repository ownership. Hosting retains its dedicated verified claim mapping.
- Invalid/empty scope is an error, not a wildcard or organization-wide grant.
  Environment scope must identify its project. The service verifies parent
  relationships from authoritative state; a caller cannot assert them into truth.
- Distinguish authenticated principal from caller-authored labels. Constructing
  a Go value or decoding JSON does not authenticate anyone. Only verified adapter
  output enters the authorization path; tokens and secrets never enter these types.
- Action names are supplied by the enforcing service. Unknown actions, subject
  kinds, scopes or unsupported security-contract versions cannot produce Allow.
  Existing public refusal strings remain unchanged through adapters.
- Missing, expired or stale policy cannot allow a protected operation. The
  caller supplies time and an authoritative policy revision/freshness bound;
  no hidden network refresh, mutable global cache or implied immediate revocation.
- Define decision outcomes as Allow, Deny and Indeterminate. Both non-Allow
  outcomes block protected actions; adapters preserve denial versus dependency
  outage for diagnostics. These names are proposed, not a new HTTP response API.
- A future pure evaluator consumes an explicit complete policy snapshot and
  verified request context. Role union is narrowed by scope, client and delegation
  ceilings. It must not guess membership, accept opaque wildcard grants or replace
  established channel policy during a structural refactor.

## Implementation ladder

| Work | Small reviewable output | Acceptance / release condition |
| --- | --- | --- |
| C3.7: map and characterize | Exact source/type inventory in auth and hosting; consumer fixtures for identity and scope | Name both adapter call sites and preserve existing identity/refusal behavior |
| C3.8: identity and scope modules | Only value types/validation needed by those fixtures; separate modules and versions | Independent builds; rename/name-reuse, issuer/subject collision, empty/wrong scope and serialization cases |
| C3.9: access contract and adapter parity | Explicit requests/decisions/action scope; keep current service evaluators behind adapters initially | Existing allowed/denied cases and refusal reasons unchanged; forged labels and unknown actions do not gain rights |
| C3.10: admission and consumer adoption | Package release configuration, CI coverage, versioned adapter PRs and consumption evidence | Both consumers run against released packages with workspace off; remove duplicate mechanics only afterward |

An evaluator, full membership projection, delegated administration and new
contributor policy are later C3.2-C3.5 implementation, with their own approvals
and threat/failure fixtures. A representation-only PR does not complete them.
Biohazard can start with configured scope and its current access mechanism;
its browser/Node client uses a service contract, not Go internals. Add Node or
other language packages only when a concrete consumer needs a published format.

## Packaging, quality and compatibility

Create module directories only when implementing a selected slice. Add each to
`go/go.work`, release-please configuration and the release manifest, and add its
CI caller job. The reusable Go checks live in this repository; each module runs
with `GOWORK=off`, formatting, build, vet, race tests, lint, tidy and vulnerability
checks. Current baseline is Go 1.27.1. No framework or provider dependency is
needed for the initial modules.

Use real consumer characterization tests plus a small set of shared vectors
once a wire format exists. Specify serialization/version and unknown-field rules
before exposing a contract; internal Go layout alone is not a public wire schema.
Authorization envelopes reject unknown security-significant fields; additive
non-security metadata follows its separately specified compatibility rules.

Release tags are `go/identity/vX.Y.Z`, `go/scope/vX.Y.Z` and
`go/access/vX.Y.Z`. Access pins released dependency versions; a local workspace
must not hide unavailable module versions. Below v1, breaking changes take a
minor bump and name affected consumers. Public types and evaluators never advance
deployment pins or grant access automatically.

[rfc]: https://github.com/jrepp/t1-hosting/blob/main/docs-cms/rfcs/rfc-009-shared-content-services-and-protocols.md
[domain]: https://github.com/jrepp/t1-hosting/blob/main/docs/shared-content/domain-model.md
