# API Reference

## OpenClawInstance (v1alpha1)

**Group**: `openclaw.rocks`
**Version**: `v1alpha1`
**Kind**: `OpenClawInstance`
**Scope**: Namespaced

An `OpenClawInstance` represents a single deployment of the OpenClaw AI assistant in a Kubernetes cluster. The operator watches these resources and reconciles a full stack of dependent objects (StatefulSet, Service, RBAC, NetworkPolicy, storage, and more).

### Print Columns

When listing resources with `kubectl get openclawinstances`, the following columns are displayed:

| Column    | JSON Path                                          |
|-----------|----------------------------------------------------|
| Phase     | `.status.phase`                                    |
| Ready     | `.status.conditions[?(@.type=='Ready')].status`    |
| Gateway   | `.status.gatewayEndpoint`                          |
| Age       | `.metadata.creationTimestamp`                      |

---

## Spec Fields

### spec.registry

Global container image registry override. When set, this registry replaces the registry part of all container images used by the instance (main container, sidecars, init containers).

| Field       | Type       | Default | Description                                                                                       |
|-------------|------------|---------|---------------------------------------------------------------------------------------------------|
| `registry`  | `string`   | --      | Global registry hostname/port to use for all images. Example: `my-registry.example.com` or `my-registry:5000`. |

**Example:**

```yaml
spec:
  registry: my-registry.example.com
```

**Transformation examples:**

| Original image | With `registry: my-registry.example.com` |
|----------------|-------------------------------------------|
| `ghcr.io/openclaw/openclaw:latest` | `my-registry.example.com/openclaw/openclaw:latest` |
| `nginx:1.27-alpine` | `my-registry.example.com/nginx:1.27-alpine` |
| `ollama/ollama:latest` | `my-registry.example.com/ollama/ollama:latest` |
| `ghcr.io/openclaw/openclaw@sha256:abc123` | `my-registry.example.com/openclaw/openclaw@sha256:abc123` |


### spec.image

Container image configuration for the main OpenClaw workload.

| Field          | Type                         | Default                        | Description                                                       |
|----------------|------------------------------|--------------------------------|-------------------------------------------------------------------|
| `repository`   | `string`                     | `ghcr.io/openclaw/openclaw`    | Container image repository.                                       |
| `tag`          | `string`                     | `latest`                       | Container image tag.                                              |
| `digest`       | `string`                     | --                             | Image digest (overrides `tag` if set). Format: `sha256:abc...`.   |
| `pullPolicy`   | `string`                     | `IfNotPresent`                 | Image pull policy. One of: `Always`, `IfNotPresent`, `Never`.     |
| `pullSecrets`  | `[]LocalObjectReference`     | --                             | List of Secrets for pulling from private registries.              |

### spec.config

Configuration for the OpenClaw application (`openclaw.json`).

| Field          | Type                  | Default       | Description                                                                |
|----------------|-----------------------|---------------|----------------------------------------------------------------------------|
| `configMapRef` | `ConfigMapKeySelector`| --            | Reference to an external ConfigMap. If set, `raw` is ignored.              |
| `raw`          | `RawConfig`           | --            | Inline JSON configuration. The operator creates a managed ConfigMap.       |
| `mergeMode`    | `string`              | `overwrite`   | How config is applied to the PVC. `overwrite` replaces on every restart. `merge` deep-merges with existing PVC config, preserving runtime changes. **Caveat:** in merge mode, removing a key from the CR does not delete it from the PVC - temporarily use `replace` to wipe stale keys. |
| `format`       | `string`              | `json`        | Config file format. `json` (standard JSON) or `json5` (JSON5 with comments/trailing commas). JSON5 requires `configMapRef` - inline `raw` must be valid JSON. JSON5 is converted to standard JSON by the init container using npx json5. |

**ConfigMapKeySelector:**

| Field  | Type     | Default          | Description                            |
|--------|----------|------------------|----------------------------------------|
| `name` | `string` | (required)       | Name of the ConfigMap.                 |
| `key`  | `string` | `openclaw.json`  | Key within the ConfigMap to mount.     |

### spec.workspace

Configures initial workspace files seeded into the instance. Files are copied once on first boot and never overwritten, so agent modifications survive pod restarts.

| Field                  | Type                      | Default | Description                                                                                       |
|------------------------|---------------------------|---------|---------------------------------------------------------------------------------------------------|
| `configMapRef`         | `ConfigMapNameSelector`   | --      | Reference to an external ConfigMap whose keys become workspace files. See sub-fields below. |
| `initialFiles`         | `map[string]string`       | --      | Maps filenames to their content. Each file is written to the workspace directory only if it does not already exist. Max 50 entries. |
| `initialDirectories`   | `[]string`                | --      | Directories to create (`mkdir -p`) inside the workspace directory. Nested paths like `tools/scripts` are allowed. Max 20 items. |
| `additionalWorkspaces` | `[]AdditionalWorkspace`   | --      | Additional agent workspaces for multi-agent setups. Each entry seeds files to `~/.openclaw/workspace-<name>/`. Max 10 items. See sub-fields below. |
| `bootstrap`            | `BootstrapSpec`           | --      | Controls operator-managed `BOOTSTRAP.md` injection. See sub-fields below. |

#### spec.workspace.configMapRef

| Field  | Type     | Default | Description                                                      |
|--------|----------|---------|------------------------------------------------------------------|
| `name` | `string` | --      | **(Required)** Name of the ConfigMap in the same namespace as the instance. All keys in the ConfigMap are written as files to the workspace directory. |

**Merge priority** (highest wins): operator-injected files > inline `initialFiles` > external `configMapRef` > skill packs.

The controller watches the referenced ConfigMap for changes and re-reconciles automatically. If the ConfigMap is missing or contains invalid filenames, the `WorkspaceReady` status condition is set to `False`.

**How seeding works:** The operator merges all workspace file sources into a single operator-managed ConfigMap, which is mounted read-only on the init container. The init container copies files to the PVC (writable) using seed-once semantics (`[ -f target ] || cp source target`). The main container only mounts the PVC -- ConfigMaps are never mounted directly on the main container, so agents can freely modify their workspace files.

**Seed-once, never overwrite:** Files are only written when they don't already exist on the PVC. If an agent modifies its workspace files at runtime (e.g. updating SOUL.md via the self-improvement skill), those changes persist across pod restarts. Updating the ConfigMap or `initialFiles` only affects new instances or files that have been manually deleted from the PVC.

#### spec.workspace.bootstrap

Controls the operator-managed `BOOTSTRAP.md` file, which is seeded into the default workspace to guide first-run agent onboarding (setting identity, meeting the user, picking a persona). The agent deletes `BOOTSTRAP.md` after applying it.

| Field     | Type   | Default | Description |
|-----------|--------|---------|-------------|
| `enabled` | `bool` | `true`  | When `false`, the operator does not inject `BOOTSTRAP.md` into the workspace ConfigMap and the init container skips its seeding step. Set this to `false` after the agent has completed bootstrap so it isn't re-run on every pod restart or config change. |

Because the init container seeds via `[ -f target ] || cp source target`, it always re-copies `BOOTSTRAP.md` once the agent deletes it -- which would force the onboarding flow to run again on every restart. Flipping `enabled` to `false` is the clean opt-out.

```yaml
spec:
  workspace:
    bootstrap:
      enabled: false
```

`ENVIRONMENT.md`, self-configure files, and skill-pack files are not affected by this field.

#### spec.workspace.additionalWorkspaces[]

Each entry configures a named workspace for a secondary agent. The operator seeds files to `~/.openclaw/workspace-<name>/`.

| Field              | Type                    | Default | Description                                                                                       |
|--------------------|-------------------------|---------|---------------------------------------------------------------------------------------------------|
| `name`             | `string`                | --      | **(Required)** Workspace identifier. Must be a DNS label (`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`), max 63 chars. Seeds to `~/.openclaw/workspace-<name>/`. |
| `configMapRef`     | `ConfigMapNameSelector` | --      | Reference to an external ConfigMap whose keys become workspace files. |
| `initialFiles`     | `map[string]string`     | --      | Maps filenames to their content. Max 50 entries. |
| `initialDirectories` | `[]string`            | --      | Directories to create inside this workspace. Max 20 items. |

Per-workspace merge priority (highest wins): operator-injected `ENVIRONMENT.md` > inline `initialFiles` > external `configMapRef`. Note: `BOOTSTRAP.md`, self-configure files, and skill packs are only injected into the default workspace.

The agent workspace path in `spec.config.raw.agents.list[].workspace` must match the convention `~/.openclaw/workspace-<name>` where `<name>` is the `additionalWorkspaces[].name` value.

```yaml
spec:
  workspace:
    configMapRef:
      name: my-workspace-files      # all keys become workspace files
    initialDirectories:
      - tools/scripts
      - data
    initialFiles:                    # inline files override configMapRef
      README.md: |
        # My Workspace
        This workspace is managed by OpenClaw.
    additionalWorkspaces:
      - name: scheduler
        configMapRef:
          name: scheduler-workspace
        initialFiles:
          SOUL.md: "I am the scheduler"
  config:
    raw:
      agents:
        list:
          - id: main
          - id: scheduler
            workspace: "~/.openclaw/workspace-scheduler"
```

**GitOps example with Kustomize:**

When using Kustomize `configMapGenerator` with `configMapRef`, two settings are required:

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: my-namespace                # ConfigMaps must be in the same namespace as the instance

generatorOptions:
  disableNameSuffixHash: true          # operator looks up ConfigMaps by exact name

resources:
  - instance.yaml

configMapGenerator:
  - name: my-workspace-files           # default agent workspace
    files:
      - agents/main/SOUL.md
      - agents/main/AGENT.md
  - name: scheduler-workspace          # additional agent workspace
    files:
      - agents/scheduler/SOUL.md
      - agents/scheduler/TOOLS.md
```

- **`disableNameSuffixHash: true`** -- Required. Without this, kustomize appends a content hash to ConfigMap names (e.g. `my-workspace-files-57k7g4dthc`), causing the operator to fail with `ConfigMapNotFound`. The operator handles rollout detection via its own config hash annotation, so the kustomize hash is unnecessary.
- **`namespace`** -- Required. Generated ConfigMaps must be in the same namespace as the instance. Without this, kustomize creates them in the `default` namespace.

### spec.skills

| Field    | Type       | Default | Description                                                                                       |
|----------|------------|---------|---------------------------------------------------------------------------------------------------|
| `skills` | `[]string` | --      | Skills to install. Three formats supported: ClawHub identifiers (e.g., `mcp-server-fetch`), npm packages with `npm:` prefix (e.g., `npm:@openclaw/matrix`), and GitHub-hosted skill packs with `pack:` prefix (e.g., `pack:openclaw-rocks/skills/image-gen`). ClawHub installs are idempotent - already-installed skills are skipped gracefully, making restarts safe with persistent storage. npm lifecycle scripts are disabled for security. Max 20 items. SSA list type: `set` -- each skill is individually tracked for field ownership. |

```yaml
spec:
  skills:
    - "mcp-server-fetch"                                     # ClawHub
    - "npm:@openclaw/matrix"                                # npm package
    - "pack:openclaw-rocks/skills/image-gen"                # skill pack (latest)
    - "pack:openclaw-rocks/skills/image-gen@v1.0.0"         # skill pack (pinned)
