# Bookinfo Service Helm Chart — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a single reusable Helm chart that replaces all 6 services' duplicated Kustomize manifests, supporting CQRS split deployments and Argo Events resource generation.

**Architecture:** One `bookinfo-service` chart installed N times with per-service `values.yaml` overrides. CQRS is a boolean toggle that produces read+write Deployments. EventSource/Sensor are generated from a structured `cqrs.endpoints` map with DLQ auto-wiring.

**Tech Stack:** Helm 3, chart-testing (ct), chart-releaser-action, GitHub Pages, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-04-14-bookinfo-helm-chart-design.md`

---

## File Map

### New Files — Chart

| File | Responsibility |
|---|---|
| `charts/bookinfo-service/Chart.yaml` | Chart metadata and version |
| `charts/bookinfo-service/values.yaml` | Full defaults (CQRS off, no events) |
| `charts/bookinfo-service/values.schema.json` | JSON Schema for `helm lint` validation |
| `charts/bookinfo-service/templates/_helpers.tpl` | Naming, labels, selectors, common helpers |
| `charts/bookinfo-service/templates/configmap.yaml` | ConfigMap from `config` map + computed fields |
| `charts/bookinfo-service/templates/deployment.yaml` | Read/single Deployment |
| `charts/bookinfo-service/templates/deployment-write.yaml` | Write Deployment (CQRS only) |
| `charts/bookinfo-service/templates/service.yaml` | ClusterIP Service for read/single |
| `charts/bookinfo-service/templates/service-write.yaml` | ClusterIP Service for write (CQRS only) |
| `charts/bookinfo-service/templates/eventsource.yaml` | One EventSource per `cqrs.endpoints` entry |
| `charts/bookinfo-service/templates/eventsource-service.yaml` | K8s Service exposing EventSource webhook |
| `charts/bookinfo-service/templates/sensor.yaml` | Sensor with triggers + DLQ auto-generation |
| `charts/bookinfo-service/templates/hpa.yaml` | HPA (autoscaling only) |
| `charts/bookinfo-service/templates/NOTES.txt` | Post-install usage notes |

### New Files — CI Test Values

| File | Responsibility |
|---|---|
| `charts/bookinfo-service/ci/values-ratings-cqrs.yaml` | ct test: full CQRS + EventSource + Sensor |
| `charts/bookinfo-service/ci/values-productpage-simple.yaml` | ct test: single deployment, no events |
| `charts/bookinfo-service/ci/values-dlqueue-no-dlq.yaml` | ct test: CQRS + events, DLQ disabled |

### New Files — Per-Service Values

| File | Responsibility |
|---|---|
| `deploy/details/values-local.yaml` | Details local k8s config |
| `deploy/reviews/values-local.yaml` | Reviews local k8s config (multi-endpoint) |
| `deploy/ratings/values-local.yaml` | Ratings local k8s config |
| `deploy/notification/values-local.yaml` | Notification local k8s config |
| `deploy/productpage/values-local.yaml` | Productpage local k8s config |
| `deploy/dlqueue/values-local.yaml` | DLQueue local k8s config |

### New Files — CI/CD Workflows

| File | Responsibility |
|---|---|
| `.github/workflows/helm-lint-test.yml` | PR validation: ct lint + ct install |
| `.github/workflows/helm-release.yml` | Main push: chart-releaser to GitHub Pages |
| `ct.yaml` | chart-testing configuration |

### Modified Files

| File | Change |
|---|---|
| `Makefile` | `k8s-deploy` and `k8s-rebuild` switch to `helm upgrade --install`; add `helm-lint` and `helm-template` targets |
| `CLAUDE.md` | Update Deploy Structure and Local Kubernetes sections |
| `.claude/skills/new-service/SKILL.md` | Step 8: Kustomize → Helm values |
| `.claude/rules/release.md` | Add chart release process section |

### Deleted After Validation (Task 12)

All `deploy/{details,reviews,ratings,notification,productpage,dlqueue}/base/` and `deploy/{details,reviews,ratings,notification,productpage,dlqueue}/overlays/` directories.

---

## Task 1: Chart Scaffold — Chart.yaml, values.yaml, _helpers.tpl

**Files:**
- Create: `charts/bookinfo-service/Chart.yaml`
- Create: `charts/bookinfo-service/values.yaml`
- Create: `charts/bookinfo-service/templates/_helpers.tpl`

- [ ] **Step 1: Create Chart.yaml**

```yaml
# charts/bookinfo-service/Chart.yaml
apiVersion: v2
name: bookinfo-service
description: Reusable Helm chart for event-driven-bookinfo microservices
type: application
version: 0.1.0
appVersion: "0.0.0"
```

- [ ] **Step 2: Create values.yaml with full defaults**

```yaml
# charts/bookinfo-service/values.yaml

# -- Service identity
nameOverride: ""
fullnameOverride: ""
serviceName: ""

image:
  repository: event-driven-bookinfo/service
  tag: latest
  pullPolicy: IfNotPresent

# -- Deployment
replicaCount: 1
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi

# -- Ports
ports:
  http: 8080
  admin: 9090

# -- Probes
probes:
  liveness:
    path: /healthz
    port: admin
    initialDelaySeconds: 10
    periodSeconds: 10
    failureThreshold: 5
  readiness:
    path: /readyz
    port: admin
    initialDelaySeconds: 5
    periodSeconds: 5
    failureThreshold: 3

# -- Config (injected as ConfigMap env vars)
config:
  LOG_LEVEL: "info"
  STORAGE_BACKEND: "memory"

# -- CQRS
cqrs:
  enabled: false
  read:
    replicas: 1
    resources: {}
  write:
    replicas: 1
    resources: {}
  eventBusName: kafka
  endpoints: {}

# -- Sensor defaults
sensor:
  retryStrategy:
    steps: 3
    duration: 2s
    factor: "2.0"
    jitter: "1"
  atLeastOnce: true
  dlq:
    enabled: true
    url: ""
    retryStrategy:
      steps: 5
      duration: 2s
      factor: "2.0"
      jitter: "1"

# -- Observability
observability:
  otelEndpoint: ""
  pyroscopeAddress: ""

# -- Autoscaling
autoscaling:
  enabled: false
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilization: 70
```

- [ ] **Step 3: Create _helpers.tpl**

```gotpl
{{/* charts/bookinfo-service/templates/_helpers.tpl */}}

