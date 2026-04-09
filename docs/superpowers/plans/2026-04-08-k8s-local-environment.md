# Local Kubernetes Development Environment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the full event-driven bookinfo stack on a local k3d cluster via `make run-k8s`, with CQRS read/write deployment split, Envoy Gateway API routing, Argo Events + Kafka event bus, full observability (Prometheus, Grafana, Tempo, Loki, Alloy), and PostgreSQL.

**Architecture:** Layered Make targets create a k3d cluster, install platform infrastructure (Strimzi Kafka, Argo Events, Envoy Gateway) and observability (kube-prometheus-stack, Tempo, Loki, Alloy), then deploy the bookinfo apps with CQRS split. Each layer has health gates. All kubectl/helm calls are scoped to the k3d context.

**Tech Stack:** k3d, Helm 3, Kustomize, Strimzi (Kafka KRaft), Argo Events, Envoy Gateway API, kube-prometheus-stack, Grafana Tempo, Grafana Loki, Grafana Alloy, PostgreSQL 17

**Spec:** `docs/superpowers/specs/2026-04-08-k8s-local-environment-design.md`

---

## File Structure

### New files to create

```
deploy/
├── gateway/
│   ├── base/
│   │   ├── gateway.yaml              # Gateway: default-gw in platform NS
│   │   ├── reference-grant.yaml      # Allow bookinfo HTTPRoutes → platform Gateway
│   │   └── kustomization.yaml
│   └── overlays/local/
│       ├── kustomization.yaml
│       └── httproutes.yaml           # 4 HTTPRoutes: productpage + 3 webhooks
│
├── argo-events/overlays/local/
│   ├── kustomization.yaml
│   ├── eventbus.yaml                 # Kafka EventBus → Strimzi
│   └── sensors/
│       ├── book-added-sensor.yaml    # Triggers → details-write, notification
│       ├── review-submitted-sensor.yaml
│       └── rating-submitted-sensor.yaml
│
├── postgres/local/
│   ├── kustomization.yaml
│   ├── statefulset.yaml              # Postgres 17 StatefulSet + PVC
│   ├── service.yaml                  # ClusterIP postgres:5432
│   └── init-configmap.yaml           # init-databases.sql as ConfigMap
│
├── details/overlays/local/
│   ├── kustomization.yaml            # Extends base + adds write deployment
│   ├── deployment-read-patch.yaml    # Patches base deploy: role=read label
│   ├── deployment-write.yaml         # New write Deployment
│   ├── service-write.yaml            # ClusterIP details-write:80
│   └── configmap-patch.yaml          # Postgres + OTLP env vars
│
├── reviews/overlays/local/           # Same pattern as details
│   ├── kustomization.yaml
│   ├── deployment-read-patch.yaml
│   ├── deployment-write.yaml
│   ├── service-write.yaml
│   └── configmap-patch.yaml
│
├── ratings/overlays/local/           # Same pattern as details
│   ├── kustomization.yaml
│   ├── deployment-read-patch.yaml
│   ├── deployment-write.yaml
│   ├── service-write.yaml
│   └── configmap-patch.yaml
│
├── productpage/overlays/local/       # Read-only (no write deployment)
│   ├── kustomization.yaml
│   ├── deployment-patch.yaml
│   └── configmap-patch.yaml
│
├── notification/overlays/local/      # Write-only (no read deployment)
│   ├── kustomization.yaml
│   ├── deployment-patch.yaml
│   └── configmap-patch.yaml
│
├── platform/local/
│   ├── strimzi-values.yaml           # Strimzi operator Helm values
│   ├── kafka-cluster.yaml            # Kafka CR (KRaft single-node)
│   ├── kafka-nodepool.yaml           # KafkaNodePool: dual-role
│   └── argo-events-values.yaml       # Argo Events controller Helm values
│
└── observability/local/
    ├── kube-prometheus-stack-values.yaml
    ├── tempo-values.yaml
    ├── loki-values.yaml
    ├── alloy-logs-values.yaml        # DaemonSet for log collection
    ├── alloy-logs-config.alloy       # Alloy config: loki.source.kubernetes
    ├── alloy-metrics-traces-values.yaml  # Deployment for metrics+traces
    └── alloy-metrics-traces-config.alloy # Alloy config: OTLP receiver + prometheus.scrape
```

### Files to modify

```
Makefile                              # Add all k8s-* targets
.gitignore                            # Already has .superpowers/ (added in brainstorm)
```

---

### Task 1: Makefile k8s Foundation — Context Safety and Variables

**Files:**
- Modify: `Makefile`

This task adds the k8s variable block, color definitions (already exist — reuse them), and the context validation guard function that every k8s target will call.

- [ ] **Step 1: Add k8s variables and context guard to Makefile**

Append this block after the existing `# ─── Cleanup ─` section and before `# ─── Help ─`:

```makefile
# ─── Kubernetes (local) ──────────────────────────────────────────────────────

K8S_CLUSTER    := bookinfo-local
K8S_CONTEXT    := k3d-$(K8S_CLUSTER)
K8S_NS_PLATFORM     := platform
K8S_NS_OBSERVABILITY := observability
K8S_NS_BOOKINFO     := bookinfo
KUBECTL        := kubectl --context=$(K8S_CONTEXT)
HELM           := helm --kube-context=$(K8S_CONTEXT)

# Context safety guard — call at the top of every k8s target.
# Verifies the context exists and belongs to a k3d cluster.
define k8s-guard
	@if ! kubectl config get-contexts $(K8S_CONTEXT) >/dev/null 2>&1; then \
		printf "$(RED)ERROR: context '$(K8S_CONTEXT)' not found.$(NC)\n"; \
		printf "  Run $(CYAN)make k8s-cluster$(NC) first.\n"; \
		exit 1; \
	fi
	@if ! echo "$(K8S_CONTEXT)" | grep -q '^k3d-'; then \
		printf "$(RED)ERROR: context '$(K8S_CONTEXT)' is not a k3d cluster. Refusing to proceed.$(NC)\n"; \
		exit 1; \
	fi
endef
```

- [ ] **Step 2: Verify the Makefile parses**

Run: `make help`

Expected: All existing targets listed, no syntax errors. The new variables and guard don't produce output on their own.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add k8s variables and context safety guard to Makefile"
```

---

### Task 2: k3d Cluster Creation Target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add k8s-cluster and stop-k8s targets**

Append after the `define k8s-guard` block:

```makefile
.PHONY: k8s-cluster
k8s-cluster: ## Create k3d cluster with port mappings for Gateway + observability
	@if k3d cluster list $(K8S_CLUSTER) >/dev/null 2>&1; then \
		printf "$(GREEN)Cluster '$(K8S_CLUSTER)' already exists.$(NC)\n"; \
	else \
		printf "$(BOLD)Creating k3d cluster '$(K8S_CLUSTER)'...$(NC)\n"; \
		k3d cluster create $(K8S_CLUSTER) \
			--api-port 6550 \
			-p "8080:80@loadbalancer" \
			-p "8443:443@loadbalancer" \
			-p "3000:30300@server:0" \
			-p "9090:30900@server:0" \
			--k3s-arg "--disable=traefik@server:0" \
			--wait; \
	fi
	@printf "$(BOLD)Verifying cluster...$(NC)\n"
	$(KUBECTL) cluster-info
	@printf "\n$(GREEN)$(BOLD)Cluster '$(K8S_CLUSTER)' ready.$(NC)\n\n"

