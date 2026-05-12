---
title: OpenClawClusterDefaults
description: Cluster-wide defaults for OpenClawInstance fields via a singleton CR, with an example and field behavior notes.
---

# OpenClawClusterDefaults

The `OpenClawClusterDefaults` is a cluster-scoped singleton that fills in unset `OpenClawInstance` fields at reconcile time. The name **must** be `cluster` -- any other name is ignored so typos do not silently churn the fleet. Per-instance fields always win, so a default only applies when the corresponding instance field is unset; defaults never get written back into the stored instance, so to introspect what will actually render look at the resulting StatefulSet or ConfigMap. The CRD's field reference lives in the [API Reference](api-reference.md#openclawclusterdefaults).

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
