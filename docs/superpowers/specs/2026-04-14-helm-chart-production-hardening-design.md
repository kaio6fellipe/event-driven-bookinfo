# Helm Chart Production Hardening — Design Specification

**Date:** 2026-04-14
**Status:** Draft
**Scope:** Add production-ready fields to bookinfo-service Helm chart (10 items)

## Overview

Harden the `bookinfo-service` Helm chart for production use. Add security contexts, service accounts, secret references, PDB, scheduling controls, annotations, and HTTPRoute enhancements (hostnames, timeouts). All new fields have safe defaults — existing local deployments are unaffected.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Secret management | Reference existing Secret (`existingSecret`) | Chart doesn't create secrets — secrets belong to platform (External Secrets, Vault, sealed-secrets). Compatible with cloud-managed DBs (GCP Cloud SQL, AWS RDS) |
| ServiceAccount | Chart creates with configurable annotations | Standard Helm pattern. `create: true` by default, annotations for Workload Identity / IRSA. `create: false` to reference external SA |
| EventSource Service | Keep chart-managed `eventsource-service.yaml` | Argo Events only auto-creates Service when `spec.service` is set (docs say "testing only"). Chart-managed gives full control for production routing |

## Items

### 1. HTTPRoute Enhancements

**New values:**
```yaml
gateway:
  name: default-gw
  namespace: platform
  sectionName: web
  hostnames: []            # e.g., ["bookinfo.example.com"]
  timeouts:
    request: ""            # e.g., "10s"
    backendRequest: ""     # e.g., "5s"
```

**Template changes** (`httproute.yaml`):
- Add `spec.hostnames` when `gateway.hostnames` is non-empty (applied to all generated HTTPRoutes)
- Add `rules[].timeouts` when either timeout field is set

### 2. SecurityContext

**New values (secure defaults):**
```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65534
  runAsGroup: 65534
  fsGroup: 65534
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Add `spec.template.spec.securityContext` from `podSecurityContext`
- Add `containers[0].securityContext` from `securityContext`

### 3. ServiceAccount

**New values:**
```yaml
serviceAccount:
  create: true
  name: ""                 # defaults to fullname
  annotations: {}          # e.g., iam.gke.io/gcp-service-account
```

**New template** (`serviceaccount.yaml`):
- Creates ServiceAccount when `serviceAccount.create: true`
- Name defaults to `bookinfo-service.fullname`

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Add `spec.template.spec.serviceAccountName`

**New helper** (`_helpers.tpl`):
- `bookinfo-service.serviceAccountName` — returns explicit name or auto-derived name

### 4. Existing Secret Reference

**New values:**
```yaml
existingSecret: ""         # name of pre-existing Secret
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- When set, add `envFrom: [{secretRef: {name: ...}}]` alongside configMapRef

### 5. PodDisruptionBudget

**New values:**
```yaml
pdb:
  enabled: false
  minAvailable: 1
```

**New template** (`pdb.yaml`):
- Creates PDB when `pdb.enabled: true`
- Targets read deployment (or single deployment when CQRS off) via selector labels

### 6. ImagePullSecrets

**New values:**
```yaml
imagePullSecrets: []       # e.g., [{name: "ghcr-pull-secret"}]
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Add `spec.template.spec.imagePullSecrets` when non-empty

### 7. Pod Scheduling

**New values:**
```yaml
nodeSelector: {}
tolerations: []
affinity: {}
topologySpreadConstraints: []
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Add all four fields to pod spec when non-empty

### 8. Pod Annotations and Labels

**New values:**
```yaml
podAnnotations: {}
podLabels: {}
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Merge `podAnnotations` into `spec.template.metadata.annotations`
- Merge `podLabels` into `spec.template.metadata.labels`

### 9. Service Annotations

**New values:**
```yaml
serviceAnnotations: {}
```

**Template changes** (`service.yaml`, `service-write.yaml`):
- Add `metadata.annotations` when non-empty

### 10. Extra Environment Variables

**New values:**
```yaml
extraEnv: []               # e.g., [{name: "DEBUG", value: "true"}]
```

**Template changes** (`deployment.yaml`, `deployment-write.yaml`):
- Append to container `env` list after POD_NAME/POD_NAMESPACE

## Template Change Matrix

| Template | Items Applied |
|---|---|
| `deployment.yaml` | 2, 3, 4, 6, 7, 8, 10 |
| `deployment-write.yaml` | 2, 3, 4, 6, 7, 8, 10 |
| `service.yaml` | 9 |
| `service-write.yaml` | 9 |
| `httproute.yaml` | 1 |
| `serviceaccount.yaml` | 3 (new) |
| `pdb.yaml` | 5 (new) |
| `_helpers.tpl` | 3 (new helper) |
| `values.yaml` | All items |
| `values.schema.json` | All items |

## What Stays Unchanged

- `configmap.yaml` — no changes needed
- `eventsource.yaml` — no changes needed (kept chart-managed, no `spec.service`)
- `eventsource-service.yaml` — kept as-is (production-grade, chart-managed)
- `sensor.yaml` — no changes needed
- `hpa.yaml` — no changes needed (memory/custom metrics deferred to when needed)
- `NOTES.txt` — no changes needed
- Per-service values files — no changes needed (all new fields have safe defaults)
- CI test values — no changes needed

## Backwards Compatibility

All new fields have empty/false/no-op defaults. Existing `values-local.yaml` files and CI test values work without modification. The only behavioral change is that the **default securityContext is now restrictive** — services using distroless images (all current services) will work fine. If a future service needs writable filesystem, it overrides `securityContext.readOnlyRootFilesystem: false` in its values file.
