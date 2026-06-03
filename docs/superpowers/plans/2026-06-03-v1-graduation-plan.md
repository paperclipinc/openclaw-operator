# openclaw-operator: v1 API Graduation Migration Plan

- **Status:** Proposed (2026-06-03)
- **Owner:** stubbi (jannes@aqora.io)
- **Companion spec:** `docs/superpowers/specs/2026-06-03-v1-graduation-design.md`
- **Scope:** Implementation plan and checklists. NO code is implemented in this PR; this is the executable runbook for the graduation work that follows.

This plan turns the design into ordered, releasable units of work. Each phase is a separate PR (Conventional Commits, one feature per commit, each ending with the required `Co-Authored-By` trailer). Phases that change API types MUST run `make generate && make manifests && make sync-chart-crds && make sync-bundle-crds` and commit regenerated files. New managed resources (the migration Job RBAC) get `+kubebuilder:rbac` markers and a `make manifests` re-run so `config/rbac` and the Helm chart RBAC stay in sync.

Kinds in scope: `OpenClawInstance` (`oci`), `OpenClawSelfConfig` (`ocsc`), `OpenClawClusterDefaults` (`occd`). All graduate in lockstep.

## Guardrails (apply to every phase)

- No em/en dashes anywhere (ASCII only). Run `grep -rnP '[\x{2013}\x{2014}]'` on changed files; must be clean.
- Use `controllerutil.CreateOrUpdate` for any managed resource (Reconcile Guard CI forbids bare `r.Update`).
- Resource builders stay pure functions in `internal/resources/` with unit tests; controllers in `internal/controller/`.
- After any API type change: `make generate && make manifests && make sync-chart-crds && make sync-bundle-crds` (and `api-docs` if present); commit regenerated output.
- Add `+kubebuilder:rbac` markers for the migration Job's permissions; re-run `make manifests` (Helm RBAC Sync check).
- Validate before each PR: `go build ./...`; `go vet ./...`; `make lint` (best effort); `go test ./internal/resources/... ./api/...`; `make test` (envtest, best effort, CI runs it); helm sync hack scripts; `helm lint`; `operator-sdk bundle validate` if the CSV changed.

## Phase 0: groundwork (this PR, docs only)

- [x] Design spec committed (`specs/2026-06-03-v1-graduation-design.md`).
- [x] This plan committed.
- Outcome: agreed migration path. No code.

## Phase A: introduce v1 hub, v1alpha1 spoke, conversion webhook (target v0.35.0)

Goal: `v1` served-not-storage; conversion webhook live; no data moves.

1. **Scaffold `api/v1`.**
   - `operator-sdk create api --group openclaw --version v1 --kind OpenClawInstance` (and SelfConfig, ClusterDefaults), or hand-add `api/v1/` mirroring `api/v1alpha1/` exactly. Schemas are byte-for-byte clones of the frozen v0.34 `v1alpha1` types.
   - `api/v1/groupversion_info.go`: `Version: "v1"`.
   - Mark `api/v1` types `+kubebuilder:object:root=true`, `+kubebuilder:subresource:status`, and copy printer columns and short names (`oci`, `ocsc`, `occd`) verbatim.
   - Do NOT add `+kubebuilder:storageversion` to v1 yet (it stays on `v1alpha1` in Phase A).
2. **Conversion glue on the spoke.**
   - Implement `ConvertTo(dstRaw conversion.Hub)` / `ConvertFrom(srcRaw conversion.Hub)` on each `api/v1alpha1` kind against the `api/v1` hub. Field-by-field copy (mechanical; identical schemas).
   - Move the reconcilers and `internal/resources/` builder signatures to operate on `api/v1` types (hub). Builders stay pure functions; only the imported type version changes. Unit tests in `internal/resources/` updated accordingly.
3. **Conversion webhook plumbing.**
   - Add `config/crd/patches/webhook_in_openclaw{instances,selfconfigs,clusterdefaults}.yaml` (`spec.conversion.strategy: Webhook`).
   - Add `config/crd/patches/cainjection_in_openclaw*.yaml` for cert-manager CA injection.
   - Register the conversion webhook on the existing webhook server in `cmd/` (same Service/port as admission webhooks).
   - Helm chart: add CRD `conversion.webhook.clientConfig.caBundle` cert wiring to the existing cert-manager templates.
   - CSV: add a `webhookdefinitions` entry of `type: ConversionWebhook` for the three CRDs; ensure cert-manager `Certificate` objects are stripped from the bundle (OLM owns conversion certs).
4. **Regenerate + sync.** `make generate manifests sync-chart-crds sync-bundle-crds`. Commit `config/crd/bases/*`, chart CRDs, bundle CRDs, CSV.
5. **Tests.**
   - `api/` round-trip fuzz test `TestConvertRoundTrip` for all three kinds (`v1alpha1 -> v1 -> v1alpha1` lossless).
   - envtest: create a `v1alpha1` object, read it as `v1`, assert equality; and vice versa.
6. **Validate + PR.** Run the full gate list. PR title `feat(api): introduce openclaw.rocks/v1 served-not-storage with conversion webhook`. Base `main`. Do NOT enable auto-merge.