.PHONY: stop-k8s
stop-k8s: ## Delete k3d cluster and all resources
	@if k3d cluster list $(K8S_CLUSTER) >/dev/null 2>&1; then \
		printf "$(BOLD)Deleting cluster '$(K8S_CLUSTER)'...$(NC)\n"; \
		k3d cluster delete $(K8S_CLUSTER); \
		printf "$(GREEN)Cluster deleted.$(NC)\n"; \
	else \
		printf "Cluster '$(K8S_CLUSTER)' does not exist.\n"; \
	fi
```

Note: `--disable=traefik` is required because k3s ships with Traefik by default and we use Envoy Gateway instead. This avoids port conflicts on 80/443.

- [ ] **Step 2: Verify cluster creation**

Run: `make k8s-cluster`

Expected: Cluster created, `kubectl cluster-info` shows the k3d cluster running. Re-running prints "already exists".

- [ ] **Step 3: Verify cluster deletion**

Run: `make stop-k8s`

Expected: Cluster deleted cleanly.

- [ ] **Step 4: Re-create cluster for subsequent tasks**

Run: `make k8s-cluster`

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add k8s-cluster and stop-k8s Make targets"
```

---

### Task 3: Platform Helm Values — Strimzi, Argo Events

**Files:**
- Create: `deploy/platform/local/strimzi-values.yaml`
- Create: `deploy/platform/local/argo-events-values.yaml`

- [ ] **Step 1: Create Strimzi operator Helm values**

```yaml
# deploy/platform/local/strimzi-values.yaml
# Strimzi Kafka Operator - local dev values
# Chart: strimzi/strimzi-kafka-operator
replicas: 1

resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 512Mi

# Watch all namespaces so Kafka CRs in platform NS and
# EventBus resources work correctly
watchAnyNamespace: true
```

- [ ] **Step 2: Create Argo Events controller Helm values**

```yaml
# deploy/platform/local/argo-events-values.yaml
# Argo Events Controller - local dev values
# Chart: argo/argo-events
controller:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 250m
      memory: 256Mi

# Watch all namespaces so EventSources/Sensors in bookinfo NS are processed
configs:
  jetstream:
    versions:
      - version: latest
        natsImage: nats:latest
        metricsExporterImage: natsio/prometheus-nats-exporter:latest
        configReloaderImage: natsio/nats-server-config-reloader:latest
        startCommand: /nats-server
```

- [ ] **Step 3: Commit**

```bash
git add deploy/platform/local/
git commit -m "feat: add Strimzi and Argo Events Helm values for local k8s"
```

---

### Task 4: Platform Kafka & EventBus CRDs

**Files:**
- Create: `deploy/platform/local/kafka-nodepool.yaml`
- Create: `deploy/platform/local/kafka-cluster.yaml`

- [ ] **Step 1: Create KafkaNodePool for single-node KRaft**

```yaml
# deploy/platform/local/kafka-nodepool.yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaNodePool
metadata:
  name: dual-role
  namespace: platform
  labels:
    strimzi.io/cluster: bookinfo-kafka
spec:
  replicas: 1
  roles:
    - controller
    - broker
  storage:
    type: jbod
    volumes:
      - id: 0
        type: persistent-claim
        size: 1Gi
        kraftMetadata: shared
  resources:
    requests:
      cpu: 100m
      memory: 512Mi
    limits:
      cpu: 500m
      memory: 1Gi
```

- [ ] **Step 2: Create Kafka cluster CR**

```yaml
# deploy/platform/local/kafka-cluster.yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: bookinfo-kafka
  namespace: platform
  annotations:
    strimzi.io/node-pools: enabled
    strimzi.io/kraft: enabled
spec:
  kafka:
    version: 3.9.0
    listeners:
      - name: plain
        port: 9092
        type: internal
        tls: false
    config:
      offsets.topic.replication.factor: 1
      transaction.state.log.replication.factor: 1
      transaction.state.log.min.isr: 1
      default.replication.factor: 1
      min.insync.replicas: 1
      num.partitions: 3
  entityOperator:
    topicOperator: {}
    userOperator: {}
```

- [ ] **Step 3: Commit**

```bash
git add deploy/platform/local/
git commit -m "feat: add Kafka KRaft single-node CRDs for local k8s"
```

---

### Task 5: k8s-platform Make Target

**Files:**
- Modify: `Makefile`

This target installs Envoy Gateway, Strimzi operator, Kafka cluster, and Argo Events. Each component has a health gate.

- [ ] **Step 1: Add k8s-platform target**

Append after the `stop-k8s` target:

```makefile
.PHONY: k8s-platform
k8s-platform: ## Install platform: Envoy Gateway, Strimzi, Kafka, Argo Events
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Platform Layer ═══$(NC)\n\n"
	@# ── Namespaces ──
	@$(KUBECTL) create namespace $(K8S_NS_PLATFORM) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@# ── Envoy Gateway ──
	@printf "$(BOLD)[1/5] Installing Envoy Gateway...$(NC)\n"
	@$(HELM) upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
		--version v1.7.0 \
		-n envoy-gateway-system --create-namespace \
		--wait --timeout 120s
	@printf "  $(GREEN)Envoy Gateway controller ready.$(NC)\n"
	@# ── Strimzi Operator ──
	@printf "$(BOLD)[2/5] Installing Strimzi operator...$(NC)\n"
	@$(HELM) repo add strimzi https://strimzi.io/charts/ --force-update 2>/dev/null || true
	@$(HELM) upgrade --install strimzi strimzi/strimzi-kafka-operator \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/strimzi-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Strimzi operator ready.$(NC)\n"
	@# ── Kafka Cluster ──
	@printf "$(BOLD)[3/5] Deploying Kafka cluster (KRaft)...$(NC)\n"
	@$(KUBECTL) apply -f deploy/platform/local/kafka-nodepool.yaml
	@$(KUBECTL) apply -f deploy/platform/local/kafka-cluster.yaml
	@printf "  Waiting for Kafka cluster to be ready (this takes ~60-90s)...\n"
	@$(KUBECTL) wait kafka/bookinfo-kafka -n $(K8S_NS_PLATFORM) \
		--for=condition=Ready --timeout=300s
	@printf "  $(GREEN)Kafka cluster ready.$(NC)\n"
	@# ── Argo Events ──
	@printf "$(BOLD)[4/5] Installing Argo Events controller...$(NC)\n"
	@$(HELM) repo add argo https://argoproj.github.io/argo-helm --force-update 2>/dev/null || true
	@$(HELM) upgrade --install argo-events argo/argo-events \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/argo-events-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Argo Events controller ready.$(NC)\n"
	@# ── Gateway: default-gw ──
	@printf "$(BOLD)[5/5] Applying Gateway default-gw...$(NC)\n"
	@$(KUBECTL) apply -k deploy/gateway/base/
	@printf "  Waiting for Gateway to be programmed...\n"
	@$(KUBECTL) wait gateway/default-gw -n $(K8S_NS_PLATFORM) \
		--for=condition=Programmed --timeout=120s
	@printf "  $(GREEN)Gateway default-gw programmed.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Platform layer complete.$(NC)\n\n"
```