```

**Skill packs** (`pack:owner/repo/path[@ref]`) are resolved from GitHub repos in one of two modes:

- **Manifest mode** -- the pack path contains a `skillpack.json` that declares files to seed into the workspace, directories to create, and config entries to inject into `config.raw.skills.entries`. User-defined config entries take precedence over pack defaults.
- **Raw-repo mode** -- when no `skillpack.json` is present, the operator falls back to installing the pack path directly. If a `SKILL.md` exists at the path, every file beneath it is seeded into the workspace at `skills/<basename>/<relative-path>` (preserving nested directories). This is intended for community repositories like `fluxcd/agent-skills` that use a conventional layout without manifests. Raw mode does not inject `config.raw.skills.entries`; use manifest mode if you need that. Installation is refused if GitHub truncates the tree response for very large repositories.

The operator caches resolved packs for 5 minutes. Set `GITHUB_TOKEN` on the operator for private repo access and to raise GitHub rate limits.

### spec.plugins

| Field     | Type       | Default | Description                                                                                       |
|-----------|------------|---------|---------------------------------------------------------------------------------------------------|
| `plugins` | `[]string` | --      | Plugins to install. Entries are npm package names (e.g., `@martian-engineering/lossless-claw`). An optional `npm:` prefix is accepted and stripped before installation. npm lifecycle scripts are disabled for security. Max 20 items. |

```yaml
spec:
  plugins:
    - "@martian-engineering/lossless-claw"
    - "some-other-plugin"
```

Plugins are installed in a dedicated `init-plugins` init container into `~/.openclaw/extensions/<name>/`, where `<name>` is the unscoped npm package basename (so `@openclaw/brave-plugin` becomes `~/.openclaw/extensions/brave-plugin/`). This is the layout the OpenClaw gateway's plugin discovery walks - subdirectories of `~/.openclaw/extensions/` containing a `package.json`, bundle manifest, or default entry file are auto-loaded, while `node_modules/` is explicitly skipped by the loader.

The init container stages each install via `npm pack` into a tmp dir, runs `npm install --omit=dev` in the extracted package to populate per-plugin runtime deps, and then atomically renames the result into place so a partial/failed install is never visible to the gateway. The PVC backs `~/.openclaw/`, so installs persist across pod restarts.

> Migrating from earlier operator versions (<= 0.31.0): plugins were installed under `~/.openclaw/node_modules/<pkg>/` and were not auto-discovered. Users typically worked around this by adding `plugins.load.paths` entries to their gateway config pointing at the node_modules path. Once upgraded, that workaround can be removed - plugins land in the documented `~/.openclaw/extensions/<name>/` location and the gateway picks them up automatically. The stale node_modules entries are left in place to avoid disturbing other PVC state and are harmless.

### spec.envFrom

| Field     | Type                  | Default | Description                                                                       |
|-----------|-----------------------|---------|-----------------------------------------------------------------------------------|
| `envFrom` | `[]EnvFromSource`     | --      | Sources to populate environment variables (e.g., Secrets for `ANTHROPIC_API_KEY`). |

Standard Kubernetes `EnvFromSource`. Commonly used to inject API keys from a Secret:

```yaml
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
```

### spec.env

| Field | Type           | Default | Description                                         |
|-------|----------------|---------|-----------------------------------------------------|
| `env` | `[]EnvVar`     | --      | Individual environment variables to set. SSA list type: `map` (key: `name`) -- each env var is individually tracked for field ownership. |

Standard Kubernetes `EnvVar`. Example:

```yaml
spec:
  env:
    - name: LOG_LEVEL
      value: "debug"
```

### spec.resources

Compute resource requirements for the main OpenClaw container.

| Field                | Type     | Default  | Description                          |
|----------------------|----------|----------|--------------------------------------|
| `requests.cpu`       | `string` | `500m`   | Minimum CPU (e.g., `500m`, `2`).     |
| `requests.memory`    | `string` | `1Gi`    | Minimum memory (e.g., `512Mi`).      |
| `limits.cpu`         | `string` | `2000m`  | Maximum CPU.                         |
| `limits.memory`      | `string` | `4Gi`    | Maximum memory.                      |

### spec.security

Security-related configuration for the instance.

#### spec.security.podSecurityContext

| Field                 | Type                          | Default          | Description                                                                                |
|-----------------------|-------------------------------|------------------|--------------------------------------------------------------------------------------------|
| `runAsUser`           | `*int64`                      | `1000`           | UID to run as. Setting to `0` is rejected by webhook.                                      |
| `runAsGroup`          | `*int64`                      | `1000`           | GID to run as.                                                                             |
| `fsGroup`             | `*int64`                      | `1000`           | Supplemental group for volume ownership.                                                   |
| `fsGroupChangePolicy` | `*PodFSGroupChangePolicy`    | --               | Behavior for changing volume ownership. `OnRootMismatch` skips recursive chown when ownership already matches, improving startup time for large PVCs. `Always` recursively chowns on every mount (Kubernetes default). |
| `runAsNonRoot`        | `*bool`                       | `true`           | Require non-root execution. Warns if set to `false`.                                       |

#### spec.security.containerSecurityContext

| Field                      | Type              | Default | Description                                                    |
|----------------------------|-------------------|---------|----------------------------------------------------------------|
| `allowPrivilegeEscalation` | `*bool`           | `false` | Allow privilege escalation. Warns if set to `true`.            |
| `readOnlyRootFilesystem`   | `*bool`           | `true`  | Mount root filesystem as read-only. Writable paths: PVC at `~/.openclaw/`, `~/.local/` (pip user installs), `~/.cache/` (package caches), `~/.config/` (CLI config dir, e.g. Codex / ACP runtimes), and `/tmp` emptyDir. |
| `capabilities`             | `*Capabilities`   | Drop ALL | Linux capabilities to add or drop.                            |
| `runAsNonRoot`             | `*bool`           | --      | Require non-root execution for the main container. When not set, inherits from `podSecurityContext.runAsNonRoot` (defaults to `true`). Set to `false` to allow the main container to run as root without contradicting the pod-level setting. |
| `runAsUser`                | `*int64`          | --      | UID to run the main container as. When not set, inherits from `podSecurityContext.runAsUser` via Kubernetes. |

#### spec.security.networkPolicy

| Field                      | Type                              | Default | Description                                                  |
|----------------------------|-----------------------------------|---------|--------------------------------------------------------------|
| `enabled`                  | `*bool`                           | `true`  | Create a NetworkPolicy. Warns if disabled.                   |
| `allowedIngressCIDRs`      | `[]string`                        | --      | CIDRs allowed to reach the instance.                         |
| `allowedIngressNamespaces` | `[]string`                        | --      | Namespaces allowed to reach the instance.                    |
| `allowedEgressCIDRs`       | `[]string`                        | --      | CIDRs the instance can reach (in addition to HTTPS/DNS).     |
| `allowDNS`                 | `*bool`                           | `true`  | Allow DNS resolution (UDP/TCP port 53).                      |
| `additionalEgress`         | `[]NetworkPolicyEgressRule`       | --      | Custom egress rules appended to the default DNS + HTTPS rules. Use this to allow traffic to cluster-internal services on non-standard ports. |

#### spec.security.rbac

| Field                        | Type                  | Default | Description                                                                              |
|------------------------------|-----------------------|---------|------------------------------------------------------------------------------------------|
| `createServiceAccount`       | `*bool`               | `true`  | Create a dedicated ServiceAccount for this instance.                                     |
| `serviceAccountName`         | `string`              | --      | Use an existing ServiceAccount (only when `createServiceAccount` is `false`).            |
| `serviceAccountAnnotations`  | `map[string]string`   | --      | Annotations to add to the managed ServiceAccount. Use for cloud provider integrations like AWS IRSA or GCP Workload Identity. |
| `additionalRules`            | `[]RBACRule`          | --      | Custom RBAC rules appended to the generated Role.                                        |

**RBACRule:**

| Field       | Type       | Description                                    |
|-------------|------------|------------------------------------------------|
| `apiGroups` | `[]string` | API groups (e.g., `[""]` for core, `["apps"]`).|
| `resources` | `[]string` | Resources (e.g., `["pods"]`).                  |
| `verbs`     | `[]string` | Verbs (e.g., `["get", "list"]`).               |

#### spec.security.caBundle

Injects a custom CA certificate bundle into all containers. Use this in environments with TLS-intercepting proxies or private CAs.

| Field            | Type     | Default         | Description                                                                              |
|------------------|----------|-----------------|------------------------------------------------------------------------------------------|
| `configMapName`  | `string` | --              | Name of a ConfigMap containing the CA bundle. The ConfigMap should have a key matching `key`. |
| `secretName`     | `string` | --              | Name of a Secret containing the CA bundle. Only one of `configMapName` or `secretName` should be set. |
| `key`            | `string` | `ca-bundle.crt` | Key in the ConfigMap or Secret containing the CA bundle PEM file.                        |

```yaml
spec:
  security:
    caBundle:
      configMapName: corporate-ca
      key: ca-bundle.crt
