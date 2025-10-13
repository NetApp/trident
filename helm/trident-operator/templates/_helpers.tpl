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

{{/*
Trident ExcludeAutosupport
*/}}
{{- define "trident.excludeAutosupport" -}}
{{- if .Values.tridentExcludeAutosupport | printf "%v" | eq "true" }}
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
Determines if rancher roles should be created by checking for the presence of the cattle-system namespace
or annotations with the prefix "cattle.io/" in the namespace where the chart is being installed.
Override auto-detection and force install the roles by setting Values.forceInstallRancherClusterRoles to 'true'.
*/}}
{{- define "shouldInstallRancherRoles" -}}
{{- $isRancher := false -}}
{{- $currentNs := .Release.Namespace -}}
{{- $currentNsObj := lookup "v1" "Namespace" "" $currentNs -}}
{{- /* Check if 'forceInstallRancherClusterRoles' is set */ -}}
{{- if .Values.forceInstallRancherClusterRoles }}
    {{- $isRancher = true -}}
{{- end }}
{{- /* Check if the annotation prefix "cattle.io/" exists on the namespace */ -}}
{{- if $currentNsObj }}
  {{- range $key, $value := $currentNsObj.metadata.annotations }}
    {{- if hasPrefix "cattle.io/" $key }}
      {{- $isRancher = true -}}
    {{- end }}
  {{- end }}
{{- end }}
{{- /* Check if cattle-system ns exists */ -}}
{{- $cattleNs := lookup "v1" "Namespace" "" "cattle-system" -}}
{{- if $cattleNs }}
  {{- $isRancher = true -}}
{{- end }}
{{- $isRancher -}}
{{- end }}

{{/*
Helper functions to render resource requests and limits for each container of trident from values.yaml
*/}}
{{- define "trident.resources.controller" -}}
{{- range $key, $val := . }}
{{- $containerName := $key }}
{{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory }}
{{ $containerName }}:
{{- if or $val.requests.cpu $val.requests.memory }}
  requests:
{{- if $val.requests.cpu }}
    cpu: {{ $val.requests.cpu }}
{{- end }}
{{- if $val.requests.memory }}
    memory: {{ $val.requests.memory }}
{{- end }}
{{- end }}
{{- if or $val.limits.cpu $val.limits.memory }}
  limits:
{{- if $val.limits.cpu }}
    cpu: {{ $val.limits.cpu }}
{{- end }}
{{- if $val.limits.memory }}
    memory: {{ $val.limits.memory }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{- define "trident.resources.node.linux" -}}
{{- range $key, $val := . }}
{{- $containerName := $key }}
{{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory }}
{{ $containerName }}:
{{- if or $val.requests.cpu $val.requests.memory }}
  requests:
{{- if $val.requests.cpu }}
    cpu: {{ $val.requests.cpu }}
{{- end }}
{{- if $val.requests.memory }}
    memory: {{ $val.requests.memory }}
{{- end }}
{{- end }}
{{- if or $val.limits.cpu $val.limits.memory }}
  limits:
{{- if $val.limits.cpu }}
    cpu: {{ $val.limits.cpu }}
{{- end }}
{{- if $val.limits.memory }}
    memory: {{ $val.limits.memory }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{- define "trident.resources.node.windows" -}}
{{- range $key, $val := . }}
{{- $containerName := $key }}
{{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory }}
{{ $containerName }}:
{{- if or $val.requests.cpu $val.requests.memory }}
  requests:
{{- if $val.requests.cpu }}
    cpu: {{ $val.requests.cpu }}
{{- end }}
{{- if $val.requests.memory }}
    memory: {{ $val.requests.memory }}
{{- end }}
{{- end }}
{{- if or $val.limits.cpu $val.limits.memory }}
  limits:
{{- if $val.limits.cpu }}
    cpu: {{ $val.limits.cpu }}
{{- end }}
{{- if $val.limits.memory }}
    memory: {{ $val.limits.memory }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Helper functions to check if resources are actually defined (not just empty structure)
*/}}
{{- define "trident.hasControllerResources" -}}
  {{- $hasResources := false -}}
  {{- if .Values.resources -}}
    {{- if .Values.resources.controller -}}
      {{- range $key, $val := .Values.resources.controller -}}
        {{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory -}}
          {{- $hasResources = true -}}
        {{- end -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- if $hasResources -}}true{{- end -}}
{{- end -}}

{{- define "trident.hasNodeLinuxResources" -}}
  {{- $hasResources := false -}}
  {{- if .Values.resources -}}
    {{- if .Values.resources.node -}}
      {{- if .Values.resources.node.linux -}}
        {{- range $key, $val := .Values.resources.node.linux -}}
          {{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory -}}
            {{- $hasResources = true -}}
          {{- end -}}
        {{- end -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- if $hasResources -}}true{{- end -}}
{{- end -}}

{{- define "trident.hasNodeWindowsResources" -}}
  {{- $hasResources := false -}}
  {{- if .Values.resources -}}
    {{- if .Values.resources.node -}}
      {{- if .Values.resources.node.windows -}}
        {{- range $key, $val := .Values.resources.node.windows -}}
          {{- if or $val.requests.cpu $val.requests.memory $val.limits.cpu $val.limits.memory -}}
            {{- $hasResources = true -}}
          {{- end -}}
        {{- end -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
  {{- if $hasResources -}}true{{- end -}}
{{- end -}}