- [ ] **Step 2: Verify target syntax**

Run: `make help`

Expected: `k8s-platform` appears in help output.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add k8s-platform Make target with health gates"
```

---

### Task 6: Gateway API Base Manifests

**Files:**
- Create: `deploy/gateway/base/gateway.yaml`
- Create: `deploy/gateway/base/reference-grant.yaml`
- Create: `deploy/gateway/base/kustomization.yaml`

- [ ] **Step 1: Create Gateway resource**

```yaml
# deploy/gateway/base/gateway.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: default-gw
  namespace: platform
spec:
  gatewayClassName: eg
  listeners:
    - name: web
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: webhooks
      protocol: HTTP
      port: 443
      allowedRoutes:
        namespaces:
          from: All
```

- [ ] **Step 2: Create ReferenceGrant**

```yaml
# deploy/gateway/base/reference-grant.yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-bookinfo-httproutes
  namespace: platform
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: bookinfo
  to:
    - group: gateway.networking.k8s.io
      kind: Gateway
```

- [ ] **Step 3: Create kustomization.yaml**

```yaml
# deploy/gateway/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - gateway.yaml
  - reference-grant.yaml
```

- [ ] **Step 4: Validate kustomize build**

Run: `kustomize build deploy/gateway/base/`

Expected: Outputs Gateway and ReferenceGrant YAML without errors.

- [ ] **Step 5: Commit**

```bash
git add deploy/gateway/
git commit -m "feat: add Gateway API base manifests (default-gw + ReferenceGrant)"
```

---

### Task 7: Gateway API Local Overlay — HTTPRoutes

**Files:**
- Create: `deploy/gateway/overlays/local/httproutes.yaml`
- Create: `deploy/gateway/overlays/local/kustomization.yaml`

- [ ] **Step 1: Create HTTPRoutes for bookinfo**

```yaml
# deploy/gateway/overlays/local/httproutes.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: productpage
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: web
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: productpage
          port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: book-added-webhook
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: webhooks
  rules:
    - matches:
        - path:
            type: Exact
            value: /v1/book-added
      backendRefs:
        - name: book-added-eventsource-svc
          port: 12000
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: review-submitted-webhook
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: webhooks
  rules:
    - matches:
        - path:
            type: Exact
            value: /v1/review-submitted
      backendRefs:
        - name: review-submitted-eventsource-svc
          port: 12001
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: rating-submitted-webhook
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: webhooks
  rules:
    - matches:
        - path:
            type: Exact
            value: /v1/rating-submitted
      backendRefs:
        - name: rating-submitted-eventsource-svc
          port: 12002
```

- [ ] **Step 2: Create kustomization.yaml**

```yaml
# deploy/gateway/overlays/local/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - httproutes.yaml
```

- [ ] **Step 3: Validate kustomize build**

Run: `kustomize build deploy/gateway/overlays/local/`

Expected: Outputs 4 HTTPRoute resources without errors.

- [ ] **Step 4: Commit**

```bash
git add deploy/gateway/overlays/local/
git commit -m "feat: add HTTPRoutes for bookinfo local overlay"
```

---

### Task 8: Observability Helm Values — kube-prometheus-stack, Tempo, Loki

**Files:**
- Create: `deploy/observability/local/kube-prometheus-stack-values.yaml`
- Create: `deploy/observability/local/tempo-values.yaml`
- Create: `deploy/observability/local/loki-values.yaml`

- [ ] **Step 1: Create kube-prometheus-stack values**

```yaml
# deploy/observability/local/kube-prometheus-stack-values.yaml
# kube-prometheus-stack - local dev values
# Chart: prometheus-community/kube-prometheus-stack

prometheus:
  prometheusSpec:
    replicas: 1
    retention: 24h
    enableRemoteWriteReceiver: true
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
      limits:
        cpu: 500m
        memory: 512Mi
    storageSpec:
      volumeClaimTemplate:
        spec:
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 2Gi
  service:
    type: NodePort
    nodePort: 30900

grafana:
  enabled: true
  adminPassword: admin
  service:
    type: NodePort
    nodePort: 30300
  additionalDataSources:
    - name: Tempo
      type: tempo
      url: http://tempo.observability.svc.cluster.local:3100
      access: proxy
      isDefault: false
      jsonData:
        tracesToLogsV2:
          datasourceUid: loki
          filterByTraceID: true
        tracesToMetrics:
          datasourceUid: prometheus
        serviceMap:
          datasourceUid: prometheus
        nodeGraph:
          enabled: true
    - name: Loki
      type: loki
      uid: loki
      url: http://loki.observability.svc.cluster.local:3100
      access: proxy
      isDefault: false
      jsonData:
        derivedFields:
          - datasourceUid: tempo
            matcherRegex: '"trace_id":"(\w+)"'
            name: TraceID
            url: "$${__value.raw}"
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 300m
      memory: 256Mi

# Disable components not needed for local dev
alertmanager:
  enabled: false

kubeControllerManager:
  enabled: false

kubeScheduler:
  enabled: false

kubeProxy:
  enabled: false

kubeEtcd:
  enabled: false
```

- [ ] **Step 2: Create Tempo values**

```yaml
# deploy/observability/local/tempo-values.yaml
# Grafana Tempo - local dev values (monolithic mode)
# Chart: grafana/tempo

tempo:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: "0.0.0.0:4317"
        http:
          endpoint: "0.0.0.0:4318"
  storage:
    trace:
      backend: local
      local:
        path: /var/tempo/traces
      wal:
        path: /var/tempo/wal

persistence:
  enabled: true
  size: 2Gi

resources:
  requests:
    cpu: 50m
    memory: 128Mi
  limits:
    cpu: 300m
    memory: 512Mi
```

- [ ] **Step 3: Create Loki values**

```yaml
# deploy/observability/local/loki-values.yaml
# Grafana Loki - local dev values (SingleBinary mode)
# Chart: grafana/loki

loki:
  commonConfig:
    replication_factor: 1
  schemaConfig:
    configs:
      - from: "2024-04-01"
        store: tsdb
        object_store: filesystem
        schema: v13
        index:
          prefix: loki_index_
          period: 24h
  storage:
    type: filesystem
  limits_config:
    allow_structured_metadata: true
    volume_enabled: true
  auth_enabled: false

deploymentMode: SingleBinary

singleBinary:
  replicas: 1
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 300m
      memory: 512Mi
  persistence:
    enabled: true
    size: 2Gi

# Zero out all other deployment modes
backend:
  replicas: 0
read:
  replicas: 0
write:
  replicas: 0
ingester:
  replicas: 0
querier:
  replicas: 0
queryFrontend:
  replicas: 0
queryScheduler:
  replicas: 0
distributor:
  replicas: 0
compactor:
  replicas: 0
indexGateway:
  replicas: 0
bloomCompactor:
  replicas: 0
bloomGateway:
  replicas: 0

# Disable minio — use filesystem storage
minio:
  enabled: false

# Disable gateway — access Loki directly
gateway:
  enabled: false