{{/*
Expand the name of the chart.
*/}}
{{- define "bookinfo-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this.
*/}}
{{- define "bookinfo-service.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart label value.
*/}}
{{- define "bookinfo-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "bookinfo-service.labels" -}}
helm.sh/chart: {{ include "bookinfo-service.chart" . }}
{{ include "bookinfo-service.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
part-of: event-driven-bookinfo
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "bookinfo-service.selectorLabels" -}}
app: {{ include "bookinfo-service.fullname" . }}
{{- end }}

{{/*
Write selector labels (adds role: write).
*/}}
{{- define "bookinfo-service.writeSelectorLabels" -}}
{{ include "bookinfo-service.selectorLabels" . }}
role: write
{{- end }}

{{/*
Read selector labels (adds role: read). Only used when CQRS is enabled.
*/}}
{{- define "bookinfo-service.readSelectorLabels" -}}
{{ include "bookinfo-service.selectorLabels" . }}
role: read
{{- end }}

{{/*
ConfigMap name.
*/}}
{{- define "bookinfo-service.configmapName" -}}
{{ include "bookinfo-service.fullname" . }}
{{- end }}

{{/*
Resolve SERVICE_NAME: explicit serviceName or fallback to fullname.
*/}}
{{- define "bookinfo-service.serviceName" -}}
{{- default (include "bookinfo-service.fullname" .) .Values.serviceName }}
{{- end }}

{{/*
Resolve the write deployment URL for "url: self" triggers.
*/}}
{{- define "bookinfo-service.writeURL" -}}
{{- if .Values.cqrs.enabled -}}
http://{{ include "bookinfo-service.fullname" . }}-write.{{ .Release.Namespace }}.svc.cluster.local
{{- else -}}
http://{{ include "bookinfo-service.fullname" . }}.{{ .Release.Namespace }}.svc.cluster.local
{{- end -}}
{{- end }}

{{/*
Sensor name: derived from release name.
*/}}
{{- define "bookinfo-service.sensorName" -}}
{{ include "bookinfo-service.fullname" . }}-sensor
{{- end }}
```

- [ ] **Step 4: Verify chart scaffolding**

Run: `helm lint charts/bookinfo-service`
Expected: `1 chart(s) linted, 0 chart(s) failed` (will warn about no templates yet — that's OK)

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/Chart.yaml charts/bookinfo-service/values.yaml charts/bookinfo-service/templates/_helpers.tpl
git commit -m "feat(helm): scaffold bookinfo-service chart with Chart.yaml, values, and helpers"
```

---

## Task 2: ConfigMap Template

**Files:**
- Create: `charts/bookinfo-service/templates/configmap.yaml`

- [ ] **Step 1: Create configmap.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/configmap.yaml */}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
data:
  SERVICE_NAME: {{ include "bookinfo-service.serviceName" . | quote }}
  HTTP_PORT: {{ .Values.ports.http | quote }}
  ADMIN_PORT: {{ .Values.ports.admin | quote }}
  {{- range $key, $value := .Values.config }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
  {{- with .Values.observability.otelEndpoint }}
  OTEL_EXPORTER_OTLP_ENDPOINT: {{ . | quote }}
  {{- end }}
  {{- with .Values.observability.pyroscopeAddress }}
  PYROSCOPE_SERVER_ADDRESS: {{ . | quote }}
  {{- end }}
```

- [ ] **Step 2: Verify template renders**

Run: `helm template test-svc charts/bookinfo-service --set serviceName=ratings | grep -A 20 'kind: ConfigMap'`
Expected: ConfigMap with `SERVICE_NAME: "ratings"`, `HTTP_PORT: "8080"`, `ADMIN_PORT: "9090"`, `LOG_LEVEL: "info"`, `STORAGE_BACKEND: "memory"`

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/configmap.yaml
git commit -m "feat(helm): add ConfigMap template"
```

---

## Task 3: Deployment and Service Templates (Single Mode)

**Files:**
- Create: `charts/bookinfo-service/templates/deployment.yaml`
- Create: `charts/bookinfo-service/templates/service.yaml`

- [ ] **Step 1: Create deployment.yaml**

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
      labels:
        {{- include "bookinfo-service.labels" . | nindent 8 }}
        {{- if .Values.cqrs.enabled }}
        role: read
        {{- end }}
    spec:
      containers:
        - name: {{ include "bookinfo-service.serviceName" . }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.ports.http }}
            - name: admin
              containerPort: {{ .Values.ports.admin }}
          envFrom:
            - configMapRef:
                name: {{ include "bookinfo-service.configmapName" . }}
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
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

- [ ] **Step 2: Create service.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/service.yaml */}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
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

- [ ] **Step 3: Verify single-mode rendering**

Run: `helm template ratings charts/bookinfo-service --set serviceName=ratings --set image.repository=event-driven-bookinfo/ratings`
Expected: Deployment named `ratings` with 1 replica, no `role` labels, Service selector `app: ratings`

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/templates/deployment.yaml charts/bookinfo-service/templates/service.yaml
git commit -m "feat(helm): add Deployment and Service templates (single mode)"
```

---

## Task 4: CQRS Write Deployment and Service Templates

**Files:**
- Create: `charts/bookinfo-service/templates/deployment-write.yaml`
- Create: `charts/bookinfo-service/templates/service-write.yaml`

- [ ] **Step 1: Create deployment-write.yaml**

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
      labels:
        {{- include "bookinfo-service.labels" . | nindent 8 }}
        role: write
    spec:
      containers:
        - name: {{ include "bookinfo-service.serviceName" . }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.ports.http }}
            - name: admin
              containerPort: {{ .Values.ports.admin }}
          envFrom:
            - configMapRef:
                name: {{ include "bookinfo-service.configmapName" . }}
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
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

- [ ] **Step 2: Create service-write.yaml**

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

- [ ] **Step 3: Verify CQRS rendering**

Run: `helm template ratings charts/bookinfo-service --set cqrs.enabled=true --set serviceName=ratings --set image.repository=event-driven-bookinfo/ratings`
Expected: Two Deployments (`ratings` with `role: read`, `ratings-write` with `role: write`), two Services

- [ ] **Step 4: Verify non-CQRS skips write templates**

Run: `helm template ratings charts/bookinfo-service --set serviceName=ratings | grep 'name: ratings-write'`
Expected: No output (write resources not rendered)

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/templates/deployment-write.yaml charts/bookinfo-service/templates/service-write.yaml
git commit -m "feat(helm): add CQRS write Deployment and Service templates"
```

---

## Task 5: EventSource and EventSource Service Templates

**Files:**
- Create: `charts/bookinfo-service/templates/eventsource.yaml`
- Create: `charts/bookinfo-service/templates/eventsource-service.yaml`

