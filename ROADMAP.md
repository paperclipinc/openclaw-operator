# Roadmap

> Current version: **v0.33.0** -- API `v1alpha1`, tracking toward a stable `v1` release.

This document is the high-level direction. For the exhaustive feature list see [README.md](README.md), and for per-release details see [CHANGELOG.md](CHANGELOG.md).

## Where we are

The operator is feature-complete for single-cluster OpenClaw fleet management. As of v0.33.0:

- **20+ feature areas** shipped end-to-end (declarative CRD, hardened pods, observability, backup/restore, auto-update, HPA, PDB, NetworkPolicy, RBAC, ingress, gateway proxy, sidecars, init containers, self-configure, cluster defaults, workspace seeding, skill packs, runtime deps, Tailscale, Ollama, web terminal, Chromium with anti-bot CDP proxy).
- **Validating + defaulting webhooks** cover field-level invariants and provide actionable warnings on common misconfigurations.
- **Hardened by default**: non-root UID 1000, `readOnlyRootFilesystem`, drop ALL capabilities, seccomp `RuntimeDefault`, default-deny NetworkPolicy, shared PID namespace for zombie reaping.
- **Supply chain**: multi-arch Docker images, Cosign keyless signatures, SBOM attestations, Trivy + gosec in CI, Operator SDK scorecard tests.
- **Distribution**: GHCR images + Helm chart, OperatorHub bundle auto-submitted on release, ArtifactHub indexed.
- **Test coverage**: unit tests for all resource builders, envtest integration suite, e2e suite running on kind on every PR, performance benchmarks.

Open issues: **0**. The backlog from the original community-chart gap analysis (Feb 2026) is fully shipped.

## Shipped

A condensed view -- see README and CHANGELOG for the full set.

### Foundation (v0.5 - v0.8)
CRD with webhooks; full StatefulSet lifecycle; inline + external config; PVC with backup/restore-on-delete; Chromium sidecar; Ingress + TLS + HSTS + rate limiting; default-deny NetworkPolicy; per-instance RBAC; PDB; ServiceMonitor; Helm chart; OperatorHub bundle.

### Hardening & Phase 1-3 backlog (v0.9 - v0.10)
Auto-update with rollback; config merge mode; declarative skills; secret rotation detection; read-only rootfs; auto-generated gateway token auth; SA annotations (IRSA / Workload Identity); CA bundle injection; `fsGroupChangePolicy`; `SecretsReady` condition; provider-aware webhook warnings; config schema warnings; custom init containers; JSON5 config; runtime deps (pnpm, Python/uv).

### Platform breadth (v0.11 - v0.20)
Topology spread constraints; Operator SDK scorecard CI; resource-builder benchmarks; ttyd web terminal sidecar; nginx gateway proxy sidecar; Tailscale tailnet sidecar with persistent state; Chromium `extraArgs`/`extraEnv` and persistent profiles; GitHub-based skill pack resolution; periodic backup CronJob; IRSA / Pod Identity for S3 backups; `restoreFrom` clone/migrate workflow; HTTP `httpGet` probes.

### Maturity & DX (v0.21 - v0.33)
Optional gateway proxy split (`spec.gateway`); OTel Collector sidecar for operator metrics; `OpenClawClusterDefaults` singleton CR; `OpenClawSelfConfig` agent self-modification CR with allowlist policy; declarative plugin installation (`spec.plugins`); workspace `configMapRef` + `additionalWorkspaces` for multi-agent; global registry override; Chromium CDP anti-bot proxy; per-replica PVCs for HPA mode via VolumeClaimTemplates; `spec.podAnnotations`; `spec.availability.runtimeClassName` (gVisor / Kata); `spec.shareProcessNamespace` for zombie reaping (default `true` in v0.33.0).

## Active focus

### Stabilization (v0.34.x - v0.40.x)
- Dogfood `shareProcessNamespace` default flip across deployed fleets; gather production reports.
- Tighten validating webhook coverage for sidecar conflicts and resource-quantity edge cases.
- Reduce reconcile chattiness on high-frequency status updates.
- Extend e2e coverage for upgrade paths (rolling instances across operator minor versions).
- Keep responding to community reports -- none currently open, but the operator now has enough surface area that surprises will surface as adoption grows.

### Documentation & onboarding
- Platform-specific deployment guides are present (`docs/deployment.md`) but uneven in depth -- even out AWS / GCP / on-prem / Talos coverage.
- Worked example for multi-instance deployments (one OpenClawInstance per tenant) using `OpenClawClusterDefaults`.
- Migration recipes (one-line `kubectl patch` snippets) for users moving from the community Helm chart.

## Toward v1

API stability is the v1 gate. The path is `v1alpha1 -> v1beta1 -> v1`, not a direct jump.

### Step 1: ship `v1beta1` alongside `v1alpha1`
- Scaffold conversion webhook (`kubebuilder create webhook --conversion`).
- Implement conversions both directions; round-trip tests.
- Designate `v1beta1` as storage version; mark `v1alpha1` deprecated-but-served.
- Document migration path; add a `kube-storage-version-migrator` job example.
- Required *before* tagging: a 1-2 minor release window with **no API surface changes** so we know the shape is settled.

### Step 2: graduate to `v1`
Tag `v1` once all of the following are true:
- `v1beta1` has been stable for at least 3 minor releases with no breaking changes.
- Independent production reports from at least 3 separate operators (we have one strong report; need more).
- API deprecation policy is published.
- Conformance test suite covers idempotency, upgrade paths, and negative cases.
- Operator binary itself is bumped to `1.0.0` at the same time.

Going to `v1` directly from `v1alpha1` would lock in fields that have weeks of production data rather than months -- once `v1`, removing or renaming a field requires a multi-release deprecation window. `v1beta1` is the cost-of-doing-business stepping stone.

## Future / exploratory

Lower-confidence ideas that aren't actively being worked:

- **Multi-cluster federation** -- one CR managing fleets across clusters; significant scope, no current pull.
- **AI provider health monitoring** -- surface upstream API/model availability as a CR condition.
- **Cost-aware sizing** -- recommend or auto-tune resource requests/limits from observed usage.
- **Sandboxed runtimes** -- first-class gVisor / Kata recipes (the field exists today; what's missing is verified Chromium-under-gVisor support and an example deployment).
- **`kubernetes-sigs/agent-sandbox` as deployment alternative** -- opt-in via a future `spec.deployment.strategy: Sandbox` field, sitting alongside the default StatefulSet. The unique wins are `SandboxWarmPool` (multi-tenant SaaS provisioning latency) and native hibernation with shutdown policies (idle-cost optimization). **Gated on upstream graduation**: trigger is when [Epic #740](https://github.com/kubernetes-sigs/agent-sandbox/issues/740) lands and a release ships the `v1beta1` API. Today the upstream is still `v1alpha1` with active breaking changes; stacking our `v1alpha1` on theirs would compound API instability.
- **`OpenClawInstance` templates / presets** -- opinionated starting points for common LLM provider setups, layered on top of `OpenClawClusterDefaults`.

If any of these become priorities, they get promoted into Active focus with concrete scope.
