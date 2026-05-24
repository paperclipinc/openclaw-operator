---
title: OpenClaw Operator
description: Kubernetes operator for managing OpenClaw AI agent instances.
---

# OpenClaw Operator

The OpenClaw Operator runs [OpenClaw](https://openclaw.ai) AI agents on
Kubernetes with production-grade security, observability, and lifecycle
management.

A single `OpenClawInstance` custom resource defines the whole stack:
StatefulSet, Service, RBAC, NetworkPolicy, PVC, PodDisruptionBudget,
Ingress, and ServiceMonitor. The operator reconciles that resource into
a hardened, monitored, self-healing deployment.

For the project pitch and a feature overview, see the
[README](https://github.com/openclaw-rocks/openclaw-operator).

## Where to start

- [Deployment](deployment.md): install the operator on Kind, GKE, EKS,
  AKS, or any conformant Kubernetes cluster.
- [Architecture](architecture.md): how the operator reconciles
  `OpenClawInstance` resources.
- [Full example](full-example.md): an end-to-end working configuration.
- [Custom providers](custom-providers.md): wire any AI provider via
  environment variables.
- [API reference](api-reference.md): every field on every CRD.

## Operations

- [Backup and restore](backup-restore.md)
- [Troubleshooting](troubleshooting.md)
- Runbooks for paging-worthy alerts live under
  [runbooks/](runbooks/).
- Prometheus alert rules and Grafana dashboards live under
  [monitoring/](monitoring/).

## Self-configuration and adaptation

- [Cluster defaults](openclaw-cluster-defaults.md)
- [Self-config workflow](openclaw-self-config.md)
- [Model fallback](model-fallback.md)
- [External secrets](external-secrets.md)