- [ ] **Step 1: Create eventsource.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/eventsource.yaml */}}
{{- range $eventName, $endpoint := .Values.cqrs.endpoints }}
---
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: {{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  eventBusName: {{ $.Values.cqrs.eventBusName }}
  {{- with $.Values.observability.otelEndpoint }}
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ printf "%s-eventsource" $eventName | quote }}
  {{- end }}
  webhook:
    {{ $eventName }}:
      port: {{ $endpoint.port | quote }}
      endpoint: {{ $endpoint.endpoint }}
      method: {{ $endpoint.method }}
{{- end }}
```

- [ ] **Step 2: Create eventsource-service.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/eventsource-service.yaml */}}
{{- range $eventName, $endpoint := .Values.cqrs.endpoints }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $eventName }}-eventsource-svc
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  selector:
    eventsource-name: {{ $eventName }}
  ports:
    - port: {{ $endpoint.port }}
      targetPort: {{ $endpoint.port }}
{{- end }}
```

- [ ] **Step 3: Verify EventSource rendering**

Run:
```bash
helm template ratings charts/bookinfo-service \
  --set cqrs.enabled=true \
  --set 'cqrs.endpoints.rating-submitted.port=12002' \
  --set 'cqrs.endpoints.rating-submitted.method=POST' \
  --set 'cqrs.endpoints.rating-submitted.endpoint=/v1/ratings' \
  --set 'observability.otelEndpoint=http://alloy:4317'
```
Expected: One EventSource named `rating-submitted` with webhook port `"12002"`, endpoint `/v1/ratings`, method `POST`, OTEL env vars. One Service named `rating-submitted-eventsource-svc` with port 12002.

- [ ] **Step 4: Verify no EventSource when endpoints empty**

Run: `helm template ratings charts/bookinfo-service --set serviceName=ratings | grep 'kind: EventSource'`
Expected: No output

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/templates/eventsource.yaml charts/bookinfo-service/templates/eventsource-service.yaml
git commit -m "feat(helm): add EventSource and EventSource Service templates"
```

---

## Task 6: Sensor Template with DLQ Auto-Generation

This is the most complex template. It iterates over all endpoints, builds dependencies, and generates triggers with auto-generated DLQ triggers.

**Files:**
- Create: `charts/bookinfo-service/templates/sensor.yaml`

- [ ] **Step 1: Create sensor.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/sensor.yaml */}}
{{- $hasEndpoints := false }}
{{- range $_, $ep := .Values.cqrs.endpoints }}
{{- if $ep.triggers }}
{{- $hasEndpoints = true }}
{{- end }}
{{- end }}
{{- if $hasEndpoints }}
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: {{ include "bookinfo-service.sensorName" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
spec:
  eventBusName: {{ .Values.cqrs.eventBusName }}
  {{- with .Values.observability.otelEndpoint }}
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ include "bookinfo-service.sensorName" $ | quote }}
  {{- end }}
  dependencies:
    {{- range $eventName, $endpoint := $.Values.cqrs.endpoints }}
    - name: {{ $eventName }}-dep
      eventSourceName: {{ $eventName }}
      eventName: {{ $eventName }}
    {{- end }}
  triggers:
    {{- range $eventName, $endpoint := $.Values.cqrs.endpoints }}
    {{- range $trigger := $endpoint.triggers }}
    {{- /* Resolve trigger URL */ -}}
    {{- $triggerURL := $trigger.url }}
    {{- if eq $trigger.url "self" }}
    {{- if $.Values.cqrs.enabled }}
    {{- $triggerURL = printf "http://%s-write.%s.svc.cluster.local%s" (include "bookinfo-service.fullname" $) $.Release.Namespace $endpoint.endpoint }}
    {{- else }}
    {{- $triggerURL = printf "http://%s.%s.svc.cluster.local%s" (include "bookinfo-service.fullname" $) $.Release.Namespace $endpoint.endpoint }}
    {{- end }}
    {{- end }}
    {{- /* Resolve trigger method */ -}}
    {{- $triggerMethod := default "POST" $trigger.method }}
    - template:
        name: {{ $trigger.name }}
        {{- if $trigger.conditions }}
        conditions: {{ $trigger.conditions }}
        {{- end }}
        http:
          url: {{ $triggerURL }}
          method: {{ $triggerMethod }}
          headers:
            Content-Type: application/json
            {{- with $trigger.headers }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          payload:
            {{- range $p := $trigger.payload }}
            {{- if eq (kindOf $p) "string" }}
            {{- /* Handle "passthrough" shorthand */ }}
            - src:
                dependencyName: {{ $eventName }}-dep
                dataKey: body
              dest: ""
            {{- else }}
            - src:
                dependencyName: {{ default (printf "%s-dep" $eventName) $p.src.dependencyName }}
                {{- with $p.src.dataKey }}
                dataKey: {{ . }}
                {{- end }}
                {{- with $p.src.value }}
                value: {{ . | quote }}
                {{- end }}
                {{- with $p.src.contextKey }}
                contextKey: {{ . }}
                {{- end }}
              dest: {{ $p.dest }}
            {{- end }}
            {{- end }}
      atLeastOnce: {{ $.Values.sensor.atLeastOnce }}
      retryStrategy:
        steps: {{ $.Values.sensor.retryStrategy.steps }}
        duration: {{ $.Values.sensor.retryStrategy.duration }}
        factor: {{ $.Values.sensor.retryStrategy.factor }}
        jitter: {{ $.Values.sensor.retryStrategy.jitter }}
      {{- /* Auto-generate DLQ trigger */ -}}
      {{- if $.Values.sensor.dlq.enabled }}
      {{- $esURL := printf "http://%s-eventsource-svc.%s.svc.cluster.local:%v%s" $eventName $.Release.Namespace $endpoint.port $endpoint.endpoint }}
      dlqTrigger:
        template:
          name: dlq-{{ $trigger.name }}
          http:
            url: {{ $.Values.sensor.dlq.url }}
            method: POST
            headers:
              Content-Type: application/json
            payload:
              - src:
                  dependencyName: {{ $eventName }}-dep
                  dataKey: body
                dest: original_payload
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: id
                dest: event_id
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: type
                dest: event_type
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: source
                dest: event_source
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: subject
                dest: event_subject
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: time
                dest: event_timestamp
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: datacontenttype
                dest: datacontenttype
              - src:
                  dependencyName: {{ $eventName }}-dep
                  dataKey: header
                dest: original_headers
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ include "bookinfo-service.sensorName" $ | quote }}
                dest: sensor_name
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $trigger.name | quote }}
                dest: failed_trigger
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $esURL | quote }}
                dest: eventsource_url
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $.Release.Namespace | quote }}
                dest: namespace
        atLeastOnce: true
        retryStrategy:
          steps: {{ $.Values.sensor.dlq.retryStrategy.steps }}
          duration: {{ $.Values.sensor.dlq.retryStrategy.duration }}
          factor: {{ $.Values.sensor.dlq.retryStrategy.factor }}
          jitter: {{ $.Values.sensor.dlq.retryStrategy.jitter }}
      {{- end }}
    {{- end }}
    {{- end }}
{{- end }}
```

- [ ] **Step 2: Verify sensor rendering with a ratings-like values file**