```

- [ ] **Step 4: Commit**

```bash
git add deploy/observability/local/
git commit -m "feat: add kube-prometheus-stack, Tempo, Loki Helm values for local k8s"
```

---

### Task 9: Alloy Configs — Logs DaemonSet + Metrics/Traces Deployment

**Files:**
- Create: `deploy/observability/local/alloy-logs-config.alloy`
- Create: `deploy/observability/local/alloy-logs-values.yaml`
- Create: `deploy/observability/local/alloy-metrics-traces-config.alloy`
- Create: `deploy/observability/local/alloy-metrics-traces-values.yaml`

- [ ] **Step 1: Create Alloy logs config (DaemonSet pipeline)**

```alloy
// deploy/observability/local/alloy-logs-config.alloy
// Collects container logs from all pods, enriches with k8s metadata, ships to Loki.

discovery.kubernetes "pods" {
  role = "pod"
  selectors {
    role  = "pod"
    field = "spec.nodeName=" + coalesce(sys.env("HOSTNAME"), constants.hostname)
  }
}

discovery.relabel "pod_logs" {
  targets = discovery.kubernetes.pods.targets

  rule {
    source_labels = ["__meta_kubernetes_namespace"]
    action        = "replace"
    target_label  = "namespace"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_name"]
    action        = "replace"
    target_label  = "pod"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_container_name"]
    action        = "replace"
    target_label  = "container"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app"]
    action        = "replace"
    target_label  = "app"
  }
  rule {
    source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_container_name"]
    action        = "replace"
    target_label  = "job"
    separator     = "/"
    replacement   = "$1"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_uid", "__meta_kubernetes_pod_container_name"]
    action        = "replace"
    target_label  = "__path__"
    separator     = "/"
    replacement   = "/var/log/pods/*$1/*.log"
  }
}

loki.source.kubernetes "pod_logs" {
  targets    = discovery.relabel.pod_logs.output
  forward_to = [loki.process.pod_logs.receiver]
}

loki.process "pod_logs" {
  stage.static_labels {
    values = {
      cluster = "bookinfo-local",
    }
  }
  forward_to = [loki.write.default.receiver]
}

loki.write "default" {
  endpoint {
    url = "http://loki.observability.svc.cluster.local:3100/loki/api/v1/push"
  }
}
```

- [ ] **Step 2: Create Alloy logs Helm values**

```yaml
# deploy/observability/local/alloy-logs-values.yaml
# Grafana Alloy - log collection DaemonSet
# Chart: grafana/alloy

alloy:
  configMap:
    create: false
  mounts:
    varlog: true
  clustering:
    enabled: false

controller:
  type: daemonset

configMap:
  create: true
  name: alloy-logs-config
  key: config.alloy
```

Note: The actual config file content will be loaded via a ConfigMap created from the `.alloy` file in the Make target.

- [ ] **Step 3: Create Alloy metrics/traces config (Deployment pipeline)**

```alloy
// deploy/observability/local/alloy-metrics-traces-config.alloy
// Receives OTLP traces from apps, scrapes Prometheus metrics, forwards to backends.

// ── OTLP Trace Receiver ──
otelcol.receiver.otlp "default" {
  grpc {
    endpoint = "0.0.0.0:4317"
  }
  http {
    endpoint = "0.0.0.0:4318"
  }
  output {
    traces = [otelcol.processor.k8sattributes.default.input]
  }
}

// ── K8s Metadata Enrichment ──
otelcol.processor.k8sattributes "default" {
  extract {
    metadata = [
      "k8s.namespace.name",
      "k8s.deployment.name",
      "k8s.pod.name",
      "k8s.pod.uid",
      "k8s.node.name",
    ]
  }
  output {
    traces = [otelcol.processor.batch.default.input]
  }
}

// ── Batch Processor ──
otelcol.processor.batch "default" {
  timeout = "2s"
  send_batch_size = 512
  output {
    traces = [otelcol.exporter.otlp.tempo.input]
  }
}

// ── Export Traces to Tempo ──
otelcol.exporter.otlp "tempo" {
  client {
    endpoint = "tempo.observability.svc.cluster.local:4317"
    tls {
      insecure = true
    }
  }
}

// ── Prometheus Metrics Scraping ──
// Discover pods with admin port annotation in bookinfo namespace
discovery.kubernetes "bookinfo_pods" {
  role = "pod"
  namespaces {
    names = ["bookinfo"]
  }
}

discovery.relabel "bookinfo_metrics" {
  targets = discovery.kubernetes.bookinfo_pods.targets

  // Only keep pods with the admin port (9090)
  rule {
    source_labels = ["__meta_kubernetes_pod_container_port_number"]
    regex         = "9090"
    action        = "keep"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_container_port_number"]
    target_label  = "__address__"
    replacement   = "$1"
    regex         = "(.*)"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_ip"]
    target_label  = "__address__"
    replacement   = "$1:9090"
  }
  rule {
    source_labels = ["__meta_kubernetes_namespace"]
    target_label  = "namespace"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_name"]
    target_label  = "pod"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app"]
    target_label  = "app"
  }
  rule {
    source_labels = ["__meta_kubernetes_pod_label_role"]
    target_label  = "role"
  }
  rule {
    replacement  = "/metrics"
    target_label = "__metrics_path__"
  }
}

prometheus.scrape "bookinfo" {
  targets    = discovery.relabel.bookinfo_metrics.output
  forward_to = [prometheus.remote_write.default.receiver]
  scrape_interval = "15s"
}

prometheus.remote_write "default" {
  endpoint {
    url = "http://prometheus-kube-prometheus-prometheus.observability.svc.cluster.local:9090/api/v1/write"
  }
}
```

- [ ] **Step 4: Create Alloy metrics/traces Helm values**

```yaml
# deploy/observability/local/alloy-metrics-traces-values.yaml
# Grafana Alloy - metrics + traces Deployment
# Chart: grafana/alloy

alloy:
  configMap:
    create: false
  clustering:
    enabled: false
  extraPorts:
    - name: otlp-grpc
      port: 4317
      targetPort: 4317
      protocol: TCP
    - name: otlp-http
      port: 4318
      targetPort: 4318
      protocol: TCP

controller:
  type: deployment
  replicas: 1

configMap:
  create: true
  name: alloy-metrics-traces-config
  key: config.alloy

serviceAccount:
  create: true

rbac:
  create: true
