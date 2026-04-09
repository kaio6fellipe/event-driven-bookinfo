# Headlamp Kubernetes Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy Headlamp as the Kubernetes dashboard in the local k8s environment, accessible at `http://localhost:4466`.

**Architecture:** Official Headlamp Helm chart installed into the `observability` namespace with NodePort service (30444) exposed via k3d port mapping (4466:30444). Same pattern as Grafana and Prometheus.

**Tech Stack:** Helm, k3d, Makefile

---

### Task 1: Create Headlamp Helm values file

**Files:**
- Create: `deploy/observability/local/headlamp-values.yaml`

- [ ] **Step 1: Create the values file**

Create `deploy/observability/local/headlamp-values.yaml`:

```yaml
# Headlamp Kubernetes Dashboard
# Helm chart: headlamp/headlamp
# Repo: https://charts.kubernetes-sigs.io/headlamp

service:
  type: NodePort
  port: 80
  nodePort: 30444

clusterRoleBinding:
  create: true
  clusterRoleName: cluster-admin

ingress:
  enabled: false
```

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/headlamp-values.yaml
git commit -m "feat(observability): add Headlamp Helm values for local k8s"
```

---

### Task 2: Update Makefile — k3d port mapping, observability target, status output

**Files:**
- Modify: `Makefile:232` (k8s-cluster target — add port mapping)
- Modify: `Makefile:298-338` (k8s-observability target — add Headlamp step, update step counts)
- Modify: `Makefile:423-440` (k8s-status target — add Headlamp URL)

- [ ] **Step 1: Add k3d port mapping for Headlamp**

In `Makefile`, inside the `k8s-cluster` target, add a new port mapping line after the Prometheus port mapping (line 232):

Find:
```makefile
			-p "9090:30900@server:0" \
			--k3s-arg "--disable=traefik@server:0" \
```

Replace with:
```makefile
			-p "9090:30900@server:0" \
			-p "4466:30444@server:0" \
			--k3s-arg "--disable=traefik@server:0" \
```

- [ ] **Step 2: Update k8s-observability step counts from [N/6] to [N/7]**

In the `k8s-observability` target, update all step labels from `/6]` to `/7]`:

- `[1/6]` → `[1/7]`
- `[2/6]` → `[2/7]`
- `[3/6]` → `[3/7]`
- `[4/6]` → `[4/7]`
- `[5/6]` → `[5/7]`
- `[6/6]` → `[6/7]`

- [ ] **Step 3: Add Headlamp install step after Grafana dashboards**

In the `k8s-observability` target, add the Headlamp step after the Grafana dashboards block (after line 337: `@printf "  $(GREEN)Grafana dashboards applied.$(NC)\n"`):

```makefile
	@printf "$(BOLD)[7/7] Installing Headlamp dashboard...$(NC)\n"
	@$(HELM) repo add headlamp https://charts.kubernetes-sigs.io/headlamp --force-update 2>/dev/null || true
	@$(HELM) upgrade --install headlamp headlamp/headlamp \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/headlamp-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Headlamp dashboard ready.$(NC)\n"
```

- [ ] **Step 4: Add Headlamp URL to k8s-status output**

In the `k8s-status` target, add the Headlamp URL after the Prometheus line (after line 435: `@printf "  $(CYAN)Prometheus:$(NC)   http://localhost:9090\n"`):

```makefile
	@printf "  $(CYAN)Headlamp:$(NC)     http://localhost:4466\n"
```

- [ ] **Step 5: Update k8s-cluster target description**

In the `k8s-cluster` target help comment (line 223), update the description to mention Headlamp:

Find:
```makefile
k8s-cluster: ##@Kubernetes Create k3d cluster with port mappings for Gateway + observability
```

Replace with:
```makefile
k8s-cluster: ##@Kubernetes Create k3d cluster with port mappings for Gateway, observability + Headlamp
```

- [ ] **Step 6: Update k8s-observability target description**

In the `k8s-observability` target help comment (line 299), update the description to mention Headlamp:

Find:
```makefile
k8s-observability: ##@Kubernetes Install observability: Prometheus, Grafana, Tempo, Loki, Alloy
```

Replace with:
```makefile
k8s-observability: ##@Kubernetes Install observability: Prometheus, Grafana, Tempo, Loki, Alloy, Headlamp
```

- [ ] **Step 7: Commit**

```bash
git add Makefile
git commit -m "feat(observability): add Headlamp to k8s cluster setup and status"
```

---

### Task 3: Update CLAUDE.md access documentation

**Files:**
- Modify: `CLAUDE.md:55`

- [ ] **Step 1: Add Headlamp to the Access line**

In `CLAUDE.md`, update the Access line (line 55):

Find:
```
**Access:** Productpage http://localhost:8080, Webhooks POST http://localhost:8080/v1/* (method-based CQRS routing), Grafana http://localhost:3000, Prometheus http://localhost:9090
```

Replace with:
```
**Access:** Productpage http://localhost:8080, Webhooks POST http://localhost:8080/v1/* (method-based CQRS routing), Grafana http://localhost:3000, Prometheus http://localhost:9090, Headlamp http://localhost:4466
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add Headlamp URL to project access documentation"
```

---

### Task 4: Deploy and verify

**Files:** None (verification only)

- [ ] **Step 1: Recreate the k3d cluster with the new port mapping**

The new `-p "4466:30444@server:0"` port mapping requires recreating the cluster:

```bash
make stop-k8s
make run-k8s
```

This will create a fresh cluster with the Headlamp port mapping, install all platform + observability + app layers, and print status with the new Headlamp URL.

- [ ] **Step 2: Verify Headlamp pod is running**

```bash
kubectl --context=k3d-bookinfo-local get pods -n observability -l app.kubernetes.io/name=headlamp
```

Expected: One pod in `Running` state.

- [ ] **Step 3: Verify Headlamp is accessible**

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:4466
```

Expected: `200`

- [ ] **Step 4: Verify k8s-status output includes Headlamp**

```bash
make k8s-status
```

Expected: Output includes `Headlamp:     http://localhost:4466`