Create a temporary test file and render:

Run:
```bash
cat > /tmp/test-ratings.yaml << 'EOF'
serviceName: ratings
image:
  repository: event-driven-bookinfo/ratings
  tag: local
cqrs:
  enabled: true
  endpoints:
    rating-submitted:
      port: 12002
      method: POST
      endpoint: /v1/ratings
      triggers:
        - name: create-rating
          url: self
          payload:
            - passthrough
        - name: notify-rating-submitted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New rating submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
sensor:
  dlq:
    enabled: true
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/dlq-event-received"
observability:
  otelEndpoint: "http://alloy:4317"
EOF
helm template ratings charts/bookinfo-service -f /tmp/test-ratings.yaml --namespace bookinfo
```

Expected: Sensor named `ratings-sensor` with:
- 1 dependency: `rating-submitted-dep`
- 2 main triggers: `create-rating` (url resolves to `http://ratings-write.bookinfo.svc.cluster.local/v1/ratings`), `notify-rating-submitted`
- 2 DLQ triggers: `dlq-create-rating`, `dlq-notify-rating-submitted`
- OTEL env vars on sensor template container

- [ ] **Step 3: Verify DLQ disabled produces no dlqTrigger blocks**

Run:
```bash
cat > /tmp/test-dlqueue.yaml << 'EOF'
serviceName: dlqueue
cqrs:
  enabled: true
  endpoints:
    dlq-event-received:
      port: 12004
      method: POST
      endpoint: /dlq-event-received
      triggers:
        - name: ingest-dlq-event
          url: self
          payload:
            - passthrough
sensor:
  dlq:
    enabled: false
EOF
helm template dlqueue charts/bookinfo-service -f /tmp/test-dlqueue.yaml --namespace bookinfo | grep dlqTrigger
```