```

### spec.storage

Persistent storage configuration.

#### spec.storage.persistence

| Field           | Type                            | Default            | Description                                          |
|-----------------|---------------------------------|--------------------|------------------------------------------------------|
| `enabled`       | `*bool`                         | `true`             | Enable persistent storage via PVC.                   |
| `storageClass`  | `*string`                       | (cluster default)  | StorageClass name. Immutable after creation.         |
| `size`          | `string`                        | `10Gi`             | PVC size.                                            |
| `accessModes`   | `[]PersistentVolumeAccessMode`  | `[ReadWriteOnce]`  | PVC access modes.                                    |
| `existingClaim` | `string`                        | --                 | Name of an existing PVC to use instead of creating one. |
| `orphan`        | `*bool`                         | `true`             | When `true` (the default), the operator removes the owner reference from the managed PVC before deleting the CR so the PVC is **retained** after deletion. Set to `false` to have the PVC garbage-collected with the CR. Has no effect when `existingClaim` is set (user-managed PVCs are never touched). |

### spec.chromium

Optional Chromium sidecar for browser automation.

| Field                      | Type              | Default                        | Description                                                                                                          |
|----------------------------|-------------------|--------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `enabled`                  | `bool`            | `false`                        | Enable the Chromium sidecar container.                                                                               |
| `image.repository`         | `string`          | `chromedp/headless-shell`         | Chromium container image repository.                                                                                 |
| `image.tag`                | `string`          | `latest`                       | Chromium image tag.                                                                                                  |
| `image.digest`             | `string`          | --                             | Chromium image digest for supply chain security.                                                                     |
| `resources.requests.cpu`   | `string`          | `250m`                         | Chromium minimum CPU.                                                                                                |
| `resources.requests.memory`| `string`          | `512Mi`                        | Chromium minimum memory.                                                                                             |
| `resources.limits.cpu`     | `string`          | `1000m`                        | Chromium maximum CPU.                                                                                                |
| `resources.limits.memory`  | `string`          | `2Gi`                          | Chromium maximum memory.                                                                                             |
| `persistence.enabled`      | `bool`            | `false`                        | Enable persistent storage for browser profiles. When true, cookies, localStorage, session tokens, and cached credentials survive pod restarts. |
| `persistence.storageClass` | `*string`         | --                             | StorageClass for the Chromium profile PVC. Uses cluster default if empty.                                            |
| `persistence.size`         | `string`          | `1Gi`                          | Requested storage size for the Chromium profile PVC.                                                                 |
| `persistence.existingClaim`| `string`          | --                             | Name of a pre-existing PVC. When set, `storageClass` and `size` are ignored.                                         |
| `extraArgs`                | `[]string`        | --                             | Additional command-line arguments passed to the Chromium process, appended to the built-in anti-bot defaults (`--disable-blink-features=AutomationControlled`, `--disable-features=AutomationControlled`, `--no-first-run`). |
| `extraEnv`                 | `[]EnvVar`        | --                             | Additional environment variables for the Chromium sidecar container, merged with operator-managed variables.         |

When enabled, the sidecar:

- Runs Chromium directly with `--remote-debugging-port=9222` (no browserless proxy layer).
- Exposes Chrome DevTools Protocol on port 9222.
- Runs as UID 65534 (nobody).
- Mounts a memory-backed emptyDir at `/dev/shm` (1Gi) for shared memory.
- Mounts an emptyDir at `/tmp` for scratch space.
- Anti-bot flags and `extraArgs` are passed directly as container args.
- When `persistence.enabled` is true, mounts a PVC at `/chromium-data` and passes `--user-data-dir=/chromium-data` to Chrome, persisting cookies, localStorage, IndexedDB, cached credentials, and session tokens across pod restarts.

When Chromium is enabled, the operator also auto-configures browser profiles in the OpenClaw config. Both `"default"` and `"chrome"` profiles are set to point at the sidecar's CDP endpoint via the headless CDP Service DNS name. This ensures browser tool calls work regardless of which profile name the LLM passes.

### spec.tailscale

Optional Tailscale integration for secure tailnet access without Ingress or LoadBalancer. Runs a Tailscale sidecar (`tailscaled`) that handles serve/funnel declaratively.

| Field                | Type                     | Default                            | Description                                                                |
|----------------------|--------------------------|------------------------------------|----------------------------------------------------------------------------|
| `enabled`            | `bool`                   | `false`                            | Enable Tailscale integration (adds sidecar + init container).              |
| `mode`               | `string`                 | `serve`                            | Tailscale mode. `serve` exposes to tailnet members only. `funnel` exposes to the public internet via Tailscale Funnel. |
| `image.repository`   | `string`                 | `ghcr.io/tailscale/tailscale`      | Tailscale sidecar container image repository.                              |
| `image.tag`          | `string`                 | `latest`                           | Tailscale sidecar container image tag.                                     |
| `image.digest`       | `string`                 | --                                 | Container image digest for supply chain security (overrides tag).          |
| `authKeySecretRef`   | `*LocalObjectReference`  | --                                 | Reference to a Secret containing the Tailscale auth key. Use ephemeral+reusable keys from the Tailscale admin console. |
| `authKeySecretKey`   | `string`                 | `authkey`                          | Key in the referenced Secret containing the auth key.                      |
| `hostname`           | `string`                 | (instance name)                    | Tailscale device name. Defaults to the OpenClawInstance name.              |
| `authSSO`            | `bool`                   | `false`                            | Enable passwordless login for tailnet members. Sets `gateway.auth.allowTailscale=true` in the OpenClaw config. |
| `resources.requests.cpu` | `string`             | `50m`                              | CPU request for the Tailscale sidecar.                                     |
| `resources.requests.memory` | `string`          | `64Mi`                             | Memory request for the Tailscale sidecar.                                  |
| `resources.limits.cpu` | `string`               | `200m`                             | CPU limit for the Tailscale sidecar.                                       |
| `resources.limits.memory` | `string`            | `256Mi`                            | Memory limit for the Tailscale sidecar.                                    |

When enabled, the operator:

- Adds a **Tailscale sidecar** running `tailscaled` in userspace mode (`TS_USERSPACE=true`). The sidecar handles serve/funnel declaratively via `TS_SERVE_CONFIG`.
- Adds an **init container** (`init-tailscale-bin`) that copies the `tailscale` CLI binary to a shared volume (`/tailscale-bin`), making it available to the main container for `tailscale whois` (SSO auth).
- Creates a **state Secret** (`<instance>-ts-state`) and sets `TS_KUBE_SECRET` so Tailscale persists node identity and TLS certificates across pod restarts. This prevents hostname incrementing and Let's Encrypt certificate re-issuance.
- Grants the pod's ServiceAccount `get/update/patch` on the state Secret and enables `AutomountServiceAccountToken` so containerboot can access the Kubernetes API.
- Sets `TS_SOCKET` on the main container pointing to the sidecar's Unix socket.
- Prepends `/tailscale-bin` to the main container's `PATH`.
- Adds `tailscale-serve.json` to the ConfigMap with the serve/funnel configuration.
- When `authSSO` is true, sets `gateway.auth.allowTailscale=true` so tailnet members can authenticate without a gateway token.
- Adds STUN (UDP 3478) and WireGuard (UDP 41641) egress rules to the NetworkPolicy.

### spec.ollama

Optional Ollama sidecar for local LLM inference.

| Field                      | Type     | Default          | Description                                                                |
|----------------------------|----------|------------------|----------------------------------------------------------------------------|
| `enabled`                  | `bool`   | `false`          | Enable the Ollama sidecar container.                                       |
| `image.repository`         | `string` | `ollama/ollama`  | Ollama container image repository.                                         |
| `image.tag`                | `string` | `latest`         | Ollama image tag.                                                          |
| `image.digest`             | `string` | --               | Ollama image digest for supply chain security.                             |
| `models`                   | `[]string` | --             | Models to pre-pull during pod init (e.g., `["llama3.2", "nomic-embed-text"]`). Max 10 items. |
| `resources.requests.cpu`   | `string` | --               | Ollama minimum CPU.                                                        |
| `resources.requests.memory`| `string` | --               | Ollama minimum memory.                                                     |
| `resources.limits.cpu`     | `string` | --               | Ollama maximum CPU.                                                        |
| `resources.limits.memory`  | `string` | --               | Ollama maximum memory.                                                     |
| `storage.sizeLimit`        | `string` | `20Gi`           | Size limit for the emptyDir model cache volume.                            |
| `storage.existingClaim`    | `string` | --               | Name of an existing PVC for persistent model storage (overrides emptyDir). |
| `gpu`                      | `*int32` | --               | Number of NVIDIA GPUs to allocate (sets `nvidia.com/gpu` resource limit). Minimum: 0. |

When enabled, the operator:

- Adds an Ollama sidecar container to the pod.
- If `models` are specified, adds an init container (`init-ollama`) that pre-pulls the listed models so they are ready when the pod starts.
- The model cache uses an emptyDir by default (bounded by `storage.sizeLimit`). Set `storage.existingClaim` to use a PVC for persistent model storage across pod restarts.
- GPU allocation requires the NVIDIA device plugin to be installed on the cluster.

```yaml
spec:
  ollama:
    enabled: true
    models:
      - llama3.2
      - nomic-embed-text
    resources:
      requests:
        cpu: "2"
        memory: 4Gi
      limits:
        cpu: "8"
        memory: 16Gi
    storage:
      sizeLimit: 40Gi
    gpu: 1
```

### spec.webTerminal

Optional ttyd web terminal sidecar for browser-based shell access and debugging.

| Field                      | Type     | Default         | Description                                                                    |
|----------------------------|----------|-----------------|--------------------------------------------------------------------------------|
| `enabled`                  | `bool`   | `false`         | Enable the ttyd web terminal sidecar container.                                |
| `image.repository`         | `string` | `tsl0922/ttyd`  | Web terminal container image repository.                                       |
| `image.tag`                | `string` | `latest`        | Web terminal image tag.                                                        |
| `image.digest`             | `string` | --              | Web terminal image digest for supply chain security.                           |
| `resources.requests.cpu`   | `string` | `50m`           | Web terminal minimum CPU.                                                      |
| `resources.requests.memory`| `string` | `64Mi`          | Web terminal minimum memory.                                                   |
| `resources.limits.cpu`     | `string` | `200m`          | Web terminal maximum CPU.                                                      |
| `resources.limits.memory`  | `string` | `128Mi`         | Web terminal maximum memory.                                                   |
| `readOnly`                 | `bool`   | `false`         | Start ttyd in read-only mode (view-only, no input). Data volume mount is also set to read-only. |
| `credential.secretRef.name`| `string` | --              | Name of a Secret with `username` and `password` keys for basic auth.           |

When enabled, the operator:

- Adds a ttyd sidecar container on port 7681 to the pod.
- Mounts the instance data volume at `/home/openclaw/.openclaw` for inspection.
- Adds a `/tmp` emptyDir volume for the web terminal container.
- Adds the web terminal port to the Service and NetworkPolicy.
- Runs as UID 1000 (same as the main container) for shared file permissions on the data volume.

```yaml
spec:
  webTerminal:
    enabled: true
    readOnly: true
    credential:
      secretRef:
        name: terminal-credentials
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
```

### spec.initContainers

| Field            | Type            | Default | Description                                                              |
|------------------|-----------------|---------|--------------------------------------------------------------------------|
| `initContainers` | `[]Container`   | --      | Additional init containers to run before the main container. They run after the operator-managed init containers. Max 10 items. |

Standard Kubernetes `Container` spec. The following names are reserved by the operator and rejected by the webhook: `init-config`, `init-pnpm`, `init-python`, `init-skills`, `init-plugins`, `init-ollama`, `init-plugin-runtime-deps`.

```yaml
spec:
  initContainers:
    - name: wait-for-db
      image: busybox:1.37
      command: ["sh", "-c", "until nc -z postgres.db.svc 5432; do sleep 2; done"]
    - name: seed-data
      image: my-seeder:latest
      volumeMounts:
        - name: data
          mountPath: /data
