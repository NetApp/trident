{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "trident.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "trident.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "trident.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "trident.labels" -}}
helm.sh/chart: {{ include "trident.chart" . }}
{{ include "trident.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "trident.selectorLabels" -}}
app.kubernetes.io/name: {{ include "trident.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Trident operator level
*/}}
{{- define "trident-operator.logLevel" -}}
{{- .Values.operatorLogLevel }}
{{- end }}

{{/*
Trident operator debug
*/}}
{{- define "trident-operator.debug" -}}
{{- .Values.operatorDebug }}
{{- end }}

{{/*
Trident operator image
*/}}
{{- define "trident-operator.image" -}}
{{- if .Values.operatorImage }}
{{- .Values.operatorImage }}
{{- else if .Values.imageRegistry }}
{{- .Values.imageRegistry }}/trident-operator:{{ .Values.operatorImageTag | default .Chart.AppVersion }}
{{- else }}
{{- "" }}docker.io/netapp/trident-operator:{{ .Values.operatorImageTag | default .Chart.AppVersion }}
{{- end }}
{{- end }}

{{/*
Trident force detach
*/}}
{{- define "trident.enableForceDetach" -}}
{{- if .Values.enableForceDetach | printf "%v" | eq "true" }}
{{- "true" }}
{{- else }}
{{- "false" }}
{{- end }}
{{- end }}

{{/*
Trident IPv6
*/}}
{{- define "trident.IPv6" -}}
{{- if .Values.tridentIPv6 | printf "%v" | eq "true" }}
{{- "true" }}
{{- else }}
{{- "false" }}
{{- end }}
{{- end }}

{{/*
Trident SilenceAutosupport
*/}}
{{- define "trident.silenceAutosupport" -}}
{{- if .Values.tridentSilenceAutosupport | printf "%v" | eq "true" }}
{{- "true" }}
{{- else }}
{{- "false" }}
{{- end }}
{{- end }}

Trident AutoSupport image
*/}}
{{- define "trident.autosupportImage" -}}
{{- if .Values.tridentAutosupportImage }}
{{- .Values.tridentAutosupportImage }}
{{- else if .Values.imageRegistry }}
{{- .Values.imageRegistry }}/trident-autosupport:{{ .Values.tridentAutosupportImageTag | default .Chart.AppVersion | trunc 5}}
{{- else }}
{{- "" }}docker.io/netapp/trident-autosupport:{{ .Values.tridentAutosupportImageTag | default .Chart.AppVersion | trunc 5}}
{{- end }}
{{- end }}

{{/*
Trident log level
*/}}
{{- define "trident.logLevel" -}}
{{- .Values.tridentLogLevel }}
{{- end }}

{{/*
Trident debug (equivalent to debug level)
*/}}
{{- define "trident.debug" -}}
{{- .Values.tridentDebug }}
{{- end }}

{{/*
Trident logging workflows
*/}}
{{- define "trident.logWorkflows" -}}
{{- .Values.tridentLogWorkflows }}
{{- end }}

{{/*
Trident logging layers
*/}}
{{- define "trident.logLayers" -}}
{{- .Values.tridentLogLayers }}
{{- end }}

{{/*
Trident log format
*/}}
{{- define "trident.logFormat" -}}
{{- if eq .Values.tridentLogFormat "json" }}
{{- .Values.tridentLogFormat }}
{{- else }}
{{- "text" }}
{{- end }}
{{- end }}

{{/*
Trident audit log
*/}}
{{- define "trident.disableAuditLog" -}}
{{- if .Values.tridentDisableAuditLog | printf "%v" | eq "true" }}
{{- "true" }}
{{- else }}
{{- "false" }}
{{- end }}
{{- end }}

{{/*
Trident probe port
*/}}
{{- define "trident.probePort" -}}
{{- if eq .Values.tridentProbePort "json" }}
{{- .Values.tridentProbePort }}
{{- else }}
{{- 17546 }}
{{- end }}
{{- end }}

{{/*
Trident image
*/}}
{{- define "trident.image" -}}
{{- if .Values.tridentImage }}
{{- .Values.tridentImage }}
{{- else if .Values.imageRegistry }}
{{- .Values.imageRegistry }}/trident:{{ .Values.tridentImageTag | default .Chart.AppVersion }}
{{- else }}
{{- "" }}docker.io/netapp/trident:{{ .Values.tridentImageTag | default .Chart.AppVersion }}
{{- end }}
{{- end }}

{{/*
Trident image pull policy
*/}}
{{- define "imagePullPolicy" -}}
{{- if .Values.imagePullPolicy }}
{{- .Values.imagePullPolicy }}
{{- else }}
{{- "IfNotPresent" }}
{{- end }}
{{- end }}

{{/*
Trident orchestrator namespace
*/}}
{{- define "trident.orchestratorNamespace" -}}
{{- if .Values.orchestratorNamespace }}
{{- .Values.orchestratorNamespace }}
{{- else }}
{{- .Release.Namespace }}{{- end }}
{{- end }}
