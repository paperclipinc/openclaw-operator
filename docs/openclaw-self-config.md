---
title: OpenClawSelfConfig
description: Lifecycle, server-side apply behavior, protected resources, and a usage example for the OpenClawSelfConfig CRD.
---

# OpenClawSelfConfig

The `OpenClawSelfConfig` CRD lets a running agent request changes to its own `OpenClawInstance` spec, gated by an allowlist policy. The CRD's field reference lives in the [API Reference](api-reference.md#openclawselfconfig).

## Lifecycle

1. Agent creates an `OpenClawSelfConfig` resource -- status starts as `Pending`
2. Operator fetches the parent `OpenClawInstance` and validates:
   - `selfConfigure.enabled` must be `true` (otherwise: `Denied`)
   - All requested action categories must be in `allowedActions` (otherwise: `Denied`)
   - Protected config keys (`gateway.*`) and env var names are rejected (otherwise: `Failed`)
3. Operator applies changes to the parent instance spec
4. Status transitions to `Applied` (success) or `Failed` (error)
5. An owner reference is set to the parent instance for garbage collection
6. Terminal requests are auto-deleted after 1 hour

## Server-Side Apply and Field Ownership

The SelfConfig controller uses Kubernetes Server-Side Apply (SSA) with the field manager name `openclaw-selfconfig`. This enables fine-grained field ownership tracking:

- **Skills** (`+listType=set`): Each skill name is individually owned. Multiple field managers can each own different skills on the same instance.
- **Env vars** (`+listType=map`, key: `name`): Each env var is individually owned by the field manager that last set it.
- **Workspace files** (map fields): Each file entry under `initialFiles` is individually owned.
- **Config raw**: Owned atomically as a single field.

When a SelfConfig request attempts to remove an item owned by another field manager, the removal is skipped and the operator emits a `Warning` / `SelfConfigSkippedRemoval` event identifying the owning manager. The status message includes details about any skipped removals.

## Protected Resources

The following are protected and cannot be modified via self-configure:

- **Config keys**: `gateway` (entire subtree) -- prevents breaking gateway auth
- **Environment variables**: `HOME`, `PATH`, `OPENCLAW_GATEWAY_TOKEN`, `OPENCLAW_INSTANCE_NAME`, `OPENCLAW_NAMESPACE`, `OPENCLAW_DISABLE_BONJOUR`, `CHROMIUM_URL`, `OLLAMA_HOST`, `TS_AUTHKEY`, `TS_HOSTNAME`, `NODE_EXTRA_CA_CERTS`, `NPM_CONFIG_CACHE`, `NPM_CONFIG_IGNORE_SCRIPTS`

## Example

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