```

- [ ] **Step 5: Commit**

```bash
git add deploy/observability/local/
git commit -m "feat: add Alloy configs for log collection and metrics/traces pipeline"
```

---

### Task 10: k8s-observability Make Target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add k8s-observability target**

Append after `k8s-platform`:

```makefile
.PHONY: k8s-observability
k8s-observability: ## Install observability: Prometheus, Grafana, Tempo, Loki, Alloy
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Observability Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_OBSERVABILITY) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@# ── kube-prometheus-stack ──
	@printf "$(BOLD)[1/5] Installing kube-prometheus-stack...$(NC)\n"
	@$(HELM) repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install prometheus prometheus-community/kube-prometheus-stack \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/kube-prometheus-stack-values.yaml \
		--wait --timeout 300s
	@printf "  $(GREEN)kube-prometheus-stack ready.$(NC)\n"
	@# ── Tempo ──
	@printf "$(BOLD)[2/5] Installing Tempo...$(NC)\n"
	@$(HELM) repo add grafana https://grafana.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install tempo grafana/tempo \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/tempo-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Tempo ready.$(NC)\n"
	@# ── Loki ──
	@printf "$(BOLD)[3/5] Installing Loki...$(NC)\n"
	@$(HELM) upgrade --install loki grafana/loki \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/loki-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Loki ready.$(NC)\n"
	@# ── Alloy (logs - DaemonSet) ──
	@printf "$(BOLD)[4/5] Installing Alloy (logs)...$(NC)\n"
	@$(KUBECTL) create configmap alloy-logs-config \
		-n $(K8S_NS_OBSERVABILITY) \
		--from-file=config.alloy=deploy/observability/local/alloy-logs-config.alloy \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -
	@$(HELM) upgrade --install alloy-logs grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-logs-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (logs) ready.$(NC)\n"
	@# ── Alloy (metrics+traces - Deployment) ──
	@printf "$(BOLD)[5/5] Installing Alloy (metrics+traces)...$(NC)\n"
	@$(KUBECTL) create configmap alloy-metrics-traces-config \
		-n $(K8S_NS_OBSERVABILITY) \
		--from-file=config.alloy=deploy/observability/local/alloy-metrics-traces-config.alloy \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -
	@$(HELM) upgrade --install alloy-metrics-traces grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-metrics-traces-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (metrics+traces) ready.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Observability layer complete.$(NC)\n\n"
```

- [ ] **Step 2: Verify syntax**

Run: `make help`

Expected: `k8s-observability` appears in help output.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add k8s-observability Make target with health gates"
```

---

### Task 11: PostgreSQL Manifests for bookinfo

**Files:**
- Create: `deploy/postgres/local/statefulset.yaml`
- Create: `deploy/postgres/local/service.yaml`
- Create: `deploy/postgres/local/init-configmap.yaml`
- Create: `deploy/postgres/local/kustomization.yaml`

- [ ] **Step 1: Create PostgreSQL StatefulSet**

```yaml
# deploy/postgres/local/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: bookinfo
  labels:
    app: postgres
    part-of: event-driven-bookinfo
spec:
  serviceName: postgres
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
        part-of: event-driven-bookinfo
    spec:
      containers:
        - name: postgres
          image: postgres:17-alpine
          ports:
            - name: postgres
              containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: bookinfo
            - name: POSTGRES_PASSWORD
              value: bookinfo
          volumeMounts:
            - name: pgdata
              mountPath: /var/lib/postgresql/data
            - name: init-scripts
              mountPath: /docker-entrypoint-initdb.d
          livenessProbe:
            exec:
              command:
                - pg_isready
                - -U
                - bookinfo
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            exec:
              command:
                - pg_isready
                - -U
                - bookinfo
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
      volumes:
        - name: init-scripts
          configMap:
            name: postgres-init
  volumeClaimTemplates:
    - metadata:
        name: pgdata
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 1Gi
```

- [ ] **Step 2: Create PostgreSQL Service**

```yaml
# deploy/postgres/local/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: bookinfo
  labels:
    app: postgres
    part-of: event-driven-bookinfo
spec:
  type: ClusterIP
  selector:
    app: postgres
  ports:
    - name: postgres
      port: 5432
      targetPort: 5432
```

- [ ] **Step 3: Create init ConfigMap**

```yaml
# deploy/postgres/local/init-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-init
  namespace: bookinfo
data:
  init.sql: |
    CREATE DATABASE bookinfo_ratings;
    CREATE DATABASE bookinfo_details;
    CREATE DATABASE bookinfo_reviews;
    CREATE DATABASE bookinfo_notification;
```

- [ ] **Step 4: Create kustomization.yaml**

```yaml
# deploy/postgres/local/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - statefulset.yaml
  - service.yaml
  - init-configmap.yaml
```

- [ ] **Step 5: Validate kustomize build**

Run: `kustomize build deploy/postgres/local/`

Expected: Outputs StatefulSet, Service, and ConfigMap resources.

- [ ] **Step 6: Commit**

```bash
git add deploy/postgres/
git commit -m "feat: add PostgreSQL StatefulSet manifests for local k8s"
```

---

### Task 12: Local Kustomize Overlay — Details (Read/Write Template)

**Files:**
- Create: `deploy/details/overlays/local/kustomization.yaml`
- Create: `deploy/details/overlays/local/deployment-read-patch.yaml`
- Create: `deploy/details/overlays/local/deployment-write.yaml`
- Create: `deploy/details/overlays/local/service-write.yaml`
- Create: `deploy/details/overlays/local/configmap-patch.yaml`

- [ ] **Step 1: Create kustomization.yaml**

```yaml
# deploy/details/overlays/local/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base
  - deployment-write.yaml
  - service-write.yaml

patches:
  - path: deployment-read-patch.yaml
    target:
      kind: Deployment
      name: details
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: details

images:
  - name: event-driven-bookinfo/details
    newTag: local
```

- [ ] **Step 2: Create read deployment patch**

This patches the base deployment to add the `role: read` label so the read Service selector can target it specifically.

```yaml
# deploy/details/overlays/local/deployment-read-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: details
  labels:
    role: read
spec:
  replicas: 1
  template:
    metadata:
      labels:
        role: read
    spec:
      containers:
        - name: details
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits: {}
```

- [ ] **Step 3: Create write deployment**

```yaml
# deploy/details/overlays/local/deployment-write.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: details-write
  namespace: bookinfo
  labels:
    app: details
    role: write
    part-of: event-driven-bookinfo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: details
      role: write
  template:
    metadata:
      labels:
        app: details
        role: write
        part-of: event-driven-bookinfo
      annotations:
        profiles.grafana.com/cpu.scrape: "true"
        profiles.grafana.com/cpu.port: "9090"
        profiles.grafana.com/memory.scrape: "true"
        profiles.grafana.com/memory.port: "9090"
        profiles.grafana.com/goroutine.scrape: "true"
        profiles.grafana.com/goroutine.port: "9090"
        profiles.grafana.com/block.scrape: "true"
        profiles.grafana.com/block.port: "9090"
        profiles.grafana.com/mutex.scrape: "true"
        profiles.grafana.com/mutex.port: "9090"
    spec:
      containers:
        - name: details
          image: event-driven-bookinfo/details:local
          ports:
            - name: http
              containerPort: 8080
            - name: admin
              containerPort: 9090
          envFrom:
            - configMapRef:
                name: details
          livenessProbe:
            httpGet:
              path: /healthz
              port: admin
            initialDelaySeconds: 10
            periodSeconds: 10
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /readyz
              port: admin
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
```

- [ ] **Step 4: Create write Service**

```yaml
# deploy/details/overlays/local/service-write.yaml
apiVersion: v1
kind: Service
metadata:
  name: details-write
  namespace: bookinfo
  labels:
    app: details
    role: write
    part-of: event-driven-bookinfo
spec:
  type: ClusterIP
  selector:
    app: details
    role: write
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

- [ ] **Step 5: Create configmap patch**

```yaml
# deploy/details/overlays/local/configmap-patch.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: details
data:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_details?sslmode=disable"
  RUN_MIGRATIONS: "true"
  OTEL_EXPORTER_OTLP_ENDPOINT: "alloy-metrics-traces.observability.svc.cluster.local:4317"
