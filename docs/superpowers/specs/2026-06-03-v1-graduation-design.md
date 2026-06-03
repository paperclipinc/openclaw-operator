# openclaw-operator: v1 API Graduation Design

- **Status:** Proposed (2026-06-03)
- **Owner:** stubbi (jannes@aqora.io)
- **Repo:** `paperclipinc/openclaw-operator`
- **API group:** `openclaw.rocks`
- **Current state:** `v1alpha1` only, operator at v0.34, live on OperatorHub with real users
- **Reference:** hermes-operator `docs/api-versioning.md` and `docs/conditions.md` (v1-from-day-one stability contract)
- **Scope:** DESIGN ONLY. This document and the companion plan describe how to graduate the API to a stable `v1`. No conversion or controller code is implemented here.

## 0. TL;DR

openclaw-operator has shipped 34 minor releases on `openclaw.rocks/v1alpha1` and is in production via OperatorHub. The alpha label no longer reflects the maturity of the API. This design graduates the three CRDs to a stable `openclaw.rocks/v1` using the standard Kubernetes hub-and-spoke multi-version pattern:

1. Introduce `v1` alongside `v1alpha1`. `v1` becomes the hub type.
2. Because the v0.34 schema is already what we want to freeze, the initial `v1` schema is identical to `v1alpha1`, so conversion is a **trivial structural round-trip** (no field renames). A conversion webhook is still required by Kubernetes whenever a CRD serves more than one version with different `storage`, but the conversion functions themselves are mechanical copies.
3. Ship a release (`vX`, targeted v0.35.0) where `v1` is **served but not storage**; `v1alpha1` stays the storage version. No data moves yet.
4. Run a **storage-version migration** of all existing CRs in the field (re-write each object so its stored form is `v1`), then flip the CRD storage version to `v1`.
5. Mark `v1alpha1` **deprecated** (served, warned), then eventually **served=false**, observing the minimum overlap window.

The three CRD kinds are `OpenClawInstance` (short `oci`), `OpenClawSelfConfig` (short `ocsc`), and `OpenClawClusterDefaults` (cluster-scoped singleton, short `occd`). All three graduate together in lockstep; the API group version is a single surface.

## 1. Context and goals

### Current API surface

| Kind | Scope | Short | Storage version today | Webhooks today |
|---|---|---|---|---|
| `OpenClawInstance` | Namespaced | `oci` | `v1alpha1` | validation (and defaulting) |
| `OpenClawSelfConfig` | Namespaced | `ocsc` | `v1alpha1` | validation |
| `OpenClawClusterDefaults` | Cluster (singleton `cluster`) | `occd` | `v1alpha1` | validation |

`api/v1alpha1/groupversion_info.go` declares `GroupVersion = {Group: "openclaw.rocks", Version: "v1alpha1"}`. There is currently no conversion webhook and no second served version.

### Goals

- **G1:** Promote the public contract to `openclaw.rocks/v1` so the API stability guarantees in the hermes versioning policy apply (no breaking changes for the life of v1.x; breaking changes require v2 + conversion + a 6 month overlap).
- **G2:** Zero downtime and zero data loss for existing CRs already stored in etcd as `v1alpha1`, including objects created via OperatorHub installs in the field.
- **G3:** No forced action by users at upgrade time. `kubectl get oci` keeps working; existing GitOps manifests pinned to `v1alpha1` keep applying until the deprecation window closes, with a clear migration runway.
- **G4:** Keep the operator runnable both locally (in-process, `make run` / single binary) and under OLM on a cluster, without bifurcating the conversion or migration design. (Per repo policy: new subsystems work locally AND cloud-k8s without forking the design.)
- **G5:** Adopt the hermes stability documents as the canonical reference once on v1: publish an openclaw `docs/api-versioning.md` and `docs/conditions.md` analog at the moment v1 becomes storage.

### Non-goals

- **NG1:** No schema redesign. v1 is intentionally a byte-for-byte schema clone of the frozen v0.34 `v1alpha1` shape. Any field cleanup is a separate, later, additive change under v1 or a future v2.
- **NG2:** No new CRD kinds.
- **NG3:** No change to controller reconcile logic beyond what the hub type and conversion require. Builders in `internal/resources/` are untouched.
- **NG4:** This is not a v2. There is no breaking field change. If we discover one is needed, it follows the hermes v2 path, not this graduation.

