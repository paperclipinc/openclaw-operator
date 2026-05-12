---
title: OpenClawClusterDefaults
description: Cluster-wide defaults for OpenClawInstance fields via a singleton CR, with an example and field behavior notes.
---

# OpenClawClusterDefaults

The `OpenClawClusterDefaults` (singleton, name `cluster`) fills in unset `OpenClawInstance` fields cluster-wide. Per-instance fields always win. The CRD's field reference lives in the [API Reference](api-reference.md#openclawclusterdefaults-v1alpha1).

Cluster-scoped singleton that fills in unset fields on every `OpenClawInstance` at reconcile time. The name **must** be `cluster` - any other name is ignored so typos do not silently churn the fleet. Changes to the singleton trigger re-reconciliation of every existing instance.

**Precedence**: per-instance fields always win. A cluster default is only applied when the corresponding instance field is unset. This means the defaults are invisible in `kubectl get openclawinstance -o yaml` (they never get written back into the stored instance) - to introspect what will actually render, look at the resulting StatefulSet or ConfigMap.

## Example

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