```

- [ ] **Step 6: Validate kustomize build**

Run: `kustomize build deploy/details/overlays/local/`

Expected: Outputs 2 Deployments (details + details-write), 2 Services (details + details-write), 1 ConfigMap — all in namespace `bookinfo`.

- [ ] **Step 7: Commit**

```bash
git add deploy/details/overlays/local/
git commit -m "feat: add details local overlay with read/write deployment split"
```

---

### Task 13: Local Kustomize Overlays — Reviews, Ratings

**Files:**
- Create: `deploy/reviews/overlays/local/` (5 files)
- Create: `deploy/ratings/overlays/local/` (5 files)

These follow the exact same pattern as details (Task 12), with service-specific values substituted.

- [ ] **Step 1: Create reviews local overlay**

Create `deploy/reviews/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base
  - deployment-write.yaml
  - service-write.yaml

patches:
  - path: deployment-read-patch.yaml
    target:
      kind: Deployment
      name: reviews
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: reviews

images:
  - name: event-driven-bookinfo/reviews
    newTag: local
```

Create `deploy/reviews/overlays/local/deployment-read-patch.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reviews
  labels:
    role: read
spec:
  replicas: 1
  template:
    metadata:
      labels:
        role: read
    spec:
      containers:
        - name: reviews
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits: {}
```

Create `deploy/reviews/overlays/local/deployment-write.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reviews-write
  namespace: bookinfo
  labels:
    app: reviews
    role: write
    part-of: event-driven-bookinfo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: reviews
      role: write
  template:
    metadata:
      labels:
        app: reviews
        role: write
        part-of: event-driven-bookinfo
      annotations:
        profiles.grafana.com/cpu.scrape: "true"
        profiles.grafana.com/cpu.port: "9090"
        profiles.grafana.com/memory.scrape: "true"
        profiles.grafana.com/memory.port: "9090"
        profiles.grafana.com/goroutine.scrape: "true"
        profiles.grafana.com/goroutine.port: "9090"
        profiles.grafana.com/block.scrape: "true"
        profiles.grafana.com/block.port: "9090"
        profiles.grafana.com/mutex.scrape: "true"
        profiles.grafana.com/mutex.port: "9090"
    spec:
      containers:
        - name: reviews
          image: event-driven-bookinfo/reviews:local
          ports:
            - name: http
              containerPort: 8080
            - name: admin
              containerPort: 9090
          envFrom:
            - configMapRef:
                name: reviews
          livenessProbe:
            httpGet:
              path: /healthz
              port: admin
            initialDelaySeconds: 10
            periodSeconds: 10
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /readyz
              port: admin
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
```

Create `deploy/reviews/overlays/local/service-write.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: reviews-write
  namespace: bookinfo
  labels:
    app: reviews
    role: write
    part-of: event-driven-bookinfo
spec:
  type: ClusterIP
  selector:
    app: reviews
    role: write
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

Create `deploy/reviews/overlays/local/configmap-patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: reviews
data:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_reviews?sslmode=disable"
  RUN_MIGRATIONS: "true"
  OTEL_EXPORTER_OTLP_ENDPOINT: "alloy-metrics-traces.observability.svc.cluster.local:4317"
  RATINGS_SERVICE_URL: "http://ratings"
```

- [ ] **Step 2: Create ratings local overlay**

Create `deploy/ratings/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base
  - deployment-write.yaml
  - service-write.yaml

patches:
  - path: deployment-read-patch.yaml
    target:
      kind: Deployment
      name: ratings
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: ratings

images:
  - name: event-driven-bookinfo/ratings
    newTag: local
```

Create `deploy/ratings/overlays/local/deployment-read-patch.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ratings
  labels:
    role: read
spec:
  replicas: 1
  template:
    metadata:
      labels:
        role: read
    spec:
      containers:
        - name: ratings
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits: {}
```

Create `deploy/ratings/overlays/local/deployment-write.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ratings-write
  namespace: bookinfo
  labels:
    app: ratings
    role: write
    part-of: event-driven-bookinfo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ratings
      role: write
  template:
    metadata:
      labels:
        app: ratings
        role: write
        part-of: event-driven-bookinfo
      annotations:
        profiles.grafana.com/cpu.scrape: "true"
        profiles.grafana.com/cpu.port: "9090"
        profiles.grafana.com/memory.scrape: "true"
        profiles.grafana.com/memory.port: "9090"
        profiles.grafana.com/goroutine.scrape: "true"
        profiles.grafana.com/goroutine.port: "9090"
        profiles.grafana.com/block.scrape: "true"
        profiles.grafana.com/block.port: "9090"
        profiles.grafana.com/mutex.scrape: "true"
        profiles.grafana.com/mutex.port: "9090"
    spec:
      containers:
        - name: ratings
          image: event-driven-bookinfo/ratings:local
          ports:
            - name: http
              containerPort: 8080
            - name: admin
              containerPort: 9090
          envFrom:
            - configMapRef:
                name: ratings
          livenessProbe:
            httpGet:
              path: /healthz
              port: admin
            initialDelaySeconds: 10
            periodSeconds: 10
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /readyz
              port: admin
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
```

Create `deploy/ratings/overlays/local/service-write.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ratings-write
  namespace: bookinfo
  labels:
    app: ratings
    role: write
    part-of: event-driven-bookinfo
spec:
  type: ClusterIP
  selector:
    app: ratings
    role: write
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

Create `deploy/ratings/overlays/local/configmap-patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ratings
data:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_ratings?sslmode=disable"
  RUN_MIGRATIONS: "true"
  OTEL_EXPORTER_OTLP_ENDPOINT: "alloy-metrics-traces.observability.svc.cluster.local:4317"
```

- [ ] **Step 3: Validate kustomize builds**

Run: `kustomize build deploy/reviews/overlays/local/ && kustomize build deploy/ratings/overlays/local/`

Expected: Both produce 2 Deployments, 2 Services, 1 ConfigMap each.

- [ ] **Step 4: Commit**

```bash
git add deploy/reviews/overlays/local/ deploy/ratings/overlays/local/
git commit -m "feat: add reviews and ratings local overlays with read/write split"
```

---

### Task 14: Local Kustomize Overlays — Productpage (Read-Only), Notification (Write-Only)

**Files:**
- Create: `deploy/productpage/overlays/local/` (3 files)
- Create: `deploy/notification/overlays/local/` (3 files)

- [ ] **Step 1: Create productpage local overlay**

Create `deploy/productpage/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base

patches:
  - path: deployment-patch.yaml
    target:
      kind: Deployment
      name: productpage
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: productpage

images:
  - name: event-driven-bookinfo/productpage
    newTag: local
```

Create `deploy/productpage/overlays/local/deployment-patch.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: productpage
  labels:
    role: read
spec:
  replicas: 1
  template:
    metadata:
      labels:
        role: read
    spec:
      containers:
        - name: productpage
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits: {}
```

Create `deploy/productpage/overlays/local/configmap-patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: productpage
data:
  LOG_LEVEL: "debug"
  OTEL_EXPORTER_OTLP_ENDPOINT: "alloy-metrics-traces.observability.svc.cluster.local:4317"
  DETAILS_SERVICE_URL: "http://details"
  REVIEWS_SERVICE_URL: "http://reviews"
  RATINGS_SERVICE_URL: "http://ratings"
