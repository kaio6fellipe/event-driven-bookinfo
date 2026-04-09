# Headlamp Kubernetes Dashboard — Local Deployment

**Date:** 2026-04-09
**Status:** Approved

## Goal

Deploy Headlamp as the Kubernetes dashboard in the local k8s environment, following the same Helm + NodePort + k3d port mapping pattern used by Grafana and Prometheus.

## Approach

Install the official `headlamp/headlamp` Helm chart into the `observability` namespace with a NodePort service exposed via k3d port mapping at `http://localhost:4466`.

## Changes

### New file: `deploy/observability/local/headlamp-values.yaml`

Helm values for the Headlamp chart:
- Service type: `NodePort`, nodePort `30444`
- `cluster-admin` ClusterRoleBinding for full local dev access
- No plugins, no custom branding, no auth

### Modified: `Makefile`

**`k8s-cluster` target** — add k3d port mapping:
```
-p "4466:30444@server:0"
```

**`k8s-observability` target** — add Headlamp as step `[7/7]` after Grafana dashboards:
```
helm repo add headlamp https://headlamp-k8s.github.io/headlamp/
helm upgrade --install headlamp headlamp/headlamp \
    -n observability \
    -f deploy/observability/local/headlamp-values.yaml \
    --wait --timeout 120s
```

**`k8s-status` target** — add Headlamp URL to access URLs output:
```
Headlamp:     http://localhost:4466
```

### Modified: `CLAUDE.md`

Add Headlamp to the Access section: `Headlamp http://localhost:4466`

## Port Mapping Summary

| Tool | localhost | NodePort |
|------|-----------|----------|
| Productpage | :8080 | (loadbalancer) |
| Grafana | :3000 | 30300 |
| Prometheus | :9090 | 30900 |
| Headlamp | :4466 | 30444 |

## RBAC

The Headlamp service account gets a `cluster-admin` ClusterRoleBinding. This is acceptable for a local dev cluster (k3d). No token-based auth or OIDC — Headlamp will have full read/write access to all cluster resources.

The Headlamp Helm chart supports `clusterRoleBinding.create: true` with a configurable role, which handles RBAC setup declaratively.

## Out of Scope

- Headlamp plugins
- Custom branding or theming
- Authentication (OIDC, token)
- Production deployment considerations
- Ingress or Gateway routing for Headlamp