## 2. Why graduate, and why now

- v0.34 is mature; the `v1alpha1` label scares conservative adopters and some corporate policies refuse alpha CRDs.
- The schema has been stable across many minor releases; we are confident freezing it.
- OperatorHub presence means the cost of a future breaking change is already high, so locking in a stable contract now (and routing all future breaks through a real v2 process) is strictly better than continuing to mutate an "alpha" that users actually depend on.

## 3. Hub-and-spoke model

Kubernetes CRD multi-version requires exactly one **hub** (the in-memory representation the controller reconciles and the type stored in etcd at the storage version) and any number of **spokes** that convert to and from the hub.

Decision: **`v1` is the hub.** `v1alpha1` becomes a spoke that implements `ConvertTo(hub)` / `ConvertFrom(hub)`.

Rationale for v1-as-hub (not v1alpha1-as-hub):
- The controller and all `internal/resources/` builders should operate on the long-lived type. The alpha type is the one slated for removal, so it should be the spoke that carries conversion glue, not the controller's working type.
- This matches the hermes "v1 is hub and storage" end state. After the storage flip, openclaw looks exactly like hermes: `v1` hub+storage, with a (now legacy) `v1alpha1` spoke that will be retired.

Concretely (described, not implemented here):
- New `api/v1/` package with `openclawinstance_types.go`, `openclawselfconfig_types.go`, `openclawclusterdefaults_types.go`, `groupversion_info.go` (`Version: "v1"`), and generated deepcopy.
- `api/v1` types carry `+kubebuilder:object:root=true` and, at the appropriate phase, `+kubebuilder:storageversion`.
- `api/v1alpha1` types implement `conversion.Convertible` (`ConvertTo`, `ConvertFrom`) against the `v1` hub. Since schemas are identical, each converter is a field-by-field copy; deepcopy-generated structs make this mechanical. A round-trip fuzz test (`TestConvertRoundTrip`) asserts `v1alpha1 -> v1 -> v1alpha1` is lossless for all three kinds.
- The controller's manager registers both schemes; reconcilers switch their working type to `api/v1`.

## 4. Storage version: which, and when to flip

| Phase | Served versions | Storage version | Notes |
|---|---|---|---|
| Today (v0.34) | `v1alpha1` | `v1alpha1` | single version |
| Phase A (v0.35.0) | `v1alpha1`, `v1` | `v1alpha1` | `v1` served-not-storage; conversion webhook live; **no data moves** |
| Phase B (v0.35.x) | `v1alpha1`, `v1` | `v1alpha1` -> migrate -> `v1` | run storage-version migration of existing CRs, THEN flip `storage: true` to `v1` |
| Phase C (v0.36.0) | `v1alpha1` (deprecated), `v1` | `v1` | `v1alpha1` marked `deprecated: true` with a `deprecationWarning` |
| Phase D (>= Phase C + 6 months) | `v1` (`v1alpha1` served=false) | `v1` | apply against `v1alpha1` is rejected with a pointer to the conversion path |
| Phase E (later) | `v1` | `v1` | `v1alpha1` removed from the CRD entirely |

