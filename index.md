---
layout: default
---

# event-driven-bookinfo — Helm Charts

This is the Helm chart repository for [event-driven-bookinfo](https://github.com/kaio6fellipe/event-driven-bookinfo).

## Usage

```bash
helm repo add bookinfo https://kaio6fellipe.github.io/event-driven-bookinfo
helm repo update
helm search repo bookinfo
```

## Available Charts

| Chart | Description |
|---|---|
| `bookinfo-service` | Reusable chart for all bookinfo microservices — supports CQRS, Argo Events, DLQ auto-wiring |

## Install a Service

```bash
helm install ratings bookinfo/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  -n bookinfo
```

## Source

Chart source and documentation: [github.com/kaio6fellipe/event-driven-bookinfo](https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/charts/bookinfo-service)