```

- [ ] **Step 2: Create notification local overlay**

Create `deploy/notification/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base

patches:
  - path: deployment-patch.yaml
    target:
      kind: Deployment
      name: notification
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: notification

images:
  - name: event-driven-bookinfo/notification
    newTag: local
```

Create `deploy/notification/overlays/local/deployment-patch.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification
  labels:
    role: write
spec:
  replicas: 1
  template:
    metadata:
      labels:
        role: write
    spec:
      containers:
        - name: notification
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits: {}
```

Create `deploy/notification/overlays/local/configmap-patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification
data:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_notification?sslmode=disable"
  RUN_MIGRATIONS: "true"
  OTEL_EXPORTER_OTLP_ENDPOINT: "alloy-metrics-traces.observability.svc.cluster.local:4317"
```

- [ ] **Step 3: Validate kustomize builds**

Run: `kustomize build deploy/productpage/overlays/local/ && kustomize build deploy/notification/overlays/local/`

Expected: Each produces 1 Deployment, 1 Service, 1 ConfigMap.

- [ ] **Step 4: Commit**

```bash
git add deploy/productpage/overlays/local/ deploy/notification/overlays/local/
git commit -m "feat: add productpage (read-only) and notification (write-only) local overlays"
```

---

### Task 15: Argo Events Local Overlay

**Files:**
- Create: `deploy/argo-events/overlays/local/kustomization.yaml`
- Create: `deploy/argo-events/overlays/local/eventbus.yaml`
- Create: `deploy/argo-events/overlays/local/sensors/book-added-sensor.yaml`
- Create: `deploy/argo-events/overlays/local/sensors/review-submitted-sensor.yaml`
- Create: `deploy/argo-events/overlays/local/sensors/rating-submitted-sensor.yaml`

- [ ] **Step 1: Create EventBus pointing to Strimzi Kafka**

```yaml
# deploy/argo-events/overlays/local/eventbus.yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: kafka
  namespace: bookinfo
spec:
  kafka:
    url: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    topic: argo-events
    version: "3.9.0"
```

- [ ] **Step 2: Create book-added sensor with -write targets**

```yaml
# deploy/argo-events/overlays/local/sensors/book-added-sensor.yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: book-added-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  dependencies:
    - name: book-added-dep
      eventSourceName: book-added
      eventName: book-added
  triggers:
    - template:
        name: create-detail
        http:
          url: http://details-write.bookinfo.svc.cluster.local/v1/details
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-book-added
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: book-added-dep
                value: "New book added"
              dest: body
            - src:
                dependencyName: book-added-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: book-added-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 3: Create review-submitted sensor with -write targets**

```yaml
# deploy/argo-events/overlays/local/sensors/review-submitted-sensor.yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: review-submitted-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  dependencies:
    - name: review-submitted-dep
      eventSourceName: review-submitted
      eventName: review-submitted
  triggers:
    - template:
        name: create-review
        http:
          url: http://reviews-write.bookinfo.svc.cluster.local/v1/reviews
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-submitted
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: review-submitted-dep
                value: "New review submitted"
              dest: body
            - src:
                dependencyName: review-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 4: Create rating-submitted sensor with -write targets**

```yaml
# deploy/argo-events/overlays/local/sensors/rating-submitted-sensor.yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: rating-submitted-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  dependencies:
    - name: rating-submitted-dep
      eventSourceName: rating-submitted
      eventName: rating-submitted
  triggers:
    - template:
        name: create-rating
        http:
          url: http://ratings-write.bookinfo.svc.cluster.local/v1/ratings
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-rating-submitted
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: rating-submitted-dep
                value: "New rating submitted"
              dest: body
            - src:
                dependencyName: rating-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: rating-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 5: Create kustomization.yaml**

Note: The EventSource files already exist at `deploy/argo-events/eventsources/` (created with the original project). The `../../eventsources/` paths reference these existing files. Kustomize will apply the `namespace: bookinfo` override to them.

```yaml
# deploy/argo-events/overlays/local/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - eventbus.yaml
  - ../../eventsources/book-added.yaml
  - ../../eventsources/review-submitted.yaml
  - ../../eventsources/rating-submitted.yaml
  - sensors/book-added-sensor.yaml
  - sensors/review-submitted-sensor.yaml
  - sensors/rating-submitted-sensor.yaml
```

- [ ] **Step 6: Validate kustomize build**

Run: `kustomize build deploy/argo-events/overlays/local/`

Expected: Outputs EventBus, 3 EventSources, and 3 Sensors — all in namespace `bookinfo`.

- [ ] **Step 7: Commit**

```bash
git add deploy/argo-events/overlays/local/
git commit -m "feat: add Argo Events local overlay with -write sensor targets"
```

---

### Task 16: k8s-deploy Make Target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add k8s-deploy target**

Append after `k8s-observability`:

```makefile
.PHONY: k8s-deploy
k8s-deploy: ## Build images, import to k3d, deploy apps + Argo Events + HTTPRoutes
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Application Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_BOOKINFO) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@# ── Build + Import Images ──
	@printf "$(BOLD)[1/6] Building Docker images...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Building $$svc...\n"; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . || exit 1; \
	done
	@printf "$(BOLD)[2/6] Importing images to k3d...$(NC)\n"
	@for svc in $(SERVICES); do \
		k3d image import event-driven-bookinfo/$$svc:local -c $(K8S_CLUSTER) || exit 1; \
	done
	@printf "  $(GREEN)Images imported.$(NC)\n"
	@# ── PostgreSQL ──
	@printf "$(BOLD)[3/6] Deploying PostgreSQL...$(NC)\n"
	@$(KUBECTL) apply -k deploy/postgres/local/
	@$(KUBECTL) wait statefulset/postgres -n $(K8S_NS_BOOKINFO) \
		--for=jsonpath='{.status.readyReplicas}'=1 --timeout=120s
	@printf "  $(GREEN)PostgreSQL ready.$(NC)\n"
	@# ── App Deployments (local overlays) ──
	@printf "$(BOLD)[4/6] Deploying services...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Applying $$svc local overlay...\n"; \
		$(KUBECTL) apply -k deploy/$$svc/overlays/local/ || exit 1; \
	done
	@# ── Argo Events ──
	@printf "$(BOLD)[5/6] Deploying Argo Events resources...$(NC)\n"
	@$(KUBECTL) apply -k deploy/argo-events/overlays/local/
	@# ── HTTPRoutes ──
	@printf "$(BOLD)[6/6] Applying HTTPRoutes...$(NC)\n"
	@$(KUBECTL) apply -k deploy/gateway/overlays/local/
	@# ── Health gate ──
	@printf "\n$(BOLD)Waiting for deployments...$(NC)\n"
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) wait deployment/$$dep -n $(K8S_NS_BOOKINFO) \
			--for=condition=Available --timeout=120s || true; \
	done
	@printf "\n$(GREEN)$(BOLD)Application layer complete.$(NC)\n\n"
```