The storage flip in Phase B is the single most delicate step. The rule (enforced in the plan's checklist) is: **never flip the storage version until every stored object has been re-encoded at the new version.** Flipping first and migrating later leaves objects in etcd whose stored bytes are the old version with the old version dropped from `storedVersions`, which is unrecoverable without the old schema. Migrate, confirm `status.storedVersions == ["v1"]` on the CRD, then flip.

## 5. Conversion webhook and OLM cert implications

### Conversion webhook

A conversion webhook is mandatory the moment the CRD serves two versions and they can differ in stored form. Even though our conversion is a trivial round-trip, the API server still calls the webhook to translate between served and stored versions (for example, a `kubectl get oci.v1` while the stored object is still `v1alpha1` during Phase A/B).

- `config/crd/patches/webhook_in_openclaw*.yaml` add `spec.conversion.strategy: Webhook` referencing the operator's service.
- `config/crd/patches/cainjection_in_openclaw*.yaml` wire cert-manager CA injection into the CRD's `conversion.webhook.clientConfig.caBundle`.
- The conversion endpoint is served by the same webhook server the validating/defaulting webhooks already use; it is one more path on the existing service, so no new Service/port is introduced.

### Local vs OLM cert story (G4)

- **Local / `make run`:** controller-runtime's envtest and the local webhook server use a self-signed cert generated into a temp dir; the kustomize overlay for local dev injects that CA. The conversion path is exercised by the same self-signed cert as the existing admission webhooks, so local development gains nothing new to configure.
- **Plain manifests / Helm:** cert-manager issues the serving cert and injects the CA into the `ValidatingWebhookConfiguration`, the `MutatingWebhookConfiguration`, AND now the CRD `conversion` block. The Helm chart's cert templates must add the CRD conversion caBundle target. (Helm RBAC Sync and the chart's cert wiring stay in sync via `make manifests` + `make sync-chart-crds`.)
- **OLM / OperatorHub:** OLM does NOT use cert-manager. OLM provisions and rotates serving certs for webhooks itself and injects the CA into webhook configs that the CSV declares under `spec.webhookdefinitions`. For conversion webhooks specifically, OLM supports `type: ConversionWebhook` webhook definitions in the CSV; OLM then owns the cert and patches the CRD's `conversion.webhook.clientConfig.caBundle`. This means:
  - The CSV gains a `webhookdefinitions` entry of `type: ConversionWebhook` listing the converted CRDs (`openclawinstances`, `openclawselfconfigs`, `openclawclusterdefaults`) and the conversion path/port.
  - We must NOT also ship a cert-manager `Certificate` in the OLM bundle for the conversion webhook; OLM and cert-manager would fight over the caBundle. The bundle build (`make bundle`) strips cert-manager objects; OLM injects its own. This is the standard divergence between the Helm/manifest install (cert-manager) and the OLM install (OLM-managed certs), and it is already how the existing admission webhooks are handled.
  - OLM requires the conversion webhook's CRD to be **owned** by the CSV. All three CRDs are already owned CRDs in the CSV, so this is additive.

### Failure-mode note

A conversion webhook sits on the read/write path of every CR. If it is down, `kubectl get oci` fails for the affected versions. Mitigations in the plan: the conversion handler has no external dependencies (pure in-process struct copy), the webhook server is part of the operator Deployment with a readiness probe, and during the trivial-round-trip phase there is no logic that can throw beyond a nil guard. We also keep `v1alpha1` as storage until migration completes so a webhook outage in Phase A degrades reads of the `v1` view but never blocks writes to the still-native `v1alpha1` storage.

## 6. Storage-version migration for existing CRs

Existing clusters have `OpenClawInstance` / `OpenClawSelfConfig` / `OpenClawClusterDefaults` objects stored as `v1alpha1`. After Phase A the CRD still stores `v1alpha1`; before flipping to `v1` (Phase B) every stored object must be re-encoded as `v1`.

Two mechanisms, both designed (operator picks one per environment; the plan recommends the StorageVersionMigration CR where available and the one-shot Job as the universal fallback, satisfying G4):

1. **`StorageVersionMigration` (storage-version-migrator).** On clusters running the upstream kube-storage-version-migrator (GKE, OpenShift, and OLM-managed clusters where it is available), create one `migration.k8s.io/v1alpha1` `StorageVersionMigration` per resource. The controller lists and re-writes (no-op update) every object, causing the API server to re-encode it at the current storage version. We sequence this AFTER the webhook is live (Phase A) and BEFORE the storage flip.
2. **One-shot migrate Job (universal fallback, also the local story).** A `Job` (image: the operator binary with a `migrate` subcommand, or `kubectl get ... -o yaml | kubectl replace`) that lists all CRs across namespaces and issues a no-op patch to each, forcing re-encode. This works on any cluster including local kind/minikube and is what `make run` / single-binary local installs use. The Job carries its own minimal RBAC (`get`, `list`, `patch` on the three resources) added via `+kubebuilder:rbac` markers so `config/rbac` and the Helm chart RBAC stay in sync.

