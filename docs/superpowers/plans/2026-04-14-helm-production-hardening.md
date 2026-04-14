# Helm Chart Production Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 10 production-readiness features to the bookinfo-service Helm chart: security contexts, service accounts, secret references, PDB, scheduling, annotations, imagePullSecrets, extraEnv, HTTPRoute hostnames/timeouts, and service annotations.

**Architecture:** All changes are additive to existing templates via conditional blocks. New fields have safe defaults (empty/false/no-op) so existing deployments are unaffected. Two new templates: serviceaccount.yaml and pdb.yaml.

**Tech Stack:** Helm 3, Kubernetes Gateway API v1

**Spec:** `docs/superpowers/specs/2026-04-14-helm-chart-production-hardening-design.md`

---

## File Map

### New Files

| File | Responsibility |
|---|---|
| `charts/bookinfo-service/templates/serviceaccount.yaml` | Conditional ServiceAccount creation |
| `charts/bookinfo-service/templates/pdb.yaml` | Conditional PodDisruptionBudget |

### Modified Files

| File | Changes |
|---|---|
| `charts/bookinfo-service/values.yaml` | Add all 10 new value sections |
| `charts/bookinfo-service/values.schema.json` | Add schemas for new fields |
| `charts/bookinfo-service/templates/_helpers.tpl` | Add `serviceAccountName` helper |
| `charts/bookinfo-service/templates/deployment.yaml` | Add securityContext, SA, secrets, imagePullSecrets, scheduling, annotations, extraEnv |
| `charts/bookinfo-service/templates/deployment-write.yaml` | Same additions as deployment.yaml |
| `charts/bookinfo-service/templates/service.yaml` | Add serviceAnnotations |
| `charts/bookinfo-service/templates/service-write.yaml` | Add serviceAnnotations |
| `charts/bookinfo-service/templates/httproute.yaml` | Add hostnames, timeouts |

---

## Task 1: values.yaml + _helpers.tpl + values.schema.json

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`
- Modify: `charts/bookinfo-service/templates/_helpers.tpl`
- Modify: `charts/bookinfo-service/values.schema.json`

- [ ] **Step 1: Add all new sections to values.yaml**

Insert these sections into `charts/bookinfo-service/values.yaml`. Add after the `image:` section (line 9) and before `# -- Deployment`:

```yaml

# -- Service Account
serviceAccount:
  create: true
  name: ""
  annotations: {}

# -- Image pull secrets
imagePullSecrets: []

```

Add after the `resources:` section (line 19) and before `# -- Ports`:

```yaml

# -- Pod security
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

# -- Pod scheduling
nodeSelector: {}
tolerations: []
affinity: {}
topologySpreadConstraints: []

# -- Pod metadata
podAnnotations: {}
podLabels: {}

# -- Extra environment variables
extraEnv: []

# -- Existing secret to mount as env vars
existingSecret: ""

```

Add after the `autoscaling:` section (line 85) and before `# -- Gateway`:

```yaml

# -- Pod Disruption Budget
pdb:
  enabled: false
  minAvailable: 1

# -- Service annotations
serviceAnnotations: {}

```

Add `hostnames` and `timeouts` to the existing `gateway:` section. Change it from:

```yaml
# -- Gateway parentRef for HTTPRoutes
gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

to:

```yaml
# -- Gateway parentRef for HTTPRoutes
gateway:
  name: default-gw
  namespace: platform
  sectionName: web
  hostnames: []
  timeouts:
    request: ""
    backendRequest: ""
```

- [ ] **Step 2: Add serviceAccountName helper to _helpers.tpl**

Append this to the end of `charts/bookinfo-service/templates/_helpers.tpl`:

```gotpl