- [ ] **Step 2: Add k8s-seed target for database seeding**

```makefile
.PHONY: k8s-seed
k8s-seed: ## Seed databases in k8s PostgreSQL
	$(k8s-guard)
	@printf "\n$(BOLD)Seeding databases...$(NC)\n\n"
	@for svc in details ratings reviews notification; do \
		seed_file="services/$$svc/seeds/seed.sql"; \
		if [ -f "$$seed_file" ]; then \
			$(KUBECTL) exec -n $(K8S_NS_BOOKINFO) statefulset/postgres -- \
				psql -U bookinfo -d bookinfo_$$svc -c "$$(cat $$seed_file)" > /dev/null 2>&1; \
			printf "  $(GREEN)%-14s$(NC) seeded\n" "$$svc"; \
		fi; \
	done
	@printf "\n"
```

- [ ] **Step 3: Verify syntax**

Run: `make help`

Expected: `k8s-deploy` and `k8s-seed` appear in help output.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "feat: add k8s-deploy and k8s-seed Make targets"
```

---

### Task 17: Orchestrator and Utility Make Targets

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add run-k8s, k8s-rebuild, k8s-status, k8s-logs targets**

Append after `k8s-seed`:

```makefile
.PHONY: run-k8s
run-k8s: ## Full local k8s setup: cluster → platform → observability → deploy
	@printf "\n$(BOLD)$(CYAN)════════════════════════════════════════$(NC)\n"
	@printf "$(BOLD)$(CYAN)  Bookinfo Local Kubernetes Environment  $(NC)\n"
	@printf "$(BOLD)$(CYAN)════════════════════════════════════════$(NC)\n\n"
	@$(MAKE) --no-print-directory k8s-cluster
	@$(MAKE) --no-print-directory k8s-platform
	@$(MAKE) --no-print-directory k8s-observability
	@$(MAKE) --no-print-directory k8s-deploy
	@$(MAKE) --no-print-directory k8s-seed
	@$(MAKE) --no-print-directory k8s-status

.PHONY: k8s-rebuild
k8s-rebuild: ## Fast iteration: rebuild images, reimport, rollout restart
	$(k8s-guard)
	@printf "\n$(BOLD)Rebuilding and redeploying...$(NC)\n\n"
	@for svc in $(SERVICES); do \
		printf "  Building $$svc...\n"; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . || exit 1; \
	done
	@for svc in $(SERVICES); do \
		k3d image import event-driven-bookinfo/$$svc:local -c $(K8S_CLUSTER) || exit 1; \
	done
	@for svc in $(SERVICES); do \
		$(KUBECTL) apply -k deploy/$$svc/overlays/local/ || exit 1; \
	done
	@# Rollout restart to pick up new images
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) rollout restart deployment/$$dep -n $(K8S_NS_BOOKINFO) 2>/dev/null || true; \
	done
	@printf "\n$(BOLD)Waiting for rollouts...$(NC)\n"
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) rollout status deployment/$$dep -n $(K8S_NS_BOOKINFO) --timeout=120s 2>/dev/null || true; \
	done
	@printf "\n$(GREEN)$(BOLD)Rebuild complete.$(NC)\n\n"

.PHONY: k8s-status
k8s-status: ## Show pod status and access URLs
	$(k8s-guard)
	@printf "\n$(BOLD)Pod Status:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_BOOKINFO) -o wide 2>/dev/null || true
	@printf "\n$(BOLD)Platform:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_PLATFORM) 2>/dev/null || true
	@printf "\n$(BOLD)Observability:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_OBSERVABILITY) 2>/dev/null || true
	@printf "\n$(BOLD)Access URLs:$(NC)\n\n"
	@printf "  $(CYAN)Productpage:$(NC)  http://localhost:8080\n"
	@printf "  $(CYAN)Grafana:$(NC)      http://localhost:3000  (admin/admin)\n"
	@printf "  $(CYAN)Prometheus:$(NC)   http://localhost:9090\n"
	@printf "\n$(BOLD)Webhooks (via Gateway):$(NC)\n\n"
	@printf "  $(CYAN)book-added:$(NC)         curl -X POST http://localhost:8443/v1/book-added -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "  $(CYAN)review-submitted:$(NC)   curl -X POST http://localhost:8443/v1/review-submitted -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "  $(CYAN)rating-submitted:$(NC)   curl -X POST http://localhost:8443/v1/rating-submitted -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "\n"

.PHONY: k8s-logs
k8s-logs: ## Tail logs from bookinfo namespace
	$(k8s-guard)
	$(KUBECTL) logs -n $(K8S_NS_BOOKINFO) -l part-of=event-driven-bookinfo -f --max-log-requests=10
```

- [ ] **Step 2: Verify all targets in help**

Run: `make help`

Expected: All new targets listed: `run-k8s`, `stop-k8s`, `k8s-cluster`, `k8s-platform`, `k8s-observability`, `k8s-deploy`, `k8s-seed`, `k8s-rebuild`, `k8s-status`, `k8s-logs`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add run-k8s orchestrator and utility Make targets"
```

---

### Task 18: End-to-End Validation

This task validates the full setup works by running `make run-k8s` and testing the read and write paths.

- [ ] **Step 1: Run full setup**

Run: `make run-k8s`

Expected: All layers complete without errors. Final output shows pod status and URLs. This will take 5-10 minutes on first run.

- [ ] **Step 2: Verify all pods are running**

Run: `make k8s-status`

Expected: All pods in `bookinfo`, `platform`, and `observability` namespaces show `Running` or `Ready`.

- [ ] **Step 3: Test read path — productpage via Gateway**

Run: `curl -s http://localhost:8080 | head -20`

Expected: HTML response from productpage.

- [ ] **Step 4: Test write path — fire book-added event via Gateway**

Run:

```bash
curl -X POST http://localhost:8443/v1/book-added \
  -H "Content-Type: application/json" \
  -d '{"title":"Test Book","author":"Test Author","isbn":"978-0-00-000000-0","year":2026,"pages":200}'
```

Expected: 200 OK from the EventSource webhook. After a few seconds, the book-added sensor should trigger and create a detail in the details-write service + send a notification.

- [ ] **Step 5: Verify event was processed — check details**

Run: `curl -s http://localhost:8080` (refresh productpage)

Expected: The "Test Book" detail should appear in the productpage (read from details-read via productpage).

- [ ] **Step 6: Test observability — Grafana**

Open `http://localhost:3000` in browser. Login with admin/admin.

Expected: Grafana loads. Navigate to Explore → select Loki → query `{namespace="bookinfo"}` → see log entries from the services. Select Tempo → see traces. Select Prometheus → query `http_server_requests_total` → see metrics.

- [ ] **Step 7: Test teardown**

Run: `make stop-k8s`

Expected: Cluster deleted cleanly.

- [ ] **Step 8: Test rebuild workflow**

Run:

```bash
make run-k8s          # full setup
# make a code change...
make k8s-rebuild      # fast iteration
```

Expected: `k8s-rebuild` completes much faster than `run-k8s` (skips cluster/platform/observability).

- [ ] **Step 9: Final commit (if any fixes were needed)**

```bash
git add -A
git commit -m "fix: adjustments from e2e validation of local k8s environment"
```