```

### spec.sidecars

| Field      | Type            | Default | Description                                                                                       |
|------------|-----------------|---------|---------------------------------------------------------------------------------------------------|
| `sidecars` | `[]Container`   | --      | Additional sidecar containers to inject into the pod. Use for custom sidecars like database proxies, log forwarders, or service meshes. |

Standard Kubernetes `Container` spec. Sidecar containers run alongside the main OpenClaw container. If your sidecar replaces the built-in gateway proxy, set `spec.gateway.enabled: false` to avoid running both.

```yaml
spec:
  sidecars:
    - name: cloud-sql-proxy
      image: gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.8.0
      args: ["--structured-logs", "my-project:us-central1:my-db"]
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
```

### spec.sidecarVolumes

| Field            | Type         | Default | Description                                                                |
|------------------|--------------|---------|----------------------------------------------------------------------------|
| `sidecarVolumes` | `[]Volume`   | --      | Additional volumes to make available to sidecar containers.                |

Standard Kubernetes `Volume` spec. Use this to mount ConfigMaps, Secrets, or other volumes that your custom sidecars need.

```yaml
spec:
  sidecarVolumes:
    - name: proxy-config
      configMap:
        name: cloud-sql-proxy-config
```

### spec.extraVolumes

| Field          | Type         | Default | Description                                                                       |
|----------------|--------------|---------|-----------------------------------------------------------------------------------|
| `extraVolumes` | `[]Volume`   | --      | Additional volumes to add to the pod. These volumes are available to the main container via `extraVolumeMounts`. Max 10 items. |

Standard Kubernetes `Volume` spec.

### spec.extraVolumeMounts

| Field               | Type              | Default | Description                                                                                       |
|---------------------|-------------------|---------|---------------------------------------------------------------------------------------------------|
| `extraVolumeMounts` | `[]VolumeMount`   | --      | Additional volume mounts to add to the main container. Use with `extraVolumes` to mount ConfigMaps, Secrets, NFS shares, or CSI volumes. Max 10 items. |

Standard Kubernetes `VolumeMount` spec.

```yaml
spec:
  extraVolumes:
    - name: shared-data
      nfs:
        server: nfs.example.com
        path: /exports/data
    - name: custom-certs
      secret:
        secretName: my-tls-certs
  extraVolumeMounts:
    - name: shared-data
      mountPath: /mnt/shared
      readOnly: true
    - name: custom-certs
      mountPath: /etc/custom-certs
      readOnly: true
```

### spec.networking

Network-related configuration for the instance.

#### spec.networking.service

| Field         | Type                  | Default      | Description                                               |
|---------------|-----------------------|--------------|-----------------------------------------------------------|
| `type`        | `string`              | `ClusterIP`  | Service type. One of: `ClusterIP`, `LoadBalancer`, `NodePort`. |
| `annotations` | `map[string]string`   | --           | Annotations to add to the Service.                        |
| `ports`       | `[]ServicePortSpec`   | --           | Custom ports exposed on the Service. When set, replaces the default gateway and canvas ports. |

**ServicePortSpec:**

| Field        | Type     | Default | Description                                        |
|--------------|----------|---------|----------------------------------------------------|
| `name`       | `string` | --      | Name of the port (required).                       |
| `port`       | `int32`  | --      | Port number exposed on the Service (required, 1-65535). |
| `targetPort` | `*int32` | `port`  | Port on the container to route to (defaults to `port`). |
| `protocol`   | `string` | `TCP`   | Protocol for the port. One of: `TCP`, `UDP`, `SCTP`. |

When `ports` is not set, the Service exposes these default ports:

| Port Name   | Port   | Target Port | Description                     |
|-------------|--------|-------------|---------------------------------|
| `gateway`   | 18789  | 18790       | OpenClaw WebSocket gateway (via nginx proxy sidecar). |
| `canvas`    | 18793  | 18794       | OpenClaw Canvas HTTP server (via nginx proxy sidecar). |
| `chromium`  | 9222   | 9222        | Chrome DevTools Protocol via nginx CDP proxy (only if Chromium sidecar is enabled). Browserless listens internally on port 9224. |

The gateway and canvas ports route through an nginx reverse proxy sidecar because the gateway process binds to loopback (`127.0.0.1`). The proxy listens on dedicated ports (`0.0.0.0`) and forwards traffic to loopback. This avoids CWE-319 plaintext WebSocket security errors on non-loopback addresses.

**Note:** Custom ports fully replace the defaults, including the Chromium port. If you use custom ports and have the Chromium sidecar enabled, include the Chromium port (9222) explicitly.

**Custom ports example:**

```yaml
networking:
  service:
    type: ClusterIP
    ports:
      - name: http
        port: 3978
        targetPort: 3978
```

#### spec.networking.ingress

| Field         | Type                | Default | Description                                         |
|---------------|---------------------|---------|-----------------------------------------------------|
| `enabled`     | `bool`              | `false` | Create an Ingress resource.                         |
| `className`   | `*string`           | --      | IngressClass to use (e.g., `nginx`, `traefik`).     |
| `annotations` | `map[string]string` | --      | Custom annotations added to the Ingress.            |
| `hosts`       | `[]IngressHost`     | --      | List of hosts to route traffic for.                 |
| `tls`         | `[]IngressTLS`      | --      | TLS termination configuration. Warns if empty.      |
| `security`    | `IngressSecuritySpec`| --     | Ingress security settings (HTTPS redirect, HSTS, rate limiting). |

**IngressHost:**

| Field   | Type            | Description                                 |
|---------|-----------------|---------------------------------------------|
| `host`  | `string`        | Fully qualified domain name.                |
| `paths` | `[]IngressPath` | Paths to route. Defaults to `[{path: "/"}]`.|

**IngressPath:**

| Field      | Type     | Default    | Description                                                              |
|------------|----------|------------|--------------------------------------------------------------------------|
| `path`     | `string` | `/`        | URL path.                                                                |
| `pathType` | `string` | `Prefix`   | Path matching. One of: `Prefix`, `Exact`, `ImplementationSpecific`.      |
| `port`     | `*int32` | `18789`    | Backend service port number. Defaults to the gateway port when not set.  |

**Custom backend port example:**

```yaml
networking:
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: aibot.example.com
        paths:
          - path: /api/messages
            pathType: Prefix
            port: 3978
    tls:
      - hosts:
          - aibot.example.com
        secretName: certificate-aibot-tls
```

**IngressTLS:**

| Field        | Type       | Description                                         |
|--------------|------------|-----------------------------------------------------|
| `hosts`      | `[]string` | Hostnames covered by the TLS certificate.           |
| `secretName` | `string`   | Secret containing the TLS key pair.                 |

**IngressSecuritySpec:**

| Field                            | Type                      | Default | Description                                    |
|----------------------------------|---------------------------|---------|------------------------------------------------|
| `forceHTTPS`                     | `*bool`                   | `true`  | Redirect HTTP to HTTPS.                        |
| `enableHSTS`                     | `*bool`                   | `true`  | Add HSTS headers.                              |
| `rateLimiting.enabled`           | `*bool`                   | `true`  | Enable rate limiting.                          |
| `rateLimiting.requestsPerSecond` | `*int32`                  | `10`    | Maximum requests per second.                   |
| `basicAuth`                      | `*IngressBasicAuthSpec`   | --      | Optional HTTP Basic Authentication. See below. |

**IngressBasicAuthSpec:**

| Field            | Type     | Default     | Description                                                                                       |
|------------------|----------|-------------|---------------------------------------------------------------------------------------------------|
| `enabled`        | `*bool`  | `false`     | Enable HTTP Basic Authentication on the Ingress.                                                  |
| `existingSecret` | `string` | --          | Name of an existing Secret containing htpasswd content in a key named `auth`. When set, no new Secret is generated. |
| `username`       | `string` | `openclaw`  | Username for the auto-generated htpasswd Secret. Ignored when `existingSecret` is set.            |
| `realm`          | `string` | `OpenClaw`  | Authentication realm shown in browser prompts.                                                    |

When `enabled: true` and no `existingSecret` is provided, the operator:
- Generates a random 40-hex-character password
- Creates a `<name>-basic-auth` Secret with three keys:
  - `auth` - htpasswd-formatted line (used by ingress controllers)
  - `username` - plaintext username
  - `password` - plaintext password
- Tracks the secret name in `status.managedResources.basicAuthSecret`

**nginx-ingress:** The `auth-type`, `auth-secret`, and `auth-realm` annotations are set automatically.

**Traefik:** A `traefik.io/v1alpha1/Middleware` resource named `<name>-basic-auth` is created in the same namespace (requires Traefik CRDs to be installed), and the `traefik.ingress.kubernetes.io/router.middlewares` annotation is set to reference it.

**Retrieve the auto-generated credentials:**

```bash
kubectl get secret my-agent-basic-auth -o jsonpath='{.data.username}' | base64 -d
kubectl get secret my-agent-basic-auth -o jsonpath='{.data.password}' | base64 -d
```

To rotate credentials, delete the Secret and the operator will generate a new one on next reconcile.

The operator automatically adds WebSocket-related annotations for nginx-ingress (proxy timeouts, HTTP/1.1 upgrade).

### spec.probes

Health probe configuration for the main OpenClaw container. All probes use HTTP GET requests through the nginx proxy sidecar on port 18790 - liveness and startup probes check `/healthz`, while readiness probes check `/readyz`.

#### spec.probes.liveness

| Field                 | Type     | Default | Description                                           |
|-----------------------|----------|---------|-------------------------------------------------------|
| `enabled`             | `*bool`  | `true`  | Enable the liveness probe.                            |
| `initialDelaySeconds` | `*int32` | `30`    | Seconds to wait before the first check.               |
| `periodSeconds`       | `*int32` | `10`    | Seconds between checks.                              |
| `timeoutSeconds`      | `*int32` | `5`     | Seconds before the check times out.                  |
| `failureThreshold`    | `*int32` | `3`     | Consecutive failures before restarting the container. |

#### spec.probes.readiness

| Field                 | Type     | Default | Description                                           |
|-----------------------|----------|---------|-------------------------------------------------------|
| `enabled`             | `*bool`  | `true`  | Enable the readiness probe.                           |
| `initialDelaySeconds` | `*int32` | `5`     | Seconds to wait before the first check.               |
| `periodSeconds`       | `*int32` | `5`     | Seconds between checks.                              |
| `timeoutSeconds`      | `*int32` | `3`     | Seconds before the check times out.                  |
| `failureThreshold`    | `*int32` | `3`     | Consecutive failures before removing from endpoints. |

#### spec.probes.startup

| Field                 | Type     | Default | Description                                           |
|-----------------------|----------|---------|-------------------------------------------------------|
| `enabled`             | `*bool`  | `true`  | Enable the startup probe.                             |
| `initialDelaySeconds` | `*int32` | `5`     | Seconds to wait before the first check.               |
| `periodSeconds`       | `*int32` | `5`     | Seconds between checks.                              |
| `timeoutSeconds`      | `*int32` | `3`     | Seconds before the check times out.                  |
| `failureThreshold`    | `*int32` | `60`    | Consecutive failures before killing the container. Allows up to 300s startup. |

### spec.observability

Metrics and logging configuration.

#### spec.observability.metrics

| Field                       | Type                | Default | Description                                   |
|-----------------------------|---------------------|---------|-----------------------------------------------|
| `enabled`                   | `*bool`             | `true`  | Enable the metrics pipeline. When enabled, the operator injects `diagnostics.otel` config into OpenClaw to push OTLP metrics, adds an OTel Collector sidecar that exposes a Prometheus scrape endpoint, and creates the Service port and NetworkPolicy ingress rule. |
| `port`                      | `*int32`            | `9090`  | Prometheus metrics port exposed by the OTel Collector sidecar. Used for the Service port and ServiceMonitor target. |
| `serviceMonitor.enabled`    | `*bool`             | `false` | Create a Prometheus `ServiceMonitor`.         |
| `serviceMonitor.interval`   | `string`            | `30s`   | Prometheus scrape interval.                   |
| `serviceMonitor.labels`     | `map[string]string` | --      | Labels to add to the ServiceMonitor (for Prometheus selector matching). |
| `prometheusRule.enabled`    | `*bool`             | `false` | Create a `PrometheusRule` with operator alerts. |
| `prometheusRule.labels`     | `map[string]string` | --      | Labels to add to the PrometheusRule (for Prometheus rule selector matching). |
| `prometheusRule.runbookBaseURL` | `string`         | `https://openclaw.rocks/docs/runbooks` | Base URL for alert runbook links. |
| `grafanaDashboard.enabled`  | `*bool`             | `false` | Create Grafana dashboard ConfigMaps (operator overview + instance detail). |
| `grafanaDashboard.labels`   | `map[string]string` | --      | Extra labels to add to dashboard ConfigMaps. |
| `grafanaDashboard.folder`   | `string`            | `OpenClaw` | Grafana folder for the dashboards. |

