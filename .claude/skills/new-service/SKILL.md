---
description: Scaffold a new hexagonal architecture service with all layers
arguments:
  - name: service
    description: Name of the new service (lowercase, e.g. inventory)
    required: true
---

# /new-service

Scaffold a new hexagonal architecture service following the project's established patterns.

## What gets created

```
services/{{service}}/
├── cmd/main.go                              # Composition root
└── internal/
    ├── core/
    │   ├── domain/{{service}}.go            # Domain entity with validation
    │   ├── domain/{{service}}_test.go       # Domain tests
    │   ├── port/inbound.go                  # Service interface
    │   └── port/outbound.go                 # Repository interface
    └── adapter/
        ├── inbound/http/
        │   ├── handler.go                   # HTTP handlers
        │   ├── handler_test.go              # httptest tests
        │   └── dto.go                       # Request/response DTOs
        └── outbound/memory/
            └── {{service}}_repository.go    # In-memory adapter
```

## Steps

1. Ask the user for the domain entity fields and API routes.
2. Create the domain entity with validation and tests.
3. Create inbound and outbound port interfaces.
4. Create the service layer implementing the inbound port.
5. Create the in-memory repository adapter.
6. Create HTTP handler with DTOs and tests.
7. Create the composition root (cmd/main.go) wiring everything with pkg/ packages.
8. Add a Dockerfile and Helm values file (`deploy/{{service}}/values-local.yaml`). Use `deploy/ratings/values-local.yaml` as a template for services with CQRS+events, or `deploy/notification/values-local.yaml` for simple services.
9. Run `go test ./services/{{service}}/...` to verify.

## Patterns to Follow

- Look at `services/ratings/` as the reference implementation.
- Use `pkg/config`, `pkg/logging`, `pkg/metrics`, `pkg/server`, `pkg/telemetry`, `pkg/profiling`, `pkg/health`.
- Register a business metric counter via OTel in the service layer.
- Admin port serves /healthz, /readyz, /metrics, /debug/pprof/*.