{{/*
Service account name.
*/}}
{{- define "bookinfo-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "bookinfo-service.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
```

- [ ] **Step 3: Add all new field schemas to values.schema.json**

Add these properties to the top-level `properties` object in `charts/bookinfo-service/values.schema.json`:

```json
    "serviceAccount": {
      "type": "object",
      "properties": {
        "create": { "type": "boolean" },
        "name": { "type": "string" },
        "annotations": { "type": "object" }
      }
    },
    "imagePullSecrets": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" }
        }
      }
    },
    "podSecurityContext": { "type": "object" },
    "securityContext": { "type": "object" },
    "nodeSelector": { "type": "object" },
    "tolerations": { "type": "array" },
    "affinity": { "type": "object" },
    "topologySpreadConstraints": { "type": "array" },
    "podAnnotations": { "type": "object" },
    "podLabels": { "type": "object" },
    "extraEnv": { "type": "array" },
    "existingSecret": { "type": "string" },
    "pdb": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean" },
        "minAvailable": {}
      }
    },
    "serviceAnnotations": { "type": "object" },
```

- [ ] **Step 4: Verify helm lint passes**

Run: `helm lint charts/bookinfo-service`
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/values.yaml charts/bookinfo-service/templates/_helpers.tpl charts/bookinfo-service/values.schema.json
git commit -m "feat(helm): add production hardening values, SA helper, and schema"
```

---

## Task 2: Deployment Templates — All Production Fields

**Files:**
- Modify: `charts/bookinfo-service/templates/deployment.yaml`
- Modify: `charts/bookinfo-service/templates/deployment-write.yaml`

- [ ] **Step 1: Rewrite deployment.yaml with all production fields**

Replace the entire content of `charts/bookinfo-service/templates/deployment.yaml` with:

```gotpl
{{/* charts/bookinfo-service/templates/deployment.yaml */}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
    {{- if .Values.cqrs.enabled }}
    role: read
    {{- end }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ if .Values.cqrs.enabled }}{{ .Values.cqrs.read.replicas }}{{ else }}{{ .Values.replicaCount }}{{ end }}
  {{- end }}
  selector:
    matchLabels:
      {{- if .Values.cqrs.enabled }}
      {{- include "bookinfo-service.readSelectorLabels" . | nindent 6 }}
      {{- else }}
      {{- include "bookinfo-service.selectorLabels" . | nindent 6 }}
      {{- end }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "bookinfo-service.labels" . | nindent 8 }}
        {{- if .Values.cqrs.enabled }}
        role: read
        {{- end }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      serviceAccountName: {{ include "bookinfo-service.serviceAccountName" . }}
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.topologySpreadConstraints }}
      topologySpreadConstraints:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: {{ include "bookinfo-service.serviceName" . }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          {{- with .Values.securityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            - name: http
              containerPort: {{ .Values.ports.http }}
            - name: admin
              containerPort: {{ .Values.ports.admin }}
          envFrom:
            - configMapRef:
                name: {{ include "bookinfo-service.configmapName" . }}
            {{- with .Values.existingSecret }}
            - secretRef:
                name: {{ . }}
            {{- end }}
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            {{- with .Values.extraEnv }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          livenessProbe:
            httpGet:
              path: {{ .Values.probes.liveness.path }}
              port: {{ .Values.probes.liveness.port }}
            initialDelaySeconds: {{ .Values.probes.liveness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
            failureThreshold: {{ .Values.probes.liveness.failureThreshold }}
          readinessProbe:
            httpGet:
              path: {{ .Values.probes.readiness.path }}
              port: {{ .Values.probes.readiness.port }}
            initialDelaySeconds: {{ .Values.probes.readiness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
            failureThreshold: {{ .Values.probes.readiness.failureThreshold }}
          resources:
            {{- if and .Values.cqrs.enabled .Values.cqrs.read.resources }}
            {{- toYaml .Values.cqrs.read.resources | nindent 12 }}
            {{- else }}
            {{- toYaml .Values.resources | nindent 12 }}
            {{- end }}
```

- [ ] **Step 2: Rewrite deployment-write.yaml with all production fields**

Replace the entire content of `charts/bookinfo-service/templates/deployment-write.yaml` with:

