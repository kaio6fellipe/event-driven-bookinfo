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

{{/*
Consumer sensor name: derived from release name (separate from CQRS sensor).
*/}}
{{- define "bookinfo-service.consumerSensorName" -}}
{{ include "bookinfo-service.fullname" . }}-consumer-sensor
{{- end }}