#### spec.observability.logging

| Field    | Type     | Default | Description                                              |
|----------|----------|---------|----------------------------------------------------------|
| `level`  | `string` | `info`  | Log level. One of: `debug`, `info`, `warn`, `error`.     |
| `format` | `string` | `json`  | Log format. One of: `json`, `text`.                      |

### spec.selfConfigure

Agent self-modification configuration. When enabled, the agent can create `OpenClawSelfConfig` resources to modify its own instance spec via the K8s API.

| Field            | Type                 | Default | Description                                                                     |
|------------------|----------------------|---------|---------------------------------------------------------------------------------|
| `enabled`        | `bool`               | `false` | Enable self-configuration for this instance.                                    |
| `allowedActions` | `[]SelfConfigAction` | --      | Action categories the agent is allowed to perform. If empty, no actions pass validation (fail-safe). Max 4 items. |

**SelfConfigAction values:**

| Value            | Description                                                       |
|------------------|-------------------------------------------------------------------|
| `skills`         | Add or remove skills (`spec.skills`).                             |
| `config`         | Deep-merge patches into the OpenClaw config (`spec.config.raw`).  |
| `workspaceFiles` | Add or remove initial workspace files (`spec.workspace.initialFiles`). |
| `envVars`        | Add or remove plain environment variables (`spec.env`).           |

When enabled, the operator:
- Grants the SA read access to its own `OpenClawInstance` and referenced Secrets (scoped by `resourceNames`)
- Grants `create`, `get`, `list` on `openclawselfconfigs`
- Sets `automountServiceAccountToken: true` on the SA and pod spec
- Injects `OPENCLAW_INSTANCE_NAME` and `OPENCLAW_NAMESPACE` environment variables
- Adds port 6443 egress to the NetworkPolicy for K8s API access
- Injects `SELFCONFIG.md` (skill documentation) and `selfconfig.sh` (helper script) into the workspace

### spec.suspended

Scales the workload to zero replicas when `true`. Non-runtime resources (Service, ConfigMap, RBAC, NetworkPolicy, PVC) remain fully managed. Set to `false` to resume normal operation.

| Field       | Type   | Default | Description                                                                  |
|-------------|--------|---------|------------------------------------------------------------------------------|
| `suspended` | `bool` | `false` | Scale workload to zero replicas when `true`. Mutually exclusive with `spec.availability.autoScaling.enabled`. |

When suspended:
- StatefulSet replicas are set to 0
- `status.phase` becomes `Suspended`
- `Ready` condition is `False` with reason `Suspended`
- `StatefulSetReady` condition is `True` once all pods terminate (desired state achieved)
- Auto-updates are paused and resume when unsuspended

### spec.availability

High availability and scheduling configuration.