```gotpl
{{/* charts/bookinfo-service/templates/deployment-write.yaml */}}
{{- if .Values.cqrs.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "bookinfo-service.fullname" . }}-write
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
    role: write
spec:
  replicas: {{ .Values.cqrs.write.replicas }}
  selector:
    matchLabels:
      {{- include "bookinfo-service.writeSelectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "bookinfo-service.labels" . | nindent 8 }}
        role: write
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      serviceAccountName: {{ include "bookinfo-service.serviceAccountName" . }}
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.topologySpreadConstraints }}
      topologySpreadConstraints:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: {{ include "bookinfo-service.serviceName" . }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          {{- with .Values.securityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            - name: http
              containerPort: {{ .Values.ports.http }}
            - name: admin
              containerPort: {{ .Values.ports.admin }}
          envFrom:
            - configMapRef:
                name: {{ include "bookinfo-service.configmapName" . }}
            {{- with .Values.existingSecret }}
            - secretRef:
                name: {{ . }}
            {{- end }}
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            {{- with .Values.extraEnv }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          livenessProbe:
            httpGet:
              path: {{ .Values.probes.liveness.path }}
              port: {{ .Values.probes.liveness.port }}
            initialDelaySeconds: {{ .Values.probes.liveness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
            failureThreshold: {{ .Values.probes.liveness.failureThreshold }}
          readinessProbe:
            httpGet:
              path: {{ .Values.probes.readiness.path }}
              port: {{ .Values.probes.readiness.port }}
            initialDelaySeconds: {{ .Values.probes.readiness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
            failureThreshold: {{ .Values.probes.readiness.failureThreshold }}
          resources:
            {{- if .Values.cqrs.write.resources }}
            {{- toYaml .Values.cqrs.write.resources | nindent 12 }}
            {{- else }}
            {{- toYaml .Values.resources | nindent 12 }}
            {{- end }}
{{- end }}
```

- [ ] **Step 3: Verify rendering with defaults**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep -A 3 'securityContext'
```

Expected: Both pod-level (`runAsNonRoot: true`) and container-level (`allowPrivilegeEscalation: false`) security contexts rendered.

- [ ] **Step 4: Verify serviceAccountName rendered**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep 'serviceAccountName'
```

Expected: `serviceAccountName: ratings`

- [ ] **Step 5: Verify existingSecret not rendered when empty**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep 'secretRef'
```

Expected: No output

- [ ] **Step 6: Verify existingSecret renders when set**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo \
  --set existingSecret=ratings-db-creds | grep -A 1 'secretRef'
```

Expected: `secretRef:` with `name: ratings-db-creds`

- [ ] **Step 7: Commit**

```bash
git add charts/bookinfo-service/templates/deployment.yaml charts/bookinfo-service/templates/deployment-write.yaml
git commit -m "feat(helm): add production fields to deployment templates

SecurityContext, ServiceAccount, existingSecret, imagePullSecrets,
scheduling (nodeSelector, tolerations, affinity, topologySpreadConstraints),
podAnnotations, podLabels, extraEnv."
```

---

## Task 3: ServiceAccount, PDB, Service Annotations, HTTPRoute Enhancements

**Files:**
- Create: `charts/bookinfo-service/templates/serviceaccount.yaml`
- Create: `charts/bookinfo-service/templates/pdb.yaml`
- Modify: `charts/bookinfo-service/templates/service.yaml`
- Modify: `charts/bookinfo-service/templates/service-write.yaml`
- Modify: `charts/bookinfo-service/templates/httproute.yaml`