Exit criteria: cluster serves both versions; `kubectl get oci.v1.openclaw.rocks` works; etcd still stores `v1alpha1`; rollback = redeploy prior image.

## Phase B: migrate stored CRs, then flip storage (target v0.35.x)

Goal: re-encode all stored objects as `v1`, then make `v1` the storage version.

### B1: migration mechanism (ship first, do NOT flip yet)

1. **One-shot migrate Job (universal + local).**
   - Add a `migrate` subcommand to the operator binary (or a Job that does `list + no-op patch` across all namespaces for the three resources).
   - Job RBAC via `+kubebuilder:rbac:groups=openclaw.rocks,resources=openclawinstances;openclawselfconfigs;openclawclusterdefaults,verbs=get;list;patch`. Re-run `make manifests`; commit `config/rbac` and Helm RBAC.
   - Idempotent: gated by a CRD annotation `openclaw.rocks/storage-migration-complete` set after a clean pass.
2. **StorageVersionMigration CRs (where supported).**
   - For clusters with kube-storage-version-migrator, generate one `migration.k8s.io/v1alpha1` `StorageVersionMigration` per resource. Document as the preferred path on GKE/OpenShift/OLM-managed clusters.
3. **OperatorHub hands-off path.** Operator runs the idempotent migration on startup (annotation-gated) so field installs migrate without admin action.

PR: `feat: storage-version migration for openclaw CRs`. No storage flip in this PR.

### B2: storage flip (separate PR/release, gated)

1. Verify on a representative cluster: CRD `status.storedVersions` no longer contains `v1alpha1` after migration (or contains only `v1` once flipped and re-listed). If `v1alpha1` lingers, an object was missed: STOP, re-run migration.
2. Move `+kubebuilder:storageversion` from `api/v1alpha1` to `api/v1`. Regenerate + sync all manifests/chart/bundle.
3. Validate; PR `feat(api)!: flip storage version to openclaw.rocks/v1`. The `!` marks the operationally significant change (not an API break for users).

Exit criteria: `v1` is storage; all objects stored as `v1`; `v1alpha1` still served for compatibility.

POINT OF NO RETURN: after B2 the old schema is no longer the storage form. The gate in B2 step 1 is mandatory.

## Phase C: deprecate v1alpha1 + publish stability docs (target v0.36.0)

1. Mark `api/v1alpha1` `+kubebuilder:deprecatedversion` with a warning pointing at `docs/api-versioning.md`. Regenerate; the CRD gains `deprecated: true` + `deprecationWarning`.
2. Author `docs/api-versioning.md` and `docs/conditions.md` for openclaw, modeled on the hermes references: non-breaking vs breaking change lists, the v2 + 6-month-overlap rule, decoupled API/image/chart semver, and the full `OpenClawInstance` / `OpenClawSelfConfig` / `OpenClawClusterDefaults` conditions catalogue.
3. Release notes and OperatorHub description: announce `v1` as the stable surface, `v1alpha1` deprecated with removal timeline.
4. Validate; PR `feat(api): deprecate openclaw.rocks/v1alpha1; publish v1 stability contract`.

Exit criteria: `kubectl` warns on every `v1alpha1` op; v1 contract published.

## Phase D: stop serving v1alpha1 (>= Phase C + 6 months)

1. Set `api/v1alpha1` `served=false` (`+kubebuilder:unservedversion`). Keep it in the CRD for stored-version bookkeeping.
2. Regenerate + sync. PR `feat(api)!: stop serving openclaw.rocks/v1alpha1`.

Exit criteria: applying `v1alpha1` returns a clear error pointing at the conversion path; only `v1` is served.

## Phase E: remove v1alpha1 (later, optional)

1. Delete `api/v1alpha1/` and its CRD `spec.versions` entry. Drop the conversion glue. Regenerate + sync.
2. PR `chore(api): remove openclaw.rocks/v1alpha1`.

Exit criteria: CRD serves only `v1`; openclaw matches the hermes end state.

## Validation matrix (run per phase, capture into PR notes)

| Check | Command |
|---|---|
| Build | `go build ./...` |
| Vet | `go vet ./...` |
| Lint | `make lint` (best effort) |
| Unit | `go test ./internal/resources/... ./api/...` |
| Envtest | `make test` (best effort; CI runs it) |
| Helm CRD sync | hack sync scripts |
| Helm lint | `helm lint` |
| Bundle | `operator-sdk bundle validate ./bundle` (if CSV changed) |
| Dash scan | `grep -rnP '[\x{2013}\x{2014}]'` on changed files (must be clean) |

## Rollback summary

- Phases A, B1, C: redeploy prior operator image; `v1alpha1` remains storage (A/B1) or served (C). Fully reversible.
- Phase B2 (storage flip): NOT reversible once old schema gone. Gated on verified-complete migration; shipped as its own release so it can be held back if any cluster fails verification.
- Phases D, E: reversible by re-adding `served=true` / re-adding the version, since `v1` remains storage throughout.