| Field                             | Type                | Default | Description                                              |
|-----------------------------------|---------------------|---------|----------------------------------------------------------|
| `podDisruptionBudget.enabled`     | `*bool`             | `true`  | Create a PodDisruptionBudget.                            |
| `podDisruptionBudget.maxUnavailable` | `*int32`         | `1`     | Maximum pods that can be unavailable during disruption.  |
| `nodeSelector`                    | `map[string]string` | --      | Node labels for pod scheduling.                          |
| `tolerations`                     | `[]Toleration`      | --      | Tolerations for pod scheduling.                          |
| `affinity`                        | `*Affinity`         | --      | Affinity and anti-affinity rules.                        |
| `topologySpreadConstraints`       | `[]TopologySpreadConstraint` | --      | Topology spread constraints for pod scheduling.          |
| `runtimeClassName`                | `*string`           | --      | RuntimeClass to use for the pod. Selects an alternative container runtime (e.g. Kata Containers, gVisor). If unset, the cluster default runtime is used. See [RuntimeClass docs](https://kubernetes.io/docs/concepts/containers/runtime-class/). |
| `podAnnotations`                  | `map[string]string` | --      | Extra annotations merged into the StatefulSet pod template. Operator-managed keys (`openclaw.rocks/config-hash`, `openclaw.rocks/secret-hash`) always take precedence. |
| `autoScaling.enabled`             | `*bool`             | `false` | Create a HorizontalPodAutoscaler.                        |
| `autoScaling.minReplicas`         | `*int32`            | `1`     | Minimum number of replicas.                              |
| `autoScaling.maxReplicas`         | `*int32`            | `5`     | Maximum number of replicas.                              |
| `autoScaling.targetCPUUtilization` | `*int32`           | `80`    | Target average CPU utilization (percentage).             |
| `autoScaling.targetMemoryUtilization` | `*int32`        | --      | Target average memory utilization (percentage).          |

When `autoScaling.enabled` is `true` with persistence enabled, the operator uses StatefulSet `VolumeClaimTemplates` instead of a standalone PVC. Each replica gets its own PVC (`data-<instance>-<ordinal>`) using `size`, `storageClass`, and `accessModes` from `spec.storage.persistence`. The `existingClaim` field is ignored in this mode. PVC retention policy is `Retain` for both scale-down and deletion.

### spec.backup

Configures periodic scheduled backups to S3-compatible storage. Requires the `s3-backup-credentials` Secret in the operator namespace and persistence to be enabled.

| Field                | Type     | Default | Description                                                                                       |
|----------------------|----------|---------|---------------------------------------------------------------------------------------------------|
| `schedule`           | `string` | --      | Cron expression for periodic backups (e.g., `"0 2 * * *"` for daily at 2 AM). When set, the operator creates a CronJob that runs rclone to sync PVC data to S3. |
| `historyLimit`       | `*int32` | `3`     | Number of successful CronJob runs to retain.                                                       |
| `failedHistoryLimit` | `*int32` | `1`     | Number of failed CronJob runs to retain.                                                           |
| `timeout`            | `string` | `30m`   | Maximum duration to wait for a pre-delete backup to complete before giving up and proceeding with deletion (Go duration string, e.g. `"30m"`, `"1h"`). Covers all phases: StatefulSet scale-down, pod termination, Job execution, and Job failure retries. Minimum: `5m`, Maximum: `24h`. |
| `serviceAccountName` | `string` | --      | ServiceAccount to use for backup and restore Jobs. Set this to an IRSA-annotated or Pod Identity-enabled ServiceAccount so Jobs authenticate via the AWS credential chain instead of static credentials. Applies to all backup Jobs (pre-delete, pre-update, periodic, and restore). |
| `retentionDays`      | `*int32` | `7`     | Number of days to keep daily snapshots in S3. Snapshots older than this are pruned after each successful backup. Minimum: `1`, Maximum: `365`. |

The CronJob mounts the PVC (hot backup - no downtime) and uses pod affinity to schedule on the same node as the StatefulSet pod (required for RWO PVCs).

Periodic backups use an incremental sync strategy to minimize S3 transactions and storage costs:

1. **Incremental sync** to a fixed `latest` path - only uploads changed files
2. **Daily snapshot** - copies `latest` to `snapshots/YYYY-MM-DD` (near-free if today's snapshot already exists)
3. **Retention cleanup** - prunes snapshots older than `retentionDays`

S3 path structure:
```
backups/<tenantId>/<instanceName>/periodic/latest/              # incrementally synced
backups/<tenantId>/<instanceName>/periodic/snapshots/2026-03-13/ # daily snapshot (auto-pruned)
```

```yaml
spec:
  backup:
    schedule: "0 2 * * *"   # Daily at 2 AM UTC
    retentionDays: 7         # Keep 7 days of daily snapshots (default)
    historyLimit: 5          # Keep last 5 successful runs
    failedHistoryLimit: 2    # Keep last 2 failed runs
    timeout: "30m"           # Max time for pre-delete backup (default: 30m)
    serviceAccountName: "my-irsa-sa"  # Optional: use IRSA/Pod Identity for S3 auth
```

### spec.restoreFrom

| Field         | Type     | Default | Description                                                                                       |
|---------------|----------|---------|---------------------------------------------------------------------------------------------------|
| `restoreFrom` | `string` | --      | S3 path to restore data from (e.g., `backups/{tenantId}/{instanceId}/{timestamp}`). When set, the operator restores PVC data from this path before creating the StatefulSet. Works on both existing and new instances (enabling clone/migrate workflows). Cleared automatically after successful restore. Requires the `s3-backup-credentials` Secret to be present in the operator namespace. |

See [Backup and Restore](#backup-and-restore) for full setup instructions, including [clone/migrate workflows](#clone--migrate-an-instance).

### spec.runtimeDeps

Configures built-in init containers that install runtime dependencies to the data PVC for use by MCP servers and skills.

| Field    | Type   | Default | Description                                                                              |
|----------|--------|---------|------------------------------------------------------------------------------------------|
| `pnpm`   | `bool` | `false` | Install pnpm via corepack for npm-based MCP servers and skills. Adds the `init-pnpm` init container. |
| `python` | `bool` | `false` | Install Python 3.12 and uv for Python-based MCP servers and skills. Adds the `init-python` init container. |

```yaml
spec:
  runtimeDeps:
    pnpm: true
    python: true
```

### spec.gateway

Configures the gateway reverse proxy, authentication token, and Control UI origins.

| Field              | Type       | Default | Description                                                                                       |
|--------------------|------------|---------|---------------------------------------------------------------------------------------------------|
| `enabled`          | `*bool`    | `true`  | Enable the gateway reverse proxy (nginx) sidecar. When disabled, the gateway binds to `0.0.0.0` and probes/Service target it directly. **Do not** manually set `gateway.bind: loopback` in your config when the proxy is disabled - the pod will be unreachable. The operator emits a `GatewayBindConflict` warning event if this is detected. When disabled, the gateway serves plaintext `ws://` on `0.0.0.0` - ensure your replacement proxy or Ingress handles TLS termination (CWE-319). |
| `existingSecret`   | `string`   | --      | Name of a user-managed Secret containing the gateway token. The Secret must have a key named `token`. When set, the operator skips auto-generating a gateway token Secret and uses this Secret instead. |
| `controlUiOrigins` | `[]string` | --      | Additional allowed origins for the Control UI. The operator always auto-injects `http://localhost:18789` and `http://127.0.0.1:18789` (for port-forwarding) and derives origins from ingress hosts. Use this field to add extra origins (e.g., custom reverse proxy URLs). Max 20 items. |

When `existingSecret` is not set, the operator automatically generates a random gateway token Secret, which is tracked in `status.managedResources.gatewayTokenSecret`.

**Auto-injected settings:**

The operator always injects `gateway.controlUi.dangerouslyDisableDeviceAuth: true` into the config JSON. Device pairing (introduced in OpenClaw v2026.3.2) is fundamentally incompatible with Kubernetes because users cannot approve pairing from inside a container, connections always come through the nginx proxy sidecar (non-local), and mDNS is unavailable. If you explicitly set `gateway.controlUi.dangerouslyDisableDeviceAuth` in your config, your value takes precedence. **Do not set `gateway.mode: local`** - this desktop-only mode enforces device identity checks that cannot work behind a reverse proxy.

The operator also injects the `OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS=10000` environment variable (10 seconds). OpenClaw v2026.3.12 reduced the WebSocket handshake timeout from ~10s to 3s, which is too short for Kubernetes where plugin loading adds startup overhead. See [upstream issue #46892](https://github.com/openclaw/openclaw/issues/46892). If you set `OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS` in `spec.env`, your value takes precedence.

When accessing the Control UI through an Ingress, authenticate by appending the gateway token to the URL fragment: `https://openclaw.example.com/#token=<your-token>`.

The operator auto-injects `gateway.controlUi.allowedOrigins` into the config JSON with:
- **Localhost** (always): `http://localhost:18789`, `http://127.0.0.1:18789`
- **Ingress hosts**: `https://` if the host appears in TLS config, `http://` otherwise
- **Explicit extras**: values from `spec.gateway.controlUiOrigins`

If you set `gateway.controlUi.allowedOrigins` directly in your config JSON, the operator will not override it.

**Note:** Since OpenClaw v2026.2.24, `gateway.allowedOrigins` defaults to same-origin only. If you access the Control UI through an Ingress or other non-default hostname, set `gateway.allowedOrigins: ["*"]` in your config to avoid blocked WebSocket connections.

```yaml
spec:
  gateway:
    existingSecret: my-gateway-token
    controlUiOrigins:
      - "https://proxy.example.com"
```

### spec.autoUpdate

Configures automatic version updates from the OCI registry.

| Field                | Type     | Default | Description                                                                                       |
|----------------------|----------|---------|---------------------------------------------------------------------------------------------------|
| `enabled`            | `*bool`  | `false` | Enable automatic version updates.                                                                 |
| `checkInterval`      | `string` | `24h`   | How often to check for new versions (Go duration). Minimum: `1h`, Maximum: `168h` (7 days).       |
| `backupBeforeUpdate` | `*bool`  | `true`  | Create a backup before applying updates.                                                          |
| `rollbackOnFailure`  | `*bool`  | `true`  | Automatically revert to the previous version if the updated pod fails to become ready within `healthCheckTimeout`. |
| `healthCheckTimeout` | `string` | `10m`   | How long to wait for the updated pod to become ready before triggering a rollback (Go duration). Minimum: `2m`, Maximum: `30m`. |

When enabled, the operator periodically checks the OCI registry for newer image tags. If a new version is found, it optionally creates a backup, updates the StatefulSet image tag, and monitors the rollout. If the pod fails to become ready within the health check timeout, the operator automatically rolls back to the previous version (if `rollbackOnFailure` is enabled).

Auto-update pauses after 3 consecutive rollbacks. It resumes when a newer version becomes available.

```yaml
spec:
  autoUpdate:
    enabled: true
    checkInterval: 12h
    backupBeforeUpdate: true
    rollbackOnFailure: true
    healthCheckTimeout: 15m
```

---

## Status Fields

### status.phase

| Field   | Type     | Description                                                                    |
|---------|----------|--------------------------------------------------------------------------------|
| `phase` | `string` | Current lifecycle phase: `Pending`, `Provisioning`, `Running`, `Degraded`, `Failed`, `Terminating`, `BackingUp`, `Restoring`, `Updating`. |

### status.conditions

Standard `metav1.Condition` array. Condition types:

| Type                  | Description                                                    |
|-----------------------|----------------------------------------------------------------|
| `Ready`               | Overall readiness of the instance.                             |
| `ConfigValid`         | Configuration is valid and loaded.                             |
| `StatefulSetReady`    | StatefulSet has ready replicas.                                |
| `DeploymentReady`     | **(Deprecated)** Legacy Deployment has ready replicas. Used during migration from Deployment to StatefulSet. |
| `ServiceReady`        | Service has been created.                                      |
| `NetworkPolicyReady`  | NetworkPolicy has been applied.                                |
| `RBACReady`           | RBAC resources are in place.                                   |
| `StorageReady`        | PVC has been provisioned and is bound.                         |
| `BackupComplete`      | The backup job completed successfully.                         |
| `RestoreComplete`     | The restore job completed successfully.                        |
| `ScheduledBackupReady`| The periodic backup CronJob is configured and ready.           |
| `AutoUpdateAvailable` | A newer version is available in the OCI registry.              |
| `SecretsReady`        | All referenced Secrets exist and are accessible.               |
| `SkillPacksReady`     | Skill packs resolved successfully from GitHub (either via `skillpack.json` or raw-repo autodiscovery). `False` with reason `ResolutionFailed` when GitHub is unreachable, the pack path has no `skillpack.json` and no `SKILL.md`, or the repository tree is truncated - instance runs without skill packs (phase `Degraded`). Retried on next reconcile. |
| `WorkspaceReady`      | Workspace files seeded successfully. `False` when an external ConfigMap referenced by `spec.workspace.configMapRef` is missing or contains invalid filenames. `True` once all workspace files (from configMapRef, initialFiles, and skill packs) are seeded. |

### status.endpoints

| Field              | Type     | Description                                                  |
|--------------------|----------|--------------------------------------------------------------|
| `gatewayEndpoint`  | `string` | In-cluster endpoint for the gateway: `<name>.<ns>.svc:18789`.|
| `canvasEndpoint`   | `string` | In-cluster endpoint for canvas: `<name>.<ns>.svc:18793`.     |

### status.observedGeneration

| Field                | Type    | Description                                              |
|----------------------|---------|----------------------------------------------------------|
| `observedGeneration` | `int64` | The `.metadata.generation` last processed by the controller. |

### status.lastReconcileTime

| Field               | Type          | Description                                     |
|---------------------|---------------|-------------------------------------------------|
| `lastReconcileTime` | `*metav1.Time`| Timestamp of the last successful reconciliation.|

### status.managedResources

| Field                | Type     | Description                           |
|----------------------|----------|---------------------------------------|
| `statefulSet`        | `string` | Name of the managed StatefulSet.      |
| `deployment`         | `string` | Name of the legacy Deployment (deprecated, used during migration). |
| `service`            | `string` | Name of the managed Service.          |
| `configMap`          | `string` | Name of the managed ConfigMap.        |
| `pvc`                | `string` | Name of the managed PVC.             |
| `networkPolicy`      | `string` | Name of the managed NetworkPolicy.    |
| `podDisruptionBudget`| `string` | Name of the managed PDB.             |
| `serviceAccount`     | `string` | Name of the managed ServiceAccount.   |
| `role`               | `string` | Name of the managed Role.            |
| `roleBinding`        | `string` | Name of the managed RoleBinding.      |
| `gatewayTokenSecret` | `string` | Name of the auto-generated gateway token Secret. |
| `prometheusRule`     | `string` | Name of the managed PrometheusRule. |
| `grafanaDashboardOperator` | `string` | Name of the operator overview dashboard ConfigMap. |
| `grafanaDashboardInstance` | `string` | Name of the instance detail dashboard ConfigMap. |
| `horizontalPodAutoscaler` | `string` | Name of the managed HorizontalPodAutoscaler. |
| `backupCronJob`      | `string` | Name of the managed periodic backup CronJob. |
| `tailscaleStateSecret` | `string` | Name of the Secret used to persist Tailscale node identity and TLS certificate state. |

### status.backup and restore

| Field            | Type           | Description                                              |
|------------------|----------------|----------------------------------------------------------|
| `backingUpSince` | `*metav1.Time` | Timestamp when the instance entered the BackingUp phase. Used to enforce `spec.backup.timeout`. Cleared when the phase changes. |
| `backupJobName`  | `string`       | Name of the active backup Job.                           |
| `restoreJobName` | `string`       | Name of the active restore Job.                          |
| `lastBackupPath` | `string`       | S3 path of the last successful backup.                   |
| `lastBackupTime` | `*metav1.Time` | Timestamp of the last successful backup.                 |
| `restoredFrom`   | `string`       | S3 path this instance was restored from.                 |

### status.autoUpdate

Tracks the state of automatic version updates.

| Field                | Type           | Description                                                                              |
|----------------------|----------------|------------------------------------------------------------------------------------------|
| `lastCheckTime`      | `*metav1.Time` | When the registry was last checked for new versions.                                     |
| `latestVersion`      | `string`       | Latest version available in the registry.                                                |
| `currentVersion`     | `string`       | Version currently running.                                                               |
| `pendingVersion`     | `string`       | Set during an in-flight update to the version being applied.                             |
| `updatePhase`        | `string`       | Progress of an in-flight update. One of: `""`, `BackingUp`, `ApplyingUpdate`, `HealthCheck`, `RollingBack`. |
| `lastUpdateTime`     | `*metav1.Time` | When the last successful update was applied.                                             |
| `lastUpdateError`    | `string`       | Error message from the last failed update attempt.                                       |
| `previousVersion`    | `string`       | Version before the last update (used for rollback).                                      |
| `preUpdateBackupPath`| `string`       | S3 path of the pre-update backup (used for rollback restore).                            |
| `failedVersion`      | `string`       | Version that failed health checks and will be skipped in future checks. Cleared when a newer version becomes available. |
| `rollbackCount`      | `int32`        | Consecutive rollback count. Auto-update pauses after 3. Reset to 0 on any successful update. |

---

## Backup and Restore

The operator uses [rclone](https://rclone.org/) to sync PVC data to and from an S3-compatible backend. All backup operations are driven by a single Secret named `s3-backup-credentials` in the **operator namespace** (the namespace where the operator pod runs, typically `openclaw-operator-system`).

### S3 Credentials Secret

Create the Secret before enabling any backup or restore feature:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-backup-credentials
  namespace: openclaw-operator-system  # must match the operator namespace
stringData:
  S3_ENDPOINT: "https://s3.us-east-1.amazonaws.com"   # or any S3-compatible URL
  S3_BUCKET: "my-openclaw-backups"
  S3_ACCESS_KEY_ID: "<key-id>"
  S3_SECRET_ACCESS_KEY: "<secret-key>"
  # S3_REGION: "us-east-1"  # optional - see below
```

`S3_ENDPOINT` and `S3_BUCKET` are required. The operator uses rclone's S3 backend, which is compatible with AWS S3, Backblaze B2, MinIO, Cloudflare R2, Wasabi, Google Cloud Storage (S3-compatible), and any other S3-compatible service.

| Key | Required | Description |
|-----|----------|-------------|
| `S3_ENDPOINT` | Yes | S3-compatible endpoint URL (e.g., `https://s3.us-east-1.amazonaws.com`) |
| `S3_BUCKET` | Yes | Bucket name for backups |
| `S3_ACCESS_KEY_ID` | No | Access key ID. When omitted (together with `S3_SECRET_ACCESS_KEY`), rclone uses `--s3-env-auth=true` to authenticate via the provider's native credential chain. |
| `S3_SECRET_ACCESS_KEY` | No | Secret access key. When omitted (together with `S3_ACCESS_KEY_ID`), rclone uses `--s3-env-auth=true`. |
| `S3_REGION` | No | S3 region (e.g., `us-east-1`). Required for MinIO instances configured with a custom region. Without this, rclone defaults to `us-east-1`, which causes authentication failures on providers using a different region. |
| `S3_PROVIDER` | No | rclone S3 provider (default: `Other`). Set to `AWS` for native AWS credential chain, `GCS` for Google Cloud Storage, `Ceph` for Ceph/RadosGW, etc. Setting the correct provider enables provider-specific auth flows and optimizations. See [rclone S3 providers](https://rclone.org/s3/#s3-provider). |

### When backups run automatically

| Trigger | Condition |
|---------|-----------|
| **Pre-delete backup** | Always runs when a CR is deleted, unless `openclaw.rocks/skip-backup: "true"` annotation is set or persistence is disabled. Subject to `spec.backup.timeout` (default: 30m) - if the backup does not complete within the timeout, it is skipped and deletion proceeds. |
| **Pre-update backup** | Runs before each auto-update when `spec.autoUpdate.backupBeforeUpdate: true` (the default). |
| **Periodic (scheduled)** | Runs on a cron schedule when `spec.backup.schedule` is set. See [Periodic scheduled backups](#periodic--scheduled-backups) below. |

If the `s3-backup-credentials` Secret does not exist, pre-delete and pre-update backups are silently skipped (deletion and updates proceed normally), and the periodic backup CronJob is not created (a `ScheduledBackupReady=False` condition is set with reason `S3CredentialsMissing`).

### Backup path format

Backups are stored at:

```
s3://<bucket>/backups/<tenantId>/<instanceName>/<timestamp>
```

Where:
- `<tenantId>` is the value of the `openclaw.rocks/tenant` label on the instance, or derived from the namespace (e.g., namespace `oc-tenant-abc` yields `abc`), or the namespace name itself.
- `<instanceName>` is `metadata.name` of the `OpenClawInstance`.
- `<timestamp>` is an RFC3339 timestamp at the time the backup job runs.

The path of the last successful backup is recorded in `status.lastBackupPath`.

### Backup timeout

Pre-delete backups are subject to a configurable timeout (default: 30 minutes). If the backup does not complete within the timeout -- whether due to a stuck Job, pod termination issues, or S3 credential errors -- the operator logs a warning, emits a `BackupTimedOut` event, sets the `BackupComplete=False` condition with reason `BackupTimedOut`, and proceeds with deletion.

Configure the timeout via `spec.backup.timeout`:

```yaml
spec:
  backup:
    timeout: "1h"  # Allow up to 1 hour for pre-delete backups (default: 30m, min: 5m, max: 24h)
```

### Skip backup on delete

To delete an instance immediately without waiting for a backup (e.g., the S3 backend is unavailable):

```bash
kubectl annotate openclawinstance my-agent openclaw.rocks/skip-backup=true
kubectl delete openclawinstance my-agent
```

### Restoring an instance

Set `spec.restoreFrom` to an existing backup path. The operator runs an rclone restore job to populate the PVC before starting the StatefulSet, then clears the field automatically:

```yaml
spec:
  restoreFrom: "backups/my-tenant/my-agent/2026-01-15T10:30:00Z"
```

To find available backups, list the S3 bucket directly (e.g., `aws s3 ls s3://my-openclaw-backups/backups/`). The `status.lastBackupPath` field on any existing instance shows its last backup path.

**Restore behavior:**

- The restore Job runs **before** the StatefulSet is created (reconcile order: PVC -> restore Job -> StatefulSet)
- `spec.restoreFrom` is cleared automatically after a successful restore and the original path is recorded in `status.restoredFrom`
- The restore Job uses `spec.backup.serviceAccountName` when set, so workload identity (IRSA/Pod Identity) works for restores
- If the restore Job fails, the operator sets `RestoreComplete=False` and retries. Delete the failed Job to trigger a fresh attempt

### Clone / migrate an instance

`spec.restoreFrom` works on **new instances** (with empty PVCs), not just existing ones. This enables cloning and cross-namespace migration workflows.

**Example - clone instance A from namespace X to namespace Y:**

```yaml
# 1. Check the source instance's last backup path:
#    kubectl get openclawinstance my-agent -n ns-x -o jsonpath='{.status.lastBackupPath}'
#    -> backups/tenant-x/my-agent/2026-03-15T02:00:00Z

# 2. Create a new instance in the target namespace with restoreFrom:
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-agent-clone
  namespace: ns-y
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: latest
  restoreFrom: "backups/tenant-x/my-agent/2026-03-15T02:00:00Z"
  backup:
    serviceAccountName: "openclaw-backup"  # if using workload identity
```

The operator creates the PVC, runs the restore Job (rclone sync from S3 to the new PVC), then starts the StatefulSet with the restored data. The new instance gets a fresh gateway token - the source instance is unaffected.

**ArgoCD integration:** The operator auto-clears `spec.restoreFrom` after a successful restore. To prevent ArgoCD from detecting this as drift, add it to `ignoreDifferences`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
spec:
  ignoreDifferences:
    - group: openclaw.rocks
      kind: OpenClawInstance
      jsonPointers:
        - /spec/restoreFrom
```

### Periodic / scheduled backups

Set `spec.backup.schedule` to a cron expression to enable periodic backups:

```yaml
spec:
  backup:
    schedule: "0 2 * * *"     # Daily at 2 AM UTC
    historyLimit: 3            # Successful job runs to retain (default: 3)
    failedHistoryLimit: 1      # Failed job runs to retain (default: 1)
```

The operator creates a Kubernetes CronJob (`<instance>-backup-periodic`) that:

- Mounts the PVC with **fsGroup** matching the StatefulSet (hot backup - no downtime or StatefulSet scale-down)
- Uses **pod affinity** to co-locate on the same node as the StatefulSet pod (required for RWO PVCs)
- Stores each run under a unique timestamped path: `backups/<tenantId>/<instanceName>/periodic/<YYYYMMDDTHHMMSSz>`
- Uses `ConcurrencyPolicy: Forbid` to prevent overlapping backup runs
- Runs with the same rclone image and security context (UID/GID 1000) as on-delete backups

**Requirements:** persistence must be enabled and the `s3-backup-credentials` Secret must exist. If either is missing, the CronJob is not created and a `ScheduledBackupReady=False` condition is set.

**Removing the schedule:** set `spec.backup.schedule` to an empty string (or remove the `backup` section entirely) and the CronJob is automatically deleted.

### Workload Identity (cloud-native auth)

Instead of static credentials, you can use your cloud provider's workload identity to authenticate backup Jobs:

- **AWS EKS**: [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) or [EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html) with `S3_PROVIDER=AWS`
- **GKE**: [Workload Identity Federation](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) with `S3_PROVIDER=GCS` (using GCS S3-compatible endpoint)
- **AKS**: [Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview) with static HMAC keys or a compatible S3 provider

The setup has three parts: (1) a ServiceAccount with provider-specific annotations, (2) the `s3-backup-credentials` Secret without static keys, and (3) `spec.backup.serviceAccountName` on the instance.

**Example (AWS IRSA):**

1. Create an IRSA-annotated ServiceAccount in the instance namespace:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openclaw-backup
  namespace: oc-tenant-my-tenant
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/openclaw-backup-role
```

2. Omit `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY` from the credentials Secret and set `S3_PROVIDER`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-backup-credentials
  namespace: openclaw-operator-system
stringData:
  S3_ENDPOINT: "https://s3.us-east-1.amazonaws.com"
  S3_BUCKET: "my-openclaw-backups"
  S3_REGION: "us-east-1"
  S3_PROVIDER: "AWS"  # enables AWS-native credential chain
```

3. Reference the ServiceAccount in the instance spec:

```yaml
spec:
  backup:
    schedule: "0 2 * * *"
    serviceAccountName: "openclaw-backup"
```

When `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY` are omitted, the operator passes `--s3-env-auth=true` to rclone, which uses the provider's native credential chain. The `serviceAccountName` is set on all backup and restore Job pods so they inherit the cloud IAM role.

Setting `S3_PROVIDER` to the correct value (e.g., `AWS`, `GCS`) enables provider-specific optimizations in rclone. When left unset, it defaults to `Other` which works with any S3-compatible backend using static credentials.

---

## Related Guides

- [Model Fallback Chains](model-fallback.md) - configure multi-provider fallback via environment variables
- [Custom AI Providers](custom-providers.md) - Ollama sidecar, vLLM, and other self-hosted models
- [External Secrets Operator Integration](external-secrets.md) - sync API keys from AWS, Vault, GCP, etc.

---

## OpenClawSelfConfig (v1alpha1)

**Group**: `openclaw.rocks`
**Version**: `v1alpha1`
**Kind**: `OpenClawSelfConfig`
**Scope**: Namespaced
**Short name**: `ocsc`

An `OpenClawSelfConfig` represents a request from an agent to modify its own `OpenClawInstance` spec. The operator validates the request against the parent instance's `selfConfigure.allowedActions` policy and applies approved changes.

### Print Columns

| Column    | JSON Path                          |
|-----------|------------------------------------|
| Instance  | `.spec.instanceRef`                |
| Phase     | `.status.phase`                    |
| Age       | `.metadata.creationTimestamp`      |

### Spec Fields

| Field                | Type                      | Default | Description                                                                   |
|----------------------|---------------------------|---------|-------------------------------------------------------------------------------|
| `instanceRef`        | `string`                  | (required) | Name of the parent `OpenClawInstance` in the same namespace.               |
| `addSkills`          | `[]string`                | --      | Skills to add. Max 10 items.                                                  |
| `removeSkills`       | `[]string`                | --      | Skills to remove. Max 10 items.                                               |
| `configPatch`        | `RawConfig`               | --      | Partial JSON to deep-merge into `spec.config.raw`. Protected keys under `gateway` are rejected. |
| `addWorkspaceFiles`  | `map[string]string`       | --      | Filenames to content to add to workspace. Max 10 entries.                     |
| `removeWorkspaceFiles` | `[]string`              | --      | Workspace filenames to remove. Max 10 items.                                  |
| `addEnvVars`         | `[]SelfConfigEnvVar`      | --      | Environment variables to add (plain values only, no secret refs). Max 10 items. |
| `removeEnvVars`      | `[]string`                | --      | Environment variable names to remove. Max 10 items.                           |

**SelfConfigEnvVar:**

| Field   | Type     | Description                   |
|---------|----------|-------------------------------|
| `name`  | `string` | Environment variable name.    |
| `value` | `string` | Environment variable value.   |

### Status Fields

| Field            | Type          | Description                                                  |
|------------------|---------------|--------------------------------------------------------------|
| `phase`          | `string`      | Processing state: `Pending`, `Applied`, `Failed`, `Denied`.  |
| `message`        | `string`      | Human-readable details about the current phase.              |
| `completionTime` | `*metav1.Time`| Timestamp when the request reached a terminal phase.         |

### Lifecycle

1. Agent creates an `OpenClawSelfConfig` resource -- status starts as `Pending`
2. Operator fetches the parent `OpenClawInstance` and validates:
   - `selfConfigure.enabled` must be `true` (otherwise: `Denied`)
   - All requested action categories must be in `allowedActions` (otherwise: `Denied`)
   - Protected config keys (`gateway.*`) and env var names are rejected (otherwise: `Failed`)
3. Operator applies changes to the parent instance spec
4. Status transitions to `Applied` (success) or `Failed` (error)
5. An owner reference is set to the parent instance for garbage collection
6. Terminal requests are auto-deleted after 1 hour

### Server-Side Apply and Field Ownership

The SelfConfig controller uses Kubernetes Server-Side Apply (SSA) with the field manager name `openclaw-selfconfig`. This enables fine-grained field ownership tracking:

- **Skills** (`+listType=set`): Each skill name is individually owned. Multiple field managers can each own different skills on the same instance.
- **Env vars** (`+listType=map`, key: `name`): Each env var is individually owned by the field manager that last set it.
- **Workspace files** (map fields): Each file entry under `initialFiles` is individually owned.
- **Config raw**: Owned atomically as a single field.

When a SelfConfig request attempts to remove an item owned by another field manager, the removal is skipped and the operator emits a `Warning` / `SelfConfigSkippedRemoval` event identifying the owning manager. The status message includes details about any skipped removals.

### Protected Resources

The following are protected and cannot be modified via self-configure:

- **Config keys**: `gateway` (entire subtree) -- prevents breaking gateway auth
- **Environment variables**: `HOME`, `PATH`, `OPENCLAW_GATEWAY_TOKEN`, `OPENCLAW_INSTANCE_NAME`, `OPENCLAW_NAMESPACE`, `OPENCLAW_DISABLE_BONJOUR`, `CHROMIUM_URL`, `OLLAMA_HOST`, `TS_AUTHKEY`, `TS_HOSTNAME`, `NODE_EXTRA_CA_CERTS`, `NPM_CONFIG_CACHE`, `NPM_CONFIG_IGNORE_SCRIPTS`

### Example

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawSelfConfig
metadata:
  name: add-fetch-skill
spec:
  instanceRef: my-agent
  addSkills:
    - "mcp-server-fetch"
  addEnvVars:
    - name: MY_CUSTOM_VAR
      value: "hello"
```

---

## Full Example

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: "0.5.0"
    pullPolicy: IfNotPresent
    pullSecrets:
      - name: ghcr-secret

  config:
    mergeMode: merge
    raw:
      mcpServers:
        filesystem:
          command: npx
          args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]

  workspace:
    initialDirectories:
      - tools/scripts
    initialFiles:
      CLAUDE.md: |
        # Project Instructions
        Use the filesystem MCP server for file operations.

  skills:
    - "mcp-server-fetch"
    - "npm:@openclaw/matrix"

  envFrom:
    - secretRef:
        name: openclaw-api-keys

  selfConfigure:
    enabled: true
    allowedActions:
      - skills
      - config

  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      cpu: "4"
      memory: 8Gi

  security:
    podSecurityContext:
      runAsUser: 1000
      runAsGroup: 1000
      fsGroup: 1000
      fsGroupChangePolicy: OnRootMismatch
      runAsNonRoot: true
    containerSecurityContext:
      allowPrivilegeEscalation: false
    networkPolicy:
      enabled: true
      allowedIngressNamespaces:
        - monitoring
      allowDNS: true
      additionalEgress:
        - to:
            - namespaceSelector:
                matchLabels:
                  app: postgres
          ports:
            - protocol: TCP
              port: 5432
    rbac:
      createServiceAccount: true
      serviceAccountAnnotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/openclaw-role
    caBundle:
      configMapName: corporate-ca
      key: ca-bundle.crt

  storage:
    persistence:
      enabled: true
      storageClass: gp3
      size: 50Gi
      accessModes:
        - ReadWriteOnce

  chromium:
    enabled: true
    image:
      repository: chromedp/headless-shell
      tag: "stable"
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: "2"
        memory: 4Gi
    persistence:
      enabled: true
      size: 2Gi

  ollama:
    enabled: true
    models:
      - llama3.2
      - nomic-embed-text
    resources:
      requests:
        cpu: "2"
        memory: 4Gi
      limits:
        cpu: "8"
        memory: 16Gi
    storage:
      sizeLimit: 40Gi
    gpu: 1

  networking:
    service:
      type: ClusterIP
    ingress:
      enabled: true
      className: nginx
      hosts:
        - host: openclaw.example.com
          paths:
            - path: /
              pathType: Prefix
      tls:
        - hosts:
            - openclaw.example.com
          secretName: openclaw-tls
      security:
        forceHTTPS: true
        enableHSTS: true
        rateLimiting:
          enabled: true
          requestsPerSecond: 20

  probes:
    liveness:
      enabled: true
      initialDelaySeconds: 60
      periodSeconds: 15
    readiness:
      enabled: true
      initialDelaySeconds: 10
    startup:
      enabled: true
      failureThreshold: 60

  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: 15s
        labels:
          release: prometheus
    logging:
      level: info
      format: json

  availability:
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
    nodeSelector:
      node-type: compute
    tolerations:
      - key: dedicated
        operator: Equal
        value: openclaw
        effect: NoSchedule
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: openclaw

  runtimeDeps:
    pnpm: true
    python: true

  gateway:
    existingSecret: my-gateway-token

  autoUpdate:
    enabled: true
    checkInterval: 12h
    backupBeforeUpdate: true
    rollbackOnFailure: true
    healthCheckTimeout: 15m
```

---

## OpenClawClusterDefaults (v1alpha1)

**Group**: `openclaw.rocks`
**Version**: `v1alpha1`
**Kind**: `OpenClawClusterDefaults`
**Scope**: Cluster
**Short name**: `occd`

Cluster-scoped singleton that fills in unset fields on every `OpenClawInstance` at reconcile time. The name **must** be `cluster` - any other name is ignored so typos do not silently churn the fleet. Changes to the singleton trigger re-reconciliation of every existing instance.

**Precedence**: per-instance fields always win. A cluster default is only applied when the corresponding instance field is unset. This means the defaults are invisible in `kubectl get openclawinstance -o yaml` (they never get written back into the stored instance) - to introspect what will actually render, look at the resulting StatefulSet or ConfigMap.

### Spec fields

| Field | Type | Description |
|-------|------|-------------|
| `registry` | `string` | Default container image registry override applied when `spec.registry` is unset on an instance. Replaces the registry prefix of main, sidecar, and init-container images. |
| `image.repository` | `string` | Default image repository when the instance's `spec.image.repository` is unset. |
| `image.tag` | `string` | Default image tag when the instance has neither `spec.image.tag` nor `spec.image.digest` set. Never overrides a digest-pinned instance. |
| `image.digest` | `string` | Default image digest when `spec.image.digest` is unset. |
| `image.pullPolicy` | `PullPolicy` | Default pull policy when unset on the instance. |
| `image.pullSecrets` | `[]LocalObjectReference` | Default pull secrets. Applied only when the instance has no pull secrets of its own (replace-only, not merged). |
| `env` | `[]EnvVar` | Default environment variables merged into every instance's container env. Cluster defaults come first; instance entries with a matching `name` override the default's value in place; instance-only names are appended. |
| `runtimeDeps.pnpm` | `bool` | When true, opts every instance into the pnpm runtime init container unless the instance has already set it true. Booleans are OR-merged because `unset` and `false` are indistinguishable. |
| `runtimeDeps.python` | `bool` | Same OR-merge semantics for the Python/uv runtime init container. |

### Example

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawClusterDefaults
metadata:
  name: cluster
spec:
  registry: "<account>.dkr.ecr.<region>.amazonaws.com.cn"
  image:
    tag: v0.28.0
  env:
    - name: NPM_CONFIG_REGISTRY
      value: https://registry.npmmirror.com
    - name: PIP_INDEX_URL
      value: https://mirrors.aliyun.com/pypi/simple/
  runtimeDeps:
    python: true
```

### Status fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]metav1.Condition` | Current state of the singleton. |
| `observedGeneration` | `int64` | Generation of the spec most recently processed by the operator. |