- [ ] **Step 1: Create serviceaccount.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/serviceaccount.yaml */}}
{{- if .Values.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "bookinfo-service.serviceAccountName" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

- [ ] **Step 2: Create pdb.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/pdb.yaml */}}
{{- if .Values.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
spec:
  minAvailable: {{ .Values.pdb.minAvailable }}
  selector:
    matchLabels:
      {{- if .Values.cqrs.enabled }}
      {{- include "bookinfo-service.readSelectorLabels" . | nindent 6 }}
      {{- else }}
      {{- include "bookinfo-service.selectorLabels" . | nindent 6 }}
      {{- end }}
{{- end }}
```

- [ ] **Step 3: Add annotations to service.yaml**

Replace the content of `charts/bookinfo-service/templates/service.yaml` with:

```gotpl
{{/* charts/bookinfo-service/templates/service.yaml */}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
  {{- with .Values.serviceAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: ClusterIP
  selector:
    {{- if .Values.cqrs.enabled }}
    {{- include "bookinfo-service.readSelectorLabels" . | nindent 4 }}
    {{- else }}
    {{- include "bookinfo-service.selectorLabels" . | nindent 4 }}
    {{- end }}
  ports:
    - name: http
      port: 80
      targetPort: {{ .Values.ports.http }}
```

- [ ] **Step 4: Add annotations to service-write.yaml**

Replace the content of `charts/bookinfo-service/templates/service-write.yaml` with:

```gotpl
{{/* charts/bookinfo-service/templates/service-write.yaml */}}
{{- if .Values.cqrs.enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "bookinfo-service.fullname" . }}-write
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
    role: write
  {{- with .Values.serviceAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: ClusterIP
  selector:
    {{- include "bookinfo-service.writeSelectorLabels" . | nindent 4 }}
  ports:
    - name: http
      port: 80
      targetPort: {{ .Values.ports.http }}
{{- end }}
```

- [ ] **Step 5: Add hostnames and timeouts to httproute.yaml**

Replace the content of `charts/bookinfo-service/templates/httproute.yaml` with:

```gotpl
{{/* charts/bookinfo-service/templates/httproute.yaml */}}
{{- range $eventName, $endpoint := .Values.cqrs.endpoints }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}-write
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  {{- with $.Values.gateway.hostnames }}
  hostnames:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    - matches:
        - path:
            type: Exact
            value: {{ $endpoint.endpoint }}
          method: {{ $endpoint.method }}
      backendRefs:
        - name: {{ $eventName }}-eventsource-svc
          port: {{ $endpoint.port }}
      {{- if or $.Values.gateway.timeouts.request $.Values.gateway.timeouts.backendRequest }}
      timeouts:
        {{- with $.Values.gateway.timeouts.request }}
        request: {{ . | quote }}
        {{- end }}
        {{- with $.Values.gateway.timeouts.backendRequest }}
        backendRequest: {{ . | quote }}
        {{- end }}
      {{- end }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}-read
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  {{- with $.Values.gateway.hostnames }}
  hostnames:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: {{ $endpoint.endpoint }}
          method: GET
      backendRefs:
        - name: {{ include "bookinfo-service.fullname" $ }}
          port: 80
      {{- if or $.Values.gateway.timeouts.request $.Values.gateway.timeouts.backendRequest }}
      timeouts:
        {{- with $.Values.gateway.timeouts.request }}
        request: {{ . | quote }}
        {{- end }}
        {{- with $.Values.gateway.timeouts.backendRequest }}
        backendRequest: {{ . | quote }}
        {{- end }}
      {{- end }}
{{- end }}
{{- range $route := .Values.routes }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ $route.name }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  {{- with $.Values.gateway.hostnames }}
  hostnames:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    - matches:
        - path:
            type: {{ $route.pathType }}
            value: {{ $route.path }}
          method: {{ $route.method }}
      backendRefs:
        - name: {{ include "bookinfo-service.fullname" $ }}
          port: 80
      {{- if or $.Values.gateway.timeouts.request $.Values.gateway.timeouts.backendRequest }}
      timeouts:
        {{- with $.Values.gateway.timeouts.request }}
        request: {{ . | quote }}
        {{- end }}
        {{- with $.Values.gateway.timeouts.backendRequest }}
        backendRequest: {{ . | quote }}
        {{- end }}
      {{- end }}
{{- end }}
```

- [ ] **Step 6: Verify ServiceAccount renders when create=true**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep -A 5 'kind: ServiceAccount'
```

Expected: ServiceAccount named `ratings`

- [ ] **Step 7: Verify PDB not rendered by default**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep 'PodDisruptionBudget'
```

Expected: No output

- [ ] **Step 8: Verify PDB renders when enabled**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo \
  --set pdb.enabled=true | grep -A 10 'PodDisruptionBudget'
```

Expected: PDB with `minAvailable: 1`, selector matching read labels

- [ ] **Step 9: Verify HTTPRoute hostnames render when set**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo \
  --set 'gateway.hostnames[0]=bookinfo.example.com' | grep -A 2 'hostnames'
```

Expected: `hostnames:` with `- bookinfo.example.com` under each HTTPRoute

- [ ] **Step 10: Verify HTTPRoute hostnames NOT rendered when empty (default)**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep 'hostnames'
```

Expected: No output

- [ ] **Step 11: Verify HTTPRoute timeouts render when set**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo \
  --set 'gateway.timeouts.request=10s' \
  --set 'gateway.timeouts.backendRequest=5s' | grep -A 3 'timeouts'
```

Expected: `timeouts:` with `request: "10s"` and `backendRequest: "5s"`

- [ ] **Step 12: Run full helm lint**

Run:
```bash
for svc in productpage details reviews ratings notification dlqueue; do
  helm lint charts/bookinfo-service -f deploy/$svc/values-local.yaml || exit 1
done
echo "All lints passed."
```

Expected: `All lints passed.`

- [ ] **Step 13: Commit**

```bash
git add charts/bookinfo-service/templates/serviceaccount.yaml charts/bookinfo-service/templates/pdb.yaml charts/bookinfo-service/templates/service.yaml charts/bookinfo-service/templates/service-write.yaml charts/bookinfo-service/templates/httproute.yaml
git commit -m "feat(helm): add ServiceAccount, PDB, service annotations, HTTPRoute hostnames/timeouts"
```

---

## Task 4: E2E Validation

Full k8s lifecycle test to confirm production hardening doesn't break existing deployments.

**Files:** None (runtime validation)

- [ ] **Step 1: Tear down any existing cluster**

Run: `make stop-k8s`

- [ ] **Step 2: Stand up the full environment**

Run: `make run-k8s`

Expected: All pods Running. SecurityContext defaults may cause issues if any service writes to the filesystem — check for CrashLoopBackOff.

- [ ] **Step 3: Verify pod status**

Run: `make k8s-status`

Expected: All pods Running. If productpage crashes due to `readOnlyRootFilesystem: true`, check logs and override in values-local.yaml if needed.

- [ ] **Step 4: Verify ServiceAccounts created**

Run:
```bash
kubectl --context=k3d-bookinfo-local get serviceaccounts -n bookinfo | grep -v default
```

Expected: One SA per helm-installed service (ratings, details, reviews, notification, productpage, dlqueue)

- [ ] **Step 5: Verify sync reads + async write**

Run:
```bash
curl -sf http://localhost:8080/v1/details | python3 -c "import json,sys; print(f'{len(json.load(sys.stdin))} details')"
curl -sf http://localhost:8080/v1/ratings/p0001 | python3 -c "import json,sys; print(json.load(sys.stdin)['product_id'])"
curl -sf http://localhost:8080/ | head -3

curl -sf -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"p0001","stars":5,"reviewer":"prod-hardening-test"}'
sleep 5
curl -sf http://localhost:8080/v1/ratings/p0001 | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'count={d[\"count\"]}')"
```

Expected: All reads return data, async write creates rating (count=1).

- [ ] **Step 6: Run k6 load test**

Run: `make k8s-load DURATION=30s BASE_RATE=2`

Expected: 0% error rate, all checks pass.

- [ ] **Step 7: Fix any issues**

If securityContext causes crashes (e.g., `readOnlyRootFilesystem` blocks temp file writes), add overrides to the affected service's values-local.yaml. Commit fixes.

- [ ] **Step 8: Tear down cluster**

Run: `make stop-k8s`