After either path completes, verify on the CRD:
- `status.storedVersions` lists ONLY `v1` (the API server prunes old entries once no stored object uses them). If `v1alpha1` lingers in `storedVersions`, an object was missed; do not flip.

Only then set `v1` `storage: true` and `v1alpha1` `storage: false` (Phase B).

For OperatorHub installs, the migration is bundled as a one-shot operation tied to the v0.35.x release: the operator runs the migration on startup (idempotent; gated by a CRD-level annotation marking completion) so field installs migrate without the cluster admin running a manual Job. This keeps the field-upgrade hands-off (G3).

## 7. CRD served / deprecated version lifecycle

Per-version markers on the CRD (driven by kubebuilder markers on the Go types, regenerated via `make manifests` + `make sync-chart-crds` + `make sync-bundle-crds`):

- Phase A: `v1alpha1 {served: true, storage: true}`, `v1 {served: true, storage: false}`.
- Phase B: after migration, `v1alpha1 {served: true, storage: false}`, `v1 {served: true, storage: true}`.
- Phase C: `v1alpha1 {served: true, storage: false, deprecated: true, deprecationWarning: "openclaw.rocks/v1alpha1 is deprecated; use openclaw.rocks/v1. See https://github.com/paperclipinc/openclaw-operator/blob/main/docs/api-versioning.md"}`.
- Phase D: `v1alpha1 {served: false, storage: false, deprecated: true}` (kept in the CRD so stored-version bookkeeping and any straggler reads return a clear error).
- Phase E: `v1alpha1` removed from `spec.versions` entirely; CRD serves only `v1`.

The `deprecated`/`deprecationWarning` fields cause `kubectl` to print a warning on every `v1alpha1` operation in Phase C, giving users a loud, non-fatal nudge before Phase D makes it fatal.

## 8. OperatorHub bundle and channel implications