Expected: No output (no DLQ triggers generated)

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/templates/sensor.yaml
git commit -m "feat(helm): add Sensor template with DLQ auto-generation"
```

---

## Task 7: HPA and NOTES.txt Templates

**Files:**
- Create: `charts/bookinfo-service/templates/hpa.yaml`
- Create: `charts/bookinfo-service/templates/NOTES.txt`

- [ ] **Step 1: Create hpa.yaml**

```gotpl
{{/* charts/bookinfo-service/templates/hpa.yaml */}}
{{- if .Values.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "bookinfo-service.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetCPUUtilization }}
{{- end }}
```

- [ ] **Step 2: Create NOTES.txt**

```gotpl
{{/* charts/bookinfo-service/templates/NOTES.txt */}}
{{ include "bookinfo-service.serviceName" . }} deployed.

Service: {{ include "bookinfo-service.fullname" . }}.{{ .Release.Namespace }}.svc.cluster.local:80
Admin:   port-forward to {{ .Values.ports.admin }} for /healthz, /readyz, /metrics, /debug/pprof/*

{{- if .Values.cqrs.enabled }}
CQRS mode: read ({{ .Values.cqrs.read.replicas }} replica(s)) + write ({{ .Values.cqrs.write.replicas }} replica(s))
Write Service: {{ include "bookinfo-service.fullname" . }}-write.{{ .Release.Namespace }}.svc.cluster.local:80
{{- end }}

{{- range $eventName, $endpoint := .Values.cqrs.endpoints }}
EventSource: {{ $eventName }} (port {{ $endpoint.port }}, {{ $endpoint.method }} {{ $endpoint.endpoint }})
{{- end }}
```

- [ ] **Step 3: Verify HPA renders only when enabled**

Run: `helm template ratings charts/bookinfo-service --set autoscaling.enabled=true | grep 'kind: HorizontalPodAutoscaler'`
Expected: One match

Run: `helm template ratings charts/bookinfo-service | grep 'kind: HorizontalPodAutoscaler'`
Expected: No output

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/templates/hpa.yaml charts/bookinfo-service/templates/NOTES.txt
git commit -m "feat(helm): add HPA and NOTES.txt templates"
```

---

## Task 8: Per-Service Values Files

**Files:**
- Create: `deploy/details/values-local.yaml`
- Create: `deploy/reviews/values-local.yaml`
- Create: `deploy/ratings/values-local.yaml`
- Create: `deploy/notification/values-local.yaml`
- Create: `deploy/productpage/values-local.yaml`
- Create: `deploy/dlqueue/values-local.yaml`

- [ ] **Step 1: Create deploy/ratings/values-local.yaml**

```yaml
# deploy/ratings/values-local.yaml
serviceName: ratings
image:
  repository: event-driven-bookinfo/ratings
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_ratings?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  endpoints:
    rating-submitted:
      port: 12002
      method: POST
      endpoint: /v1/ratings
      triggers:
        - name: create-rating
          url: self
          payload:
            - passthrough
        - name: notify-rating-submitted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New rating submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel

sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/dlq-event-received"
```

- [ ] **Step 2: Create deploy/details/values-local.yaml**

```yaml
# deploy/details/values-local.yaml
serviceName: details
image:
  repository: event-driven-bookinfo/details
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_details?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  endpoints:
    book-added:
      port: 12000
      method: POST
      endpoint: /v1/details
      triggers:
        - name: create-detail
          url: self
          payload:
            - passthrough
        - name: notify-book-added
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body.title
              dest: subject
            - src:
                value: "New book added"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel

sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/dlq-event-received"
```

- [ ] **Step 3: Create deploy/reviews/values-local.yaml**

```yaml
# deploy/reviews/values-local.yaml
serviceName: reviews
image:
  repository: event-driven-bookinfo/reviews
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_reviews?sslmode=disable"
  RUN_MIGRATIONS: "true"
  RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  endpoints:
    review-submitted:
      port: 12001
      method: POST
      endpoint: /v1/reviews
      triggers:
        - name: create-review
          url: self
          payload:
            - passthrough
        - name: notify-review-submitted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New review submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
    review-deleted:
      port: 12003
      method: POST
      endpoint: /v1/reviews/delete
      triggers:
        - name: delete-review-write
          url: self
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
        - name: delete-review-read
          url: "http://reviews.bookinfo.svc.cluster.local/v1/reviews/delete"
          method: POST
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
        - name: notify-review-deleted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "Review deleted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel

sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/dlq-event-received"
```

- [ ] **Step 4: Create deploy/notification/values-local.yaml**

```yaml
# deploy/notification/values-local.yaml
serviceName: notification
image:
  repository: event-driven-bookinfo/notification
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_notification?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 5: Create deploy/productpage/values-local.yaml**

```yaml
# deploy/productpage/values-local.yaml
serviceName: productpage
image:
  repository: event-driven-bookinfo/productpage
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "memory"
  DETAILS_SERVICE_URL: "http://details.bookinfo.svc.cluster.local"
  REVIEWS_SERVICE_URL: "http://reviews.bookinfo.svc.cluster.local"
  REDIS_URL: "redis://redis-master.bookinfo.svc.cluster.local:6379"
  TEMPLATE_DIR: "/app/templates"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 6: Create deploy/dlqueue/values-local.yaml**

```yaml
# deploy/dlqueue/values-local.yaml
serviceName: dlqueue
image:
  repository: event-driven-bookinfo/dlqueue
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_dlqueue?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

sensor:
  dlq:
    enabled: false

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  endpoints:
    dlq-event-received:
      port: 12004
      method: POST
      endpoint: /dlq-event-received
      triggers:
        - name: ingest-dlq-event
          url: self
          payload:
            - passthrough
```

- [ ] **Step 7: Validate all 6 values files render without errors**

Run:
```bash
for svc in productpage details reviews ratings notification dlqueue; do
  echo "=== $svc ===" && \
  helm template $svc charts/bookinfo-service -f deploy/$svc/values-local.yaml --namespace bookinfo > /dev/null && \
  echo "OK" || echo "FAIL"
done
```

Expected: All 6 print `OK`

- [ ] **Step 8: Spot-check reviews multi-endpoint produces 2 EventSources, 1 Sensor**

Run:
```bash
helm template reviews charts/bookinfo-service -f deploy/reviews/values-local.yaml --namespace bookinfo | grep -c 'kind: EventSource'
helm template reviews charts/bookinfo-service -f deploy/reviews/values-local.yaml --namespace bookinfo | grep -c 'kind: Sensor'
```

Expected: `2` EventSources, `1` Sensor

- [ ] **Step 9: Commit**

```bash
git add deploy/details/values-local.yaml deploy/reviews/values-local.yaml deploy/ratings/values-local.yaml deploy/notification/values-local.yaml deploy/productpage/values-local.yaml deploy/dlqueue/values-local.yaml
git commit -m "feat(helm): add per-service values files for local k8s"
```

---

## Task 9: CI Test Values and values.schema.json

**Files:**
- Create: `charts/bookinfo-service/ci/values-ratings-cqrs.yaml`
- Create: `charts/bookinfo-service/ci/values-productpage-simple.yaml`
- Create: `charts/bookinfo-service/ci/values-dlqueue-no-dlq.yaml`
- Create: `charts/bookinfo-service/values.schema.json`

- [ ] **Step 1: Create ci/values-ratings-cqrs.yaml**

```yaml
# charts/bookinfo-service/ci/values-ratings-cqrs.yaml
# ct install test: CQRS + EventSource + Sensor + DLQ
serviceName: ratings
image:
  repository: event-driven-bookinfo/ratings
  tag: latest

cqrs:
  enabled: true
  endpoints:
    rating-submitted:
      port: 12002
      method: POST
      endpoint: /v1/ratings
      triggers:
        - name: create-rating
          url: self
          payload:
            - passthrough

sensor:
  dlq:
    url: "http://dlq-eventsource-svc:12004/dlq-event-received"
```

- [ ] **Step 2: Create ci/values-productpage-simple.yaml**

```yaml
# charts/bookinfo-service/ci/values-productpage-simple.yaml
# ct install test: single deployment, no CQRS, no events
serviceName: productpage
image:
  repository: event-driven-bookinfo/productpage
  tag: latest

config:
  DETAILS_SERVICE_URL: "http://details"
  REVIEWS_SERVICE_URL: "http://reviews"
  TEMPLATE_DIR: "/app/templates"
```

- [ ] **Step 3: Create ci/values-dlqueue-no-dlq.yaml**

```yaml
# charts/bookinfo-service/ci/values-dlqueue-no-dlq.yaml
# ct install test: CQRS + events, DLQ triggers disabled
serviceName: dlqueue
image:
  repository: event-driven-bookinfo/dlqueue
  tag: latest

sensor:
  dlq:
    enabled: false

cqrs:
  enabled: true
  endpoints:
    dlq-event-received:
      port: 12004
      method: POST
      endpoint: /dlq-event-received
      triggers:
        - name: ingest-dlq-event
          url: self
          payload:
            - passthrough
```

- [ ] **Step 4: Create values.schema.json**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "serviceName": { "type": "string" },
    "nameOverride": { "type": "string" },
    "fullnameOverride": { "type": "string" },
    "image": {
      "type": "object",
      "properties": {
        "repository": { "type": "string" },
        "tag": { "type": "string" },
        "pullPolicy": { "type": "string", "enum": ["Always", "IfNotPresent", "Never"] }
      },
      "required": ["repository", "tag"]
    },
    "replicaCount": { "type": "integer", "minimum": 0 },
    "resources": { "type": "object" },
    "ports": {
      "type": "object",
      "properties": {
        "http": { "type": "integer" },
        "admin": { "type": "integer" }
      }
    },
    "probes": { "type": "object" },
    "config": { "type": "object", "additionalProperties": { "type": "string" } },
    "cqrs": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean" },
        "read": { "type": "object" },
        "write": { "type": "object" },
        "eventBusName": { "type": "string" },
        "endpoints": { "type": "object" }
      }
    },
    "sensor": {
      "type": "object",
      "properties": {
        "retryStrategy": { "type": "object" },
        "atLeastOnce": { "type": "boolean" },
        "dlq": {
          "type": "object",
          "properties": {
            "enabled": { "type": "boolean" },
            "url": { "type": "string" },
            "retryStrategy": { "type": "object" }
          }
        }
      }
    },
    "observability": {
      "type": "object",
      "properties": {
        "otelEndpoint": { "type": "string" },
        "pyroscopeAddress": { "type": "string" }
      }
    },
    "autoscaling": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean" },
        "minReplicas": { "type": "integer" },
        "maxReplicas": { "type": "integer" },
        "targetCPUUtilization": { "type": "integer" }
      }
    }
  }
}
```

- [ ] **Step 5: Run helm lint with schema validation**

Run: `helm lint charts/bookinfo-service -f charts/bookinfo-service/ci/values-ratings-cqrs.yaml`
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 6: Commit**

```bash
git add charts/bookinfo-service/ci/ charts/bookinfo-service/values.schema.json
git commit -m "feat(helm): add CI test values and JSON Schema for validation"
```

---

## Task 10: Makefile Updates

**Files:**
- Modify: `Makefile:354-444` (k8s-deploy and k8s-rebuild targets)

- [ ] **Step 1: Replace k8s-deploy target**

In `Makefile`, replace the `k8s-deploy` target (lines 354-393). The new target keeps steps 1-4 (namespace, images, postgres, redis) and replaces step 5 (kustomize apply) with helm upgrade:

Find the block starting with `.PHONY: k8s-deploy` and ending before `.PHONY: k8s-seed`.

Replace the service deployment section (the `[5/6] Deploying services...` block at line 381-385) with:

```makefile
	@printf "$(BOLD)[5/6] Deploying services via Helm...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Installing $$svc...\n"; \
		$(HELM) upgrade --install $$svc charts/bookinfo-service \
			--namespace $(K8S_NS_BOOKINFO) \
			-f deploy/$$svc/values-local.yaml || exit 1; \
	done
```

Also update the wait block (line 389-392) to include dlqueue deployments:

```makefile
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write; do \
		$(KUBECTL) wait deployment/$$dep -n $(K8S_NS_BOOKINFO) \
			--for=condition=Available --timeout=120s 2>/dev/null || true; \
	done
```

- [ ] **Step 2: Replace k8s-rebuild target**

Replace the service redeployment in `k8s-rebuild` (line 432-433) with:

```makefile
	@for svc in $(SERVICES); do \
		$(HELM) upgrade --install $$svc charts/bookinfo-service \
			--namespace $(K8S_NS_BOOKINFO) \
			-f deploy/$$svc/values-local.yaml || exit 1; \
	done
```

Update the rollout restart list (line 437-438) to include dlqueue:

```makefile
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write; do \
		$(KUBECTL) rollout restart deployment/$$dep -n $(K8S_NS_BOOKINFO) 2>/dev/null || true; \
	done
```

And the rollout status wait (line 441-442):

```makefile
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write; do \
		$(KUBECTL) rollout status deployment/$$dep -n $(K8S_NS_BOOKINFO) --timeout=120s 2>/dev/null || true; \
	done
```

- [ ] **Step 3: Add helm-lint and helm-template targets**

Append after the `k8s-load-stop` target:

```makefile
# ─── Helm ──────────────────────────────────────────────────────────────────

.PHONY: helm-lint
helm-lint: ##@Helm Lint the bookinfo-service chart
	helm lint charts/bookinfo-service
	@for svc in $(SERVICES); do \
		if [ -f deploy/$$svc/values-local.yaml ]; then \
			printf "  Linting with $$svc values...\n"; \
			helm lint charts/bookinfo-service -f deploy/$$svc/values-local.yaml || exit 1; \
		fi; \
	done
	@printf "$(GREEN)All lints passed.$(NC)\n"

.PHONY: helm-template
helm-template: ##@Helm Dry-run render for a service: make helm-template SERVICE=<name>
ifndef SERVICE
	$(error SERVICE is not set. Usage: make helm-template SERVICE=<name>)
endif
	helm template $(SERVICE) charts/bookinfo-service \
		-f deploy/$(SERVICE)/values-local.yaml \
		--namespace $(K8S_NS_BOOKINFO)
```

- [ ] **Step 4: Verify Makefile syntax**

Run: `make help | grep -E 'helm-lint|helm-template|k8s-deploy|k8s-rebuild'`
Expected: All four targets appear in help output

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "feat(helm): update Makefile to deploy via helm upgrade --install"
```

---

## Task 11: CI/CD Workflows

**Files:**
- Create: `.github/workflows/helm-lint-test.yml`
- Create: `.github/workflows/helm-release.yml`
- Create: `ct.yaml`

- [ ] **Step 1: Create ct.yaml**

```yaml
# ct.yaml
chart-dirs:
  - charts
target-branch: main
helm-extra-args: --timeout 120s
```

- [ ] **Step 2: Create helm-lint-test.yml**

```yaml
# .github/workflows/helm-lint-test.yml
name: Helm Lint & Test

on:
  pull_request:
    paths:
      - "charts/**"

permissions: {}

jobs:
  lint-test:
    name: Lint & Test
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
        with:
          fetch-depth: 0

      - name: Set up Helm
        uses: azure/setup-helm@b9e51907a09c216f16ebe8536097933489208112 # v4
        with:
          version: v3.17.0

      - name: Set up chart-testing
        uses: helm/chart-testing-action@e6669bcd63d7cb57cb4380c33043eebe5d111992 # v2

      - name: List changed charts
        id: list-changed
        run: |
          changed=$(ct list-changed --config ct.yaml)
          if [[ -n "$changed" ]]; then
            echo "changed=true" >> "$GITHUB_OUTPUT"
          fi

      - name: Lint charts
        if: steps.list-changed.outputs.changed == 'true'
        run: ct lint --config ct.yaml

      - name: Create Kind cluster
        if: steps.list-changed.outputs.changed == 'true'
        uses: helm/kind-action@a1b0e391336a6ee6713a0583f8c6240d70863de3 # v1

      - name: Install Argo Events CRDs
        if: steps.list-changed.outputs.changed == 'true'
        run: |
          kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-events/stable/manifests/install.yaml 2>/dev/null || \
          kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-events/master/api/jsonschema/schema.json 2>/dev/null || true

      - name: Install charts
        if: steps.list-changed.outputs.changed == 'true'
        run: ct install --config ct.yaml
```

- [ ] **Step 3: Create helm-release.yml**

```yaml
# .github/workflows/helm-release.yml
name: Helm Release

on:
  push:
    branches:
      - main
    paths:
      - "charts/**"

permissions:
  contents: write

jobs:
  release:
    name: Release Chart
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
        with:
          fetch-depth: 0

      - name: Configure Git
        run: |
          git config user.name "$GITHUB_ACTOR"
          git config user.email "$GITHUB_ACTOR@users.noreply.github.com"

      - name: Set up Helm
        uses: azure/setup-helm@b9e51907a09c216f16ebe8536097933489208112 # v4
        with:
          version: v3.17.0

      - name: Run chart-releaser
        uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1
        with:
          charts_dir: charts
        env:
          CR_TOKEN: "${{ secrets.GITHUB_TOKEN }}"
```

- [ ] **Step 4: Verify workflow YAML is valid**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/helm-lint-test.yml')); print('helm-lint-test.yml OK')"
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/helm-release.yml')); print('helm-release.yml OK')"
```

Expected: Both print `OK`

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/helm-lint-test.yml .github/workflows/helm-release.yml ct.yaml
git commit -m "ci(helm): add chart-testing and chart-releaser workflows"
```

---

## Task 12: Documentation Updates

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/skills/new-service/SKILL.md`
- Modify: `.claude/rules/release.md`

- [ ] **Step 1: Update CLAUDE.md Deploy Structure section**

Find the `## Deploy Structure` section and replace it with:

```markdown
## Deploy Structure

```
charts/
  bookinfo-service/          # Reusable Helm chart for all 6 services
    Chart.yaml
    values.yaml
    templates/
    ci/                      # chart-testing test values
deploy/
├── <service>/values-local.yaml  # Per-service Helm values for local k8s
├── gateway/base/                # Gateway, GatewayClass, ReferenceGrant
├── gateway/overlays/local/      # HTTPRoutes for bookinfo
├── observability/local/         # Helm values: Prometheus, Grafana, Tempo, Loki, Alloy
├── platform/local/              # Helm values: Strimzi, Argo Events; Kafka CRDs; EventBus
├── redis/local/                 # Helm values: Bitnami Redis
└── postgres/local/              # StatefulSet, Service, init ConfigMap
```
```

- [ ] **Step 2: Update CLAUDE.md Local Kubernetes section**

In the `## Local Kubernetes` section, find the line mentioning `k8s-rebuild` and ensure the descriptions are accurate. No changes needed to the target names themselves, only the comments if they reference kustomize.

- [ ] **Step 3: Update CLAUDE.md to add Helm commands section**

After the `## Build Commands` section, add:

```markdown
## Helm Commands

```bash
make helm-lint            # Lint chart with all per-service values files
make helm-template SERVICE=ratings  # Dry-run render for a specific service
helm upgrade --install ratings charts/bookinfo-service -f deploy/ratings/values-local.yaml -n bookinfo
```
```

- [ ] **Step 4: Update .claude/skills/new-service/SKILL.md step 8**

Replace step 8:

Old:
```
8. Add a Dockerfile and Kustomize base manifests.
```

New:
```
8. Add a Dockerfile and Helm values file (`deploy/{{service}}/values-local.yaml`). Use `deploy/ratings/values-local.yaml` as a template for services with CQRS+events, or `deploy/notification/values-local.yaml` for simple services.
```

- [ ] **Step 5: Update .claude/rules/release.md**

Append a new section at the end:

```markdown
## Chart Release Process

The `bookinfo-service` Helm chart has its own independent version lifecycle.

### Chart Versioning
- Chart version (`version` in `Chart.yaml`): bumped when templates or helpers change
- App version (`appVersion`): informational only — each service sets `image.tag` at install time
- Chart version is independent of service tags (`{service}-vX.Y.Z`)

### Workflows
- `helm-lint-test.yml`: runs on PRs touching `charts/**` — ct lint + ct install
- `helm-release.yml`: runs on main push touching `charts/**` — chart-releaser publishes to GitHub Pages

### Using the Chart
```bash
helm repo add bookinfo https://kaio6fellipe.github.io/event-driven-bookinfo
helm repo update
helm install ratings bookinfo/bookinfo-service -f deploy/ratings/values-local.yaml -n bookinfo
```
```

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md .claude/skills/new-service/SKILL.md .claude/rules/release.md
git commit -m "docs: update CLAUDE.md, skills, and release rules for Helm chart"
```

---

## Task 13: Validation — helm template vs Existing Manifests

This task validates that the Helm-rendered output matches the existing Kustomize manifests structurally.

**Files:** None (read-only validation)

- [ ] **Step 1: Render the Helm chart for ratings**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo > /tmp/helm-ratings.yaml
```

- [ ] **Step 2: Build the existing kustomize output for ratings**

Run:
```bash
kubectl kustomize deploy/ratings/overlays/local/ > /tmp/kustomize-ratings.yaml
```

- [ ] **Step 3: Compare key resources**

Manually compare the two outputs for:
- Deployment names and labels
- Service selectors and ports
- ConfigMap data keys and values
- EventSource webhook config
- Sensor trigger URLs, payload mappings, DLQ trigger structure

The Helm output will have slightly different label sets (includes `helm.sh/chart`, `app.kubernetes.io/managed-by`) — this is expected. The critical match is: same Deployments, Services, EventSource webhooks, Sensor triggers, and DLQ payload fields.

- [ ] **Step 4: Fix any discrepancies found**

If the comparison reveals missing fields, wrong URLs, or incorrect payload mappings, fix the relevant template or values file and re-render until the output matches.

- [ ] **Step 5: Run helm lint on all values files one final time**

Run:
```bash
for svc in productpage details reviews ratings notification dlqueue; do
  helm lint charts/bookinfo-service -f deploy/$svc/values-local.yaml || exit 1
done
echo "All lints passed."
```

Expected: `All lints passed.`

- [ ] **Step 6: Commit any fixes**

```bash
git add -A charts/ deploy/
git commit -m "fix(helm): align template output with existing kustomize manifests"
```

(Skip this step if no fixes were needed.)

---

## Task 14: End-to-End Validation — Full k8s Lifecycle

This is the critical validation task. Deploy the Helm chart to a real k3d cluster, verify all services work, check observability, and run load tests. **Do not proceed to Task 15 (kustomize removal) until this passes.**

**Files:** None (runtime validation)

- [ ] **Step 1: Tear down any existing cluster**

Run: `make stop-k8s`
Expected: Cluster deleted (or "cluster not found" if none exists)

- [ ] **Step 2: Stand up the full environment with Helm-based deploys**

Run: `make run-k8s`

This will:
1. Create k3d cluster
2. Install platform layer (Envoy Gateway, Strimzi Kafka, Argo Events)
3. Install observability layer (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy)
4. Build Docker images, import to k3d
5. Deploy PostgreSQL + Redis
6. **Deploy services via `helm upgrade --install`** (the new path)
7. Apply HTTPRoutes
8. Seed databases

Expected: All pods reach `Running`/`Ready` state. The command completes without errors.

- [ ] **Step 3: Verify pod status**

Run: `make k8s-status`

Expected: All pods in `bookinfo` namespace are `Running` and `Ready`:
- `productpage`, `details`, `details-write`, `reviews`, `reviews-write`, `ratings`, `ratings-write`, `notification`, `dlqueue`, `dlqueue-write`, `postgres`, `redis`
- EventSource pods (created by Argo Events from the EventSource CRDs)
- Sensor pods (created by Argo Events from the Sensor CRDs)

If any pods are in `CrashLoopBackOff` or `Error`, check logs:
```bash
kubectl --context=k3d-bookinfo-local logs -n bookinfo <pod-name>
```

- [ ] **Step 4: Verify sync reads — GET requests through Gateway**

Run:
```bash
# Get all details (should return seeded data)
curl -s http://localhost:8080/v1/details | jq .

# Get all reviews
curl -s http://localhost:8080/v1/reviews | jq .

# Get all ratings
curl -s http://localhost:8080/v1/ratings | jq .
```

Expected: All three return JSON arrays with seeded data (200 OK).

- [ ] **Step 5: Verify async writes — POST through EventSource webhook**

Run:
```bash
# Submit a new book (routed to book-added EventSource → Sensor → details-write)
curl -s -X POST http://localhost:8080/v1/details \
  -H 'Content-Type: application/json' \
  -d '{"title":"Helm Chart Testing","author":"Test Author","year":2026,"isbn":"978-0-000-00000-0"}' | jq .

# Wait for async processing
sleep 5

# Verify the book was created
curl -s http://localhost:8080/v1/details | jq '.[] | select(.title=="Helm Chart Testing")'
```

Expected: The POST returns 200 (accepted by EventSource). After ~5s, the GET returns the new book entry.

- [ ] **Step 6: Verify rating submission (end-to-end CQRS flow)**

Run:
```bash
# Submit a rating (routed to rating-submitted EventSource → Sensor → ratings-write + notification)
curl -s -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"test-product","stars":5,"reviewer":"helm-tester"}' | jq .

sleep 5

# Verify rating was created
curl -s http://localhost:8080/v1/ratings | jq '.[] | select(.reviewer=="helm-tester")'
```

Expected: Rating appears in GET response. Notification service received an event (check logs):
```bash
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l app=notification --tail=20
```

- [ ] **Step 7: Verify review submission + deletion (multi-endpoint sensor)**

Run:
```bash
# Submit a review
curl -s -X POST http://localhost:8080/v1/reviews \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"test-product","reviewer":"helm-tester","text":"Helm works!","stars":5}' | jq .

sleep 5

# Verify review was created
REVIEW_ID=$(curl -s http://localhost:8080/v1/reviews | jq -r '.[] | select(.reviewer=="helm-tester") | .id')
echo "Created review: $REVIEW_ID"

# Delete the review
curl -s -X POST http://localhost:8080/v1/reviews/delete \
  -H 'Content-Type: application/json' \
  -d "{\"review_id\":\"$REVIEW_ID\"}" | jq .

sleep 5

# Verify review was deleted
curl -s http://localhost:8080/v1/reviews | jq '.[] | select(.reviewer=="helm-tester")'
```

Expected: Review is created, then deleted. Both operations go through EventSource → Sensor → write service.

- [ ] **Step 8: Verify productpage renders (BFF aggregation)**

Run:
```bash
curl -s http://localhost:8080/ | head -20
```

Expected: HTML response from productpage (HTMX-rendered page aggregating details + reviews).

- [ ] **Step 9: Verify distributed tracing in Grafana**

Open Grafana at `http://localhost:3000` (admin/admin).

1. Navigate to **Explore** → select **Tempo** data source
2. Search for recent traces (last 15 minutes)
3. Verify traces show the full request flow:
   - Gateway → EventSource → Sensor → write-service
   - Gateway → read-service (for GET requests)
4. Confirm `service.name` labels match the Helm-deployed services (e.g., `ratings`, `details-write`)

If no traces appear, check Alloy is receiving OTLP data:
```bash
kubectl --context=k3d-bookinfo-local logs -n observability -l app.kubernetes.io/name=alloy --tail=20
```

- [ ] **Step 10: Verify metrics in Prometheus**

Open Prometheus at `http://localhost:9090`.

Run query: `http_requests_total{namespace="bookinfo"}`

Expected: Counters from all bookinfo services.

- [ ] **Step 11: Run k6 load test**

Run:
```bash
make k8s-load DURATION=30s BASE_RATE=2
```

Expected: k6 completes with no errors. Check output for:
- `http_req_failed` rate should be `0.00%` or very low
- `http_req_duration` p95 should be reasonable (< 500ms)

After the test, check Grafana's k6 dashboard at `http://localhost:3000` for the load test metrics.

- [ ] **Step 12: Verify helm upgrade works (idempotent re-deploy)**

Run:
```bash
make k8s-rebuild
```

Expected: All `helm upgrade --install` commands succeed. Services restart and return to `Ready` state. No data loss (seeded data still accessible).

- [ ] **Step 13: Fix any issues found**

If any step above fails, investigate and fix:
- Template rendering issues → fix in `charts/bookinfo-service/templates/`
- Values issues → fix in `deploy/{service}/values-local.yaml`
- Makefile issues → fix in `Makefile`

Re-run the failing steps until all pass.

- [ ] **Step 14: Commit any fixes**

```bash
git add -A charts/ deploy/ Makefile
git commit -m "fix(helm): resolve e2e validation issues"
```

(Skip if no fixes were needed.)

---

## Task 15: Remove Old Kustomize Manifests

Only after Task 14 e2e validation passes completely.

**Files:**
- Delete: `deploy/{details,reviews,ratings,notification,productpage,dlqueue}/base/`
- Delete: `deploy/{details,reviews,ratings,notification,productpage,dlqueue}/overlays/`

- [ ] **Step 1: Remove base and overlays directories**

Run:
```bash
for svc in details reviews ratings notification productpage dlqueue; do
  rm -rf deploy/$svc/base deploy/$svc/overlays
done
```

- [ ] **Step 2: Verify only values files remain per service**

Run:
```bash
for svc in details reviews ratings notification productpage dlqueue; do
  echo "=== $svc ===" && ls deploy/$svc/
done
```

Expected: Each directory contains only `values-local.yaml` (and potentially future `values-dev.yaml`, etc.)

- [ ] **Step 3: Verify infra directories are untouched**

Run:
```bash
ls deploy/gateway/ deploy/observability/ deploy/platform/ deploy/postgres/ deploy/redis/ deploy/k6/
```

Expected: All infra directories still have their original contents

- [ ] **Step 4: Final helm lint**

Run: `make helm-lint`
Expected: All lints pass

- [ ] **Step 5: Commit**

```bash
git add -A deploy/
git commit -m "chore: remove old kustomize base and overlay manifests

Replaced by bookinfo-service Helm chart + per-service values files."
```

---

## Task 16: Post-Removal E2E Re-Validation

Ensure the system still works after kustomize manifests are deleted. This catches any hidden dependencies on the old files.

**Files:** None (runtime validation)

- [ ] **Step 1: Tear down and rebuild from scratch**

Run:
```bash
make stop-k8s
make run-k8s
```

Expected: Full environment comes up successfully using only Helm-based deploys. No references to old kustomize overlays.

- [ ] **Step 2: Quick smoke test**

Run:
```bash
# Verify reads
curl -sf http://localhost:8080/v1/details | jq length
curl -sf http://localhost:8080/v1/reviews | jq length
curl -sf http://localhost:8080/v1/ratings | jq length

# Verify async write
curl -s -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"post-migration-test","stars":4,"reviewer":"final-check"}'

sleep 5

curl -s http://localhost:8080/v1/ratings | jq '.[] | select(.reviewer=="final-check")'
```

Expected: All reads return data. Async write completes successfully.

- [ ] **Step 3: Run final k6 load test**

Run: `make k8s-load DURATION=30s BASE_RATE=2`
Expected: 0% error rate, comparable latency to pre-migration.

- [ ] **Step 4: Commit confirmation (no code changes expected)**

If everything passes, no commit needed. If fixes are needed:
```bash
git add -A
git commit -m "fix(helm): resolve post-migration issues"
```

- [ ] **Step 5: Tear down cluster**

Run: `make stop-k8s`