- **Owned CRDs in the CSV:** all three CRDs are already owned; the CSV `spec.customresourcedefinitions.owned` entries gain `version: v1` and the conversion webhook definition (Section 5).
- **`olm.skipRange` / `replaces`:** the v0.35.0 bundle `replaces` v0.34.x as usual. No new channel is created; openclaw uses a single `stable` channel (confirmed in `bundle/metadata/annotations.yaml`). Graduating the API version does NOT require a new OLM channel, because the operator semver is independent of the API group version (the hermes policy explicitly decouples these three surfaces: API group version, operator image semver, chart semver).
- **Upgrade ordering on OperatorHub:** OLM upgrades the operator Deployment first (which brings the conversion webhook online), then applies the new CRD with both versions. Because the new CSV's CRD still has `v1alpha1` as storage in Phase A, OLM's CRD-update safety check (it refuses a CRD update that drops the current storage version) passes. The storage flip to `v1` ships in a later bundle (v0.35.x or v0.36.0) only after migration, so no single bundle both serves a new storage version and leaves stale stored objects.
- **`operator-sdk bundle validate`** must pass after the CSV gains the conversion webhook definition; the plan runs it as a gate.
- **community-operators submission:** the existing CI that submits to RedHat community-operators-prod (see repo history, e.g. #532) carries the updated bundle. No structural change to that pipeline.

## 9. Backward-compatibility guarantees

During and after graduation:
- **Reads:** `kubectl get oci`, `kubectl get oci.v1alpha1.openclaw.rocks`, and `kubectl get oci.v1.openclaw.rocks` all return the same object, transparently converted, through Phase D.
- **Writes:** existing manifests and GitOps repos pinned to `apiVersion: openclaw.rocks/v1alpha1` keep applying cleanly through Phase C (with a deprecation warning) and stop working only at Phase D, after >= 6 months of overlap.
- **No field semantics change.** Because v1 is a schema clone, a `v1alpha1` object and its `v1` projection are field-identical. Defaulting and validation webhooks apply identically to both served versions.
- **Conditions and reason codes are preserved.** The status conditions on `OpenClawInstance` (the `Ready` rollup and subsystem conditions visible in the printer columns) carry over unchanged; once on v1 they become part of the published stability contract, mirroring hermes `docs/conditions.md`.
- **Short names** (`oci`, `ocsc`, `occd`) are preserved; removing a short name would be a breaking change per the hermes policy.

## 10. Phased rollout summary

The end-to-end sequence (one operator release stream, no merges forced on users):

1. **vX = v0.35.0 (additive):** add `api/v1` hub types, `v1alpha1` spoke conversion, conversion webhook plumbing (kustomize patches + Helm cert wiring + CSV ConversionWebhook), CSV owned-CRD `v1` entries. `v1` served-not-storage. Round-trip and envtest conversion tests green. No data moves.
2. **Migrate (v0.35.x):** ship the storage-version migration (StorageVersionMigration CRs where available; bundled idempotent startup migration for OperatorHub; one-shot Job for local/manual). Confirm `storedVersions == ["v1alpha1]` cleared to allow `["v1"]` after flip.
3. **Flip storage (v0.35.x or v0.36.0):** set `v1` `storage: true`. Only after migration confirms no stored `v1alpha1` objects remain.
4. **Deprecate v1alpha1 (v0.36.0):** `deprecated: true` + warning. Publish `docs/api-versioning.md` and `docs/conditions.md` (the openclaw analogs of the hermes references) at this point: openclaw's contract is now "v1 is the stable surface."
5. **Stop serving v1alpha1 (>= step 4 + 6 months):** `served: false`.
6. **Remove v1alpha1 (later):** drop from `spec.versions`.

## 11. Risks and rollback

| Risk | Likelihood | Impact | Mitigation / rollback |
|---|---|---|---|
| Conversion webhook down blocks CR reads | Low | High | Pure in-process copy, no external deps; readiness-gated; keep `v1alpha1` as storage until migration done so writes never depend on conversion in Phase A. Rollback: redeploy prior operator image; CRD still stores `v1alpha1`. |
| Storage flipped before migration complete (data loss) | Low | Critical | Hard gate: verify CRD `status.storedVersions` contains no `v1alpha1` before flipping. The flip is a separate PR/release from the migration, never combined. Rollback is NOT possible once old schema is gone, hence the gate. |
| OLM and cert-manager both manage conversion caBundle | Medium | Medium | Bundle build strips cert-manager certs; OLM owns conversion certs via `ConversionWebhook` definition. Helm/manifest install uses cert-manager only. Documented divergence, mirrors existing admission webhook handling. |
| OperatorHub refuses CRD update (drops current storage version) | Low | Medium | Phase A keeps `v1alpha1` as storage, so the first bundle never drops the current storage version. Storage flip ships only after migration. |
| Round-trip conversion is lossy (some field not copied) | Low | High | Schemas identical by construction; `TestConvertRoundTrip` fuzz test in `api/` gates every change. CI conversion test in envtest. |
| Users hard-pin `v1alpha1` and miss deprecation | Medium | Low | 6 month overlap, `kubectl` deprecation warnings in Phase C, release-note callouts, OperatorHub description note. |
| Local-only installs lack storage-version-migrator | Medium | Low | One-shot migrate Job + idempotent startup migration both work without any cluster add-on (G4). |

**Rollback posture overall:** every phase before the storage flip (Phase B) is fully reversible by redeploying the prior operator image, because `v1alpha1` remains the storage version and no data has moved. The storage flip is the point of no return; it is gated on a verified-complete migration and shipped as its own release so it can be held back independently if migration verification fails on any cluster.

## 12. Reference

- hermes-operator `docs/api-versioning.md`: the stability contract openclaw adopts once on v1 (non-breaking vs breaking change lists, the v2 + 6 month overlap rule, decoupled API/image/chart semver).
- hermes-operator `docs/conditions.md`: the shape of the conditions catalogue openclaw publishes at Phase C.
- Kubernetes CRD versioning: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
- Kubebuilder multiversion / conversion: https://book.kubebuilder.io/multiversion-tutorial/conversion.html
- kube-storage-version-migrator: https://github.com/kubernetes-sigs/kube-storage-version-migrator
- OLM webhook (incl. ConversionWebhook) support: https://olm.operatorframework.io/docs/advanced-tasks/adding-admission-and-conversion-webhooks/
